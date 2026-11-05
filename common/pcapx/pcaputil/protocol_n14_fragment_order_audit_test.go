package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
)

// Authored controls, not captures from a real peer. The UDP/TCP checksums and
// complete transport body are constructed before adding any extension headers.
func n14AuditTransport(t *testing.T, tcp bool, dnsID uint16) ([]byte, []byte, byte) {
	t.Helper()
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv6}
	ip := &layers.IPv6{Version: 6, HopLimit: 37, TrafficClass: 0x2a, FlowLabel: 0x12345, SrcIP: net.ParseIP("2001:db8::1"), DstIP: net.ParseIP("2001:db8::2")}
	var transport, app gopacket.SerializableLayer
	next := byte(17)
	if tcp {
		next = 6
		stream := &layers.TCP{SrcPort: 40000, DstPort: 80, Seq: 100, ACK: true, PSH: true, Window: 4096}
		require.NoError(t, stream.SetNetworkLayerForChecksum(ip))
		transport, app = stream, gopacket.Payload("GET /n14-audit HTTP/1.1\r\nHost: audit.invalid\r\n\r\n")
	} else {
		udp := &layers.UDP{SrcPort: 40000, DstPort: 53}
		require.NoError(t, udp.SetNetworkLayerForChecksum(ip))
		transport = udp
		app = &layers.DNS{ID: dnsID, RD: true, Questions: []layers.DNSQuestion{{Name: []byte("audit.invalid"), Type: layers.DNSTypeA, Class: layers.DNSClassIN}}}
	}
	ip.NextHeader = layers.IPProtocol(next)
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, transport, app))
	raw := bytes.Clone(b.Bytes())
	return raw[:54], raw[54:], next
}

func n14AuditFragments(t *testing.T, profile string, tcp, atomic bool, id uint32, dnsID uint16) [][]byte {
	t.Helper()
	prefix, body, next := n14AuditTransport(t, tcp, dnsID)
	var pre []byte
	if profile == "hbh-before-fragment" {
		pre = []byte{44, 0, 1, 4, 0, 0, 0, 0}
	}
	if profile == "hbh-after-fragment" || profile == "destination-after-fragment" {
		body = append([]byte{next, 0, 1, 4, 0, 0, 0, 0}, body...)
		next = 60
		if profile == "hbh-after-fragment" {
			next = 0
		}
	}
	split := 32
	if atomic {
		split = len(body)
	}
	require.GreaterOrEqual(t, len(body), split)
	var frames [][]byte
	for off := 0; off < len(body); {
		end := len(body)
		if off == 0 {
			end = split
		}
		raw := bytes.Clone(prefix)
		raw[20] = 44
		if len(pre) != 0 {
			raw[20] = 0
		}
		binary.BigEndian.PutUint16(raw[18:20], uint16(len(pre)+8+end-off))
		raw = append(raw, pre...)
		frag := make([]byte, 8)
		frag[0] = next
		bits := uint16(off)
		if end < len(body) {
			bits |= 1
		}
		binary.BigEndian.PutUint16(frag[2:4], bits)
		binary.BigEndian.PutUint32(frag[4:8], id)
		raw = append(raw, frag...)
		frames = append(frames, append(raw, body[off:end]...))
		off = end
	}
	return frames
}

func n14AuditVLAN(raw []byte, vlan uint16) []byte {
	out := append(bytes.Clone(raw[:12]), 0x81, 0, byte(vlan>>8), byte(vlan))
	return append(out, raw[12:]...)
}

type n14AuditRecord struct {
	raw   []byte
	iface int
}

func n14AuditCapture(t *testing.T, records []n14AuditRecord) []byte {
	t.Helper()
	var b bytes.Buffer
	w, err := pcapgo.NewNgWriter(&b, layers.LinkTypeEthernet)
	require.NoError(t, err)
	_, err = w.AddInterface(pcapgo.NgInterface{LinkType: layers.LinkTypeEthernet, SnapLength: 65535})
	require.NoError(t, err)
	for i, record := range records {
		ci := gopacket.CaptureInfo{Timestamp: time.Unix(100, int64(i)*1000000), CaptureLength: len(record.raw), Length: len(record.raw), InterfaceIndex: record.iface}
		require.NoError(t, w.WritePacket(ci, record.raw))
	}
	require.NoError(t, w.Flush())
	return bytes.Clone(b.Bytes())
}

func TestN14FragmentHBHOrderCaptureIngress(t *testing.T) {
	for _, tcp := range []bool{false, true} {
		for _, order := range []string{"atomic", "ordered", "reverse"} {
			for _, workers := range []int{1, 4} {
				for _, observe := range []bool{false, true} {
					for _, protocols := range []bool{false, true} {
						if !tcp && !protocols {
							continue
						}
						t.Run(fmt.Sprintf("tcp%t/%s/w%d/observe%t/protocols%t", tcp, order, workers, observe, protocols), func(t *testing.T) {
							frames := n14AuditFragments(t, "hbh-after-fragment", tcp, order == "atomic", 0x11223344, 0x5a4b)
							if order == "reverse" {
								frames[0], frames[1] = frames[1], frames[0]
							}
							var records []n14AuditRecord
							for _, raw := range frames {
								records = append(records, n14AuditRecord{raw: raw})
							}
							var events []*ProtocolEvent
							var analysis ProtocolStats
							var assembly TCPReassemblyStats
							var mu sync.Mutex
							flows, observed := 0, 0
							var delivered bytes.Buffer
							opts := []CaptureOption{
								WithTCPReassemblyWorkers(workers),
								WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: true, Stream: true}),
								WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { assembly = s }),
								WithOnTrafficFlowCreated(func(*TrafficFlow) { mu.Lock(); defer mu.Unlock(); flows++ }),
								WithOnTrafficFlowOnDataFrameReassembled(func(_ *TrafficFlow, _ *TrafficConnection, f *TrafficFrame) {
									mu.Lock()
									defer mu.Unlock()
									delivered.Write(f.Payload)
								}),
							}
							if protocols {
								opts = append(opts, WithOnProtocolMessage(func(e *ProtocolEvent) { mu.Lock(); defer mu.Unlock(); events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { analysis = s }))
							}
							if observe {
								opts = append(opts, WithEveryPacket(func(p gopacket.Packet) {
									mu.Lock()
									defer mu.Unlock()
									require.Equal(t, frames[observed], p.Data())
									observed++
								}))
							}
							require.NoError(t, ReplayPcap(bytes.NewReader(n14AuditCapture(t, records)), opts...))
							require.Zero(t, flows, "an illegal Fragment->HBH envelope cannot establish a TCP owner")
							require.Empty(t, delivered.Bytes())
							require.Zero(t, assembly.DeliveredBytes)
							if observe {
								require.Equal(t, len(frames), observed)
							}
							if protocols {
								require.Zero(t, analysis.Messages)
								require.Zero(t, analysis.BufferedBytes)
								require.Len(t, events, 1, "only the offset-zero/atomic header refusal is published; no UDP/TCP/application event")
								e := events[0]
								require.Equal(t, "ip", e.Protocol)
								require.Equal(t, "IPv6OptionsHeaderOrder", e.ExpertCode)
								require.Equal(t, "malformed", e.Status)
								require.Zero(t, e.FlowID)
								require.Zero(t, e.TCPFlowID)
								require.Empty(t, e.Raw)
								require.Empty(t, e.SourceBytes.SHA256)
								require.Nil(t, e.SourceBytes.CompletedIn)
								want := uint64(1)
								if order == "reverse" {
									want = 2
								}
								require.Equal(t, []PacketReference{{Number: want, Domain: CaptureDomain{}}}, e.SourceBytes.PacketRefs)
							}
						})
					}
				}
			}
		}
	}
}

func TestN14FragmentLegalExtensionCaptureIngress(t *testing.T) {
	_, expected, _ := n14AuditTransport(t, false, 0x5a4b)
	expected = expected[8:] // exact DNS bytes, independently serialized before fragmentation
	for _, profile := range []string{"hbh-before-fragment", "destination-after-fragment"} {
		for _, order := range []string{"atomic", "ordered", "reverse"} {
			for _, workers := range []int{1, 4} {
				for _, deferred := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%s/w%d/deferred%t", profile, order, workers, deferred), func(t *testing.T) {
						frames := n14AuditFragments(t, profile, false, order == "atomic", 0x11223344, 0x5a4b)
						if order == "reverse" {
							frames[0], frames[1] = frames[1], frames[0]
						}
						var records []n14AuditRecord
						for _, raw := range frames {
							records = append(records, n14AuditRecord{raw: raw})
						}
						var events []*ProtocolEvent
						var stats ProtocolStats
						require.NoError(t, ReplayPcap(bytes.NewReader(n14AuditCapture(t, records)), WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })))
						require.Zero(t, stats.BufferedBytes)
						require.Len(t, events, 1)
						e := events[0]
						require.Equal(t, "dns", e.Protocol)
						require.Empty(t, e.Error)
						require.Equal(t, expected, e.Raw)
						require.NotNil(t, e.SourceBytes.CompletedIn)
						require.EqualValues(t, len(frames), e.SourceBytes.CompletedIn.Number)
						var refs []PacketReference
						for i := range frames {
							refs = append(refs, PacketReference{Number: uint64(i + 1)})
						}
						require.ElementsMatch(t, refs, e.SourceBytes.PacketRefs)
						_, err := e.GetFields()
						require.NoError(t, err)
					})
				}
			}
		}
	}
}

func TestN14FragmentHBHRefusalKeepsAtomicAndCaptureDomainsIndependent(t *testing.T) {
	for _, mode := range []string{"atomic-same-owner", "foreign-interface", "foreign-vlan"} {
		t.Run(mode, func(t *testing.T) {
			good := n14AuditFragments(t, "hbh-before-fragment", false, false, 0x11223344, 0x5a4b)
			bad := n14AuditFragments(t, "hbh-after-fragment", false, mode == "atomic-same-owner", 0x11223344, 0x4321)
			if mode == "foreign-vlan" {
				for i := range good {
					good[i] = n14AuditVLAN(good[i], 7)
				}
				for i := range bad {
					bad[i] = n14AuditVLAN(bad[i], 9)
				}
			}
			badInterface := 0
			if mode == "foreign-interface" {
				badInterface = 1
			}
			records := []n14AuditRecord{{raw: good[0]}, {raw: bad[0], iface: badInterface}, {raw: good[1]}}
			if len(bad) > 1 {
				records = append(records, n14AuditRecord{raw: bad[1], iface: badInterface})
			}
			var events []*ProtocolEvent
			require.NoError(t, ReplayPcap(bytes.NewReader(n14AuditCapture(t, records)), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })))
			require.Len(t, events, 2)
			var diagnostic, message *ProtocolEvent
			for _, e := range events {
				if e.Protocol == "dns" {
					message = e
				} else {
					diagnostic = e
				}
			}
			require.NotNil(t, diagnostic)
			require.Equal(t, "IPv6OptionsHeaderOrder", diagnostic.ExpertCode)
			require.EqualValues(t, 2, diagnostic.SourceBytes.PacketRefs[0].Number)
			require.NotNil(t, message)
			require.Empty(t, message.Error)
			wantDomain := CaptureDomain{}
			if mode == "foreign-vlan" {
				wantDomain.Encapsulation = "/vlan:7"
			}
			require.Equal(t, wantDomain, message.Domain)
			require.Equal(t, []PacketReference{{Number: 1, Domain: wantDomain}, {Number: 3, Domain: wantDomain}}, message.SourceBytes.PacketRefs)
			require.EqualValues(t, 3, message.SourceBytes.CompletedIn.Number)
		})
	}
}

// A real retained-byte owner supplies pressure; releasing that reservation is
// distinct from changing limits. This control does not assume that free tail
// extent metadata must be transactional with payload admission.
func TestN14FragmentByteBudgetReleaseAndExactTailRetry(t *testing.T) {
	var events []*ProtocolEvent
	const limit = 4096
	a := coreFragmentParser(&events, limit, 16)
	t.Cleanup(func() { a.closeFragments(); require.Zero(t, a.buffered.Load()) })
	frames := n14AuditFragments(t, "destination-after-fragment", false, false, 0x11223344, 0x5a4b)
	packet := func(i int, number uint64) gopacket.Packet {
		raw := frames[i]
		p := gopacket.NewPacket(raw, protocolPacketDecoder{decoder: layers.LinkTypeEthernet}, gopacket.Default)
		ci := gopacket.CaptureInfo{Timestamp: time.Unix(100, int64(number)*1000000), CaptureLength: len(raw), Length: len(raw)}
		p.Metadata().CaptureInfo = withEvidence(ci, captureEvidence{Ref: PacketReference{Number: number}})
		return p
	}
	_, consumed := a.networkPacket(packet(0, 1))
	require.True(t, consumed)
	tailCost := int64(len(frames[1]) - 14 - 40 - 8 + 128)
	pressure := int64(limit) - a.buffered.Load() - tailCost + 1
	require.Positive(t, pressure)
	require.True(t, a.reserveEvidence(pressure))
	_, consumed = a.networkPacket(packet(1, 2))
	require.True(t, consumed)
	require.Len(t, events, 1)
	require.Equal(t, "FragmentByteBudget", events[0].ExpertCode)
	a.buffered.Add(-pressure)
	q, consumed := a.networkPacket(packet(1, 3))
	require.False(t, consumed)
	require.NotNil(t, q)
	require.Len(t, events, 1)
	require.Equal(t, []PacketReference{{Number: 1}, {Number: 3}}, packetEvidence(q).refs)
	require.Zero(t, a.buffered.Load())
}
