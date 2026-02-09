package sfreport

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/lib/pq"
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
	minimal := false
	if raw := strings.TrimSpace(os.Getenv("SSA_DATAFLOW_MINIMAL")); raw != "" {
		minimal = raw == "1" || strings.EqualFold(raw, "true")
	}
	dotGraph := ssaapi.NewDotGraph()
	value.GenerateGraph(dotGraph)
	nodes, edges, irSourceHashes := coverNodeAndEdgeInfos(dotGraph, value, minimal)

	path := &DataFlowPath{
		Description: generatePathDescription(risk),
		Nodes:       nodes,
		Edges:       edges,
	}
	if !minimal {
		path.DotGraph = dotGraph.String()
	}
	return path, irSourceHashes, nil
}

func generatePathDescription(risk *schema.SSARisk) string {
	return fmt.Sprintf("Data flow path for %s vulnerability in %s", risk.RiskType, risk.ProgramName)
}

func coverNodeAndEdgeInfos(graph *ssaapi.DotGraph, entryValue *ssaapi.Value, minimal bool) ([]*NodeInfo, []*EdgeInfo, []string) {
	nodes := make([]*NodeInfo, 0, graph.NodeCount())
	edges := make([]*EdgeInfo, 0)
	irSourceHashes := make([]string, 0)
	entryNodeID := ""
	if graph != nil && entryValue != nil {
		// In some graph constructions, the entryValue pointer may not be present in DotGraph's node map
		// (e.g. when values are rebuilt/normalized). Prefer node-id comparison when possible.
		entryNodeID = graph.NodeName(entryValue)
	}
	markedEntry := false
	graph.ForEach(func(s string, v *ssaapi.Value) {
		rng := v.GetRange()
		if rng == nil {
			return
		}
		nodeInfo := &NodeInfo{
			NodeID:      s,
			IRCode:      v.String(),
			StartOffset: rng.GetStartOffset(),
			EndOffset:   rng.GetEndOffset(),
			IsEntryNode: (entryNodeID != "" && s == entryNodeID) || (entryNodeID == "" && !markedEntry),
		}
		if nodeInfo.IsEntryNode {
			markedEntry = true
		}
		if !minimal {
			codeRange, source := ssaapi.CoverCodeRange(rng)
			nodeInfo.SourceCode = source
			nodeInfo.SourceCodeStart = 0
			nodeInfo.CodeRange = codeRange
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
	an.TmpStartOffset = n.StartOffset
	an.TmpEndOffset = n.EndOffset
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

type SaveDataFlowCtx struct {
	db       *gorm.DB
	nodeMap  map[string]string // nodeId -> nodeid
	riskHash string
}

func NewSaveDataFlowCtx(db *gorm.DB, riskHash string) *SaveDataFlowCtx {
	return &SaveDataFlowCtx{
		db:       db,
		nodeMap:  make(map[string]string),
		riskHash: riskHash,
	}
}

func (sc *SaveDataFlowCtx) SaveDataFlow(dp *DataFlowPath) {
	if sc == nil || dp == nil || len(dp.Nodes) == 0 {
		return
	}
	// Dataflow audit nodes/edges insertion can be extremely write-heavy. Use one transaction
	// and multi-values INSERTs. (gorm v1 slice Create panics in this codebase.)
	tx := sc.db.Begin()
	if tx == nil || tx.Error != nil {
		// Fallback to the original behavior if transaction cannot be started.
		sc.saveAuditNodes(sc.db, dp.Nodes, defaultDataflowBatchSize())
		sc.saveAuditEdges(sc.db, dp.Edges, defaultDataflowBatchSize())
		return
	}
	if err := sc.saveAuditNodes(tx, dp.Nodes, defaultDataflowBatchSize()); err != nil {
		_ = tx.Rollback().Error
		log.Errorf("save dataflow nodes failed: %v", err)
		// Best-effort fallback without tx.
		_ = sc.saveAuditNodes(sc.db, dp.Nodes, defaultDataflowBatchSize())
		_ = sc.saveAuditEdges(sc.db, dp.Edges, defaultDataflowBatchSize())
		return
	}
	if err := sc.saveAuditEdges(tx, dp.Edges, defaultDataflowBatchSize()); err != nil {
		_ = tx.Rollback().Error
		log.Errorf("save dataflow edges failed: %v", err)
		_ = sc.saveAuditEdges(sc.db, dp.Edges, defaultDataflowBatchSize())
		return
	}
	if err := tx.Commit().Error; err != nil {
		_ = tx.Rollback().Error
		log.Errorf("save dataflow commit failed: %v", err)
	}
}

func (sc *SaveDataFlowCtx) SaveMinimalDataFlow(dp *StreamMinimalDataFlowPath) {
	if sc == nil || dp == nil || len(dp.Nodes) == 0 {
		return
	}
	tx := sc.db.Begin()
	if tx == nil || tx.Error != nil {
		_ = sc.saveAuditNodesMinimal(sc.db, dp.Nodes, defaultDataflowBatchSize())
		_ = sc.saveAuditEdgesMinimal(sc.db, dp.Edges, defaultDataflowBatchSize())
		return
	}
	if err := sc.saveAuditNodesMinimal(tx, dp.Nodes, defaultDataflowBatchSize()); err != nil {
		_ = tx.Rollback().Error
		log.Errorf("save minimal dataflow nodes failed: %v", err)
		_ = sc.saveAuditNodesMinimal(sc.db, dp.Nodes, defaultDataflowBatchSize())
		_ = sc.saveAuditEdgesMinimal(sc.db, dp.Edges, defaultDataflowBatchSize())
		return
	}
	if err := sc.saveAuditEdgesMinimal(tx, dp.Edges, defaultDataflowBatchSize()); err != nil {
		_ = tx.Rollback().Error
		log.Errorf("save minimal dataflow edges failed: %v", err)
		_ = sc.saveAuditEdgesMinimal(sc.db, dp.Edges, defaultDataflowBatchSize())
		return
	}
	if err := tx.Commit().Error; err != nil {
		_ = tx.Rollback().Error
		log.Errorf("save minimal dataflow commit failed: %v", err)
	}
}

func defaultDataflowBatchSize() int {
	// Allow tuning without code changes.
	// Example: SSA_STREAM_DATAFLOW_BATCH_SIZE=500
	if raw := os.Getenv("SSA_STREAM_DATAFLOW_BATCH_SIZE"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			if v > 5000 {
				return 5000
			}
			return v
		}
	}
	return 500
}

func (sc *SaveDataFlowCtx) saveAuditNodes(db *gorm.DB, nodes []*NodeInfo, batchSize int) error {
	if len(nodes) == 0 {
		return nil
	}

	toInsert := make([]*ssadb.AuditNode, 0, len(nodes))
	for _, n := range nodes {
		if n == nil {
			continue
		}
		// 存储过的不重复存储
		if sc.nodeMap[n.NodeID] != "" {
			continue
		}
		auditNode := n.ToAuditNode(sc.riskHash)
		// NodeID is generated in memory (ULID), so we can map before insert.
		sc.nodeMap[n.NodeID] = auditNode.NodeID
		toInsert = append(toInsert, auditNode)
	}

	if len(toInsert) == 0 {
		return nil
	}

	if err := insertAuditNodesMultiValues(db, toInsert, batchSize); err == nil {
		return nil
	}

	// Fallback to row-by-row to preserve best-effort behavior.
	for _, an := range toInsert {
		if an == nil {
			continue
		}
		if e := db.Create(an).Error; e != nil {
			log.Errorf("save audit node failed: %v", e)
		}
	}
	return nil
}

func (sc *SaveDataFlowCtx) saveAuditEdges(db *gorm.DB, edges []*EdgeInfo, batchSize int) error {
	if len(edges) == 0 {
		return nil
	}

	toInsert := make([]*ssadb.AuditEdge, 0, len(edges))
	for _, e := range edges {
		if e == nil {
			continue
		}
		auditEdge := e.ToAuditEdge(sc.nodeMap)
		if auditEdge == nil || auditEdge.FromNode == "" || auditEdge.ToNode == "" {
			continue
		}
		toInsert = append(toInsert, auditEdge)
	}
	if len(toInsert) == 0 {
		return nil
	}

	if err := insertAuditEdgesMultiValues(db, toInsert, batchSize); err == nil {
		return nil
	}

	// Fallback to row-by-row.
	for _, ae := range toInsert {
		if ae == nil {
			continue
		}
		if e := db.Create(ae).Error; e != nil {
			log.Errorf("save audit edge failed: %v", e)
		}
	}
	return nil
}

func (sc *SaveDataFlowCtx) saveAuditNodesMinimal(db *gorm.DB, nodes []*StreamMinimalNodeInfo, batchSize int) error {
	if len(nodes) == 0 {
		return nil
	}
	toInsert := make([]*ssadb.AuditNode, 0, len(nodes))
	for _, n := range nodes {
		if n == nil {
			continue
		}
		if sc.nodeMap[n.NodeID] != "" {
			continue
		}
		auditNode := n.ToAuditNode(sc.riskHash)
		if auditNode == nil {
			continue
		}
		sc.nodeMap[n.NodeID] = auditNode.NodeID
		toInsert = append(toInsert, auditNode)
	}
	if len(toInsert) == 0 {
		return nil
	}
	if err := insertAuditNodesMultiValues(db, toInsert, batchSize); err == nil {
		return nil
	}
	for _, an := range toInsert {
		if an == nil {
			continue
		}
		if e := db.Create(an).Error; e != nil {
			log.Errorf("save audit node failed: %v", e)
		}
	}
	return nil
}

func (sc *SaveDataFlowCtx) saveAuditEdgesMinimal(db *gorm.DB, edges []*StreamMinimalEdgeInfo, batchSize int) error {
	if len(edges) == 0 {
		return nil
	}
	toInsert := make([]*ssadb.AuditEdge, 0, len(edges))
	for _, e := range edges {
		if e == nil {
			continue
		}
		auditEdge := e.ToAuditEdge(sc.nodeMap)
		if auditEdge == nil || auditEdge.FromNode == "" || auditEdge.ToNode == "" {
			continue
		}
		toInsert = append(toInsert, auditEdge)
	}
	if len(toInsert) == 0 {
		return nil
	}
	if err := insertAuditEdgesMultiValues(db, toInsert, batchSize); err == nil {
		return nil
	}
	for _, ae := range toInsert {
		if ae == nil {
			continue
		}
		if e := db.Create(ae).Error; e != nil {
			log.Errorf("save audit edge failed: %v", e)
		}
	}
	return nil
}

func insertAuditNodesMultiValues(db *gorm.DB, items []*ssadb.AuditNode, batchSize int) error {
	if db == nil || len(items) == 0 {
		return nil
	}
	if batchSize <= 0 {
		batchSize = 500
	}

	table := db.NewScope(&ssadb.AuditNode{}).TableName()
	cols := []string{
		"created_at",
		"updated_at",
		"task_id",
		"result_id",
		"result_variable",
		"result_index",
		"risk_hash",
		"rule_name",
		"rule_title",
		"program_name",
		"is_entry_node",
		"ir_code_id",
		"node_id",
		"tmp_value",
		"tmp_value_file_hash",
		"tmp_start_offset",
		"tmp_end_offset",
		"verbose_name",
	}

	if raw := strings.TrimSpace(os.Getenv("SSA_STREAM_DATAFLOW_INSERT_MODE")); raw != "" {
		if raw == "copy" || strings.EqualFold(raw, "copy") {
			if err := insertAuditNodesCopyIn(db, table, cols, items, batchSize); err == nil {
				return nil
			} else {
				log.Warnf("dataflow nodes copy-in failed, fallback to multi-values: %v", err)
			}
		}
	}

	for i := 0; i < len(items); i += batchSize {
		j := i + batchSize
		if j > len(items) {
			j = len(items)
		}
		batch := items[i:j]
		var sb strings.Builder
		sb.Grow(256 + len(batch)*64)
		sb.WriteString("INSERT INTO ")
		sb.WriteString(table)
		sb.WriteString(" (")
		sb.WriteString(strings.Join(cols, ","))
		sb.WriteString(") VALUES ")

		args := make([]any, 0, len(batch)*16)
		for idx, n := range batch {
			if idx > 0 {
				sb.WriteByte(',')
			}
			// created_at, updated_at use NOW() for the batch.
			sb.WriteString("(NOW(),NOW(),?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)")
			args = append(args,
				n.TaskId,
				n.ResultId,
				n.ResultVariable,
				n.ResultIndex,
				n.RiskHash,
				n.RuleName,
				n.RuleTitle,
				n.ProgramName,
				n.IsEntryNode,
				n.IRCodeID,
				n.NodeID,
				n.TmpValue,
				n.TmpValueFileHash,
				n.TmpStartOffset,
				n.TmpEndOffset,
				n.VerboseName,
			)
		}
		if err := db.Exec(sb.String(), args...).Error; err != nil {
			return err
		}
	}
	return nil
}

func insertAuditEdgesMultiValues(db *gorm.DB, items []*ssadb.AuditEdge, batchSize int) error {
	if db == nil || len(items) == 0 {
		return nil
	}
	if batchSize <= 0 {
		batchSize = 500
	}

	table := db.NewScope(&ssadb.AuditEdge{}).TableName()
	cols := []string{
		"created_at",
		"updated_at",
		"task_id",
		"result_id",
		"from_node",
		"to_node",
		"program_name",
		"edge_type",
		"analysis_step",
		"analysis_label",
	}

	if raw := strings.TrimSpace(os.Getenv("SSA_STREAM_DATAFLOW_INSERT_MODE")); raw != "" {
		if raw == "copy" || strings.EqualFold(raw, "copy") {
			if err := insertAuditEdgesCopyIn(db, table, cols, items, batchSize); err == nil {
				return nil
			} else {
				log.Warnf("dataflow edges copy-in failed, fallback to multi-values: %v", err)
			}
		}
	}

	for i := 0; i < len(items); i += batchSize {
		j := i + batchSize
		if j > len(items) {
			j = len(items)
		}
		batch := items[i:j]
		var sb strings.Builder
		sb.Grow(256 + len(batch)*48)
		sb.WriteString("INSERT INTO ")
		sb.WriteString(table)
		sb.WriteString(" (")
		sb.WriteString(strings.Join(cols, ","))
		sb.WriteString(") VALUES ")

		args := make([]any, 0, len(batch)*8)
		for idx, e := range batch {
			if idx > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString("(NOW(),NOW(),?,?,?,?,?,?,?,?)")
			args = append(args,
				e.TaskId,
				e.ResultId,
				e.FromNode,
				e.ToNode,
				e.ProgramName,
				string(e.EdgeType),
				e.AnalysisStep,
				e.AnalysisLabel,
			)
		}
		if err := db.Exec(sb.String(), args...).Error; err != nil {
			return err
		}
	}
	return nil
}

func insertAuditNodesCopyIn(db *gorm.DB, table string, cols []string, items []*ssadb.AuditNode, batchSize int) error {
	if db == nil || len(items) == 0 {
		return nil
	}
	if batchSize <= 0 {
		batchSize = 500
	}
	common := db.CommonDB()
	if common == nil {
		return utils.Errorf("nil CommonDB")
	}
	// COPY is significantly faster than multi-values INSERT for large batches.
	for i := 0; i < len(items); i += batchSize {
		j := i + batchSize
		if j > len(items) {
			j = len(items)
		}
		batch := items[i:j]
		stmt, err := common.Prepare(pq.CopyIn(table, cols...))
		if err != nil {
			return err
		}
		now := time.Now()
		for _, n := range batch {
			if n == nil {
				continue
			}
			if _, err := stmt.Exec(
				now,
				now,
				n.TaskId,
				n.ResultId,
				n.ResultVariable,
				n.ResultIndex,
				n.RiskHash,
				n.RuleName,
				n.RuleTitle,
				n.ProgramName,
				n.IsEntryNode,
				n.IRCodeID,
				n.NodeID,
				n.TmpValue,
				n.TmpValueFileHash,
				n.TmpStartOffset,
				n.TmpEndOffset,
				n.VerboseName,
			); err != nil {
				_ = stmt.Close()
				return err
			}
		}
		if _, err := stmt.Exec(); err != nil {
			_ = stmt.Close()
			return err
		}
		if err := stmt.Close(); err != nil {
			return err
		}
	}
	return nil
}

func insertAuditEdgesCopyIn(db *gorm.DB, table string, cols []string, items []*ssadb.AuditEdge, batchSize int) error {
	if db == nil || len(items) == 0 {
		return nil
	}
	if batchSize <= 0 {
		batchSize = 500
	}
	common := db.CommonDB()
	if common == nil {
		return utils.Errorf("nil CommonDB")
	}
	for i := 0; i < len(items); i += batchSize {
		j := i + batchSize
		if j > len(items) {
			j = len(items)
		}
		batch := items[i:j]
		stmt, err := common.Prepare(pq.CopyIn(table, cols...))
		if err != nil {
			return err
		}
		now := time.Now()
		for _, e := range batch {
			if e == nil {
				continue
			}
			if _, err := stmt.Exec(
				now,
				now,
				e.TaskId,
				e.ResultId,
				e.FromNode,
				e.ToNode,
				e.ProgramName,
				string(e.EdgeType),
				e.AnalysisStep,
				e.AnalysisLabel,
			); err != nil {
				_ = stmt.Close()
				return err
			}
		}
		if _, err := stmt.Exec(); err != nil {
			_ = stmt.Close()
			return err
		}
		if err := stmt.Close(); err != nil {
			return err
		}
	}
	return nil
}
