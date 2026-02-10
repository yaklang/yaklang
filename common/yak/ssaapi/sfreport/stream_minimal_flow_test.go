package sfreport

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshalStreamMinimalDataFlowPath_Nil(t *testing.T) {
	raw, err := MarshalStreamMinimalDataFlowPath(nil)
	require.NoError(t, err)
	assert.Nil(t, raw)
}

func TestMarshalStreamMinimalDataFlowPath_EmptyNodesAndEdges(t *testing.T) {
	p := &DataFlowPath{Description: "empty"}
	raw, err := MarshalStreamMinimalDataFlowPath(p)
	require.NoError(t, err)
	assert.Nil(t, raw, "empty nodes+edges should return nil")
}

func TestMarshalStreamMinimalDataFlowPath_NilNodesFiltered(t *testing.T) {
	p := &DataFlowPath{
		Description: "has nils",
		Nodes:       []*NodeInfo{nil, {NodeID: "n1", IRCode: "x"}},
		Edges:       []*EdgeInfo{nil, {EdgeID: "e1", FromNodeID: "n1", ToNodeID: "n2"}},
	}
	raw, err := MarshalStreamMinimalDataFlowPath(p)
	require.NoError(t, err)
	require.NotNil(t, raw)

	var min StreamMinimalDataFlowPath
	require.NoError(t, json.Unmarshal(raw, &min))
	assert.Len(t, min.Nodes, 1)
	assert.Len(t, min.Edges, 1)
	assert.Equal(t, "n1", min.Nodes[0].NodeID)
	assert.Equal(t, "e1", min.Edges[0].EdgeID)
}

func TestMarshalStreamMinimalDataFlowPath_Roundtrip(t *testing.T) {
	p := &DataFlowPath{
		Description: "taint flow",
		Nodes: []*NodeInfo{
			{NodeID: "n1", IRCode: "source()", IRSourceHash: "h1", StartOffset: 0, EndOffset: 8, IsEntryNode: true},
			{NodeID: "n2", IRCode: "sink(x)", IRSourceHash: "h2", StartOffset: 10, EndOffset: 17},
		},
		Edges: []*EdgeInfo{
			{EdgeID: "e1", FromNodeID: "n1", ToNodeID: "n2", EdgeType: "data-flow", AnalysisStep: 1, AnalysisLabel: "step1"},
		},
	}
	raw, err := MarshalStreamMinimalDataFlowPath(p)
	require.NoError(t, err)
	require.NotNil(t, raw)

	var min StreamMinimalDataFlowPath
	require.NoError(t, json.Unmarshal(raw, &min))

	assert.Equal(t, "taint flow", min.Description)
	require.Len(t, min.Nodes, 2)
	assert.Equal(t, "n1", min.Nodes[0].NodeID)
	assert.Equal(t, "source()", min.Nodes[0].IRCode)
	assert.Equal(t, "h1", min.Nodes[0].IRSourceHash)
	assert.Equal(t, 0, min.Nodes[0].StartOffset)
	assert.Equal(t, 8, min.Nodes[0].EndOffset)
	assert.True(t, min.Nodes[0].IsEntryNode)
	assert.False(t, min.Nodes[1].IsEntryNode)

	require.Len(t, min.Edges, 1)
	assert.Equal(t, "e1", min.Edges[0].EdgeID)
	assert.Equal(t, "data-flow", min.Edges[0].EdgeType)
	assert.Equal(t, int64(1), min.Edges[0].AnalysisStep)
	assert.Equal(t, "step1", min.Edges[0].AnalysisLabel)
}

func TestMarshalStreamMinimalDataFlowPath_StripsHeavyFields(t *testing.T) {
	p := &DataFlowPath{
		Description: "flow",
		DotGraph:    "digraph { n1 -> n2 }",
		Nodes: []*NodeInfo{
			{
				NodeID:          "n1",
				IRCode:          "x",
				SourceCode:      "long source code that should be stripped",
				SourceCodeStart: 100,
				IRSourceHash:    "h1",
			},
		},
		Edges: []*EdgeInfo{
			{EdgeID: "e1", FromNodeID: "n1", ToNodeID: "n2", EdgeType: "df"},
		},
	}
	raw, err := MarshalStreamMinimalDataFlowPath(p)
	require.NoError(t, err)

	// Raw JSON should NOT contain heavy fields.
	rawStr := string(raw)
	assert.NotContains(t, rawStr, "dot_graph")
	assert.NotContains(t, rawStr, "source_code")
	assert.NotContains(t, rawStr, "source_code_start")
	assert.NotContains(t, rawStr, "code_range")
}

func TestToAuditNode_Nil(t *testing.T) {
	var n *StreamMinimalNodeInfo
	assert.Nil(t, n.ToAuditNode("hash"))
}

func TestToAuditNode_Basic(t *testing.T) {
	n := &StreamMinimalNodeInfo{
		NodeID:       "n1",
		IRCode:       "source()",
		IRSourceHash: "fh1",
		StartOffset:  5,
		EndOffset:    13,
		IsEntryNode:  true,
	}
	an := n.ToAuditNode("riskhash1")
	require.NotNil(t, an)
	assert.Equal(t, "riskhash1", an.AuditNodeStatus.RiskHash)
	assert.True(t, an.IsEntryNode)
	assert.Equal(t, int64(-1), an.IRCodeID)
	assert.Equal(t, "source()", an.TmpValue)
	assert.Equal(t, "fh1", an.TmpValueFileHash)
	assert.Equal(t, 5, an.TmpStartOffset)
	assert.Equal(t, 13, an.TmpEndOffset)
}

func TestToAuditEdge_Nil(t *testing.T) {
	var e *StreamMinimalEdgeInfo
	assert.Nil(t, e.ToAuditEdge(nil))
}

func TestToAuditEdge_NilMap(t *testing.T) {
	e := &StreamMinimalEdgeInfo{EdgeID: "e1", FromNodeID: "n1", ToNodeID: "n2"}
	assert.Nil(t, e.ToAuditEdge(nil))
}

func TestToAuditEdge_MissingNode(t *testing.T) {
	e := &StreamMinimalEdgeInfo{EdgeID: "e1", FromNodeID: "n1", ToNodeID: "n2"}
	m := map[string]string{"n1": "ulid1"} // n2 is missing
	assert.Nil(t, e.ToAuditEdge(m), "should return nil when endpoint is missing")
}

func TestToAuditEdge_Success(t *testing.T) {
	e := &StreamMinimalEdgeInfo{
		EdgeID:        "e1",
		FromNodeID:    "n1",
		ToNodeID:      "n2",
		EdgeType:      "data-flow",
		AnalysisStep:  3,
		AnalysisLabel: "propagation",
	}
	m := map[string]string{"n1": "ulid1", "n2": "ulid2"}
	ae := e.ToAuditEdge(m)
	require.NotNil(t, ae)
	assert.Equal(t, "ulid1", ae.FromNode)
	assert.Equal(t, "ulid2", ae.ToNode)
	assert.Equal(t, int64(3), ae.AnalysisStep)
	assert.Equal(t, "propagation", ae.AnalysisLabel)
}
