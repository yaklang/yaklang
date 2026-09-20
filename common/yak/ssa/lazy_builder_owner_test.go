package ssa

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestLazyBuildCurrentProgramDoesNotTraverseLibraries(t *testing.T) {
	prog, _ := newTestBuilder(t)
	lib := prog.NewLibrary("child", []string{"child"})
	parentCalls, childCalls := 0, 0
	prog.fixImportCallback = append(prog.fixImportCallback, func() { parentCalls++ })
	lib.fixImportCallback = append(lib.fixImportCallback, func() { childCalls++ })
	prog.LazyBuildCurrentProgram()
	require.Equal(t, 1, parentCalls)
	require.Zero(t, childCalls)
	lib.LazyBuildCurrentProgram()
	require.Equal(t, 1, childCalls)
}
