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

func TestPPTPV1RefusesWrongParentGrammarBeforeEveryChild(t *testing.T) {
	for _, tcp := range []bool{false, true} {
		inner := pptpV1TestInner(t, tcp, 1000, true, nil)
		payload := append([]byte{0, 0x21}, inner...)
		for _, tc := range []struct {
			name     string
			flags    uint16
			protocol uint16
			length   uint16
			body     []byte
		}{
			{"generic-IPv4-in-v1", 0x2001, 0x0800, 0, inner},
			{"generic-IPv4-without-K", 0x0001, 0x0800, 0, inner},
			{"payload-zero", 0x3001, 0x880b, 0, payload},
			{"payload-short", 0x3001, 0x880b, 2, payload},
			{"payload-overrun", 0x3001, 0x880b, uint16(len(payload) + 10), payload},
			{"data-without-S", 0x2001, 0x880b, uint16(len(payload)), payload},
			{"ACK-only-with-data", 0x2081, 0x880b, 0, payload},
			{"strict-source-route", 0x3801, 0x880b, uint16(len(payload)), payload},
			{"recursion-control", 0x3101, 0x880b, uint16(len(payload)), payload},
			{"reserved-flags", 0x3009, 0x880b, uint16(len(payload)), payload},
			{"checksum-present", 0xb001, 0x880b, uint16(len(payload)), payload},
		} {
			t.Run(fmt.Sprintf("TCP%t/%s", tcp, tc.name), func(t *testing.T) {
				gre := pptpV1TestGRE(tc.flags, tc.protocol, tc.length, 42, 0, 7, tc.body)
				packet := gopacket.NewPacket(pptpV1TestOuter(t, gre, false), protocolPacketDecoder{decoder: layers.LinkTypeEthernet}, gopacket.Default)
				require.NotNil(t, packet.ErrorLayer())
				var failure *pptpHeaderError
				require.ErrorAs(t, packet.ErrorLayer().Error(), &failure)
				require.Nil(t, packet.Layer(layers.LayerTypeUDP))
				require.Nil(t, packet.Layer(layers.LayerTypeTCP))
				packet.Metadata().CaptureInfo = withEvidence(gopacket.CaptureInfo{Timestamp: time.Unix(10000, 0)}, captureEvidence{Ref: PacketReference{Number: 1, Domain: CaptureDomain{Interface: 7}}})
				var events []*ProtocolEvent
				a := coreFragmentParser(&events, 1<<20, 16)
				out, consumed := a.networkPacket(packet)
				require.True(t, consumed)
				require.Nil(t, out)
				require.Len(t, events, 1)
				e := events[0]
				require.Equal(t, "network-encapsulation-refusal", e.Profile)
				require.Equal(t, "PPTPHeaderAdmission", e.ExpertCode)
				require.Equal(t, "limited", e.Status)
				require.Equal(t, "PPTP", e.Session["Unselected Carrier"])
				require.Equal(t, uint8(1), e.Session["Observed GRE Version"])
				require.Equal(t, "unconfirmed", e.Session["Child Header Identity"])
				require.NotEmpty(t, e.Session["Parent Admission Reason"])
				require.NotContains(t, e.Session, "Expected IP Version")
				require.Zero(t, e.FlowID)
				require.Zero(t, e.TCPFlowID)
				require.Empty(t, e.Raw)
				require.Empty(t, e.Source)
				require.Empty(t, e.Destination)
				require.Equal(t, []PacketReference{{Number: 1, Domain: e.Domain}}, e.SourceBytes.PacketRefs)
				_, err := e.GetFields()
				require.Error(t, err)
				require.Nil(t, a.udpSessions)
				require.Empty(t, a.fragments.sets)
				require.Zero(t, a.buffered.Load())
			})
		}
	}
}

func TestPPTPV1WrongEtherTypeCannotPublishARP(t *testing.T) {
	arp := &layers.ARP{AddrType: layers.LinkTypeEthernet, Protocol: layers.EthernetTypeIPv4, HwAddressSize: 6, ProtAddressSize: 4, Operation: 1, SourceHwAddress: []byte{2, 0, 0, 0, 0, 1}, SourceProtAddress: []byte{192, 0, 2, 1}, DstHwAddress: make([]byte, 6), DstProtAddress: []byte{192, 0, 2, 2}}
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true}, arp))
	wire := pptpV1TestOuter(t, pptpV1TestGRE(0x2001, 0x0806, uint16(len(b.Bytes())), 42, 0, 0, b.Bytes()), false)
	packet := gopacket.NewPacket(wire, protocolPacketDecoder{decoder: layers.LinkTypeEthernet}, gopacket.Default)
	require.Nil(t, packet.Layer(layers.LayerTypeARP))
	var events []*ProtocolEvent
	a := coreFragmentParser(&events, 1<<20, 16)
	_, consumed := a.networkPacket(packet)
	require.True(t, consumed)
	require.Len(t, events, 1)
	require.Equal(t, "ip", events[0].Protocol)
	require.Equal(t, "PPTPHeaderAdmission", events[0].ExpertCode)
}

func TestPPTPV1ACKOnlyIsAHeaderObservationWithoutChildState(t *testing.T) {
	wire := pptpV1TestOuter(t, pptpV1TestGRE(0x2081, 0x880b, 0, 42, 0, 7, nil), false)
	packet := gopacket.NewPacket(wire, protocolPacketDecoder{decoder: layers.LinkTypeEthernet}, gopacket.Default)
	require.Nil(t, packet.ErrorLayer())
	require.Nil(t, packet.Layer(layers.LayerTypePPP))
	var events []*ProtocolEvent
	a := coreFragmentParser(&events, 1<<20, 16)
	_, consumed := a.networkPacket(packet)
	require.True(t, consumed)
	require.Len(t, events, 1)
	e := events[0]
	require.Equal(t, "pptp-acknowledgment", e.Profile)
	require.Equal(t, "limited", e.Status)
	require.Equal(t, "headers", e.Completeness)
	require.Equal(t, uint16(42), e.Session["PPTP Call ID"])
	require.Equal(t, uint32(7), e.Session["PPTP Acknowledgment Number"])
	require.Equal(t, "not present", e.Session["Child Header Identity"])
	require.Equal(t, "/pptp:198.51.100.1>198.51.100.2:42", e.Domain.Encapsulation)
	require.Zero(t, e.FlowID)
	require.Zero(t, e.TCPFlowID)
	require.Nil(t, a.udpSessions)
	require.Empty(t, a.fragments.sets)
}

func TestUnselectedGeneveParentPrecedesPPTPV1GrammarClaim(t *testing.T) {
	inner := pptpV1TestInner(t, false, 0, false, nil)
	child := pptpV1TestOuter(t, pptpV1TestGRE(0x2001, 0x0800, 0, 42, 0, 0, inner), false)
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 3}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 4}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: net.IPv4(198, 51, 100, 3), DstIP: net.IPv4(198, 51, 100, 4)}
	udp := &layers.UDP{SrcPort: 45000, DstPort: 6081}
	require.NoError(t, udp.SetNetworkLayerForChecksum(ip))
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, udp, &layers.Geneve{Protocol: layers.EthernetTypeTransparentEthernetBridging, VNI: 42, OAMPacket: true}, gopacket.Payload(child)))
	packet := gopacket.NewPacket(b.Bytes(), protocolPacketDecoder{decoder: layers.LinkTypeEthernet}, gopacket.Default)
	var events []*ProtocolEvent
	a := coreFragmentParser(&events, 1<<20, 16)
	_, consumed := a.networkPacket(packet)
	require.True(t, consumed)
	require.Len(t, events, 1)
	require.Equal(t, "EncapsulationDepthOrProfile", events[0].ExpertCode)
	require.NotContains(t, events[0].Session, "Observed GRE Version")
	require.NotContains(t, events[0].Session, "Unselected Carrier")
}

func pptpV1TestCapture(t testing.TB, packets [][]byte, futureMiddle bool) []byte {
	t.Helper()
	var capture bytes.Buffer
	w := pcapgo.NewWriter(&capture)
	require.NoError(t, w.WriteFileHeader(65535, layers.LinkTypeEthernet))
	for n, wire := range packets {
		stamp := int64(100 + n)
		if futureMiddle && n == 1 {
			stamp = 10000
		}
		require.NoError(t, w.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(stamp, 0), Length: len(wire), CaptureLength: len(wire)}, wire))
	}
	return bytes.Clone(capture.Bytes())
}

func TestPPTPV1TCPReassemblyKeepsCallAcrossDifferentPayloadLengths(t *testing.T) {
	request := []byte("GET /gre-v1 HTTP/1.1\r\nHost: example.test\r\n\r\n")
	var packets [][]byte
	seq := uint32(1000)
	for n, body := range [][]byte{nil, request[:10], request[10:]} {
		inner := pptpV1TestInner(t, true, seq, n == 0, body)
		packets = append(packets, pptpV1TestOuter(t, pptpV1TestData(t, inner, 42, uint32(n)), false))
		seq += uint32(len(body))
		if n == 0 {
			seq++
		}
	}
	for _, protocols := range []bool{false, true} {
		t.Run(fmt.Sprintf("protocols%t", protocols), func(t *testing.T) {
			var mu sync.Mutex
			created := 0
			var domains []CaptureDomain
			owners := map[uint64]uint64{}
			var events []*ProtocolEvent
			opts := []CaptureOption{WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: true, Stream: true}), WithOnTrafficFlowCreated(func(f *TrafficFlow) {
				mu.Lock()
				defer mu.Unlock()
				created++
				domains = append(domains, f.CaptureDomain())
			}), WithOnTrafficFlowPacket(func(f *TrafficFlow, _ *TrafficConnection, refs []PacketReference, _ time.Time) {
				mu.Lock()
				defer mu.Unlock()
				for _, ref := range refs {
					owners[ref.Number] = f.Index
				}
			})}
			if protocols {
				opts = append(opts, WithOnProtocolMessage(func(e *ProtocolEvent) { mu.Lock(); defer mu.Unlock(); events = append(events, e) }))
			}
			require.NoError(t, ReplayPcap(bytes.NewReader(pptpV1TestCapture(t, packets, false)), opts...))
			require.Equal(t, 1, created)
			require.Equal(t, []CaptureDomain{{Encapsulation: "/pptp:198.51.100.1>198.51.100.2:42"}}, domains)
			require.Equal(t, map[uint64]uint64{1: 1, 2: 1, 3: 1}, owners)
			if protocols {
				require.Len(t, events, 1)
				require.Equal(t, "http", events[0].Protocol)
				require.Equal(t, "decoded", events[0].Status)
				require.Equal(t, request, events[0].Raw)
				require.Equal(t, uint64(1), events[0].TCPFlowID)
			}
		})
	}
}

func TestPPTPV1CallIDsAndDirectionsCannotMergeSameInnerTuple(t *testing.T) {
	inner := pptpV1TestInner(t, true, 1000, true, nil)
	var packets [][]byte
	for _, reverse := range []bool{false, true} {
		for _, call := range []uint16{42, 43} {
			packets = append(packets, pptpV1TestOuter(t, pptpV1TestData(t, inner, call, 0), reverse))
		}
	}
	for _, protocols := range []bool{false, true} {
		t.Run(fmt.Sprintf("protocols%t", protocols), func(t *testing.T) {
			var mu sync.Mutex
			domains := map[CaptureDomain]int{}
			opts := []CaptureOption{WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: true, Stream: true}), WithOnTrafficFlowCreated(func(f *TrafficFlow) {
				mu.Lock()
				defer mu.Unlock()
				domains[f.CaptureDomain()]++
			})}
			if protocols {
				opts = append(opts, WithOnProtocolMessage(func(*ProtocolEvent) {}))
			}
			require.NoError(t, ReplayPcap(bytes.NewReader(pptpV1TestCapture(t, packets, false)), opts...))
			require.Len(t, domains, 4)
			for _, count := range domains {
				require.Equal(t, 1, count)
			}
		})
	}
}

func TestPPTPV1InvalidFutureOpeningCannotReplaceMidstreamTCPCall(t *testing.T) {
	// A new observed SYN after an existing midstream observation is an admitted
	// opening. Two uncorroborated SYNs alone do not prove separate generations.
	warm := pptpV1TestOuter(t, pptpV1TestData(t, pptpV1TestInner(t, true, 1000, false, []byte("GET /warm HTTP/1.1\r\nHost: example.test\r\n\r\n")), 42, 0), false)
	opening := pptpV1TestOuter(t, pptpV1TestData(t, pptpV1TestInner(t, true, 1<<30, true, nil), 42, 1), false)
	for _, invalid := range []bool{false, true} {
		for _, protocols := range []bool{false, true} {
			t.Run(fmt.Sprintf("invalid%t/protocols%t", invalid, protocols), func(t *testing.T) {
				candidate := bytes.Clone(opening)
				if invalid {
					binary.BigEndian.PutUint16(candidate[34:], 0x3801) // reserved bit, same Call ID and data length
				}
				var mu sync.Mutex
				created := 0
				owners := map[uint64]uint64{}
				opts := []CaptureOption{WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: true, Stream: true}), WithOnTrafficFlowCreated(func(*TrafficFlow) { mu.Lock(); defer mu.Unlock(); created++ }), WithOnTrafficFlowPacket(func(f *TrafficFlow, _ *TrafficConnection, refs []PacketReference, _ time.Time) {
					mu.Lock()
					defer mu.Unlock()
					for _, ref := range refs {
						owners[ref.Number] = f.Index
					}
				})}
				if protocols {
					opts = append(opts, WithOnProtocolMessage(func(*ProtocolEvent) {}))
				}
				require.NoError(t, ReplayPcap(bytes.NewReader(pptpV1TestCapture(t, [][]byte{warm, candidate, warm}, true)), opts...))
				if invalid {
					require.Equal(t, 1, created)
					require.Equal(t, map[uint64]uint64{1: 1, 3: 1}, owners)
				} else {
					require.Equal(t, 2, created, "a real admitted distant SYN must still create a new generation")
					require.Contains(t, owners, uint64(2))
				}
			})
		}
	}
}
