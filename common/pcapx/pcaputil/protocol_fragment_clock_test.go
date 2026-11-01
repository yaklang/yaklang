package pcaputil

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func fragmentClockPacket(original gopacket.Packet, domain CaptureDomain, number uint64, stamp time.Time, malformed bool) gopacket.Packet {
	raw := bytes.Clone(original.Data())
	if malformed {
		raw = raw[:len(raw)-1]
	}
	p := gopacket.NewPacket(raw, protocolPacketDecoder{decoder: original.Layers()[0].LayerType()}, gopacket.Default)
	ci := original.Metadata().CaptureInfo
	ci.Timestamp, ci.Length, ci.CaptureLength = stamp, len(raw), len(raw)
	p.Metadata().CaptureInfo = withEvidence(ci, captureEvidence{Ref: PacketReference{Number: number, Domain: domain}})
	return p
}

func TestFragmentLifetimeIsolatedByCapturedDomain(t *testing.T) {
	for _, name := range []string{"ipv4-options", "ipv6-in-order"} {
		for _, scope := range []string{"section", "interface", "encapsulation"} {
			for _, reverse := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/reverse=%v", name, scope, reverse), func(t *testing.T) {
					var events []*ProtocolEvent
					a := coreFragmentParser(&events, 1<<20, 16)
					t.Cleanup(func() { a.closeFragments(); require.Zero(t, a.buffered.Load()) })
					packets := coreIPPackets(t, name)
					require.Len(t, packets, 2)
					domain := CaptureDomain{Section: 3, Interface: 4, Encapsulation: "/vlan:17"}
					foreign := domain
					switch scope {
					case "section":
						foreign.Section++
					case "interface":
						foreign.Interface++
					default:
						foreign.Encapsulation = "/vlan:18"
					}
					first, last := packets[0], packets[1]
					if reverse {
						first, last = last, first
					}
					_, consumed := a.networkPacket(fragmentClockPacket(first, domain, 1, time.Unix(100, 0), false))
					require.True(t, consumed)
					_, consumed = a.networkPacket(fragmentClockPacket(first, foreign, 2, time.Unix(10000, 0), false))
					require.True(t, consumed)
					require.Empty(t, events, "another interface/section/tunnel clock cannot expire captured fragments")
					q, consumed := a.networkPacket(fragmentClockPacket(last, domain, 3, time.Unix(101, 0), false))
					require.False(t, consumed)
					require.NotNil(t, q)
					var expected []byte
					for _, p := range packets {
						if f := p.Layer(layers.LayerTypeIPv6Fragment); f != nil {
							expected = append(expected, f.LayerPayload()...)
						} else {
							expected = append(expected, p.NetworkLayer().LayerPayload()...)
						}
					}
					require.Equal(t, expected, q.Data()[len(q.Data())-len(expected):])
					refs := packetEvidence(q).refs
					require.ElementsMatch(t, []PacketReference{{Number: 1, Domain: domain}, {Number: 3, Domain: domain}}, refs)
					require.Len(t, a.fragments.sets, 1, "the independent foreign fragment remains pending")
				})
			}
		}
	}
}

func TestUnknownFragmentTimeCannotBeExpiredByAnotherTimestamp(t *testing.T) {
	var events []*ProtocolEvent
	a := coreFragmentParser(&events, 1<<20, 16)
	defer a.closeFragments()
	packets := coreIPPackets(t, "ipv6-in-order")
	_, consumed := a.networkPacket(fragmentClockPacket(packets[0], CaptureDomain{}, 1, time.Time{}, false))
	require.True(t, consumed)
	foreign := fragmentClockPacket(packets[0], CaptureDomain{}, 2, time.Unix(10000, 0), false)
	raw := bytes.Clone(foreign.Data())
	f := foreign.Layer(layers.LayerTypeIPv6Fragment)
	at := len(raw) - len(f.LayerPayload()) - 8
	raw[at+7] ^= 1 // A different datagram ID in this same capture domain.
	changed := gopacket.NewPacket(raw, protocolPacketDecoder{decoder: foreign.Layers()[0].LayerType()}, gopacket.Default)
	changed.Metadata().CaptureInfo = foreign.Metadata().CaptureInfo
	_, consumed = a.networkPacket(changed)
	require.True(t, consumed)
	require.Empty(t, events, "an absent capture time does not prove a 30-second lease expired")
	q, consumed := a.networkPacket(fragmentClockPacket(packets[1], CaptureDomain{}, 3, time.Time{}, false))
	require.False(t, consumed)
	require.NotNil(t, q)
}

func TestFragmentQuarantineCannotBeReleasedByForeignOrUnknownTime(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(fmt.Sprintf("unknown=%v", unknown), func(t *testing.T) {
			var events []*ProtocolEvent
			a := coreFragmentParser(&events, 1, 2)
			defer a.closeFragments()
			first := coreIPPackets(t, "ipv6-in-order")[0]
			stamp := time.Unix(100, 0)
			if unknown {
				stamp = time.Time{}
			}
			_, consumed := a.networkPacket(fragmentClockPacket(first, CaptureDomain{Interface: 1}, 1, stamp, true))
			require.True(t, consumed)
			require.Equal(t, "FragmentLength", events[len(events)-1].ExpertCode)
			for _, step := range []struct {
				domain CaptureDomain
				stamp  time.Time
			}{{CaptureDomain{Interface: 2}, time.Unix(10000, 0)}, {CaptureDomain{Interface: 1}, time.Unix(101, 0)}} {
				_, consumed = a.networkPacket(fragmentClockPacket(first, step.domain, 2, step.stamp, false))
				require.True(t, consumed)
				require.Equal(t, "FragmentQuarantineBudget", events[len(events)-1].ExpertCode)
			}
			_, consumed = a.networkPacket(fragmentClockPacket(first, CaptureDomain{Interface: 1}, 3, time.Unix(131, 0), false))
			require.True(t, consumed)
			if unknown {
				require.Equal(t, "FragmentQuarantineBudget", events[len(events)-1].ExpertCode)
			} else {
				require.Equal(t, "FragmentByteBudget", events[len(events)-1].ExpertCode)
			}
			require.Zero(t, a.buffered.Load())
		})
	}
}

func FuzzFragmentDomainClockCannotEraseAnotherAssembly(f *testing.F) {
	for _, seed := range [][]byte{{0, 1, 2}, {255, 128, 1}, {3, 3, 0, 0}, {4, 8, 16}} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, operations []byte) {
		if len(operations) > 16 {
			return
		}
		var events []*ProtocolEvent
		a := coreFragmentParser(&events, 1<<20, 32)
		t.Cleanup(func() { a.closeFragments(); require.Zero(t, a.buffered.Load()) })
		packets := coreIPPackets(t, "ipv6-in-order")
		domain := CaptureDomain{Section: 1, Interface: 2, Encapsulation: "/vlan:17"}
		first, last := packets[0], packets[1]
		if len(operations) > 0 && operations[0]&1 != 0 {
			first, last = last, first
		}
		_, consumed := a.networkPacket(fragmentClockPacket(first, domain, 1, time.Unix(100, 0), false))
		require.True(t, consumed)
		for i, operation := range operations {
			foreign := domain
			foreign.Interface = 3 + int(operation&7)
			stamp := time.Unix(10000+int64(operation)*3600, 0)
			if operation&8 != 0 {
				stamp = time.Time{}
			}
			a.networkPacket(fragmentClockPacket(packets[int(operation&1)], foreign, uint64(i+2), stamp, false))
			require.LessOrEqual(t, len(a.fragments.sets), 32)
			require.LessOrEqual(t, a.buffered.Load(), int64(1<<20))
		}
		q, consumed := a.networkPacket(fragmentClockPacket(last, domain, uint64(len(operations)+2), time.Unix(101, 0), false))
		require.False(t, consumed)
		require.NotNil(t, q)
		var expected []byte
		for _, p := range packets {
			expected = append(expected, p.Layer(layers.LayerTypeIPv6Fragment).LayerPayload()...)
		}
		require.Equal(t, expected, q.Data()[len(q.Data())-len(expected):])
		require.ElementsMatch(t, []PacketReference{{Number: 1, Domain: domain}, {Number: uint64(len(operations) + 2), Domain: domain}}, packetEvidence(q).refs)
	})
}
