package ssadb

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
)

func newAuditEdgeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&AuditEdge{}).Error)
	return db
}

func TestNativeAuditEdgeGetters(t *testing.T) {
	db := newAuditEdgeTestDB(t)
	from := "01HZZZZZZZZZZZZZZZZZZZZZZZ"

	predecessor := &AuditEdge{
		TaskId:        "task-1",
		ResultId:      7,
		FromNode:      from,
		ToNode:        "01HZZZZZZZZZZZZZZZZZZZZZZY",
		ProgramName:   "demo",
		EdgeType:      EdgeType_Predecessor,
		AnalysisStep:  3,
		AnalysisLabel: "call",
	}
	require.NoError(t, db.Create(predecessor).Error)
	require.NoError(t, db.Create(&AuditEdge{
		FromNode: from,
		ToNode:   "depend-target",
		EdgeType: EdgeType_DependsOn,
	}).Error)
	require.NoError(t, db.Create(&AuditEdge{
		FromNode: from,
		ToNode:   "effect-target",
		EdgeType: EdgeType_EffectsOn,
	}).Error)

	edges := GetPredecessorEdgeByFromID(db, from)
	require.Len(t, edges, 1)
	require.Equal(t, predecessor.ToNode, edges[0].ToNode)
	require.Equal(t, int64(3), edges[0].AnalysisStep)
	require.Equal(t, "call", edges[0].AnalysisLabel)
	require.Equal(t, "task-1", edges[0].TaskId)
	require.Equal(t, uint(7), edges[0].ResultId)
	require.Equal(t, "demo", edges[0].ProgramName)
	require.Equal(t, EdgeType_Predecessor, edges[0].EdgeType)

	require.Equal(t, []string{"depend-target"}, GetDependEdgeOnByFromNodeId(db, from))
	require.Equal(t, []string{"effect-target"}, GetEffectOnEdgeByFromNodeId(db, from))
}

func TestNativeAuditEdgeGettersSkipSoftDeleted(t *testing.T) {
	db := newAuditEdgeTestDB(t)
	from := "01HZZZZZZZZZZZZZZZZZZZZZZZ"
	edge := &AuditEdge{
		FromNode: from,
		ToNode:   "deleted-target",
		EdgeType: EdgeType_DependsOn,
	}
	require.NoError(t, db.Create(edge).Error)
	require.NoError(t, db.Delete(edge).Error)

	require.Empty(t, GetDependEdgeOnByFromNodeId(db, from))
	require.Empty(t, GetPredecessorEdgeByFromID(db, from))
	require.Empty(t, GetEffectOnEdgeByFromNodeId(db, from))
}

func TestNativeAuditEdgeGettersEmptyInput(t *testing.T) {
	db := newAuditEdgeTestDB(t)
	require.Nil(t, GetDependEdgeOnByFromNodeId(db, ""))
	require.Nil(t, GetPredecessorEdgeByFromID(db, ""))
	require.Nil(t, GetEffectOnEdgeByFromNodeId(db, "missing"))
}

func TestNativeAuditEdgeGettersReadNullableColumns(t *testing.T) {
	db := newAuditEdgeTestDB(t)
	from := "01HZZZZZZZZZZZZZZZZZZZZZZZ"
	edge := &AuditEdge{
		FromNode:        from,
		ToNode:          "nullable-target",
		EdgeType:        EdgeType_Predecessor,
		RiskFeatureHash: "placeholder",
		TaskId:          "placeholder",
		ProgramName:     "placeholder",
		AnalysisStep:    9,
		AnalysisLabel:   "placeholder",
	}
	require.NoError(t, db.Create(edge).Error)
	// Reports imported from older versions can leave these columns NULL. GORM
	// decodes NULL into the zero value; the native path must do the same
	// instead of aborting the scan and dropping every predecessor edge.
	require.NoError(t, db.Exec(
		`UPDATE "`+TableAuditEdges+`" SET risk_feature_hash = NULL, task_id = NULL, `+
			`program_name = NULL, analysis_step = NULL, analysis_label = NULL WHERE id = ?`,
		edge.ID,
	).Error)

	edges := GetPredecessorEdgeByFromID(db, from)
	require.Len(t, edges, 1)
	require.Equal(t, edge.ToNode, edges[0].ToNode)
	require.Empty(t, edges[0].RiskFeatureHash)
	require.Empty(t, edges[0].TaskId)
	require.Empty(t, edges[0].ProgramName)
	require.Zero(t, edges[0].AnalysisStep)
	require.Empty(t, edges[0].AnalysisLabel)
}
