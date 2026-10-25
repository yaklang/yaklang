package pcaputil

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func TestFragmentBatchDiagnosticsKeepStableIDsAndHistoryTail(t *testing.T) {
	// Identical inputs and physical references, submitted in several orders.
	// EOF/timeout diagnostics share a small consumer history window: changing
	// their order must not change which physical frames still have details.
	for _, timeout := range []bool{false, true} {
		var expected []PacketReference
		for permutation := 0; permutation < 12; permutation++ {
			var events []*ProtocolEvent
			a := coreFragmentParser(&events, 1<<20, 128)
			original := coreIPPackets(t, "ipv6-in-order")[0]
			makePacket := func(id int, stamp time.Time) gopacket.Packet {
				raw := bytes.Clone(original.Data())
				f := original.Layer(layers.LayerTypeIPv6Fragment)
				at := len(raw) - len(f.LayerPayload()) - 8
				binary.BigEndian.PutUint32(raw[at+4:at+8], uint32(id))
				ci := original.Metadata().CaptureInfo
				ci.Timestamp = stamp
				ci = withEvidence(ci, captureEvidence{Ref: PacketReference{Number: uint64(id + 100), Domain: CaptureDomain{Interface: id % 4}}})
				p := gopacket.NewPacket(raw, protocolPacketDecoder{decoder: layers.LinkTypeRaw}, gopacket.Default)
				p.Metadata().CaptureInfo = ci
				return p
			}
			stamp := original.Metadata().CaptureInfo.Timestamp
			for i := 0; i < 64; i++ {
				id := (i*17 + permutation*7) % 64
				_, consumed := a.networkPacket(makePacket(id, stamp))
				require.True(t, consumed)
			}
			require.Empty(t, events)
			if timeout {
				_, consumed := a.networkPacket(makePacket(128, stamp.Add(31*time.Second)))
				require.True(t, consumed)
			} else {
				a.closeFragments()
			}
			require.Len(t, events, 64)
			var observed []PacketReference
			for i, e := range events {
				require.EqualValues(t, i+1, e.ID)
				require.Len(t, e.SourceBytes.PacketRefs, 1)
				observed = append(observed, e.SourceBytes.PacketRefs[0])
			}
			if permutation == 0 {
				expected = observed
			} else {
				require.Equal(t, expected, observed)
				require.Equal(t, expected[len(expected)-8:], observed[len(observed)-8:], "bounded history tail")
			}
			a.closeFragments()
			require.Zero(t, a.buffered.Load())
		}
	}
}

func TestIPv6ExactDuplicateFragmentKeepsBoundedAssembly(t *testing.T) {
	var events []*ProtocolEvent
	a := coreFragmentParser(&events, 1<<20, 32)
	defer a.closeFragments()
	packets := coreIPPackets(t, "ipv6-in-order")
	_, consumed := a.networkPacket(packets[0])
	require.True(t, consumed)
	cost := a.buffered.Load()
	for i := 0; i < 200; i++ {
		_, consumed = a.networkPacket(packets[0])
		require.True(t, consumed)
		require.Equal(t, cost, a.buffered.Load())
	}
	q, consumed := a.networkPacket(packets[1])
	require.False(t, consumed)
	require.NotNil(t, q)
	require.Empty(t, events)
	require.Zero(t, a.buffered.Load())
}

func FuzzFragmentDuplicateAndCaptureRepair(f *testing.F) {
	for _, ops := range [][]byte{{0, 0, 1}, {1, 0}, {2, 1, 0}, {0, 1, 0, 1}, {0, 5, 1, 4}, {0, 3, 1}} {
		f.Add(ops)
	}
	f.Fuzz(func(t *testing.T, ops []byte) {
		if len(ops) > 32 {
			return
		}
		var events []*ProtocolEvent
		a := coreFragmentParser(&events, 1<<20, 4)
		packets := coreIPPackets(t, "ipv6-in-order")
		var expected []byte
		for _, p := range packets {
			expected = append(expected, p.Layer(layers.LayerTypeIPv6Fragment).LayerPayload()...)
		}
		for i, op := range ops {
			original := packets[int(op&1)]
			raw := bytes.Clone(original.Data())
			ci := original.Metadata().CaptureInfo
			ci.Length = len(raw)
			if op&2 != 0 {
				raw = raw[:len(raw)-1]
			}
			ci.CaptureLength = len(raw)
			ci.Timestamp = ci.Timestamp.Add(time.Duration(i) * time.Microsecond)
			ci = withEvidence(ci, captureEvidence{Ref: PacketReference{Number: uint64(i + 1), Domain: CaptureDomain{Interface: int((op >> 2) & 1)}}})
			p := gopacket.NewPacket(raw, protocolPacketDecoder{decoder: layers.LinkTypeRaw}, gopacket.Default)
			p.Metadata().CaptureInfo = ci
			q, consumed := a.networkPacket(p)
			if !consumed {
				require.NotNil(t, q)
				ip := q.NetworkLayer().(*layers.IPv6)
				// The complete fragmentable suffix must be the original bytes.
				// Header/extension preservation has separate sealed-wire checks.
				require.EqualValues(t, len(q.Data())-40, ip.Length)
				require.GreaterOrEqual(t, len(q.Data()), len(expected)+40)
				require.Equal(t, expected, q.Data()[len(q.Data())-len(expected):])
			}
			require.LessOrEqual(t, len(a.fragments.sets), 4)
			require.LessOrEqual(t, a.buffered.Load(), int64(1<<20))
			require.GreaterOrEqual(t, a.buffered.Load(), int64(0))
		}
		a.closeFragments()
		require.Zero(t, a.buffered.Load())
	})
}

func TestFragmentCaptureCutCanBeRepairedButMalformedWireCannot(t *testing.T) {
	for _, captureCut := range []bool{true, false} {
		var events []*ProtocolEvent
		a := coreFragmentParser(&events, 1<<20, 32)
		packets := coreIPPackets(t, "ipv6-in-order")
		b := bytes.Clone(packets[0].Data())
		raw := b[:len(b)-2]
		p := gopacket.NewPacket(raw, protocolPacketDecoder{decoder: layers.LinkTypeRaw}, gopacket.Default)
		ci := packets[0].Metadata().CaptureInfo
		ci.CaptureLength = len(raw)
		ci.Length = len(b)
		if !captureCut {
			ci.Length = len(raw)
		}
		p.Metadata().CaptureInfo = ci
		_, consumed := a.networkPacket(p)
		require.True(t, consumed)
		_, consumed = a.networkPacket(packets[1])
		require.True(t, consumed)
		q, consumed := a.networkPacket(packets[0])
		if captureCut {
			require.False(t, consumed)
			require.NotNil(t, q)
			require.Len(t, events, 1)
			require.Equal(t, "FragmentCaptureTruncated", events[0].ExpertCode)
			require.Equal(t, "incomplete", events[0].Status)
			require.Zero(t, a.buffered.Load())
		} else {
			require.True(t, consumed)
			require.Nil(t, q)
			require.Len(t, events, 1)
			require.Equal(t, "FragmentLength", events[0].ExpertCode)
		}
		a.closeFragments()
		require.Zero(t, a.buffered.Load())
	}
}

func TestDuplicateFragmentCannotChangeEndOrFirstNextHeader(t *testing.T) {
	for _, changeEnd := range []bool{true, false} {
		var events []*ProtocolEvent
		a := coreFragmentParser(&events, 1<<20, 32)
		packets := coreIPPackets(t, "ipv6-in-order")
		selected := packets[0]
		if changeEnd {
			selected = packets[1]
		}
		_, consumed := a.networkPacket(selected)
		require.True(t, consumed)
		raw := bytes.Clone(selected.Data())
		fragment := selected.Layer(layers.LayerTypeIPv6Fragment).(*layers.IPv6Fragment)
		fragmentAt := len(raw) - len(fragment.LayerPayload()) - len(fragment.LayerContents())
		require.GreaterOrEqual(t, fragmentAt, 40)
		if changeEnd {
			raw[fragmentAt+3] ^= 1
		} else {
			raw[fragmentAt] = 6
		}
		p := gopacket.NewPacket(raw, protocolPacketDecoder{decoder: layers.LinkTypeRaw}, gopacket.Default)
		p.Metadata().CaptureInfo = selected.Metadata().CaptureInfo
		_, consumed = a.networkPacket(p)
		require.True(t, consumed)
		for _, original := range packets {
			q, done := a.networkPacket(original)
			require.True(t, done)
			require.Nil(t, q)
		}
		require.Len(t, events, 1)
		// Changing a final fragment to M=1 can also fail the earlier mandatory
		// eight-octet length check. In every case the changed envelope must be
		// rejected and must not be treated as an innocent byte duplicate.
		require.Contains(t, []string{"FragmentConflictingEnd", "FragmentOverlapRejected", "FragmentFirstHeader", "FragmentLength"}, events[0].ExpertCode)
		a.closeFragments()
		require.Zero(t, a.buffered.Load())
	}
}
