package sfreport

import (
	"encoding/json"

	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

// StreamMinimalDataFlowPath is a reduced dataflow payload used for streaming.
// It keeps only the fields required for persisting audit graph:
// node/edge ids + ir_source_hash + offsets. Heavy fields like dot graph
// and embedded source snippets are excluded.
type StreamMinimalDataFlowPath struct {
	Description string                   `json:"description"`
	Nodes       []*StreamMinimalNodeInfo `json:"nodes"`
	Edges       []*StreamMinimalEdgeInfo `json:"edges"`
}

type StreamMinimalNodeInfo struct {
	NodeID       string `json:"node_id"`
	IRCode       string `json:"ir_code"`
	IRSourceHash string `json:"ir_source_hash"`
	StartOffset  int    `json:"start_offset"`
	EndOffset    int    `json:"end_offset"`
	IsEntryNode  bool   `json:"is_entry_node"`
}

type StreamMinimalEdgeInfo struct {
	EdgeID        string `json:"edge_id"`
	FromNodeID    string `json:"from_node_id"`
	ToNodeID      string `json:"to_node_id"`
	EdgeType      string `json:"edge_type"`
	AnalysisStep  int64  `json:"analysis_step,omitempty"`
	AnalysisLabel string `json:"analysis_label,omitempty"`
}

// MarshalStreamMinimalDataFlowPath converts a full DataFlowPath to a minimal
// JSON representation for streaming. Returns nil for nil or empty paths.
func MarshalStreamMinimalDataFlowPath(p *DataFlowPath) ([]byte, error) {
	if p == nil {
		return nil, nil
	}

	nodes := make([]*StreamMinimalNodeInfo, 0, len(p.Nodes))
	for _, n := range p.Nodes {
		if n == nil {
			continue
		}
		nodes = append(nodes, &StreamMinimalNodeInfo{
			NodeID:       n.NodeID,
			IRCode:       n.IRCode,
			IRSourceHash: n.IRSourceHash,
			StartOffset:  n.StartOffset,
			EndOffset:    n.EndOffset,
			IsEntryNode:  n.IsEntryNode,
		})
	}
	edges := make([]*StreamMinimalEdgeInfo, 0, len(p.Edges))
	for _, e := range p.Edges {
		if e == nil {
			continue
		}
		edges = append(edges, &StreamMinimalEdgeInfo{
			EdgeID:        e.EdgeID,
			FromNodeID:    e.FromNodeID,
			ToNodeID:      e.ToNodeID,
			EdgeType:      e.EdgeType,
			AnalysisStep:  e.AnalysisStep,
			AnalysisLabel: e.AnalysisLabel,
		})
	}

	if len(nodes) == 0 && len(edges) == 0 {
		return nil, nil
	}

	return json.Marshal(&StreamMinimalDataFlowPath{
		Description: p.Description,
		Nodes:       nodes,
		Edges:       edges,
	})
}

func (n *StreamMinimalNodeInfo) ToAuditNode(riskHash string) *ssadb.AuditNode {
	if n == nil {
		return nil
	}
	an := ssadb.NewAuditNode()
	an.AuditNodeStatus = ssadb.AuditNodeStatus{
		RiskHash: riskHash,
	}
	an.IsEntryNode = n.IsEntryNode
	an.IRCodeID = -1
	an.TmpValue = n.IRCode
	an.TmpValueFileHash = n.IRSourceHash
	an.TmpStartOffset = n.StartOffset
	an.TmpEndOffset = n.EndOffset
	return an
}

// ToAuditEdge converts to an AuditEdge using the node-id → ULID mapping.
// Returns nil if either endpoint is missing from the mapping.
func (e *StreamMinimalEdgeInfo) ToAuditEdge(m map[string]string) *ssadb.AuditEdge {
	if e == nil || m == nil {
		return nil
	}
	from, ok1 := m[e.FromNodeID]
	to, ok2 := m[e.ToNodeID]
	if !ok1 || !ok2 {
		return nil
	}
	return &ssadb.AuditEdge{
		FromNode:      from,
		ToNode:        to,
		EdgeType:      ssadb.ValidEdgeType(e.EdgeType),
		AnalysisLabel: e.AnalysisLabel,
		AnalysisStep:  e.AnalysisStep,
	}
}
