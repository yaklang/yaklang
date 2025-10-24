package sfreport

import (
	"fmt"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type DataFlowPath struct {
	Description string      `json:"description"`
	Nodes       []*NodeInfo `json:"nodes"`
	Edges       []*EdgeInfo `json:"edges"`
	DotGraph    string      `json:"dot_graph,omitempty"`
}

type NodeInfo struct {
	NodeID          string            `json:"node_id"`
	IRCode          string            `json:"ir_code"`
	SourceCode      string            `json:"source_code"`
	SourceCodeStart int               `json:"source_code_start"`
	CodeRange       *ssaapi.CodeRange `json:"code_range"`

	// for audit
	IRSourceHash string `json:"ir_source_hash"`
	StartOffset  int    `json:"start_offset"`
	EndOffset    int    `json:"end_offset"`
	IsEntryNode  bool   `json:"is_entry_node"`
}

type EdgeInfo struct {
	EdgeID        string `json:"edge_id"`
	FromNodeID    string `json:"from_node_id"`
	ToNodeID      string `json:"to_node_id"`
	EdgeType      string `json:"edge_type"`
	AnalysisStep  int64  `json:"analysis_step"`
	AnalysisLabel string `json:"analysis_label"`
}

func GenerateDataFlowAnalysis(risk *schema.SSARisk, values ...*ssaapi.Value) (*DataFlowPath, []string, error) {
	if risk.ResultID == 0 || risk.Variable == "" {
		return nil, nil, utils.Errorf("risk has no valid result ID or variable")
	}

	var value *ssaapi.Value
	if len(values) > 0 {
		value = values[0]
	}

	if utils.IsNil(value) {
		var err error
		value, err = GetValueByRisk(risk)
		if err != nil {
			return nil, nil, utils.Errorf("get value by risk failed: %v", err)
		}
	}
	// 这儿行为图的产生是GraphKindShow而不是GraphKindDump
	// 因此产生的图数据而直接存数据库的行为是不一致的
	// 但是好像又不影响最后查看结果
	dotGraph := ssaapi.NewDotGraph()
	value.GenerateGraph(dotGraph)
	nodes, edges, irSourceHashes := coverNodeAndEdgeInfos(dotGraph, value)

	path := &DataFlowPath{
		Description: generatePathDescription(risk),
		Nodes:       nodes,
		Edges:       edges,
		DotGraph:    dotGraph.String(),
	}
	return path, irSourceHashes, nil
}

func generatePathDescription(risk *schema.SSARisk) string {
	return fmt.Sprintf("Data flow path for %s vulnerability in %s", risk.RiskType, risk.ProgramName)
}

func coverNodeAndEdgeInfos(graph *ssaapi.DotGraph, entryValue *ssaapi.Value) ([]*NodeInfo, []*EdgeInfo, []string) {
	nodes := make([]*NodeInfo, 0, graph.NodeCount())
	edges := make([]*EdgeInfo, 0)
	irSourceHashes := make([]string, 0)
	graph.ForEach(func(s string, v *ssaapi.Value) {
		rng := v.GetRange()
		if rng == nil {
			return
		}
		codeRange, source := ssaapi.CoverCodeRange(rng)
		nodeInfo := &NodeInfo{
			NodeID:          s,
			IRCode:          v.String(),
			SourceCode:      source,
			SourceCodeStart: 0,
			CodeRange:       codeRange,
			StartOffset:     rng.GetStartOffset(),
			EndOffset:       rng.GetEndOffset(),
			IsEntryNode:     entryValue != nil && v == entryValue,
		}
		irSourceHash := rng.GetEditor().GetIrSourceHash()
		nodeInfo.IRSourceHash = irSourceHash
		irSourceHashes = append(irSourceHashes, irSourceHash)
		nodes = append(nodes, nodeInfo)
	})

	edgeCache := make(map[string]struct{})
	for edgeID, edge := range graph.Graph.GetAllEdges() {
		if edge == nil {
			continue
		}

		fromNode := edge.From()
		toNode := edge.To()
		if fromNode == nil || toNode == nil {
			continue
		}

		hash := codec.Md5(fmt.Sprintf(
			"%d-%d-%s",
			fromNode.ID(),
			toNode.ID(),
			edge.Label,
		))
		if _, ok := edgeCache[hash]; ok {
			continue
		}
		edgeCache[hash] = struct{}{}

		typ := ssadb.ValidEdgeType(edge.Label)
		edgeInfo := &EdgeInfo{
			EdgeID:        fmt.Sprintf("e%d", edgeID),
			EdgeType:      string(typ),
			AnalysisLabel: edge.Label,
		}
		switch typ {
		case ssadb.EdgeType_Predecessor:
			edgeInfo.ToNodeID = nodeId(fromNode.ID())
			edgeInfo.FromNodeID = nodeId(toNode.ID())
		default:
			edgeInfo.ToNodeID = nodeId(toNode.ID())
			edgeInfo.FromNodeID = nodeId(fromNode.ID())
		}
		edges = append(edges, edgeInfo)
	}

	return nodes, edges, irSourceHashes
}

func nodeId(i int) string {
	return fmt.Sprintf("n%d", i)
}

func (n *NodeInfo) ToAuditNode(riskHash string) *ssadb.AuditNode {
	an := ssadb.NewAuditNode()
	an.AuditNodeStatus = ssadb.AuditNodeStatus{
		RiskHash: riskHash,
	}
	an.IsEntryNode = n.IsEntryNode
	an.IRCodeID = -1
	an.TmpValue = n.IRCode
	an.TmpValueFileHash = n.IRSourceHash
	if n.CodeRange != nil {
		an.TmpStartOffset = n.StartOffset
		an.TmpEndOffset = n.EndOffset
	}
	return an
}

func (e *EdgeInfo) ToAuditEdge(m map[string]string) *ssadb.AuditEdge {
	return &ssadb.AuditEdge{
		FromNode:      m[e.FromNodeID],
		ToNode:        m[e.ToNodeID],
		EdgeType:      ssadb.ValidEdgeType(e.EdgeType),
		AnalysisLabel: e.AnalysisLabel,
	}
}

type saveDataFlowCtx struct {
	db       *gorm.DB
	nodeMap  map[string]string // nodeId -> nodeid
	riskHash string
}

func newSaveDataFlowCtx(db *gorm.DB, riskHash string) *saveDataFlowCtx {
	return &saveDataFlowCtx{
		db:       db,
		nodeMap:  make(map[string]string),
		riskHash: riskHash,
	}
}

func (sc *saveDataFlowCtx) SaveDataFlow(dp *DataFlowPath) {
	if sc == nil || dp == nil || len(dp.Nodes) == 0 {
		return
	}
	sc.saveAuditNodes(dp.Nodes)
	sc.saveAuditEdges(dp.Edges)
}

func (sc *saveDataFlowCtx) saveAuditNodes(nodes []*NodeInfo) {
	if len(nodes) == 0 {
		return
	}
	for _, n := range nodes {
		// 存储过的不重复存储
		if sc.nodeMap[n.NodeID] != "" {
			continue
		}
		auditNode := n.ToAuditNode(sc.riskHash)
		if err := sc.db.Create(auditNode).Error; err != nil {
			log.Errorf("save audit node failed: %v", err)
		}
		sc.nodeMap[n.NodeID] = auditNode.NodeID
	}
}

func (sc *saveDataFlowCtx) saveAuditEdges(edges []*EdgeInfo) {
	if len(edges) == 0 {
		return
	}

	for _, e := range edges {
		auditEdge := e.ToAuditEdge(sc.nodeMap)
		if err := sc.db.Create(auditEdge).Error; err != nil {
			log.Errorf("save audit edge failed: %v", err)
		}
	}
	return
}
