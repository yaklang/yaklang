package ssaapi

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

// TestBatchSaveAuditNodesAndEdgesIsEquivalencePreserving guards the batched
// audit writers: they replace a per-row INSERT loop, so every column of every
// row must still land in the table, including rows that spill past one
// multi-row INSERT batch.
func TestBatchSaveAuditNodesAndEdgesIsEquivalencePreserving(t *testing.T) {
	db, err := consts.GetTempSSADataBase()
	require.NoError(t, err)
	// Restore the previous global SSA DB when the test ends: other tests in
	// this package read the process-wide handle, and leaving the temp DB
	// installed made their result queries see an unexpected database.
	prev := ssadb.GetDB()
	ssadb.SetDB(db)
	t.Cleanup(func() {
		ssadb.SetDB(prev)
		_ = db.Close()
	})

	const rows = 205 // Cross multiple CreateInBatches chunks for both models.
	nodes := make([]*ssadb.AuditNode, 0, rows)
	for i := 0; i < rows; i++ {
		node := ssadb.NewAuditNode()
		node.TaskId = "task-batch"
		node.ResultId = uint(i)
		node.ResultVariable = "sink"
		node.ResultIndex = uint(i)
		node.RiskHash = "risk"
		node.RuleName = "rule"
		node.RuleTitle = "Rule"
		node.ProgramName = "prog"
		node.IsEntryNode = i%2 == 0
		node.IRCodeID = int64(1000 + i)
		node.TmpValue = "value"
		node.TmpValueFileHash = "filehash"
		node.TmpStartOffset = i
		node.TmpEndOffset = i + 1
		node.VerboseName = "verbose"
		nodes = append(nodes, node)
	}
	require.NoError(t, batchSaveAuditNodes(db, nodes))

	edges := make([]*ssadb.AuditEdge, 0, rows)
	for i := 0; i < rows; i++ {
		edges = append(edges, &ssadb.AuditEdge{
			TaskId:        "task-batch",
			ResultId:      uint(i),
			FromNode:      nodes[i].NodeID,
			ToNode:        nodes[(i+1)%rows].NodeID,
			ProgramName:   "prog",
			EdgeType:      ssadb.EdgeType_Predecessor,
			AnalysisStep:  int64(i),
			AnalysisLabel: "label",
		})
	}
	require.NoError(t, batchSaveAuditEdges(db, edges))

	var nodeCount int64
	require.NoError(t, db.Model(&ssadb.AuditNode{}).Where("task_id = ?", "task-batch").Count(&nodeCount).Error)
	require.EqualValues(t, rows, nodeCount)

	var edgeCount int64
	require.NoError(t, db.Model(&ssadb.AuditEdge{}).Where("task_id = ?", "task-batch").Count(&edgeCount).Error)
	require.EqualValues(t, rows, edgeCount)

	var node ssadb.AuditNode
	require.NoError(t, db.Where("task_id = ? AND result_id = ?", "task-batch", 7).First(&node).Error)
	require.Equal(t, "sink", node.ResultVariable)
	require.Equal(t, int64(1007), node.IRCodeID)
	require.Equal(t, 7, node.TmpStartOffset)
	require.Equal(t, 8, node.TmpEndOffset)
	require.Equal(t, "verbose", node.VerboseName)
	require.False(t, node.IsEntryNode, "row 7 must keep its non-entry flag")

	var edge ssadb.AuditEdge
	require.NoError(t, db.Where("task_id = ? AND result_id = ?", "task-batch", 7).First(&edge).Error)
	require.Equal(t, nodes[7].NodeID, edge.FromNode)
	require.Equal(t, nodes[8].NodeID, edge.ToNode)
	require.Equal(t, ssadb.EdgeType_Predecessor, edge.EdgeType)
	require.Equal(t, int64(7), edge.AnalysisStep)
	require.Equal(t, "label", edge.AnalysisLabel)
}

func TestBatchSaveAuditSources(t *testing.T) {
	db := ssadb.GetDB()
	program := uuid.NewString()
	t.Cleanup(func() {
		db.Where("program_name = ?", program).Delete(&ssadb.IrSource{})
	})

	require.NoError(t, saveAuditSources(db, nil))
	require.NoError(t, saveAuditSources(db, []*ssadb.IrSource{nil}))
	const count = riskBatchSize + 1
	sources := make([]*ssadb.IrSource, 0, count+1)
	sources = append(sources, nil)
	for i := 0; i < count; i++ {
		sources = append(sources, &ssadb.IrSource{
			ProgramName:    program,
			SourceCodeHash: fmt.Sprintf("%s-%d", program, i),
			FolderPath:     "src",
			FileName:       fmt.Sprintf("file-%d.yak", i),
			QuotedCode:     `"println(123)"`,
			IsBigFile:      i%2 == 0,
		})
	}
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return saveAuditSources(tx, sources)
	}))

	var n int64
	require.NoError(t, db.Model(&ssadb.IrSource{}).Where("program_name = ?", program).Count(&n).Error)
	require.EqualValues(t, count, n, "all source rows cross the batch boundary")
	var saved ssadb.IrSource
	require.NoError(t, db.Where("program_name = ? AND file_name = ?", program, "file-500.yak").First(&saved).Error)
	require.Equal(t, "/src", saved.FolderPath, "BeforeSave still normalizes every batch row")
	require.Equal(t, `"println(123)"`, saved.QuotedCode)
	require.True(t, saved.IsBigFile)

	rollback := utils.Error("roll back source batch")
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := saveAuditSources(tx, []*ssadb.IrSource{{
			ProgramName: program, SourceCodeHash: "rolled-back", FileName: "rolled-back.yak",
		}}); err != nil {
			return err
		}
		return rollback
	})
	require.ErrorIs(t, err, rollback)
	require.NoError(t, db.Model(&ssadb.IrSource{}).Where("program_name = ?", program).Count(&n).Error)
	require.EqualValues(t, count, n, "a source batch stays in its caller's transaction")
}
