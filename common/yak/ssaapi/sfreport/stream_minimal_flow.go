package sfreport

import (
	"encoding/json"

	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

// StreamMinimalDataFlowPath is a reduced dataflow payload used for streaming.
// It intentionally keeps only the fields required for persisting audit graph:
// node/edge ids + ir_source_hash + offsets. Heavy fields like dot graph and
// embedded source snippets are excluded.
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

func MarshalStreamMinimalDataFlowPath(p *DataFlowPath) ([]byte, error) {
	if p == nil {
		return nil, nil
	}
	min := &StreamMinimalDataFlowPath{
		Description: p.Description,
		Nodes:       make([]*StreamMinimalNodeInfo, 0, len(p.Nodes)),
		Edges:       make([]*StreamMinimalEdgeInfo, 0, len(p.Edges)),
	}
	for _, n := range p.Nodes {
		if n == nil {
			continue
		}
		min.Nodes = append(min.Nodes, &StreamMinimalNodeInfo{
			NodeID:       n.NodeID,
			IRCode:       n.IRCode,
			IRSourceHash: n.IRSourceHash,
			StartOffset:  n.StartOffset,
			EndOffset:    n.EndOffset,
			IsEntryNode:  n.IsEntryNode,
		})
	}
	for _, e := range p.Edges {
		if e == nil {
			continue
		}
		min.Edges = append(min.Edges, &StreamMinimalEdgeInfo{
			EdgeID:        e.EdgeID,
			FromNodeID:    e.FromNodeID,
			ToNodeID:      e.ToNodeID,
			EdgeType:      e.EdgeType,
			AnalysisStep:  e.AnalysisStep,
			AnalysisLabel: e.AnalysisLabel,
		})
	}
	return json.Marshal(min)
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

func (e *StreamMinimalEdgeInfo) ToAuditEdge(m map[string]string) *ssadb.AuditEdge {
	if e == nil {
		return nil
	}
	return &ssadb.AuditEdge{
		FromNode:      m[e.FromNodeID],
		ToNode:        m[e.ToNodeID],
		EdgeType:      ssadb.ValidEdgeType(e.EdgeType),
		AnalysisLabel: e.AnalysisLabel,
		AnalysisStep:  e.AnalysisStep,
	}
}
