package sfreport

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func TestConvertSingleResultToSSAResultParts_NilResult(t *testing.T) {
	parts, err := ConvertSingleResultToSSAResultParts(nil, StreamPartsOptions{})
	require.NoError(t, err)
	assert.Nil(t, parts)
}

func TestConvertSingleResultToSSAResultPartsJSON_NilResult(t *testing.T) {
	raw, stats, err := ConvertSingleResultToSSAResultPartsJSON(nil, StreamPartsOptions{})
	require.NoError(t, err)
	assert.Empty(t, raw)
	require.NotNil(t, stats)
	assert.Equal(t, false, stats["has_payload"])
}

func TestNewStreamPartsOptions_Defaults(t *testing.T) {
	opts := NewStreamPartsOptions()
	assert.Equal(t, IRifyFullReportType, opts.ReportType)
	assert.True(t, opts.ShowDataflowPath)
	assert.True(t, opts.ShowFileContent)
	assert.True(t, opts.WithFile)
}

func TestDedupStrings(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"nil", nil, nil},
		{"single", []string{"a"}, []string{"a"}},
		{"duplicates", []string{"b", "a", "b", "c", "a"}, []string{"a", "b", "c"}},
		{"with_spaces", []string{" a ", "a", " b"}, []string{"a", "b"}},
		{"with_empty", []string{"a", "", "  ", "b"}, []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dedupStrings(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestNewStreamPartsOptions_DefaultDataflowDetailLevel verifies the default
// detail level is minimal (the historical behavior EmitSSAResult relied on
// before the fix).
func TestNewStreamPartsOptions_DefaultDataflowDetailLevel(t *testing.T) {
	opts := NewStreamPartsOptions()
	assert.Equal(t, DataflowDetailMinimal, opts.DataflowDetailLevel)
}

// TestWithStreamDataflowDetailLevel verifies the option setter writes the
// requested level into the options struct.
func TestWithStreamDataflowDetailLevel(t *testing.T) {
	opts := NewStreamPartsOptions(WithStreamDataflowDetailLevel(DataflowDetailFull))
	assert.Equal(t, DataflowDetailFull, opts.DataflowDetailLevel)
}

// TestMarshalDataFlowPath_FullContainsDotGraph verifies that full mode
// serialization preserves dot_graph, paths, and code_range — the fields that
// EmitSSAResult must expose by passing WithStreamDataflowDetailLevel(Full).
func TestMarshalDataFlowPath_FullContainsDotGraph(t *testing.T) {
	path := &DataFlowPath{
		Description: "taint flow",
		DotGraph:    "digraph { n1 -> n2 }",
		Paths:       [][]string{{"n1", "n2"}},
		Nodes: []*NodeInfo{
			{
				NodeID:       "n1",
				IRCode:       "source()",
				IRSourceHash: "h1",
				CodeRange:    &ssaapi.CodeRange{URL: "test.go", StartLine: 1, EndLine: 1},
			},
		},
		Edges: []*EdgeInfo{
			{EdgeID: "e1", FromNodeID: "n1", ToNodeID: "n2", EdgeType: "data-flow"},
		},
	}

	t.Run("full preserves dot_graph/paths/code_range", func(t *testing.T) {
		raw, err := MarshalDataFlowPath(path, DataflowDetailFull)
		require.NoError(t, err)
		require.NotNil(t, raw)

		var out DataFlowPath
		require.NoError(t, json.Unmarshal(raw, &out))
		assert.Equal(t, "digraph { n1 -> n2 }", out.DotGraph)
		assert.Equal(t, [][]string{{"n1", "n2"}}, out.Paths)
		require.Len(t, out.Nodes, 1)
		assert.NotNil(t, out.Nodes[0].CodeRange)
	})

	t.Run("minimal strips dot_graph/paths/code_range", func(t *testing.T) {
		raw, err := MarshalDataFlowPath(path, DataflowDetailMinimal)
		require.NoError(t, err)
		require.NotNil(t, raw)

		s := string(raw)
		assert.NotContains(t, s, "dot_graph")
		assert.NotContains(t, s, "paths")
		assert.NotContains(t, s, "code_range")

		var out DataFlowPath
		require.NoError(t, json.Unmarshal(raw, &out))
		assert.Empty(t, out.DotGraph)
		assert.Nil(t, out.Paths)
		require.Len(t, out.Nodes, 1)
		assert.Nil(t, out.Nodes[0].CodeRange)
	})
}
