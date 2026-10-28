package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

// Every control carries a real selected UDP tunnel and reaches the typed child
// failure in the stateless decoder. The unselected cases use existing policy:
// Geneve OAM, or VXLAN plus five IPIP levels exceeding the normalization budget.
// No assertion claims that every VXLAN flag/extension profile is validated.
func typedTunnelErrorWire(t testing.TB, tunnel, child string, selected bool) ([]byte, string) {
	t.Helper()
	var inner []byte
	if child == "PPTP" {
		payload := pptpV1TestInner(t, false, 0, false, nil)
		gre := pptpV1TestGRE(0x2001, 0x0800, 0, 42, 0, 0, payload)
		inner = pptpV1TestOuter(t, gre, false)
	} else {
		require.Equal(t, "short-IP", child)
		inner = append([]byte{2, 0, 0, 0, 0, 2, 2, 0, 0, 0, 0, 1, 8, 0}, 0x45, 0, 0, 20)
	}
	domain := "/" + tunnel + ":198.51.100.1~198.51.100.2:42"
	if tunnel == "vxlan" && !selected {
		payload := bytes.Clone(inner[14:])
		for depth := 0; depth < 5; depth++ {
			ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolIPv4, SrcIP: net.IPv4(203, 0, 113, 1), DstIP: net.IPv4(203, 0, 113, 2)}
			b := gopacket.NewSerializeBuffer()
			require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, ip, gopacket.Payload(payload)))
			payload = bytes.Clone(b.Bytes())
			domain += "/ipip:203.0.113.1~203.0.113.2"
		}
		inner = append(bytes.Clone(inner[:14]), payload...)
	}
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 3}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 4}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: net.IPv4(198, 51, 100, 1), DstIP: net.IPv4(198, 51, 100, 2)}
	udp := &layers.UDP{SrcPort: 45000, DstPort: 6081}
	var carrier gopacket.SerializableLayer = &layers.Geneve{Protocol: layers.EthernetTypeTransparentEthernetBridging, VNI: 42, OAMPacket: !selected}
	if tunnel == "vxlan" {
		udp.DstPort = 4789
		carrier = &layers.VXLAN{ValidIDFlag: true, VNI: 42}
	}
	require.NoError(t, udp.SetNetworkLayerForChecksum(ip))
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, udp, carrier, gopacket.Payload(inner)))
	wire := bytes.Clone(b.Bytes())
	// The outer UDP envelope is complete, so a failed child must never be
	// reclassified as a partial/unknown application on the tunnel's UDP port.
	packet := gopacket.NewPacket(wire, protocolPacketDecoder{decoder: layers.LinkTypeEthernet}, gopacket.Default)
	require.NotNil(t, packet.Layer(layers.LayerTypeUDP))
	require.NotNil(t, packet.ErrorLayer())
	if child == "PPTP" {
		var failure *pptpHeaderError
		require.ErrorAs(t, packet.ErrorLayer().Error(), &failure)
	} else {
		var failure *ipHeaderTruncatedError
		require.ErrorAs(t, packet.ErrorLayer().Error(), &failure)
	}
	require.Equal(t, uint16(len(wire)-34), binary.BigEndian.Uint16(wire[38:40]))
	return wire, domain
}

func TestTypedTunnelChildErrorPrecedesOuterUDPDispatchAndKeepsAccounting(t *testing.T) {
	for _, tunnel := range []string{"geneve", "vxlan"} {
		for _, child := range []string{"PPTP", "short-IP"} {
			for _, selected := range []bool{false, true} {
				for _, allowIncomplete := range []bool{false, true} {
					for _, protocols := range []bool{false, true} {
						t.Run(fmt.Sprintf("%s/%s/selected%t/incomplete%t/protocols%t", tunnel, child, selected, allowIncomplete, protocols), func(t *testing.T) {
							wire, encapsulation := typedTunnelErrorWire(t, tunnel, child, selected)
							var conf *CaptureConfig
							var mu sync.Mutex
							var events []*ProtocolEvent
							var stats TCPReassemblyStats
							var observed []PacketReference
							created, delivered := 0, 0
							var UDPAllocated bool
							var inputBytes int64
							var fragmentSets int
							opts := []CaptureOption{
								func(c *CaptureConfig) error { conf = c; return nil },
								WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: allowIncomplete, Stream: true}),
								WithOnTrafficFlowCreated(func(*TrafficFlow) { mu.Lock(); defer mu.Unlock(); created++ }),
								WithOnTrafficFlowPacket(func(*TrafficFlow, *TrafficConnection, []PacketReference, time.Time) {
									mu.Lock()
									defer mu.Unlock()
									delivered++
								}),
								WithOnTCPReassemblyStats(func(value TCPReassemblyStats) { mu.Lock(); defer mu.Unlock(); stats = value }),
								WithEveryPacket(func(packet gopacket.Packet) {
									mu.Lock()
									defer mu.Unlock()
									observed = append(observed, CapturePacketReference(packet.Metadata().CaptureInfo))
									parser := conf.binParser
									if parser == nil {
										parser = conf.transportNormalizer
									}
									parser.udpMu.Lock()
									UDPAllocated = parser.udpSessions != nil
									parser.udpMu.Unlock()
									inputBytes = int64(parser.input.Load())
									parser.fragments.mu.Lock()
									fragmentSets = len(parser.fragments.sets)
									parser.fragments.mu.Unlock()
								}),
							}
							if protocols {
								opts = append(opts, WithOnProtocolMessage(func(event *ProtocolEvent) { mu.Lock(); defer mu.Unlock(); events = append(events, event) }))
							}
							err := ReplayPcap(bytes.NewReader(pptpV1TestCapture(t, [][]byte{wire}, false)), opts...)
							if selected && !allowIncomplete {
								require.Error(t, err)
								if child == "PPTP" {
									require.ErrorContains(t, err, "PPTP GRE header")
								} else {
									require.ErrorContains(t, err, "ip4 header")
								}
							} else {
								require.NoError(t, err)
							}
							require.Equal(t, uint64(1), stats.CapturedPackets)
							require.Equal(t, uint64(len(wire)), stats.CapturedBytes)
							if selected {
								require.Equal(t, uint64(1), stats.DecodeErrors)
							} else {
								require.Zero(t, stats.DecodeErrors, "an unselected parent consumes the packet before speculative child error accounting")
							}
							require.Zero(t, created)
							require.Zero(t, delivered)
							require.False(t, UDPAllocated, "check before parser close can clear state")
							require.Zero(t, inputBytes, "outer UDP must never enter application dispatch")
							require.Zero(t, fragmentSets)
							require.Len(t, observed, 1)
							require.Equal(t, uint64(1), observed[0].Number)
							require.Zero(t, observed[0].Domain.Section)
							require.Zero(t, observed[0].Domain.Interface)
							if !protocols {
								require.Empty(t, events)
								return
							}
							require.Len(t, events, 1, "one precise boundary outcome; no extra outer UDP event")
							event := events[0]
							require.Equal(t, "ip", event.Protocol)
							require.Equal(t, "network", event.Transport)
							require.Equal(t, encapsulation, event.Domain.Encapsulation)
							require.Equal(t, []PacketReference{{Number: 1, Domain: event.Domain}}, event.SourceBytes.PacketRefs)
							require.Empty(t, event.Raw)
							require.Zero(t, event.FlowID)
							require.Zero(t, event.TCPFlowID)
							if !selected {
								require.Equal(t, "EncapsulationDepthOrProfile", event.ExpertCode)
								require.NotContains(t, event.Session, "Observed GRE Version")
								require.NotContains(t, event.Session, "Expected IP Version")
								require.True(t, strings.HasPrefix(event.Domain.Encapsulation, "/"+tunnel+":"))
							} else if child == "PPTP" {
								require.Equal(t, "PPTPHeaderAdmission", event.ExpertCode)
							} else {
								require.Equal(t, "IPHeaderTruncated", event.ExpertCode)
							}
						})
					}
				}
			}
		}
	}
}
