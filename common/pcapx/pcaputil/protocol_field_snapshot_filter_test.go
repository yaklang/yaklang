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
		require.False(t, filter.EvaluateSnapshots([]DisplayFieldSnapshot{s}).Complete)
	}
	s := snapshot
	s.Truncated = true
	require.False(t, filter.EvaluateSnapshots([]DisplayFieldSnapshot{s}).Complete)
	s = snapshot
	s.Incomplete = true
	require.False(t, filter.EvaluateSnapshots([]DisplayFieldSnapshot{s}).Complete)
	e.Completeness = "limited"
	require.Equal(t, "limited", e.DisplayFieldValues().Completeness)
}

func TestExplicitPartialFieldScopeSurvivesEventNormalizationAndJSON(t *testing.T) {
	for _, protocol := range []string{"pim", "l2tp", "stp"} {
		e := &ProtocolEvent{Protocol: protocol, Status: "malformed", Completeness: "limited"}
		a := &binParser{config: BinParserConfig{OnEvent: func(*BinParserEvent) {}}}
		a.emit(e)
		require.Equal(t, "malformed", e.Completeness, "wire outcome remains a malformed observation")
		require.True(t, e.DisplayFieldValues().Incomplete, "explicit field refusal survives normalization")
		e.Status, e.Completeness = "decoded", "message"
		wire, err := json.Marshal(e.DisplayFieldValues())
		require.NoError(t, err)
		var snapshot DisplayFieldSnapshot
		require.NoError(t, json.Unmarshal(wire, &snapshot))
		filter, err := CompileDisplayFilter("not stp.root.hw")
		require.NoError(t, err)
		require.False(t, filter.EvaluateSnapshots([]DisplayFieldSnapshot{snapshot}).Complete, "public edits and serialization cannot certify absent fields")
	}
}
