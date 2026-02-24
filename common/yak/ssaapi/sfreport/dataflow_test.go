package sfreport

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func cleanupByRiskHash(t *testing.T, db *gorm.DB, riskHash string) {
	t.Helper()
	nodeIDs := collectNodeIDs(db, riskHash)
	if len(nodeIDs) > 0 {
		db.Where("from_node IN (?) OR to_node IN (?)", nodeIDs, nodeIDs).Unscoped().Delete(&ssadb.AuditEdge{})
	}
	db.Where("risk_hash = ?", riskHash).Unscoped().Delete(&ssadb.AuditNode{})
}

func countNodes(db *gorm.DB, riskHash string) int {
	var c int
	db.Model(&ssadb.AuditNode{}).Where("risk_hash = ?", riskHash).Count(&c)
	return c
}

func countEdgesByNodes(db *gorm.DB, nodeIDs []string) int {
	if len(nodeIDs) == 0 {
		return 0
	}
	var c int
	db.Model(&ssadb.AuditEdge{}).Where("from_node IN (?) OR to_node IN (?)", nodeIDs, nodeIDs).Count(&c)
	return c
}

func queryNodes(db *gorm.DB, riskHash string) []*ssadb.AuditNode {
	var nodes []*ssadb.AuditNode
	db.Where("risk_hash = ?", riskHash).Find(&nodes)
	return nodes
}

func queryEdgesByNodes(db *gorm.DB, nodeIDs []string) []*ssadb.AuditEdge {
	if len(nodeIDs) == 0 {
		return nil
	}
	var edges []*ssadb.AuditEdge
	db.Where("from_node IN (?) OR to_node IN (?)", nodeIDs, nodeIDs).Find(&edges)
	return edges
}

func collectNodeIDs(db *gorm.DB, riskHash string) []string {
	var ids []string
	db.Model(&ssadb.AuditNode{}).Where("risk_hash = ?", riskHash).Pluck("node_id", &ids)
	return ids
}

func TestSaveDataFlow(t *testing.T) {
	db := ssadb.GetDB()
	require.NotNil(t, db)

	tests := []struct {
		name          string
		path          *DataFlowPath
		wantNodeCount int
		wantEdgeCount int
		verify        func(t *testing.T, db *gorm.DB, riskHash string)
	}{
		{
			name:          "nil path",
			path:          nil,
			wantNodeCount: 0,
			wantEdgeCount: 0,
		},
		{
			name:          "empty nodes",
			path:          &DataFlowPath{Description: "empty"},
			wantNodeCount: 0,
			wantEdgeCount: 0,
		},
		{
			name: "single node no edges",
			path: &DataFlowPath{
				Nodes: []*NodeInfo{
					{NodeID: "n1", IRCode: "source()", IRSourceHash: "h1", StartOffset: 0, EndOffset: 8, IsEntryNode: true},
				},
			},
			wantNodeCount: 1,
			wantEdgeCount: 0,
			verify: func(t *testing.T, db *gorm.DB, riskHash string) {
				nodes := queryNodes(db, riskHash)
				require.Len(t, nodes, 1)
				assert.Equal(t, "source()", nodes[0].TmpValue)
				assert.Equal(t, "h1", nodes[0].TmpValueFileHash)
				assert.Equal(t, 0, nodes[0].TmpStartOffset)
				assert.Equal(t, 8, nodes[0].TmpEndOffset)
				assert.True(t, nodes[0].IsEntryNode)
				assert.Equal(t, int64(-1), nodes[0].IRCodeID)
			},
		},
		{
			name: "nodes with edges",
			path: &DataFlowPath{
				Description: "taint flow",
				Nodes: []*NodeInfo{
					{NodeID: "n1", IRCode: "source()", IRSourceHash: "h1", StartOffset: 0, EndOffset: 8, IsEntryNode: true},
					{NodeID: "n2", IRCode: "sink(x)", IRSourceHash: "h2", StartOffset: 10, EndOffset: 17},
				},
				Edges: []*EdgeInfo{
					{EdgeID: "e1", FromNodeID: "n1", ToNodeID: "n2", EdgeType: string(ssadb.EdgeType_DataFlow), AnalysisStep: 1, AnalysisLabel: "step1"},
				},
			},
			wantNodeCount: 2,
			wantEdgeCount: 1,
			verify: func(t *testing.T, db *gorm.DB, riskHash string) {
				nodeIDs := collectNodeIDs(db, riskHash)
				edges := queryEdgesByNodes(db, nodeIDs)
				require.Len(t, edges, 1)
				assert.NotEmpty(t, edges[0].FromNode)
				assert.NotEmpty(t, edges[0].ToNode)
				assert.NotEqual(t, edges[0].FromNode, edges[0].ToNode)
				assert.Equal(t, ssadb.EdgeType_DataFlow, edges[0].EdgeType)
				assert.Equal(t, int64(1), edges[0].AnalysisStep)
				assert.Equal(t, "step1", edges[0].AnalysisLabel)
			},
		},
		{
			name: "nil entries filtered",
			path: &DataFlowPath{
				Nodes: []*NodeInfo{nil, {NodeID: "n1", IRCode: "x"}, nil},
				Edges: []*EdgeInfo{nil},
			},
			wantNodeCount: 1,
			wantEdgeCount: 0,
		},
		{
			name: "edge with missing node skipped",
			path: &DataFlowPath{
				Nodes: []*NodeInfo{
					{NodeID: "n1", IRCode: "x"},
				},
				Edges: []*EdgeInfo{
					{EdgeID: "e1", FromNodeID: "n1", ToNodeID: "n_missing"},
				},
			},
			wantNodeCount: 1,
			wantEdgeCount: 0,
		},
		{
			name: "many nodes batched",
			path: func() *DataFlowPath {
				nodes := make([]*NodeInfo, 50)
				for i := range nodes {
					nodes[i] = &NodeInfo{
						NodeID:       fmt.Sprintf("n%d", i),
						IRCode:       fmt.Sprintf("stmt_%d", i),
						IRSourceHash: fmt.Sprintf("h%d", i),
						StartOffset:  i * 10,
						EndOffset:    i*10 + 5,
					}
				}
				edges := make([]*EdgeInfo, 49)
				for i := range edges {
					edges[i] = &EdgeInfo{
						EdgeID:     fmt.Sprintf("e%d", i),
						FromNodeID: fmt.Sprintf("n%d", i),
						ToNodeID:   fmt.Sprintf("n%d", i+1),
						EdgeType:   string(ssadb.EdgeType_DependsOn),
					}
				}
				return &DataFlowPath{Nodes: nodes, Edges: edges}
			}(),
			wantNodeCount: 50,
			wantEdgeCount: 49,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			riskHash := "test-" + uuid.NewString()
			t.Cleanup(func() { cleanupByRiskHash(t, db, riskHash) })

			sc := NewSaveDataFlowCtx(db, riskHash)
			sc.SaveDataFlow(tt.path)

			assert.Equal(t, tt.wantNodeCount, countNodes(db, riskHash))
			nodeIDs := collectNodeIDs(db, riskHash)
			assert.Equal(t, tt.wantEdgeCount, countEdgesByNodes(db, nodeIDs))
			if tt.verify != nil {
				tt.verify(t, db, riskHash)
			}
		})
	}
}

func TestSaveDataFlow_DeduplicateNodes(t *testing.T) {
	db := ssadb.GetDB()
	require.NotNil(t, db)
	riskHash := "test-dedup-" + uuid.NewString()
	t.Cleanup(func() { cleanupByRiskHash(t, db, riskHash) })

	sc := NewSaveDataFlowCtx(db, riskHash)

	sc.SaveDataFlow(&DataFlowPath{
		Nodes: []*NodeInfo{
			{NodeID: "n1", IRCode: "source()"},
			{NodeID: "n2", IRCode: "sink()"},
		},
		Edges: []*EdgeInfo{
			{EdgeID: "e1", FromNodeID: "n1", ToNodeID: "n2", EdgeType: string(ssadb.EdgeType_DependsOn)},
		},
	})
	assert.Equal(t, 2, countNodes(db, riskHash))
	assert.Equal(t, 1, countEdgesByNodes(db, collectNodeIDs(db, riskHash)))

	sc.SaveDataFlow(&DataFlowPath{
		Nodes: []*NodeInfo{
			{NodeID: "n1", IRCode: "source()"},
			{NodeID: "n3", IRCode: "transform()"},
		},
		Edges: []*EdgeInfo{
			{EdgeID: "e2", FromNodeID: "n1", ToNodeID: "n3", EdgeType: string(ssadb.EdgeType_DependsOn)},
		},
	})

	assert.Equal(t, 3, countNodes(db, riskHash), "n1 should not be duplicated")
	assert.Equal(t, 2, countEdgesByNodes(db, collectNodeIDs(db, riskHash)))
}

func TestMarshalMinimalDataFlowPath(t *testing.T) {
	tests := []struct {
		name      string
		path      *DataFlowPath
		wantNil   bool
		wantNodes int
		wantEdges int
		verify    func(t *testing.T, raw []byte)
	}{
		{
			name:    "nil path",
			path:    nil,
			wantNil: true,
		},
		{
			name:    "empty nodes and edges",
			path:    &DataFlowPath{Description: "empty"},
			wantNil: true,
		},
		{
			name: "nil entries filtered",
			path: &DataFlowPath{
				Description: "has nils",
				Nodes:       []*NodeInfo{nil, {NodeID: "n1", IRCode: "x"}},
				Edges:       []*EdgeInfo{nil, {EdgeID: "e1", FromNodeID: "n1", ToNodeID: "n2"}},
			},
			wantNodes: 1,
			wantEdges: 1,
		},
		{
			name: "roundtrip preserves fields",
			path: &DataFlowPath{
				Description: "taint flow",
				Nodes: []*NodeInfo{
					{NodeID: "n1", IRCode: "source()", IRSourceHash: "h1", StartOffset: 0, EndOffset: 8, IsEntryNode: true},
					{NodeID: "n2", IRCode: "sink(x)", IRSourceHash: "h2", StartOffset: 10, EndOffset: 17},
				},
				Edges: []*EdgeInfo{
					{EdgeID: "e1", FromNodeID: "n1", ToNodeID: "n2", EdgeType: "data-flow", AnalysisStep: 1, AnalysisLabel: "step1"},
				},
			},
			wantNodes: 2,
			wantEdges: 1,
			verify: func(t *testing.T, raw []byte) {
				var out DataFlowPath
				require.NoError(t, json.Unmarshal(raw, &out))
				assert.Equal(t, "taint flow", out.Description)
				assert.Equal(t, "source()", out.Nodes[0].IRCode)
				assert.True(t, out.Nodes[0].IsEntryNode)
				assert.Equal(t, int64(1), out.Edges[0].AnalysisStep)
			},
		},
		{
			name: "strips heavy fields",
			path: &DataFlowPath{
				Description: "flow",
				DotGraph:    "digraph { n1 -> n2 }",
				Nodes: []*NodeInfo{
					{NodeID: "n1", IRCode: "x", SourceCode: "long source code", SourceCodeStart: 100, IRSourceHash: "h1"},
				},
				Edges: []*EdgeInfo{
					{EdgeID: "e1", FromNodeID: "n1", ToNodeID: "n2", EdgeType: "df"},
				},
			},
			wantNodes: 1,
			wantEdges: 1,
			verify: func(t *testing.T, raw []byte) {
				s := string(raw)
				assert.NotContains(t, s, "dot_graph")
				assert.NotContains(t, s, "source_code")
				assert.NotContains(t, s, "source_code_start")
				assert.NotContains(t, s, "code_range")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := MarshalMinimalDataFlowPath(tt.path)
			require.NoError(t, err)
			if tt.wantNil {
				assert.Nil(t, raw)
				return
			}
			require.NotNil(t, raw)

			var out DataFlowPath
			require.NoError(t, json.Unmarshal(raw, &out))
			assert.Len(t, out.Nodes, tt.wantNodes)
			assert.Len(t, out.Edges, tt.wantEdges)
			if tt.verify != nil {
				tt.verify(t, raw)
			}
		})
	}
}
