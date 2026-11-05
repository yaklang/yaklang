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
	"github.com/stretchr/testify/require"
)

func TestIPv4ZeroLengthIPIPParentCannotEstablishChild(t *testing.T) {
	for _, profile := range []string{"ipip", "6in4"} {
		for _, at := range []int{14, 34} {
			t.Run(fmt.Sprintf("%s/parent%d", profile, at), func(t *testing.T) {
				valid := tunnelAdmissionWire(t, profile, 1, "198.51.100.1", false, 2, false)
				bad := zeroLengthFragmentWire(t, valid, at)
				events := tunnelAdmissionReplay(t, valid, bad, valid)
				require.Len(t, events, 3)
				for _, n := range []int{0, 2} {
					require.Equal(t, "dns", events[n].Protocol)
					require.Equal(t, "decoded", events[n].Status)
					require.EqualValues(t, n+1, events[n].SourceBytes.CompletedIn.Number)
				}
				e := events[1]
				require.Equal(t, "IPv4CarrierExtentUnavailable", e.ExpertCode)
				require.Equal(t, "ipv4-carrier-extent-refusal", e.Profile)
				require.Equal(t, "headers", e.Completeness)
				require.Equal(t, "malformed", e.Status)
				require.Equal(t, false, e.Session["IP-in-IP Normalization Admitted"])
				require.Equal(t, false, e.Session["Network Transport Admitted"])
				require.EqualValues(t, 0, e.Session["Observed Total Length"])
				require.EqualValues(t, bad[at+9], e.Session["Observed Next Protocol"])
				require.Len(t, e.Session["Observed Header Layers"], (at-14)/20+2)
				require.Empty(t, e.Raw)
				require.Empty(t, e.SourceBytes.SHA256)
				require.Nil(t, e.SourceBytes.CompletedIn)
				require.Equal(t, []PacketReference{{Number: 2, Domain: e.Domain}}, e.SourceBytes.PacketRefs)
				require.Zero(t, e.FlowID)
				require.Zero(t, e.TCPFlowID)
			})
		}
	}
}

func TestIPv4ZeroLengthIPIPParentCannotCreateTCPFlow(t *testing.T) {
	for _, workers := range []int{1, 2, 4} {
		for _, protocols := range []bool{false, true} {
			t.Run(fmt.Sprintf("workers%d/protocols%t", workers, protocols), func(t *testing.T) {
				wire := zeroLengthFragmentWire(t, tunnelAdmissionWire(t, "ipip", 1, "198.51.100.1", false, 1, true), 14)
				flows := 0
				var mu sync.Mutex
				var stats TCPReassemblyStats
				opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: true, Stream: true}), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { stats = s }), WithOnTrafficFlowCreated(func(*TrafficFlow) { mu.Lock(); defer mu.Unlock(); flows++ })}
				if protocols {
					opts = append(opts, WithOnProtocolMessage(func(e *ProtocolEvent) { require.Equal(t, "IPv4CarrierExtentUnavailable", e.ExpertCode) }))
				}
				require.NoError(t, ReplayPcap(bytes.NewReader(icmpFragmentCapture(t, [][]byte{wire})), opts...))
				require.Zero(t, flows)
				require.Zero(t, stats.DeliveredBytes)
			})
		}
	}
}

func ipipParentAroundPacket(t *testing.T, packet gopacket.Packet, zero bool) []byte {
	t.Helper()
	inner := packet.NetworkLayer()
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolIPv4, SrcIP: net.ParseIP("198.51.100.1"), DstIP: net.ParseIP("198.51.100.2")}
	wire := append(bytes.Clone(inner.LayerContents()), inner.LayerPayload()...)
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, gopacket.Payload(wire)))
	if zero {
		return zeroLengthFragmentWire(t, b.Bytes(), 14)
	}
	return bytes.Clone(b.Bytes())
}

func TestIPv4ZeroLengthIPIPParentCannotAdvanceFragmentState(t *testing.T) {
	var events []*ProtocolEvent
	a := coreFragmentParser(&events, 1<<20, 16)
	t.Cleanup(func() { a.closeFragments(); require.Zero(t, a.buffered.Load()) })
	original := coreIPPackets(t, "ipv4-options")
	domain := CaptureDomain{Interface: 17, Encapsulation: "/vlan:100"}
	first := gopacket.NewPacket(ipipParentAroundPacket(t, original[0], false), protocolPacketDecoder{decoder: layers.LinkTypeEthernet}, gopacket.Default)
	_, consumed := a.networkPacket(fragmentClockPacket(first, domain, 1, time.Unix(100, 0), false))
	require.True(t, consumed)
	before := a.buffered.Load()
	var activeDomain CaptureDomain
	for key := range a.fragments.sets {
		activeDomain = key.domain
	}
	require.Contains(t, activeDomain.Encapsulation, "/ipip:")
	for i := 0; i < 4; i++ {
		wire := ipipParentAroundPacket(t, original[i%2], true)
		binary.BigEndian.PutUint16(wire[18:20], uint16(i))
		wire = zeroLengthFragmentWire(t, wire, 14)
		packet := gopacket.NewPacket(wire, protocolPacketDecoder{decoder: layers.LinkTypeEthernet}, gopacket.Default)
		packet.Metadata().CaptureInfo = withEvidence(gopacket.CaptureInfo{Timestamp: time.Unix(10000+int64(i), 0), CaptureLength: len(wire), Length: len(wire)}, captureEvidence{Ref: PacketReference{Number: uint64(i + 2), Domain: domain}})
		q, consumed := a.networkPacket(packet)
		require.True(t, consumed)
		require.Nil(t, q)
		require.Equal(t, before, a.buffered.Load(), "an invalid enclosing extent must not allocate a child ID or advance the fragment clock")
		require.Len(t, a.fragments.sets, 1)
		require.False(t, a.fragments.blocked)
	}
	require.Len(t, events, 4)
	for _, e := range events {
		require.Equal(t, "IPv4CarrierExtentUnavailable", e.ExpertCode)
	}
	last := gopacket.NewPacket(ipipParentAroundPacket(t, original[1], false), protocolPacketDecoder{decoder: layers.LinkTypeEthernet}, gopacket.Default)
	q, consumed := a.networkPacket(fragmentClockPacket(last, domain, 6, time.Unix(101, 0), false))
	require.False(t, consumed)
	require.NotNil(t, q)
	require.Equal(t, []PacketReference{{Number: 1, Domain: activeDomain}, {Number: 6, Domain: activeDomain}}, packetEvidence(q).refs)
}

func TestIPv4ZeroLengthIPIPParentPreservesStrictChildDecoderError(t *testing.T) {
	wire := zeroLengthFragmentWire(t, tunnelAdmissionWire(t, "ipip", 1, "198.51.100.1", false, 1, true), 14)
	wire = wire[:44] // Only ten bytes of the declared inner IPv4 fixed header.
	var events []*ProtocolEvent
	err := ReplayPcap(bytes.NewReader(icmpFragmentCapture(t, [][]byte{wire})), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }))
	require.Error(t, err, "an enclosing refusal must not erase the strict captured decoder error")
	require.Len(t, events, 1)
	require.Equal(t, "IPv4CarrierExtentUnavailable", events[0].ExpertCode)
	require.Zero(t, events[0].TCPFlowID)
}

func FuzzIPv4ZeroLengthIPIPParentStateIsolation(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0, 1, 2, 3, 127, 255})
	f.Fuzz(func(t *testing.T, ops []byte) {
		if len(ops) > 64 {
			return
		}
		var events []*ProtocolEvent
		a := coreFragmentParser(&events, 2048, 2)
		t.Cleanup(func() { a.closeFragments(); require.Zero(t, a.buffered.Load()) })
		original := coreIPPackets(t, "ipv4-options")
		domain := CaptureDomain{Interface: 17, Encapsulation: "/vlan:100"}
		first := gopacket.NewPacket(ipipParentAroundPacket(t, original[0], false), protocolPacketDecoder{decoder: layers.LinkTypeEthernet}, gopacket.Default)
		_, consumed := a.networkPacket(fragmentClockPacket(first, domain, 1, time.Unix(100, 0), false))
		require.True(t, consumed)
		require.Len(t, a.fragments.sets, 1)
		var key fragmentKey
		var state *fragmentSet
		for k, v := range a.fragments.sets {
			key, state = k, v
		}
		snapshot := *state
		snapshot.header = bytes.Clone(state.header)
		snapshot.parts = append([]ipFragment(nil), state.parts...)
		for i, part := range state.parts {
			snapshot.parts[i].data = bytes.Clone(part.data)
			snapshot.parts[i].parents = append([]uint64(nil), part.parents...)
			snapshot.parts[i].refs = append([]PacketReference(nil), part.refs...)
		}
		before := a.buffered.Load()
		for i, op := range ops {
			wire := ipipParentAroundPacket(t, original[int(op)&1], true)
			binary.BigEndian.PutUint16(wire[18:20], uint16(op))
			wire = zeroLengthFragmentWire(t, wire, 14)
			badDomain := domain
			if op&2 != 0 {
				badDomain.Interface++
			}
			if op&4 != 0 {
				badDomain.Encapsulation = "/vlan:101"
			}
			packet := gopacket.NewPacket(wire, protocolPacketDecoder{decoder: layers.LinkTypeEthernet}, gopacket.Default)
			q, consumed := a.networkPacket(fragmentClockPacket(packet, badDomain, uint64(i+2), time.Unix(10000+int64(i), 0), false))
			require.True(t, consumed)
			require.Nil(t, q)
			require.Len(t, a.fragments.sets, 1)
			require.Equal(t, snapshot, *a.fragments.sets[key])
			require.Equal(t, before, a.buffered.Load())
			require.False(t, a.fragments.blocked)
		}
		last := gopacket.NewPacket(ipipParentAroundPacket(t, original[1], false), protocolPacketDecoder{decoder: layers.LinkTypeEthernet}, gopacket.Default)
		q, consumed := a.networkPacket(fragmentClockPacket(last, domain, uint64(len(ops)+2), time.Unix(101, 0), false))
		require.False(t, consumed)
		require.NotNil(t, q)
		require.Equal(t, []PacketReference{{Number: 1, Domain: key.domain}, {Number: uint64(len(ops) + 2), Domain: key.domain}}, packetEvidence(q).refs)
		for _, e := range events {
			require.Equal(t, "IPv4CarrierExtentUnavailable", e.ExpertCode)
		}
	})
}
