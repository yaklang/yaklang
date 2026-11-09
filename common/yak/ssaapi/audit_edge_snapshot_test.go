package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func TestBuildAuditEdgeSnapshot(t *testing.T) {
	snapshot := buildAuditEdgeSnapshot([]ssadb.AuditEdgeTarget{
		{FromNode: "a", ToNode: "pred", EdgeType: ssadb.EdgeType_Predecessor, AnalysisStep: 3, AnalysisLabel: "call"},
		{FromNode: "a", ToNode: "dep", EdgeType: ssadb.EdgeType_DependsOn},
		{FromNode: "a", ToNode: "effect", EdgeType: ssadb.EdgeType_EffectsOn},
		{FromNode: "b", ToNode: "pred2", EdgeType: ssadb.EdgeType_Predecessor},
		{FromNode: "", ToNode: "ignored", EdgeType: ssadb.EdgeType_DependsOn},
		{FromNode: "c", ToNode: "", EdgeType: ssadb.EdgeType_DependsOn},
	})

	require.Equal(t, 6, snapshot.total)

	preds, ok := snapshot.predecessorsFor("a")
	require.True(t, ok)
	require.Len(t, preds, 1)
	require.Equal(t, "pred", preds[0].ToNode)
	require.Equal(t, int64(3), preds[0].Step)
	require.Equal(t, "call", preds[0].Label)

	deps, ok := snapshot.dependsOnFor("a")
	require.True(t, ok)
	require.Equal(t, []string{"dep"}, deps)

	effects, ok := snapshot.effectsOnFor("a")
	require.True(t, ok)
	require.Equal(t, []string{"effect"}, effects)

	missing, ok := snapshot.predecessorsFor("missing")
	require.True(t, ok)
	require.Empty(t, missing)
}

func TestAuditEdgeSnapshotEmptyIsNotAuthoritative(t *testing.T) {
	var snapshot *auditEdgeSnapshot
	_, ok := snapshot.predecessorsFor("a")
	require.False(t, ok)
	_, ok = snapshot.dependsOnFor("a")
	require.False(t, ok)
	_, ok = snapshot.effectsOnFor("a")
	require.False(t, ok)

	empty := buildAuditEdgeSnapshot(nil)
	emptyPreds, ok := empty.predecessorsFor("a")
	require.True(t, ok)
	require.Empty(t, emptyPreds)
}
