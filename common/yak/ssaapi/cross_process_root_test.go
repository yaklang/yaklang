package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCrossProcessRollbackKeepsRootActive(t *testing.T) {
	manager := newAnalysisManager()
	root, ok := manager.getCurrentIntraProcess()
	require.True(t, ok)
	restore := manager.rollbackCrossProcess()
	current, ok := manager.getCurrentIntraProcess()
	require.True(t, ok, "root analysis must stay available during rollback")
	require.Same(t, root, current)
	restore()
	require.Equal(t, 1, manager.crossProcessStack.Len())
}

func TestCrossProcessRollbackRestoresNestedFrame(t *testing.T) {
	manager := newAnalysisManager()
	inner := newIntraProcess()
	manager.crossProcessStack.Push("callee")
	manager.crossProcessMap.Set("callee", inner)
	restore := manager.rollbackCrossProcess()
	require.Equal(t, emptyStackHash, manager.crossProcessStack.Peek())
	_, ok := manager.getCurrentIntraProcess()
	require.True(t, ok)
	restore()
	current, ok := manager.getCurrentIntraProcess()
	require.True(t, ok)
	require.Same(t, inner, current)
}
