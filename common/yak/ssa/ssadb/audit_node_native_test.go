package ssadb

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
)

func newAuditNodeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&AuditNode{}).Error)
	return db
}

func TestNativeAuditNodeByIdRoundTrip(t *testing.T) {
	db := newAuditNodeTestDB(t)
	node := NewAuditNode()
	node.TaskId = "task-1"
	node.ResultId = 7
	node.ResultVariable = "sink"
	node.ResultIndex = 3
	node.RiskHash = "risk-hash"
	node.RiskFeatureHash = "feature-hash"
	node.RuleName = "rule"
	node.RuleTitle = "Rule Title"
	node.ProgramName = "prog"
	node.IsEntryNode = true
	node.IRCodeID = 42
	node.TmpValue = "user_input"
	node.TmpValueFileHash = "file-hash"
	node.TmpStartOffset = 11
	node.TmpEndOffset = 22
	node.VerboseName = "verbose"
	require.NoError(t, db.Create(node).Error)

	got, err := GetAuditNodeById(db, node.NodeID)
	require.NoError(t, err)
	require.Equal(t, node.ID, got.ID)
	require.Equal(t, "task-1", got.TaskId)
	require.Equal(t, uint(7), got.ResultId)
	require.Equal(t, "sink", got.ResultVariable)
	require.Equal(t, uint(3), got.ResultIndex)
	require.Equal(t, "risk-hash", got.RiskHash)
	require.Equal(t, "feature-hash", got.RiskFeatureHash)
	require.Equal(t, "rule", got.RuleName)
	require.Equal(t, "Rule Title", got.RuleTitle)
	require.Equal(t, "prog", got.ProgramName)
	require.True(t, got.IsEntryNode)
	require.Equal(t, int64(42), got.IRCodeID)
	require.Equal(t, node.NodeID, got.NodeID)
	require.Equal(t, "user_input", got.TmpValue)
	require.Equal(t, "file-hash", got.TmpValueFileHash)
	require.Equal(t, 11, got.TmpStartOffset)
	require.Equal(t, 22, got.TmpEndOffset)
	require.Equal(t, "verbose", got.VerboseName)
}

func TestNativeAuditNodeByIdNullableAndSoftDelete(t *testing.T) {
	db := newAuditNodeTestDB(t)
	node := NewAuditNode()
	node.TaskId = "placeholder"
	node.ProgramName = "placeholder"
	require.NoError(t, db.Create(node).Error)
	require.NoError(t, db.Exec(
		`UPDATE "`+TableAuditNodes+`" SET task_id = NULL, program_name = NULL, `+
			`result_id = NULL, is_entry_node = NULL, ir_code_id = NULL WHERE id = ?`,
		node.ID,
	).Error)

	got, err := GetAuditNodeById(db, node.NodeID)
	require.NoError(t, err)
	require.Empty(t, got.TaskId)
	require.Empty(t, got.ProgramName)
	require.Zero(t, got.ResultId)
	require.False(t, got.IsEntryNode)
	require.Zero(t, got.IRCodeID)

	require.NoError(t, db.Delete(node).Error)
	_, err = GetAuditNodeById(db, node.NodeID)
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound))
}

func TestNativeAuditNodeByIdMissing(t *testing.T) {
	db := newAuditNodeTestDB(t)
	_, err := GetAuditNodeById(db, "missing")
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound))
}

func TestNativeAuditNodesByIds(t *testing.T) {
	db := newAuditNodeTestDB(t)
	first := NewAuditNode()
	first.ProgramName = "p"
	first.TmpValue = "one"
	second := NewAuditNode()
	second.ProgramName = "p"
	second.TmpValue = "two"
	gone := NewAuditNode()
	gone.ProgramName = "p"
	require.NoError(t, db.Create(first).Error)
	require.NoError(t, db.Create(second).Error)
	require.NoError(t, db.Create(gone).Error)
	require.NoError(t, db.Delete(gone).Error)

	nodes, err := GetAuditNodesByIds(db, []string{first.NodeID, second.NodeID, gone.NodeID})
	require.NoError(t, err)
	require.Len(t, nodes, 2)
	byID := make(map[string]*AuditNode, len(nodes))
	for _, node := range nodes {
		byID[node.NodeID] = node
	}
	require.Equal(t, "one", byID[first.NodeID].TmpValue)
	require.Equal(t, "two", byID[second.NodeID].TmpValue)
	require.NotContains(t, byID, gone.NodeID)
}
