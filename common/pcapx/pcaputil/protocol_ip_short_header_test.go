package pcaputil

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func TestIncompleteIPFixedHeadersAreSourceBoundWithoutTransportAdmission(t *testing.T) {
	for _, family := range []int{4, 6} {
		required := 20
		if family == 6 {
			required = 40
		}
		for available := 0; available < required; available++ {
			for _, version := range []uint8{uint8(family), 0, 15} {
				if available == 0 && version != uint8(family) {
					continue
				}
				t.Run(fmt.Sprintf("IPv%d/bytes%d/version%d", family, available, version), func(t *testing.T) {
					wire := bytes.Clone(versionAdmissionWire(t, family)[:14+available])
					if available > 0 {
						wire[14] = wire[14]&15 | version<<4
					}
					packet := gopacket.NewPacket(wire, protocolPacketDecoder{decoder: layers.LinkTypeEthernet}, gopacket.Default)
					domain := CaptureDomain{Interface: 17}
					packet.Metadata().CaptureInfo = withEvidence(gopacket.CaptureInfo{Timestamp: time.Unix(100, 0), CaptureLength: len(wire), Length: len(wire)}, captureEvidence{Ref: PacketReference{Number: 1, Domain: domain}})
					var truncated *ipHeaderTruncatedError
					require.NotNil(t, packet.ErrorLayer())
					require.True(t, errors.As(packet.ErrorLayer().Error(), &truncated))
					if family == 4 {
						var legacy layers.IPv4
						err := legacy.DecodeFromBytes(wire[14:], gopacket.NilDecodeFeedback)
						require.EqualError(t, truncated, err.Error(), "the dependency's original capture error stays literal")
					}
					var events []*ProtocolEvent
					a := coreFragmentParser(&events, 1<<20, 16)
					out, consumed := a.networkPacket(packet)
					require.False(t, consumed)
					require.Same(t, packet, out, "the original decoder error reaches capture accounting")
					require.Len(t, events, 1)
					e := events[0]
					require.Equal(t, "IPHeaderTruncated", e.ExpertCode)
					require.Equal(t, "incomplete", e.Status)
					require.Equal(t, uint8(family), e.Session["Expected IP Version"])
					require.Equal(t, available, e.Session["Available Header Bytes"])
					require.Equal(t, required, e.Session["Required Header Bytes"])
					if available == 0 {
						require.NotContains(t, e.Session, "Observed IP Version", "missing bytes have no guessed version")
					} else {
						require.Equal(t, version, e.Session["Observed IP Version"])
					}
					require.Equal(t, []string{"Ethernet", fmt.Sprintf("IPv%d", family)}, e.Session["Observed Header Layers"])
					require.Equal(t, []PacketReference{{Number: 1, Domain: domain}}, e.SourceBytes.PacketRefs)
					require.Zero(t, e.FlowID)
					require.Zero(t, e.TCPFlowID)
					require.Empty(t, e.Source)
					require.Empty(t, e.Destination)
					require.Empty(t, e.Raw)
					require.Zero(t, a.buffered.Load())
					require.Empty(t, a.fragments.sets)
					require.Nil(t, a.udpSessions)
				})
			}
		}
	}
}

func TestIncompleteIPHeaderPreservesCaptureErrorAccountingAndObservers(t *testing.T) {
	for _, family := range []int{4, 6} {
		for _, protocols := range []bool{false, true} {
			for _, allowIncomplete := range []bool{false, true} {
				t.Run(fmt.Sprintf("IPv%d/protocols%t/allow%t", family, protocols, allowIncomplete), func(t *testing.T) {
					wire := versionAdmissionWire(t, family)[:18]
					var mu sync.Mutex
					var stats TCPReassemblyStats
					flows, observations, diagnostics := 0, 0, 0
					opts := []CaptureOption{WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: allowIncomplete, Stream: true}), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { stats = s }), WithOnTrafficFlowCreated(func(*TrafficFlow) { mu.Lock(); defer mu.Unlock(); flows++ }), WithEveryPacket(func(gopacket.Packet) { mu.Lock(); defer mu.Unlock(); observations++ })}
					if protocols {
						opts = append(opts, WithOnProtocolMessage(func(e *ProtocolEvent) {
							mu.Lock()
							defer mu.Unlock()
							require.Equal(t, "IPHeaderTruncated", e.ExpertCode)
							diagnostics++
						}))
					}
					err := ReplayPcap(bytes.NewReader(icmpFragmentCapture(t, [][]byte{wire})), opts...)
					if allowIncomplete {
						require.NoError(t, err)
					} else {
						require.Error(t, err)
					}
					require.EqualValues(t, 1, stats.DecodeErrors)
					require.Equal(t, 1, observations)
					require.Zero(t, flows)
					if protocols {
						require.Equal(t, 1, diagnostics)
					}
				})
			}
		}
	}
}

func TestAbsentIPHeaderIsNotACompleteEthernetOnlyPacket(t *testing.T) {
	for _, family := range []int{4, 6} {
		wire := versionAdmissionWire(t, family)[:14]
		var stats TCPReassemblyStats
		var events []*ProtocolEvent
		err := ReplayPcap(bytes.NewReader(icmpFragmentCapture(t, [][]byte{wire})), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { stats = s }))
		require.Error(t, err, "a declared but entirely absent fixed IP header is a decode error")
		require.EqualValues(t, 1, stats.DecodeErrors)
		require.Len(t, events, 1)
		require.Equal(t, "IPHeaderTruncated", events[0].ExpertCode)
		require.Equal(t, 0, events[0].Session["Available Header Bytes"])
		require.NotContains(t, events[0].Session, "Observed IP Version")
	}
}

func TestIncompleteIPHeaderCannotExpirePendingFragments(t *testing.T) {
	for _, name := range []string{"ipv4-options", "ipv6-in-order"} {
		t.Run(name, func(t *testing.T) {
			var events []*ProtocolEvent
			a := coreFragmentParser(&events, 1<<20, 16)
			t.Cleanup(func() { a.closeFragments(); require.Zero(t, a.buffered.Load()) })
			original := coreIPPackets(t, name)
			domain := CaptureDomain{Interface: 17}
			_, consumed := a.networkPacket(fragmentClockPacket(original[0], domain, 1, time.Unix(100, 0), false))
			require.True(t, consumed)
			buffered := a.buffered.Load()
			wire := bytes.Clone(versionAdmissionWire(t, 4)[:18])
			bad := gopacket.NewPacket(wire, protocolPacketDecoder{decoder: layers.LinkTypeEthernet}, gopacket.Default)
			bad.Metadata().CaptureInfo = withEvidence(gopacket.CaptureInfo{Timestamp: time.Unix(10000, 0), Length: len(wire), CaptureLength: len(wire)}, captureEvidence{Ref: PacketReference{Number: 2, Domain: domain}})
			out, consumed := a.networkPacket(bad)
			require.False(t, consumed)
			require.Same(t, bad, out)
			require.Equal(t, buffered, a.buffered.Load())
			require.Len(t, a.fragments.sets, 1)
			require.False(t, a.fragments.blocked)
			require.Len(t, events, 1)
			require.Equal(t, "IPHeaderTruncated", events[0].ExpertCode)
			out, consumed = a.networkPacket(fragmentClockPacket(original[1], domain, 3, time.Unix(101, 0), false))
			require.False(t, consumed)
			require.NotNil(t, out)
			require.ElementsMatch(t, []PacketReference{{Number: 1, Domain: domain}, {Number: 3, Domain: domain}}, packetEvidence(out).refs)
		})
	}
}

func TestIncompleteIPChildRequiresSelectedParent(t *testing.T) {
	for _, selected := range []bool{false, true} {
		for _, available := range []int{0, 1, 19} {
			t.Run(fmt.Sprintf("selected%t/bytes%d", selected, available), func(t *testing.T) {
				wire := tunnelAdmissionWire(t, "ipip", 1, "198.51.100.1", false, 1, false)
				if selected {
					wire = bytes.Clone(wire[:34+available])
				} else {
					prefix := append(bytes.Clone(wire[:34]), 0x30, 0)
					prefix[23] = 97 // EtherIP is not a selected native parent.
					prefix = append(prefix, wire[:14]...)
					wire = append(prefix, wire[34:34+available]...)
				}
				packet := gopacket.NewPacket(wire, protocolPacketDecoder{decoder: layers.LinkTypeEthernet}, gopacket.Default)
				packet.Metadata().CaptureInfo = withEvidence(gopacket.CaptureInfo{Timestamp: time.Unix(100, 0), CaptureLength: len(wire), Length: len(wire)}, captureEvidence{Ref: PacketReference{Number: 1, Domain: CaptureDomain{Interface: 17}}})
				var events []*ProtocolEvent
				a := coreFragmentParser(&events, 1<<20, 16)
				out, consumed := a.networkPacket(packet)
				require.Len(t, events, 1)
				e := events[0]
				if selected {
					require.False(t, consumed)
					require.Same(t, packet, out)
					require.Equal(t, "IPHeaderTruncated", e.ExpertCode)
					require.Equal(t, available, e.Session["Available Header Bytes"])
					require.Equal(t, []string{"Ethernet", "IPv4", "IPv4"}, e.Session["Observed Header Layers"])
					require.Equal(t, "/ipip:198.51.100.1~198.51.100.2", e.Domain.Encapsulation)
				} else {
					require.False(t, consumed)
					require.Same(t, packet, out)
					require.NotNil(t, out.ErrorLayer(), "refusal must preserve the original capture decoder error")
					require.Equal(t, "network-encapsulation-refusal", e.Profile)
					require.Equal(t, "unconfirmed", e.Session["Child Header Identity"])
					require.NotContains(t, e.Session, "Expected IP Version")
					require.NotContains(t, e.Session, "Available Header Bytes")
				}
				require.Zero(t, e.FlowID)
				require.Zero(t, e.TCPFlowID)
				require.Nil(t, a.udpSessions)
				require.Zero(t, a.buffered.Load())
			})
		}
	}
}

func FuzzIncompleteIPHeaderCannotMutateFragmentState(f *testing.F) {
	f.Add(uint8(4), uint8(0), uint8(0), int64(10000))
	f.Add(uint8(4), uint8(19), uint8(0xff), int64(0))
	f.Add(uint8(6), uint8(39), uint8(0x45), int64(1<<50))
	f.Add(uint8(6), uint8(1), uint8(0x60), int64(-1))
	f.Fuzz(func(t *testing.T, family, cut, first uint8, timestamp int64) {
		family = 4 + family&2
		required := 20
		if family == 6 {
			required = 40
		}
		available := int(cut) % required
		wire := bytes.Clone(versionAdmissionWire(t, int(family))[:14+available])
		if available > 0 {
			wire[14] = first
		}
		var events []*ProtocolEvent
		a := coreFragmentParser(&events, 1<<20, 16)
		t.Cleanup(func() { a.closeFragments(); require.Zero(t, a.buffered.Load()) })
		original := coreIPPackets(t, "ipv4-options")
		domain := CaptureDomain{Interface: 17}
		_, consumed := a.networkPacket(fragmentClockPacket(original[0], domain, 1, time.Unix(100, 0), false))
		require.True(t, consumed)
		buffered := a.buffered.Load()
		bad := gopacket.NewPacket(wire, protocolPacketDecoder{decoder: layers.LinkTypeEthernet}, gopacket.Default)
		bad.Metadata().CaptureInfo = withEvidence(gopacket.CaptureInfo{Timestamp: time.Unix(timestamp, 0), Length: len(wire), CaptureLength: len(wire)}, captureEvidence{Ref: PacketReference{Number: 2, Domain: domain}})
		out, consumed := a.networkPacket(bad)
		require.False(t, consumed)
		require.Same(t, bad, out)
		require.Len(t, events, 1)
		require.Equal(t, "IPHeaderTruncated", events[0].ExpertCode)
		require.Equal(t, buffered, a.buffered.Load())
		require.Len(t, a.fragments.sets, 1)
		require.False(t, a.fragments.blocked)
		require.Nil(t, a.udpSessions)
		out, consumed = a.networkPacket(fragmentClockPacket(original[1], domain, 3, time.Unix(101, 0), false))
		require.False(t, consumed)
		require.NotNil(t, out)
		require.ElementsMatch(t, []PacketReference{{Number: 1, Domain: domain}, {Number: 3, Domain: domain}}, packetEvidence(out).refs)
	})
}
