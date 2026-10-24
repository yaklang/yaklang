package pcaputil

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestInspectorIndexesKeepArrivalOwnershipAndDomains(t *testing.T) {
	v, err := NewProtocolInspector(2, 1<<20)
	require.NoError(t, err)
	a := PacketReference{Number: 7, Domain: CaptureDomain{Interface: 1}}
	b := PacketReference{Number: 7, Domain: CaptureDomain{Interface: 2}}
	first := &ProtocolEvent{ID: 9, Raw: []byte("a"), SourceBytes: ByteSource{PacketRefs: []PacketReference{a, a}}}
	v.OnEvent(first)
	v.OnEvent(&ProtocolEvent{ID: 9, Raw: []byte("b"), SourceBytes: ByteSource{PacketRefs: []PacketReference{b}}})
	first.Raw[0] = 'x'
	first.SourceBytes.PacketRefs[0].Number = 99
	d, err := v.Details(9)
	require.NoError(t, err)
	require.Equal(t, []byte("a"), d.Raw)
	require.Equal(t, []uint64{9}, v.PacketEventIDs(a, 128), "duplicate contributors are one PDU")
	require.Equal(t, []uint64{9}, v.PacketEventIDs(b, 128))
	ids := v.PacketEventIDs(a, 128)
	ids[0] = 0
	require.Equal(t, []uint64{9}, v.PacketEventIDs(a, 128))
	v.OnEvent(&ProtocolEvent{ID: 10, Raw: []byte("c"), SourceBytes: ByteSource{PacketRefs: []PacketReference{a}}})
	d, err = v.Details(9)
	require.NoError(t, err)
	require.Equal(t, []byte("b"), d.Raw, "duplicate IDs keep the oldest retained arrival")
	require.Equal(t, []uint64{10}, v.PacketEventIDs(a, 128))
	require.Equal(t, 4, v.packetIndexLinks)
	require.Empty(t, v.PacketEventIDs(PacketReference{Number: 7, Domain: CaptureDomain{Interface: 3}}, 128))
}

func TestInspectorReferenceIndexBudgetFallsBackWithoutLosingPDUs(t *testing.T) {
	v, err := NewProtocolInspector(2, 1<<20)
	require.NoError(t, err)
	v.packetIndexLimit = 2
	a, b := PacketReference{Number: 1}, PacketReference{Number: 2}
	v.OnEvent(&ProtocolEvent{ID: 1, Raw: []byte("a"), SourceBytes: ByteSource{PacketRefs: []PacketReference{a, b}}})
	v.OnEvent(&ProtocolEvent{ID: 2, Raw: []byte("b"), SourceBytes: ByteSource{PacketRefs: []PacketReference{a}}})
	require.Equal(t, 1, v.unindexed)
	require.Equal(t, []uint64{1, 2}, v.PacketEventIDs(a, 128))
	require.Equal(t, []uint64{1}, v.PacketEventIDs(b, 128))
	require.LessOrEqual(t, v.packetIndexLinks, v.packetIndexLimit)
	v.OnEvent(&ProtocolEvent{ID: 3, Raw: []byte("c")})
	require.Zero(t, v.unindexed)
	require.Empty(t, v.PacketEventIDs(b, 128))
	require.Equal(t, []uint64{2}, v.PacketEventIDs(a, 128))
	_, err = v.Details(1)
	require.ErrorContains(t, err, "absent or evicted")
}

func TestInspectorPhysicalRecordLookupDoesNotWeakenExactDomainLookup(t *testing.T) {
	v, err := NewProtocolInspector(2, 1<<20)
	require.NoError(t, err)
	a := PacketReference{Number: 7, Domain: CaptureDomain{Interface: 1, Encapsulation: "/gre:a"}}
	b := a
	b.Domain.Encapsulation = "/gre:b"
	v.OnEvent(&ProtocolEvent{ID: 1, SourceBytes: ByteSource{PacketRefs: []PacketReference{a}}})
	v.OnEvent(&ProtocolEvent{ID: 2, SourceBytes: ByteSource{PacketRefs: []PacketReference{b}}})
	require.Equal(t, []uint64{1}, v.PacketEventIDs(a, 128))
	require.Equal(t, []uint64{2}, v.PacketEventIDs(b, 128))
	physical := a
	physical.Domain.Encapsulation = ""
	require.Empty(t, v.PacketEventIDs(physical, 128))
	require.Equal(t, []uint64{1, 2}, v.PhysicalPacketEventIDs(physical, 128))
	physical.Domain.Interface = 2
	require.Empty(t, v.PhysicalPacketEventIDs(physical, 128))
}

func TestInspectorLargeOrdinaryWindowKeepsPacketQueriesIndexed(t *testing.T) {
	v, err := NewProtocolInspector(80000, 64<<20)
	require.NoError(t, err)
	for i := uint64(1); i <= 40000; i++ {
		v.OnEvent(&ProtocolEvent{ID: i, SourceBytes: ByteSource{PacketRefs: []PacketReference{{Number: i}}}})
	}
	require.Zero(t, v.Evicted())
	require.Zero(t, v.unindexed, "ordinary retained PDUs should not force a full-window scan for every packet")
	for _, id := range []uint64{1, 20000, 40000} {
		ref := PacketReference{Number: id}
		require.Equal(t, []uint64{id}, v.PacketEventIDs(ref, 128))
		require.Equal(t, []uint64{id}, v.PhysicalPacketEventIDs(ref, 128))
	}
	require.LessOrEqual(t, v.packetIndexLinks, v.packetIndexLimit)
}
