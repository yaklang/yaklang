package ssaapi

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

// TestGetPredecessorsUsesResultSnapshot proves the report path reads one
// result's edges once instead of one query per node.
func TestGetPredecessorsUsesResultSnapshot(t *testing.T) {
	db := ssadb.GetDB()
	require.NotNil(t, db)
	resultID := uint(time.Now().UnixNano() & 0x3fffffff)
	progName := "audit-edge-snapshot-query-count"
	prog := NewProgram(ssa.NewTmpProgram(progName), &Config{})

	const nodeCount = 6
	nodes := make([]*ssadb.AuditNode, nodeCount)
	values := make([]*Value, nodeCount)
	for i := range nodes {
		node := ssadb.NewAuditNode()
		node.ResultId = resultID
		node.ProgramName = progName
		node.IRCodeID = -1
		node.TmpValue = "tmp"
		require.NoError(t, db.Create(node).Error)
		nodes[i] = node
		t.Cleanup(func() { db.Unscoped().Delete(node) })
	}
	// nodes[0] -> nodes[1..5]
	for i := 1; i < nodeCount; i++ {
		edge := nodes[0].CreatePredecessorEdge(progName, nodes[i], int64(i), "step")
		require.NoError(t, db.Create(edge).Error)
		t.Cleanup(func() { db.Unscoped().Delete(edge) })
	}
	for i, node := range nodes {
		values[i] = prog.NewValueFromAuditNode(db, node.NodeID)
		require.NotNil(t, values[i])
		require.NotNil(t, values[i].auditNode)
	}

	// Semantic check through the public getter.
	require.Len(t, values[0].GetPredecessors(), nodeCount-1)

	readEdgeQueries := func() int64 {
		before := ssadb.SnapshotDBOpStats()
		for _, value := range values {
			_ = value.loadPredecessorTargets(value.auditNode)
			_ = value.loadDependOnTargets(value.auditNode)
			_ = value.loadEffectOnTargets(value.auditNode)
		}
		delta := ssadb.DeltaDBOpStats(before, ssadb.SnapshotDBOpStats())
		return delta.Ops[ssadb.DBOpQuery].Count
	}

	// Result-scoped path: the first node loads the result once, the rest hit
	// the Program cache.
	prog.resetAuditEdgeSnapshot()
	snapshotQueries := readEdgeQueries()
	require.LessOrEqual(t, snapshotQueries, int64(1),
		"snapshot should read the result once, got %d queries", snapshotQueries)

	// Per-node path, measured directly for the same graph.
	values[0].Predecessors = nil
	before := ssadb.SnapshotDBOpStats()
	for _, node := range nodes {
		_ = ssadb.GetPredecessorTargetsByFromID(db, node.NodeID)
		_ = ssadb.GetDependEdgeOnByFromNodeId(db, node.NodeID)
		_ = ssadb.GetEffectOnEdgeByFromNodeId(db, node.NodeID)
	}
	perNodeQueries := ssadb.DeltaDBOpStats(before, ssadb.SnapshotDBOpStats()).Ops[ssadb.DBOpQuery].Count
	require.GreaterOrEqual(t, perNodeQueries, int64(nodeCount*3),
		"per-node reader should query once per node and edge kind, got %d", perNodeQueries)

	t.Logf("edge reads for %d nodes: snapshot=%d queries, per-node=%d queries",
		nodeCount, snapshotQueries, perNodeQueries)
	require.Less(t, snapshotQueries, perNodeQueries)

	// A different result on the same Program reloads the slot.
	otherID := resultID + 1
	other := ssadb.NewAuditNode()
	other.ResultId = otherID
	other.ProgramName = progName
	other.IRCodeID = -1
	other.TmpValue = "tmp"
	require.NoError(t, db.Create(other).Error)
	t.Cleanup(func() { db.Unscoped().Delete(other) })
	otherEdge := other.CreatePredecessorEdge(progName, nodes[1], 1, "other")
	require.NoError(t, db.Create(otherEdge).Error)
	t.Cleanup(func() { db.Unscoped().Delete(otherEdge) })
	prog.resetAuditEdgeSnapshot()
	prog.getAuditEdgeSnapshot(otherID)
	require.Equal(t, otherID, prog.auditEdgeResultID)
}

// TestAuditEdgeSnapshotEmptyResultFallsBack keeps the per-node reader for a
// result whose graph was not stamped with its id (older writers used 0).
func TestAuditEdgeSnapshotEmptyResultFallsBack(t *testing.T) {
	db := ssadb.GetDB()
	require.NotNil(t, db)
	resultID := uint(time.Now().UnixNano() & 0x3fffffff)
	prog := NewTmpProgram("audit-edge-snapshot-empty")
	require.Nil(t, prog.getAuditEdgeSnapshot(resultID))

	node := ssadb.NewAuditNode()
	node.ResultId = resultID
	node.NodeID = "empty-" + node.NodeID
	node.IRCodeID = -1
	node.TmpValue = "tmp"
	require.NoError(t, db.Create(node).Error)
	t.Cleanup(func() { db.Unscoped().Delete(node) })

	value := prog.NewValueFromAuditNode(db, node.NodeID)
	require.NotNil(t, value)
	require.Len(t, value.GetPredecessors(), 0)
}
