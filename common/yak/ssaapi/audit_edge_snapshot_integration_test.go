package ssaapi

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func TestProgramAuditEdgeSnapshot(t *testing.T) {
	db := ssadb.GetDB()
	resultID := uint(time.Now().UnixNano() & 0x7fffffff)
	prog := NewTmpProgram("audit-edge-snapshot-test")

	node1 := ssadb.NewAuditNode()
	node1.ResultId = resultID
	node2 := ssadb.NewAuditNode()
	node2.ResultId = resultID
	node3 := ssadb.NewAuditNode()
	node3.ResultId = resultID
	for _, node := range []*ssadb.AuditNode{node1, node2, node3} {
		require.NoError(t, db.Create(node).Error)
		t.Cleanup(func() { db.Unscoped().Delete(node) })
	}

	predecessor := node1.CreatePredecessorEdge(prog.GetProgramName(), node2, 3, "call")
	depend := node1.CreateDependsOnEdge(prog.GetProgramName(), node3)
	effect := node2.CreateEffectsOnEdge(prog.GetProgramName(), node3)
	for _, edge := range []*ssadb.AuditEdge{predecessor, depend, effect} {
		require.NoError(t, db.Create(edge).Error)
		t.Cleanup(func() { db.Unscoped().Delete(edge) })
	}

	snapshot := prog.getAuditEdgeSnapshot(resultID)
	require.NotNil(t, snapshot)
	preds, ok := snapshot.predecessorsFor(node1.NodeID)
	require.True(t, ok)
	require.Len(t, preds, 1)
	require.Equal(t, node2.NodeID, preds[0].ToNode)
	require.Equal(t, int64(3), preds[0].Step)
	require.Equal(t, "call", preds[0].Label)

	deps, ok := snapshot.dependsOnFor(node1.NodeID)
	require.True(t, ok)
	require.Equal(t, []string{node3.NodeID}, deps)

	effects, ok := snapshot.effectsOnFor(node2.NodeID)
	require.True(t, ok)
	require.Equal(t, []string{node3.NodeID}, effects)

	// The snapshot is per result and cached: an edge added afterwards is not
	// visible until a new Program/slot reloads it. Report conversion must not
	// interleave writes with a cached read.
	late := node3.CreatePredecessorEdge(prog.GetProgramName(), node1, 0, "late")
	require.NoError(t, db.Create(late).Error)
	t.Cleanup(func() { db.Unscoped().Delete(late) })
	again := prog.getAuditEdgeSnapshot(resultID)
	require.Same(t, snapshot, again)
	preds, ok = again.predecessorsFor(node1.NodeID)
	require.True(t, ok)
	require.Len(t, preds, 1)
}
