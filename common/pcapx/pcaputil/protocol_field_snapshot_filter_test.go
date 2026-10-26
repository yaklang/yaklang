package pcaputil

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDisplayFilterRestoredSnapshotRetainsExact64BitValues(t *testing.T) {
	e := &ProtocolEvent{Protocol: "wireguard", Status: "decoded", Completeness: "message", Session: map[string]any{"Counter": uint64(1)<<60 | 3}}
	wire, err := json.Marshal(e.DisplayFieldValues())
	require.NoError(t, err)
	var snapshot DisplayFieldSnapshot
	require.NoError(t, json.Unmarshal(wire, &snapshot))
	filter, err := CompileDisplayFilter("wg.counter == 1152921504606846979")
	require.NoError(t, err)
	result := filter.EvaluateSnapshots([]DisplayFieldSnapshot{snapshot})
	require.True(t, result.Complete)
	require.True(t, result.Matched)
	for i, v := range snapshot.Values {
		if v.Name == "wg.counter" {
			snapshot.Values[i].ExactValue = "invalid"
		}
	}
	require.False(t, filter.EvaluateSnapshots([]DisplayFieldSnapshot{snapshot}).Complete)
}

func TestDisplayFilterOwnedSnapshotPreservesValuesAndCompleteness(t *testing.T) {
	e := &ProtocolEvent{ID: 11, Protocol: "bgp", Status: "decoded", Completeness: "message", Session: map[string]any{"Type": uint64(4)}}
	snapshot := e.DisplayFieldValues()
	filter, err := CompileDisplayFilter("bgp.type == 4")
	require.NoError(t, err)
	require.Equal(t, filter.EvaluateEvents([]*ProtocolEvent{e}), filter.EvaluateSnapshots([]DisplayFieldSnapshot{snapshot}))
	e.Session["Type"] = uint64(1)
	require.False(t, filter.EvaluateEvents([]*ProtocolEvent{e}).Matched)
	require.True(t, filter.EvaluateSnapshots([]DisplayFieldSnapshot{snapshot}).Matched)
	for _, state := range []string{"limited", "evicted"} {
		s := snapshot
		s.Completeness = state
		result := filter.EvaluateSnapshots([]DisplayFieldSnapshot{s})
		require.True(t, result.Partial)
		require.Equal(t, state == "limited", result.Complete)
		require.Equal(t, state == "limited", result.Matched)
	}
	s := snapshot
	s.Truncated = true
	require.True(t, filter.EvaluateSnapshots([]DisplayFieldSnapshot{s}).Matched)
	require.True(t, filter.EvaluateSnapshots([]DisplayFieldSnapshot{s}).Partial)
	s = snapshot
	s.Incomplete = true
	require.True(t, filter.EvaluateSnapshots([]DisplayFieldSnapshot{s}).Matched)
	require.True(t, filter.EvaluateSnapshots([]DisplayFieldSnapshot{s}).Partial)
	e.Completeness = "limited"
	require.Equal(t, "limited", e.DisplayFieldValues().Completeness)
}

func TestDisplayFilterPartialWitnessesAndUnknownAbsence(t *testing.T) {
	known := DisplayFieldSnapshot{Incomplete: true, Values: []DisplayFieldValue{{Name: "dns.qry.type", Value: uint64(1)}}}
	for _, tc := range []struct {
		expr              string
		matched, complete bool
	}{
		{"dns.qry.type", true, true},
		{"dns.qry.type == 1", true, true},
		{"dns.qry.type > 0", true, true},
		{"dns.qry.type <= 1", true, true},
		{"dns.qry.type == 28", false, false},
		{"dns.qry.type != 28", false, false},
		{"dns.qry.type != 1", false, true},
		{"not (dns.qry.type != 1)", true, true},
		{"not dns.qry.type", false, true},
		{"tls.version", false, false},
		{"not tls.version", false, false},
		{"dns.qry.type == 1 or tls.version", true, true},
		{"dns.qry.type == 1 and tls.version", false, false},
		{"not dns.qry.type and tls.version", false, true},
		{"not dns.qry.type or tls.version", false, false},
		{"not (dns.qry.type == 1 and tls.version)", false, false},
	} {
		t.Run(tc.expr, func(t *testing.T) {
			f, err := CompileDisplayFilter(tc.expr)
			require.NoError(t, err)
			result := f.EvaluateSnapshots([]DisplayFieldSnapshot{known})
			require.Equal(t, tc.matched, result.Matched)
			require.Equal(t, tc.complete, result.Complete)
			require.True(t, result.Partial)
			require.NotEmpty(t, result.Reason)
			// Every definite decision must survive adding later retained values.
			for _, extra := range [][]DisplayFieldValue{
				nil,
				{{Name: "dns.qry.type", Value: uint64(28)}},
				{{Name: "tls.version", Value: uint64(0x304)}},
				{{Name: "dns.qry.type", Value: uint64(28)}, {Name: "tls.version", Value: uint64(0x304)}},
			} {
				full := DisplayFieldSnapshot{Values: append(append([]DisplayFieldValue(nil), known.Values...), extra...)}
				if result.Complete {
					require.Equal(t, result.Matched, f.EvaluateSnapshots([]DisplayFieldSnapshot{full}).Matched)
				}
			}
		})
	}
}

func TestDisplayFilterPartialUnionDoesNotDiscardOtherPDUWitnesses(t *testing.T) {
	good := (&ProtocolEvent{Protocol: "bgp", Session: map[string]any{"Type": uint64(4)}}).DisplayFieldValues()
	bad := DisplayFieldSnapshot{Incomplete: true, Values: []DisplayFieldValue{{Name: "wg.counter", Value: uint64(7), ExactValue: "invalid"}}}
	f, err := CompileDisplayFilter("bgp.type == 4")
	require.NoError(t, err)
	for _, snapshots := range [][]DisplayFieldSnapshot{{bad, good}, {good, bad}, {{Completeness: "evicted"}, good}} {
		result := f.EvaluateSnapshots(snapshots)
		require.True(t, result.Complete)
		require.True(t, result.Matched)
		require.True(t, result.Partial)
	}
	for _, expr := range []string{"wg.counter", "wg.counter == 7", "not wg.counter"} {
		f, err := CompileDisplayFilter(expr)
		require.NoError(t, err)
		require.False(t, f.EvaluateSnapshots([]DisplayFieldSnapshot{bad}).Complete, "invalid exact values are not witnesses")
	}
	invalid := DisplayFieldSnapshot{Values: []DisplayFieldValue{{Name: "bgp.type", Value: "not-an-integer"}}}
	f, err = CompileDisplayFilter("bgp.type")
	require.NoError(t, err)
	require.False(t, f.EvaluateSnapshots([]DisplayFieldSnapshot{invalid}).Complete)
}

func TestExplicitPartialFieldScopeSurvivesEventNormalizationAndJSON(t *testing.T) {
	for _, protocol := range []string{"pim", "l2tp", "stp"} {
		e := &ProtocolEvent{Protocol: protocol, Status: "malformed", Completeness: "limited"}
		a := &binParser{config: BinParserConfig{OnEvent: func(*BinParserEvent) {}}}
		a.emit(e)
		require.Equal(t, "malformed", e.Completeness, "wire outcome remains a malformed observation")
		require.True(t, e.DisplayFieldValues().Incomplete, "explicit field refusal survives normalization")
		require.Equal(t, "limited", e.DisplayFieldValues().Completeness)
		e.Status, e.Completeness = "decoded", "message"
		wire, err := json.Marshal(e.DisplayFieldValues())
		require.NoError(t, err)
		var snapshot DisplayFieldSnapshot
		require.NoError(t, json.Unmarshal(wire, &snapshot))
		require.Equal(t, "limited", snapshot.Completeness)
		filter, err := CompileDisplayFilter("not stp.root.hw")
		require.NoError(t, err)
		require.False(t, filter.EvaluateSnapshots([]DisplayFieldSnapshot{snapshot}).Complete, "public edits and serialization cannot certify absent fields")
	}
}
