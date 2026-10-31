package sharkcli

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func queryIdentityEvent(id uint64, ref pcaputil.PacketReference, typ uint64) *pcaputil.ProtocolEvent {
	return &pcaputil.ProtocolEvent{ID: id, Protocol: "bgp", Status: "decoded", Session: map[string]any{"Type": typ}, SourceBytes: pcaputil.ByteSource{PacketRefs: []pcaputil.PacketReference{ref}}}
}

func queryIdentityResult(t *testing.T, p *capturedPacket, expr string) pcaputil.DisplayFilterResult {
	t.Helper()
	filter, err := pcaputil.CompileDisplayFilter(expr)
	require.NoError(t, err)
	events, _ := p.protocolEventSnapshot(nil)
	return filter.EvaluateSnapshots(p.protocolQuerySnapshots(events))
}

func TestFieldQueryIdentitySurvivesDetailEvictionWithoutInventingFields(t *testing.T) {
	for _, physical := range []bool{false, true} {
		history, err := pcaputil.NewProtocolInspector(1, 1<<20)
		require.NoError(t, err)
		ref := pcaputil.PacketReference{Number: 294}
		history.OnEvent(queryIdentityEvent(299, ref, 4))
		// The second same-frame observation is retained, the first is not.
		history.OnEvent(&pcaputil.ProtocolEvent{ID: 300, Protocol: "quic", Status: "decoded", SourceBytes: pcaputil.ByteSource{PacketRefs: []pcaputil.PacketReference{ref}}})
		p := &capturedPacket{reference: ref, physicalRecord: physical, history: history}
		for _, tc := range []struct {
			expr              string
			matched, complete bool
		}{
			{"pdu.id == 299", true, true},
			{"pdu.id == 301", false, false},
			{"not (pdu.id == 299)", false, true},
			{"pdu.id != 299", false, true},
			{"pdu.id != 301", false, false},
			{"bgp.type == 4", false, false},
			{"not bgp.type", false, false},
			{"not tls.version", false, false},
			{"pdu.id == 299 and not tls.version", false, false},
			{"pdu.id == 299 or tls.version == 0x304", true, true},
		} {
			result := queryIdentityResult(t, p, tc.expr)
			require.Equal(t, tc.matched, result.Matched, tc.expr)
			require.Equal(t, tc.complete, result.Complete, tc.expr)
			require.True(t, result.Partial, tc.expr)
		}
		require.Empty(t, p.eventIDs, "querying cannot grow packet-owned ID or field arrays")
		require.Empty(t, p.observedFields)
	}
}

func TestFieldQueryIdentityDoesNotMarkCompleteDetailsPartial(t *testing.T) {
	history, err := pcaputil.NewProtocolInspector(4, 1<<20)
	require.NoError(t, err)
	ref := pcaputil.PacketReference{Number: 1}
	history.OnEvent(queryIdentityEvent(1, ref, 4))
	history.OnEvent(queryIdentityEvent(2, ref, 2))
	p := &capturedPacket{reference: ref, history: history}
	for _, expr := range []string{"pdu.id == 1", "bgp.type == 4", "not tls.version", "pdu.id != 3"} {
		result := queryIdentityResult(t, p, expr)
		require.True(t, result.Matched, expr)
		require.True(t, result.Complete, expr)
		require.False(t, result.Partial, expr)
	}
}

func TestFieldQueryIdentityRejectsForeignDuplicateFieldsButRetainsKnownID(t *testing.T) {
	for _, same := range []bool{false, true} {
		history, err := pcaputil.NewProtocolInspector(2, 1<<20)
		require.NoError(t, err)
		a, b := pcaputil.PacketReference{Number: 1}, pcaputil.PacketReference{Number: 2}
		if same {
			b = a
		}
		history.OnEvent(queryIdentityEvent(7, a, 4))
		history.OnEvent(queryIdentityEvent(7, b, 2))
		p := &capturedPacket{reference: b, history: history}
		result := queryIdentityResult(t, p, "pdu.id == 7")
		require.True(t, result.Matched)
		require.True(t, result.Complete)
		require.True(t, result.Partial)
		result = queryIdentityResult(t, p, "bgp.type == 4")
		require.Equal(t, same, result.Matched, "foreign fields cannot leak through a duplicate public ID")
		require.Equal(t, same, result.Complete)
		result = queryIdentityResult(t, p, "bgp.type == 2")
		require.False(t, result.Matched)
		require.False(t, result.Complete, "the second actual arrival has no selected Details binding yet")
	}
}

func TestFieldQueryIdentityBeyondDetailWindowAndExactUint64(t *testing.T) {
	history, err := pcaputil.NewProtocolInspector(140, 1<<20)
	require.NoError(t, err)
	ref := pcaputil.PacketReference{Number: 1}
	for id := uint64(1); id <= 128; id++ {
		history.OnEvent(queryIdentityEvent(id, ref, 4))
	}
	history.OnEvent(queryIdentityEvent(math.MaxUint64, ref, 2))
	p := &capturedPacket{reference: ref, history: history}
	result := queryIdentityResult(t, p, "pdu.id == 18446744073709551615")
	require.True(t, result.Matched)
	require.True(t, result.Complete)
	require.True(t, result.Partial)
	result = queryIdentityResult(t, p, "bgp.type == 2")
	require.False(t, result.Complete, "an ID witness must not recover that PDU's clipped fields")
}

func TestFieldQueryIdentityIgnoresUnprovenCallerIDsAndOversizedSnapshots(t *testing.T) {
	history, err := pcaputil.NewProtocolInspector(1, 64)
	require.NoError(t, err)
	ref := pcaputil.PacketReference{Number: 9}
	big := queryIdentityEvent(8, ref, 4)
	big.Raw = make([]byte, 4096)
	history.OnEvent(big)
	p := &capturedPacket{reference: ref, history: history, eventIDs: []uint64{999999}}
	result := queryIdentityResult(t, p, "pdu.id == 8")
	require.True(t, result.Matched)
	require.True(t, result.Complete)
	require.True(t, result.Partial)
	result = queryIdentityResult(t, p, "pdu.id == 999999")
	require.False(t, result.Matched)
	require.False(t, result.Complete, "CLI placeholders are not source-indexed ID facts")
	result = queryIdentityResult(t, p, "bgp.type == 4")
	require.False(t, result.Complete)
}

func TestFieldQueryIdentityRefusesOldUnboundDuplicateFields(t *testing.T) {
	history, err := pcaputil.NewProtocolInspector(2, 1<<20)
	require.NoError(t, err)
	ref := pcaputil.PacketReference{Number: 2}
	old := queryIdentityEvent(7, ref, 4)
	old.SourceBytes.PacketRefs = nil
	history.OnEvent(old)
	history.OnEvent(queryIdentityEvent(7, ref, 2))
	p := &capturedPacket{reference: ref, history: history, eventIDs: []uint64{7}}
	result := queryIdentityResult(t, p, "pdu.id == 7")
	require.True(t, result.Matched)
	require.True(t, result.Complete)
	require.True(t, result.Partial)
	for _, expr := range []string{"bgp.type == 4", "bgp.type == 2", "not bgp.type"} {
		result = queryIdentityResult(t, p, expr)
		require.False(t, result.Matched, expr)
		require.False(t, result.Complete, expr)
	}
}

func TestFieldQueryIdentityCacheTracksLateContributorsAfterEviction(t *testing.T) {
	history, err := pcaputil.NewProtocolInspector(1, 1<<20)
	require.NoError(t, err)
	ref := pcaputil.PacketReference{Number: 1}
	entry := &packetEntry{packet: &capturedPacket{reference: ref, physicalRecord: true, history: history}}
	u := &tui{ring: newPacketRing(2)}
	require.NoError(t, u.applySearch("fields pdu.id == 99"))
	ctx := newSearchContext(nil)
	require.False(t, u.matchesSearch(entry, ctx))
	history.OnEvent(queryIdentityEvent(99, ref, 4))
	require.True(t, u.matchesSearch(entry, ctx))
	history.OnEvent(queryIdentityEvent(100, pcaputil.PacketReference{Number: 2}, 2))
	require.True(t, u.matchesSearch(entry, ctx), "a previously observed identity remains a definite positive")
	require.True(t, entry.fieldResult.Partial)
	require.Contains(t, u.notice, fieldFilterPartialNoticePrefix)
	allocations := testing.AllocsPerRun(100, func() {
		if !u.matchesSearch(entry, ctx) {
			panic("stable cached identity disappeared")
		}
	})
	require.Zero(t, allocations)
	// A fresh row has no known-ID cache; it must get the same true witness.
	fresh := &packetEntry{packet: entry.packet}
	require.True(t, u.matchesSearch(fresh, ctx))
	require.True(t, fresh.fieldResult.Partial)
}
