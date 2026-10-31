package pcaputil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPacketQueryRevisionTracksArrivalsNotIDsAndKeepsDomains(t *testing.T) {
	v, err := NewProtocolInspector(8, 1<<20)
	require.NoError(t, err)
	a := PacketReference{Number: 7, Domain: CaptureDomain{Interface: 1, Encapsulation: "/gre:a"}}
	b := a
	b.Domain.Encapsulation = "/gre:b"
	before := v.PacketQueryRevision(a, nil, false)
	v.OnEvent(&ProtocolEvent{ID: 99, SourceBytes: ByteSource{PacketRefs: []PacketReference{a}}})
	first := v.PacketQueryRevision(a, nil, false)
	require.NotEqual(t, before, first)
	require.Equal(t, before, v.PacketQueryRevision(b, nil, false))
	v.OnEvent(&ProtocolEvent{ID: 100, SourceBytes: ByteSource{PacketRefs: []PacketReference{b}}})
	require.Equal(t, first, v.PacketQueryRevision(a, nil, false), "an unrelated exact-domain arrival must not invalidate this packet")
	require.NotEqual(t, first, v.PacketQueryRevision(a, nil, true), "physical records can acquire another proven tunnel contributor")
	v.OnEvent(&ProtocolEvent{ID: 2, SourceBytes: ByteSource{PacketRefs: []PacketReference{a}}})
	require.Greater(t, v.PacketQueryRevision(a, nil, false).Contributions, first.Contributions, "concurrent IDs may arrive out of order")
}

func TestPacketQueryRevisionCoversInitialIDsAndDuplicateReplacement(t *testing.T) {
	v, err := NewProtocolInspector(2, 1<<20)
	require.NoError(t, err)
	ref := PacketReference{Number: 1}
	before := v.PacketQueryRevision(ref, []uint64{7}, false)
	v.OnEvent(&ProtocolEvent{ID: 7})
	first := v.PacketQueryRevision(ref, []uint64{7}, false)
	require.NotEqual(t, before, first, "an initial association may be retained without a reference index entry")
	v.OnEvent(&ProtocolEvent{ID: 7})
	second := v.PacketQueryRevision(ref, []uint64{7}, false)
	require.Equal(t, first, second, "Details keeps the oldest retained duplicate ID")
	v.OnEvent(&ProtocolEvent{ID: 8})
	replaced := v.PacketQueryRevision(ref, []uint64{7}, false)
	require.Greater(t, replaced.Evictions, second.Evictions)
	require.Greater(t, replaced.InitialIDs, second.InitialIDs, "replacing the retained binding must invalidate its field result")
}

func TestPacketQueryRevisionFallsBackForUnindexedReferenceGraphs(t *testing.T) {
	v, err := NewProtocolInspector(2, 1<<20)
	require.NoError(t, err)
	v.packetIndexLimit = 2
	a, b := PacketReference{Number: 1}, PacketReference{Number: 2}
	v.OnEvent(&ProtocolEvent{ID: 1, SourceBytes: ByteSource{PacketRefs: []PacketReference{a, b}}})
	first := v.PacketQueryRevision(a, nil, false)
	require.NotZero(t, first.Fallback)
	v.OnEvent(&ProtocolEvent{ID: 2})
	require.NotEqual(t, first, v.PacketQueryRevision(a, nil, false), "unindexed entries retain the exact global fallback")
	v.OnEvent(&ProtocolEvent{ID: 3})
	require.Zero(t, v.PacketQueryRevision(a, nil, false).Fallback)
	require.Empty(t, v.PacketEventIDs(a, 128))
	large := make([]uint64, 129)
	require.NotZero(t, v.PacketQueryRevision(a, large, false).Fallback, "oversized caller associations cannot bypass invalidation")
}

func TestPacketEventWindowReportsOverflowWithIndexedAndFallbackQueries(t *testing.T) {
	for _, indexed := range []bool{false, true} {
		v, err := NewProtocolInspector(132, 1<<20)
		require.NoError(t, err)
		if !indexed {
			v.packetIndexLimit = 0
		}
		ref := PacketReference{Number: 1}
		for id := uint64(1); id <= 128; id++ {
			v.OnEvent(&ProtocolEvent{ID: id, SourceBytes: ByteSource{PacketRefs: []PacketReference{ref}}})
		}
		for _, physical := range []bool{false, true} {
			ids, complete := v.PacketEventWindow(ref, physical)
			require.Len(t, ids, 128)
			require.True(t, complete, "exactly fitting the window does not omit fields")
		}
		v.OnEvent(&ProtocolEvent{ID: 129, SourceBytes: ByteSource{PacketRefs: []PacketReference{ref}}})
		for _, physical := range []bool{false, true} {
			ids, complete := v.PacketEventWindow(ref, physical)
			require.Len(t, ids, 128)
			require.False(t, complete, "a later matching row must not be silently discarded")
		}
	}
}
