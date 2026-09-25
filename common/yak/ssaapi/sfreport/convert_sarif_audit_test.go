package sfreport

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func TestSarifPersistedPredecessorDeduplication(t *testing.T) {
	db := ssadb.GetDB()
	name := uuid.NewString()
	editor := memedit.NewMemEditor("first\nsecond\nthird\nfourth\n")
	editor.SetUrl("/" + name + "/test.yak")
	source := ssadb.MarshalFile(editor)
	require.NoError(t, source.Save(db))
	t.Cleanup(func() { db.Unscoped().Delete(source) })
	prog := ssaapi.NewTmpProgram(name)
	var nodes []*ssadb.AuditNode
	for i := 0; i < 4; i++ {
		node := ssadb.NewAuditNode()
		node.IRCodeID = -1
		node.TmpValue = "node"
		node.TmpValueFileHash = source.SourceCodeHash
		node.TmpStartOffset = i
		node.TmpEndOffset = i + 1
		require.NoError(t, db.Create(node).Error)
		nodes = append(nodes, node)
		t.Cleanup(func() { db.Unscoped().Delete(node) })
	}
	for _, pair := range [][2]int{{0, 1}, {0, 2}, {1, 3}, {2, 3}} {
		edge := nodes[pair[0]].CreatePredecessorEdge(name, nodes[pair[1]], 0, "test")
		require.NoError(t, db.Create(edge).Error)
		t.Cleanup(func() { db.Unscoped().Delete(edge) })
	}
	// Temporary audit nodes are reloaded into distinct Value wrappers. A
	// pointer-only visited set emits the shared fourth node twice.
	first := prog.NewValueFromAuditNode(db, nodes[3].NodeID)
	second := prog.NewValueFromAuditNode(db, nodes[3].NodeID)
	require.NotNil(t, first)
	require.NotSame(t, first, second)
	require.Equal(t, first.GetAuditNodeId(), second.GetAuditNodeId())
	root := prog.NewValueFromAuditNode(db, nodes[0].NodeID)
	require.NotNil(t, root.GetRange())
	ctx := NewSarifContext()
	ctx.CreateCodeFlowsFromPredecessor(root)
	require.Len(t, ctx.codeFlows, 1)
	require.Len(t, ctx.codeFlows[0].ThreadFlows[0].Locations, 4)

	cycle := nodes[3].CreatePredecessorEdge(name, nodes[0], 0, "cycle")
	require.NoError(t, db.Create(cycle).Error)
	t.Cleanup(func() { db.Unscoped().Delete(cycle) })
	ctx = NewSarifContext()
	ctx.CreateCodeFlowsFromPredecessor(prog.NewValueFromAuditNode(db, nodes[0].NodeID))
	require.Len(t, ctx.codeFlows, 1)
	require.Len(t, ctx.codeFlows[0].ThreadFlows[0].Locations, 4,
		"a persisted cycle must terminate even when every reload creates a new wrapper")
}
