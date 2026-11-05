package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
)

// Change only the literal Total Length and repair the header checksum. Zero is
// an unavailable IP extent, not permission to use a TSO-derived fragment size.
func zeroLengthFragmentWire(t *testing.T, raw []byte, at int) []byte {
	t.Helper()
	b := bytes.Clone(raw)
	require.EqualValues(t, 4, b[at]>>4)
	h := b[at : at+int(b[at]&15)*4]
	h[2], h[3], h[10], h[11] = 0, 0, 0, 0
	var sum uint32
	for i := 0; i < len(h); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(h[i : i+2]))
	}
	for sum>>16 != 0 {
		sum = sum&65535 + sum>>16
	}
	binary.BigEndian.PutUint16(h[10:12], ^uint16(sum))
	return b
}

func TestIPv4ZeroLengthFragmentCannotSupplyReassemblyBytes(t *testing.T) {
	for _, zeroIndex := range []int{0, 1} {
		t.Run(fmt.Sprintf("fragment%d", zeroIndex), func(t *testing.T) {
			var events []*ProtocolEvent
			a := coreFragmentParser(&events, 1<<20, 16)
			t.Cleanup(func() { a.closeFragments(); require.Zero(t, a.buffered.Load()) })
			original := coreIPPackets(t, "ipv4-options")
			domain := CaptureDomain{Interface: 17, Encapsulation: "/vlan:100"}
			selected := original[zeroIndex]
			ip := selected.NetworkLayer()
			at := len(selected.Data()) - len(ip.LayerContents()) - len(ip.LayerPayload())
			raw := zeroLengthFragmentWire(t, selected.Data(), at)
			bad := gopacket.NewPacket(raw, protocolPacketDecoder{decoder: selected.Layers()[0].LayerType()}, gopacket.Default)
			bad.Metadata().CaptureInfo = withEvidence(gopacket.CaptureInfo{Timestamp: time.Unix(100, 0), CaptureLength: len(raw), Length: len(raw)}, captureEvidence{Ref: PacketReference{Number: 1, Domain: domain}})
			q, consumed := a.networkPacket(bad)
			require.True(t, consumed)
			require.Nil(t, q)
			require.Zero(t, a.buffered.Load(), "a zero extent cannot contribute bytes or a quarantine")
			require.Empty(t, a.fragments.sets)
			require.Len(t, events, 1)
			require.Equal(t, "IPv4FragmentExtentUnavailable", events[0].ExpertCode)
			require.Equal(t, []PacketReference{{Number: 1, Domain: domain}}, events[0].SourceBytes.PacketRefs)
			for i, p := range original {
				q, consumed = a.networkPacket(fragmentClockPacket(p, domain, uint64(i+2), time.Unix(101+int64(i), 0), false))
			}
			require.False(t, consumed)
			require.NotNil(t, q, "the intact retransmission can complete normally")
			require.ElementsMatch(t, []PacketReference{{Number: 2, Domain: domain}, {Number: 3, Domain: domain}}, packetEvidence(q).refs)
		})
	}
}

func TestIPv4ZeroLengthFragmentCannotExpireOrQuarantineAnExistingID(t *testing.T) {
	for _, sameDomain := range []bool{true, false} {
		t.Run(fmt.Sprint(sameDomain), func(t *testing.T) {
			var events []*ProtocolEvent
			a := coreFragmentParser(&events, 1<<20, 16)
			t.Cleanup(func() { a.closeFragments(); require.Zero(t, a.buffered.Load()) })
			original := coreIPPackets(t, "ipv4-options")
			domain := CaptureDomain{Interface: 17}
			_, consumed := a.networkPacket(fragmentClockPacket(original[0], domain, 1, time.Unix(100, 0), false))
			require.True(t, consumed)
			before := a.buffered.Load()
			selected := original[1]
			at := len(selected.Data()) - len(selected.NetworkLayer().LayerContents()) - len(selected.NetworkLayer().LayerPayload())
			raw := zeroLengthFragmentWire(t, selected.Data(), at)
			badDomain := domain
			if !sameDomain {
				badDomain.Interface++
			}
			bad := gopacket.NewPacket(raw, protocolPacketDecoder{decoder: selected.Layers()[0].LayerType()}, gopacket.Default)
			bad.Metadata().CaptureInfo = withEvidence(gopacket.CaptureInfo{Timestamp: time.Unix(10000, 0), CaptureLength: len(raw), Length: len(raw)}, captureEvidence{Ref: PacketReference{Number: 2, Domain: badDomain}})
			q, consumed := a.networkPacket(bad)
			require.True(t, consumed)
			require.Nil(t, q)
			require.Equal(t, before, a.buffered.Load(), "invalid extent cannot advance any fragment clock")
			require.Len(t, a.fragments.sets, 1)
			require.False(t, a.fragments.blocked)
			require.Len(t, events, 1)
			require.Equal(t, "IPv4FragmentExtentUnavailable", events[0].ExpertCode)
			q, consumed = a.networkPacket(fragmentClockPacket(original[1], domain, 3, time.Unix(101, 0), false))
			require.False(t, consumed)
			require.NotNil(t, q)
			require.ElementsMatch(t, []PacketReference{{Number: 1, Domain: domain}, {Number: 3, Domain: domain}}, packetEvidence(q).refs)
		})
	}
}

func TestIPv4ZeroLengthFragmentsCannotEstablishDNS(t *testing.T) {
	for _, workers := range []int{1, 2, 4} {
		for _, deferred := range []bool{false, true} {
			for _, zero := range []bool{false, true} {
				t.Run(fmt.Sprintf("workers%d/deferred%t/zero%t", workers, deferred, zero), func(t *testing.T) {
					frame := tunnelAdmissionWire(t, "ipip", 1, "198.51.100.1", false, 0, false)
					pieces := icmpFragmentFrames(t, frame, 16, false)
					if zero {
						for i, p := range pieces {
							pieces[i] = zeroLengthFragmentWire(t, p, 14)
						}
					}
					var events []*ProtocolEvent
					var mu sync.Mutex
					var stats ProtocolStats
					err := ReplayPcap(bytes.NewReader(icmpFragmentCapture(t, pieces)), WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolStats(func(s ProtocolStats) { stats = s }), WithOnProtocolMessage(func(e *ProtocolEvent) { mu.Lock(); defer mu.Unlock(); events = append(events, e) }))
					require.NoError(t, err)
					require.Zero(t, stats.BufferedBytes)
					if zero {
						require.Len(t, events, 2)
						for _, e := range events {
							require.Equal(t, "ip", e.Protocol)
							require.Equal(t, "IPv4FragmentExtentUnavailable", e.ExpertCode)
							require.Zero(t, e.FlowID)
							require.Zero(t, e.TCPFlowID)
							require.Empty(t, e.Raw)
						}
						require.Zero(t, stats.Messages)
					} else {
						require.Len(t, events, 1)
						require.Equal(t, "dns", events[0].Protocol)
						require.Empty(t, events[0].Error)
						require.EqualValues(t, 2, events[0].SourceBytes.CompletedIn.Number)
					}
				})
			}
		}
	}
}

func TestIPv4ZeroLengthTCPFragmentsCannotCreateFlowWithoutProtocolEvents(t *testing.T) {
	for _, workers := range []int{1, 2, 4} {
		for _, protocols := range []bool{false, true} {
			t.Run(fmt.Sprintf("workers%d/protocols%t", workers, protocols), func(t *testing.T) {
				frame := tunnelAdmissionWire(t, "ipip", 1, "198.51.100.1", false, 0, true)
				pieces := icmpFragmentFrames(t, frame, 24, false)
				for i, p := range pieces {
					pieces[i] = zeroLengthFragmentWire(t, p, 14)
				}
				flows := 0
				var mu sync.Mutex
				var stats TCPReassemblyStats
				opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: true, Stream: true}), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { stats = s }), WithOnTrafficFlowCreated(func(*TrafficFlow) { mu.Lock(); defer mu.Unlock(); flows++ })}
				if protocols {
					opts = append(opts, WithOnProtocolMessage(func(e *ProtocolEvent) { require.Equal(t, "IPv4FragmentExtentUnavailable", e.ExpertCode) }))
				}
				require.NoError(t, ReplayPcap(bytes.NewReader(icmpFragmentCapture(t, pieces)), opts...))
				require.Zero(t, flows)
				require.Zero(t, stats.DeliveredBytes)
			})
		}
	}
}

func FuzzIPv4ZeroLengthFragmentStateIsolation(f *testing.F) {
	for _, ops := range [][]byte{{}, {0}, {1, 255, 0, 63}, {4, 8, 16, 32, 64, 128}} {
		f.Add(ops)
	}
	f.Fuzz(func(t *testing.T, ops []byte) {
		if len(ops) > 64 {
			return
		}
		var events []*ProtocolEvent
		a := coreFragmentParser(&events, 2048, 2)
		t.Cleanup(func() { a.closeFragments(); require.Zero(t, a.buffered.Load()) })
		original := coreIPPackets(t, "ipv4-options")
		domain := CaptureDomain{Interface: 17, Encapsulation: "/vlan:100"}
		_, consumed := a.networkPacket(fragmentClockPacket(original[0], domain, 1, time.Unix(100, 0), false))
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
			snapshot.parts[i].refs = append([]PacketReference(nil), part.refs...)
			snapshot.parts[i].parents = append([]uint64(nil), part.parents...)
		}
		cost := a.buffered.Load()
		for i, op := range ops {
			p := original[int(op&1)]
			at := len(p.Data()) - len(p.NetworkLayer().LayerContents()) - len(p.NetworkLayer().LayerPayload())
			raw := zeroLengthFragmentWire(t, p.Data(), at)
			wireBefore := bytes.Clone(raw)
			other := domain
			other.Interface += int(op>>2) & 3
			if op&16 != 0 {
				other.Encapsulation = "/vlan:101"
			}
			stamp := []time.Time{{}, time.Unix(98, 0), time.Unix(10000, 0), time.Unix(100, 0)}[int(op>>5)&3]
			bad := gopacket.NewPacket(raw, protocolPacketDecoder{decoder: p.Layers()[0].LayerType()}, gopacket.DecodeOptions{NoCopy: true})
			bad.Metadata().CaptureInfo = withEvidence(gopacket.CaptureInfo{Timestamp: stamp, CaptureLength: len(raw), Length: len(raw)}, captureEvidence{Ref: PacketReference{Number: uint64(i + 2), Domain: other}})
			q, consumed := a.networkPacket(bad)
			require.True(t, consumed)
			require.Nil(t, q)
			require.Equal(t, wireBefore, raw)
			require.Len(t, a.fragments.sets, 1)
			require.Equal(t, &snapshot, a.fragments.sets[key], "even far-future or foreign-domain observations cannot alter the existing assembly")
			require.Equal(t, cost, a.buffered.Load())
			require.False(t, a.fragments.blocked)
		}
		q, consumed := a.networkPacket(fragmentClockPacket(original[1], domain, uint64(len(ops)+2), time.Unix(101, 0), false))
		require.False(t, consumed)
		require.NotNil(t, q)
		want := append(bytes.Clone(original[0].NetworkLayer().LayerPayload()), original[1].NetworkLayer().LayerPayload()...)
		require.Equal(t, want, q.NetworkLayer().LayerPayload())
		require.ElementsMatch(t, []PacketReference{{Number: 1, Domain: domain}, {Number: uint64(len(ops) + 2), Domain: domain}}, packetEvidence(q).refs)
		require.Len(t, events, len(ops))
		for _, event := range events {
			require.Equal(t, "IPv4FragmentExtentUnavailable", event.ExpertCode)
			require.Zero(t, event.FlowID)
			require.Empty(t, event.Raw)
		}
	})
}
