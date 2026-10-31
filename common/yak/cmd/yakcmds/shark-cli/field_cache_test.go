package sharkcli

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestFieldFilterCachedRedrawDoesNotCopyEventTrees(t *testing.T) {
	history, err := pcaputil.NewProtocolInspector(4, 1<<20)
	require.NoError(t, err)
	ref := pcaputil.PacketReference{Number: 1}
	history.OnEvent(&pcaputil.ProtocolEvent{ID: 99, Protocol: "bgp", Status: "decoded", Session: map[string]any{"Type": uint64(4)}, SourceBytes: pcaputil.ByteSource{PacketRefs: []pcaputil.PacketReference{ref}}})
	u := &tui{ring: newPacketRing(8)}
	require.NoError(t, u.applySearch("fields bgp.type == 4"))
	entry := &packetEntry{packet: &capturedPacket{reference: ref, physicalRecord: true, history: history, eventIDs: []uint64{99}}}
	ctx := newSearchContext(nil)
	require.True(t, u.matchesSearch(entry, ctx))
	allocations := testing.AllocsPerRun(100, func() {
		if !u.matchesSearch(entry, ctx) {
			panic("cached matching packet disappeared")
		}
	})
	require.Zero(t, allocations, "a stable redraw must not clone/decode message trees")
	history.OnEvent(&pcaputil.ProtocolEvent{ID: 100, SourceBytes: pcaputil.ByteSource{PacketRefs: []pcaputil.PacketReference{{Number: 2}}}})
	allocations = testing.AllocsPerRun(100, func() {
		if !u.matchesSearch(entry, ctx) {
			panic("unrelated arrival changed packet fields")
		}
	})
	require.Zero(t, allocations, "an unrelated packet must not force field projection again")
}

func TestFieldFilterCacheInvalidatesDuplicateIDsInitialBindingsAndQuery(t *testing.T) {
	history, err := pcaputil.NewProtocolInspector(2, 1<<20)
	require.NoError(t, err)
	ref := pcaputil.PacketReference{Number: 1}
	u := &tui{ring: newPacketRing(8)}
	require.NoError(t, u.applySearch("fields bgp.type == 4"))
	entry := &packetEntry{packet: &capturedPacket{reference: ref, history: history, eventIDs: []uint64{7}}}
	ctx := newSearchContext(nil)
	require.False(t, u.matchesSearch(entry, ctx))
	keepalive := &pcaputil.ProtocolEvent{ID: 7, Protocol: "bgp", Status: "decoded", Session: map[string]any{"Type": uint64(4)}}
	history.OnEvent(keepalive)
	require.True(t, u.matchesSearch(entry, ctx), "an initial ID can become available without a packet index link")
	update := *keepalive
	update.Session = map[string]any{"Type": uint64(2)}
	history.OnEvent(&update)
	require.True(t, u.matchesSearch(entry, ctx), "the oldest duplicate binding remains authoritative")
	history.OnEvent(&pcaputil.ProtocolEvent{ID: 8})
	require.False(t, u.matchesSearch(entry, ctx), "evicting the first duplicate changes the retained field values")
	require.NoError(t, u.applySearch("fields bgp.type == 2"))
	require.True(t, u.matchesSearch(entry, ctx), "changing the predicate must invalidate the result")
	entry.packet.eventIDs = []uint64{8}
	require.False(t, u.matchesSearch(entry, ctx), "changing capture-time associations cannot reuse the previous result")
}

func TestFieldFilterWithoutHistoryStillObservesCallerChanges(t *testing.T) {
	event := &pcaputil.ProtocolEvent{Protocol: "bgp", Session: map[string]any{"Type": uint64(4)}}
	entry := &packetEntry{packet: &capturedPacket{events: []*pcaputil.ProtocolEvent{event}}}
	u := &tui{ring: newPacketRing(1)}
	require.NoError(t, u.applySearch("fields bgp.type == 4"))
	require.True(t, u.matchesSearch(entry, newSearchContext(nil)))
	event.Session["Type"] = uint64(2)
	require.False(t, u.matchesSearch(entry, newSearchContext(nil)), "caller-owned events have no immutable history revision")
}

func TestFieldCacheEarlyContributorEvictionDoesNotBecomeFieldAbsence(t *testing.T) {
	history, err := pcaputil.NewProtocolInspector(1, 1<<20)
	require.NoError(t, err)
	ref := pcaputil.PacketReference{Number: 1}
	history.OnEvent(&pcaputil.ProtocolEvent{ID: 1, Protocol: "bgp", Status: "decoded", Session: map[string]any{"Type": uint64(4)}, SourceBytes: pcaputil.ByteSource{PacketRefs: []pcaputil.PacketReference{ref}}})
	entry := &packetEntry{packet: &capturedPacket{reference: ref, physicalRecord: true, history: history}}
	u := &tui{ring: newPacketRing(2)}
	require.NoError(t, u.applySearch("fields not bgp.type"))
	require.False(t, u.matchesSearch(entry, newSearchContext(nil)))
	require.True(t, entry.fieldResult.Complete)
	history.OnEvent(&pcaputil.ProtocolEvent{ID: 2, SourceBytes: pcaputil.ByteSource{PacketRefs: []pcaputil.PacketReference{{Number: 2}}}})
	require.False(t, u.matchesSearch(entry, newSearchContext(nil)), "a negative predicate must not match lost contributor fields")
	require.False(t, entry.fieldResult.Complete)
	require.Contains(t, u.notice, "unavailable")
	// A packet first inspected after eviction has no tombstone in the bounded
	// reference index. It must expose that uncertainty rather than invent absence.
	fresh := &packetEntry{packet: entry.packet}
	require.False(t, u.matchesSearch(fresh, newSearchContext(nil)))
	require.False(t, fresh.fieldResult.Complete)
}

func TestFieldSnapshotRejectsForeignDuplicateIDAndTruncatedPDUUnion(t *testing.T) {
	history, err := pcaputil.NewProtocolInspector(132, 1<<20)
	require.NoError(t, err)
	a, b := pcaputil.PacketReference{Number: 1}, pcaputil.PacketReference{Number: 2}
	e := &pcaputil.ProtocolEvent{ID: 7, Protocol: "bgp", Status: "decoded", Session: map[string]any{"Type": uint64(4)}, SourceBytes: pcaputil.ByteSource{PacketRefs: []pcaputil.PacketReference{a}}}
	history.OnEvent(e)
	other := *e
	other.Session = map[string]any{"Type": uint64(2)}
	other.SourceBytes.PacketRefs = []pcaputil.PacketReference{b}
	history.OnEvent(&other)
	p := &capturedPacket{history: history, reference: b, eventIDs: []uint64{7}}
	filter, err := pcaputil.CompileDisplayFilter("bgp.type == 4")
	require.NoError(t, err)
	result := filter.EvaluateEvents(p.protocolEvents())
	require.False(t, result.Matched)
	require.False(t, result.Complete, "the first retained duplicate belongs to another frame")
	for id := uint64(8); id < 137; id++ {
		history.OnEvent(&pcaputil.ProtocolEvent{ID: id, Protocol: "bgp", Status: "decoded", Session: map[string]any{"Type": uint64(4)}, SourceBytes: pcaputil.ByteSource{PacketRefs: []pcaputil.PacketReference{a}}})
	}
	p = &capturedPacket{history: history, reference: a}
	result = filter.EvaluateEvents(p.protocolEvents())
	require.True(t, result.Complete, "a retained type4 witness decides this positive predicate")
	require.True(t, result.Matched)
	require.True(t, result.Partial, "the field union is still explicitly incomplete")
	negative, err := pcaputil.CompileDisplayFilter("not tls.version")
	require.NoError(t, err)
	require.False(t, negative.EvaluateEvents(p.protocolEvents()).Complete, "unretained PDUs cannot prove absence")
}

func TestFieldFilterPartialPositiveShowsRowAndRetainsNotice(t *testing.T) {
	u := &tui{ring: newPacketRing(2)}
	event := &pcaputil.ProtocolEvent{Protocol: "bgp", Completeness: "limited", Session: map[string]any{"Type": uint64(4)}}
	entry := &packetEntry{packet: &capturedPacket{events: []*pcaputil.ProtocolEvent{event}}}
	require.NoError(t, u.applySearch("fields bgp.type == 4"))
	require.True(t, u.matchesSearch(entry, newSearchContext(nil)))
	require.Contains(t, u.notice, fieldFilterPartialNoticePrefix)
	require.Contains(t, u.notice, "field coverage limited")
	require.NotContains(t, u.notice, "packets unavailable")
	require.NotContains(t, u.notice, "retained history")
	require.NoError(t, u.applySearch("fields not tls.version"))
	require.False(t, u.matchesSearch(entry, newSearchContext(nil)))
}
