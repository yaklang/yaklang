package pcaputil

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func TestVXLANBadParentCannotPublishARPOrCreateUDPOrTCPState(t *testing.T) {
	arp := &layers.ARP{AddrType: layers.LinkTypeEthernet, Protocol: layers.EthernetTypeIPv4, HwAddressSize: 6, ProtAddressSize: 4, Operation: 1, SourceHwAddress: []byte{2, 0, 0, 0, 0, 1}, SourceProtAddress: []byte{192, 0, 2, 1}, DstHwAddress: make([]byte, 6), DstProtAddress: []byte{192, 0, 2, 2}}
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true}, arp))
	cases := map[string][]byte{
		"missing-I-DNS": append(vxlanProfileTestHeader(0, 0, 0, 42, 0), vxlanProfileTestDNS(t)...),
		"missing-I-TCP": append(vxlanProfileTestHeader(0xf7, 0xff, 0xffff, 42, 0xff), vxlanProfileTestEthernet(pptpV1TestInner(t, true, 1000, true, nil), layers.EthernetTypeIPv4)...),
		"missing-I-ARP": append(vxlanProfileTestHeader(0, 0, 0, 42, 0), vxlanProfileTestEthernet(b.Bytes(), layers.EthernetTypeARP)...),
	}
	for size := 0; size < 8; size++ {
		cases[fmt.Sprintf("prefix%d", size)] = bytes.Clone(vxlanProfileTestHeader(8, 0, 0, 42, 0)[:size])
	}
	for size := 0; size < 14; size++ {
		cases[fmt.Sprintf("Ethernet-prefix%d", size)] = append(vxlanProfileTestHeader(8, 0, 0, 42, 0), vxlanProfileTestDNS(t)[:size]...)
	}
	for name, payload := range cases {
		for _, allowIncomplete := range []bool{false, true} {
			for _, protocols := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/incomplete%t/protocols%t", name, allowIncomplete, protocols), func(t *testing.T) {
					wire := vxlanProfileTestOuter(t, payload, 55000, 4789, false)
					var conf *CaptureConfig
					var mu sync.Mutex
					var events []*ProtocolEvent
					var stats TCPReassemblyStats
					created, delivered, sources := 0, 0, 0
					var input int64
					var UDPAllocated bool
					var fragments int
					opts := []CaptureOption{func(c *CaptureConfig) error { conf = c; return nil }, WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: allowIncomplete, Stream: true}), WithOnTrafficFlowCreated(func(*TrafficFlow) { mu.Lock(); defer mu.Unlock(); created++ }), WithOnTrafficFlowPacket(func(*TrafficFlow, *TrafficConnection, []PacketReference, time.Time) {
						mu.Lock()
						defer mu.Unlock()
						delivered++
					}), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { mu.Lock(); defer mu.Unlock(); stats = s }), WithEveryPacket(func(gopacket.Packet) {
						mu.Lock()
						defer mu.Unlock()
						sources++
						parser := conf.binParser
						if parser == nil {
							parser = conf.transportNormalizer
						}
						input = int64(parser.input.Load())
						parser.udpMu.Lock()
						UDPAllocated = parser.udpSessions != nil
						parser.udpMu.Unlock()
						parser.fragments.mu.Lock()
						fragments = len(parser.fragments.sets)
						parser.fragments.mu.Unlock()
					})}
					if protocols {
						opts = append(opts, WithOnProtocolMessage(func(e *ProtocolEvent) { mu.Lock(); defer mu.Unlock(); events = append(events, e) }))
					}
					err := ReplayPcap(bytes.NewReader(pptpV1TestCapture(t, [][]byte{wire}, false)), opts...)
					if allowIncomplete {
						require.NoError(t, err)
					} else {
						require.ErrorContains(t, err, "VXLAN header")
					}
					mu.Lock()
					defer mu.Unlock()
					require.Equal(t, 1, sources)
					require.Zero(t, created)
					require.Zero(t, delivered)
					require.Zero(t, input)
					require.False(t, UDPAllocated, "check before Close can clear state")
					require.Zero(t, fragments)
					require.EqualValues(t, 1, stats.CapturedPackets)
					require.EqualValues(t, 1, stats.DecodeErrors)
					if !protocols {
						require.Empty(t, events)
						return
					}
					require.Len(t, events, 1)
					e := events[0]
					require.Equal(t, "ip", e.Protocol)
					require.Equal(t, "VXLANHeaderAdmission", e.ExpertCode)
					require.Equal(t, "limited", e.Status)
					require.Equal(t, "unconfirmed", e.Session["Child Header Identity"])
					require.Zero(t, e.FlowID)
					require.Zero(t, e.TCPFlowID)
					require.Empty(t, e.Raw)
					require.Empty(t, e.Source)
					require.Empty(t, e.Destination)
					require.Equal(t, []PacketReference{{Number: 1, Domain: e.Domain}}, e.SourceBytes.PacketRefs)
					_, fieldError := e.GetFields()
					require.Error(t, fieldError)
				})
			}
		}
	}
}

func TestVXLANUnselectedEnclosingParentPrecedesChildHeaderClaim(t *testing.T) {
	for _, gpe := range []bool{false, true} {
		port := layers.UDPPort(4789)
		header := vxlanProfileTestHeader(0, 0, 0, 42, 0)
		if gpe {
			port, header = 4790, vxlanProfileTestHeader(12, 0, 3, 42, 0)
		}
		inner := vxlanProfileTestOuter(t, append(header, vxlanProfileTestDNS(t)...), 55000, port, false)
		// A decoded child under Geneve OAM is still not an admitted child.
		outer := vxlanProfileTestOuter(t, append([]byte{0, 0x80, 0x65, 0x58, 0, 0, 7, 0}, inner...), 55001, 6081, false)
		var events []*ProtocolEvent
		var stats TCPReassemblyStats
		require.NoError(t, ReplayPcap(bytes.NewReader(pptpV1TestCapture(t, [][]byte{outer}, false)), WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: false}), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { stats = s })))
		require.Len(t, events, 1)
		require.Equal(t, "EncapsulationDepthOrProfile", events[0].ExpertCode)
		require.NotContains(t, events[0].Session, "VXLAN-GPE Version")
		require.NotContains(t, events[0].Session, "Observed VXLAN Flags")
		require.Zero(t, stats.DecodeErrors)
	}
}

func TestVXLANGPEObservationIsHeaderOnlyNotAnOuterUDPApplication(t *testing.T) {
	for _, protocols := range []bool{false, true} {
		for _, next := range []byte{0, 1, 2, 3, 4, 127} {
			flags := byte(8)
			if next != 0 {
				flags |= 4
			}
			wire := vxlanProfileTestOuter(t, append(vxlanProfileTestHeader(flags, 0, uint16(next), 42, 0), vxlanProfileTestDNS(t)...), 55000, 4790, false)
			var mu sync.Mutex
			var events []*ProtocolEvent
			var stats TCPReassemblyStats
			created := 0
			opts := []CaptureOption{WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: false, Stream: true}), WithOnTrafficFlowCreated(func(*TrafficFlow) { mu.Lock(); defer mu.Unlock(); created++ }), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { mu.Lock(); defer mu.Unlock(); stats = s })}
			if protocols {
				opts = append(opts, WithOnProtocolMessage(func(e *ProtocolEvent) { mu.Lock(); defer mu.Unlock(); events = append(events, e) }))
			}
			require.NoError(t, ReplayPcap(bytes.NewReader(pptpV1TestCapture(t, [][]byte{wire}, false)), opts...))
			mu.Lock()
			require.Zero(t, stats.DecodeErrors, "an unimplemented data profile is not malformed")
			require.Zero(t, created)
			if protocols {
				require.Len(t, events, 1)
				e := events[0]
				require.Equal(t, "VXLANProfileUnselected", e.ExpertCode)
				require.Equal(t, "headers", e.Completeness)
				require.Equal(t, uint8(next), e.Session["VXLAN-GPE Next Protocol"])
				require.Equal(t, false, e.Session["Payload Profile Implemented"])
				require.Equal(t, "/vxlan-gpe:198.51.100.1~198.51.100.2:42", e.Domain.Encapsulation)
				require.Empty(t, e.Raw)
				require.Zero(t, e.FlowID)
				require.Zero(t, e.TCPFlowID)
			}
			mu.Unlock()
		}
	}
}

func TestVXLANReservedAndEntropyChangesKeepOneTCPReassembly(t *testing.T) {
	parts := []struct {
		seq     uint32
		syn     bool
		payload string
		flags   byte
	}{{1000, true, "", 8}, {1001, false, "GET /vxlan HTTP/1.1\r\n", 0xff}, {1022, false, "Host: example.test\r\n\r\n", 0x0c}}
	parts[2].seq = parts[1].seq + uint32(len(parts[1].payload))
	var packets [][]byte
	for n, part := range parts {
		inner := vxlanProfileTestEthernet(pptpV1TestInner(t, true, part.seq, part.syn, []byte(part.payload)), layers.EthernetTypeIPv4)
		packets = append(packets, vxlanProfileTestOuter(t, append(vxlanProfileTestHeader(part.flags, byte(n*17), uint16(n), 42, byte(n)), inner...), layers.UDPPort(55000+n), 4789, false))
	}
	var events []*ProtocolEvent
	var owned []PacketReference
	created := 0
	require.NoError(t, ReplayPcap(bytes.NewReader(pptpV1TestCapture(t, packets, false)), WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: true, Stream: true}), WithOnTrafficFlowCreated(func(*TrafficFlow) { created++ }), WithOnTrafficFlowPacket(func(_ *TrafficFlow, _ *TrafficConnection, refs []PacketReference, _ time.Time) {
		owned = append(owned, refs...)
	}), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })))
	require.Equal(t, 1, created)
	require.Len(t, owned, 3)
	require.Len(t, events, 1)
	e := events[0]
	require.Equal(t, "http", e.Protocol)
	require.Equal(t, "decoded", e.Status)
	require.Equal(t, []byte(parts[1].payload+parts[2].payload), e.Raw)
	require.Equal(t, "/vxlan:198.51.100.1~198.51.100.2:42", e.Domain.Encapsulation)
	require.Equal(t, []PacketReference{{Number: 2, Domain: e.Domain}, {Number: 3, Domain: e.Domain}}, e.SourceBytes.PacketRefs)
}

func TestVXLANVNIsCannotMergeSameInnerTuple(t *testing.T) {
	inner := vxlanProfileTestEthernet(pptpV1TestInner(t, true, 1000, true, nil), layers.EthernetTypeIPv4)
	var packets [][]byte
	for _, vni := range []uint32{0, 42, 43, 0xffffff} {
		packets = append(packets, vxlanProfileTestOuter(t, append(vxlanProfileTestHeader(8, 0, 0, vni, 0), inner...), 55000, 4789, false))
	}
	domains := map[CaptureDomain]int{}
	require.NoError(t, ReplayPcap(bytes.NewReader(pptpV1TestCapture(t, packets, false)), WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: true, Stream: true}), WithOnTrafficFlowCreated(func(f *TrafficFlow) { domains[f.CaptureDomain()]++ })))
	require.Len(t, domains, 4)
	for _, count := range domains {
		require.Equal(t, 1, count)
	}
}

func TestVXLANBadIFutureOpeningCannotReplaceMidstreamTCP(t *testing.T) {
	warm := vxlanProfileTestEthernet(pptpV1TestInner(t, true, 1000, false, []byte("GET /warm HTTP/1.1\r\nHost: example.test\r\n\r\n")), layers.EthernetTypeIPv4)
	opening := vxlanProfileTestEthernet(pptpV1TestInner(t, true, 1<<30, true, nil), layers.EthernetTypeIPv4)
	for _, invalid := range []bool{false, true} {
		for _, protocols := range []bool{false, true} {
			flags := byte(8)
			if invalid {
				flags = 0
			}
			packets := [][]byte{vxlanProfileTestOuter(t, append(vxlanProfileTestHeader(8, 0, 0, 42, 0), warm...), 55000, 4789, false), vxlanProfileTestOuter(t, append(vxlanProfileTestHeader(flags, 0, 0, 42, 0), opening...), 55001, 4789, false)}
			created := 0
			opts := []CaptureOption{WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: true, Stream: true}), WithOnTrafficFlowCreated(func(*TrafficFlow) { created++ })}
			if protocols {
				opts = append(opts, WithOnProtocolMessage(func(*ProtocolEvent) {}))
			}
			require.NoError(t, ReplayPcap(bytes.NewReader(pptpV1TestCapture(t, packets, false)), opts...))
			if invalid {
				require.Equal(t, 1, created)
			} else {
				require.Equal(t, 2, created)
			}
		}
	}
}
