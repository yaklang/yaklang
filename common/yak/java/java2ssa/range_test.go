package java2ssa

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func TestSetRangeRestoresOwningBuilderAfterSwitch(t *testing.T) {
	const source = "class A {}"
	ast, err := Frontend(source)
	require.NoError(t, err)

	prog := ssa.NewTmpProgram("java-range-owner")
	owner := prog.GetAndCreateFunctionBuilder("", string(ssa.MainFunctionName))
	owner.SetEditor(prog.CreateEditor([]byte(source), "A.java", false))
	y := &singleFileBuilder{FunctionBuilder: owner}

	restoreOuter := y.SetRange(ast)
	outer := owner.CurrentRange
	require.NotNil(t, outer)
	restoreInner := y.SetRange(ast)
	require.NotNil(t, owner.CurrentRange)

	// A nested lazy build may change the frontend's current builder before its
	// deferred range restoration runs. The captured owner must still be valid.
	y.FunctionBuilder = nil
	require.NotPanics(t, restoreInner)
	require.Same(t, outer, owner.CurrentRange)
	require.NotPanics(t, restoreOuter)
	require.Nil(t, owner.CurrentRange)
}

func TestLazyFrontendForksDoNotShareMutableState(t *testing.T) {
	program := ssa.NewTmpProgram("java-lazy-fork")
	owner := program.GetAndCreateFunctionBuilder("", string(ssa.MainFunctionName))
	original := &singleFileBuilder{
		FunctionBuilder:   owner,
		constMap:          map[string]ssa.Value{"original": nil},
		fullTypeNameMap:   map[string][]string{"name": {"original"}},
		allImportPkgSlice: [][]string{{"java", "lang"}},
		selfPkgPath:       []string{"package"},
	}
	first := original.forkForLazyBuild()
	second := original.forkForLazyBuild()
	var wg sync.WaitGroup
	for _, fork := range []*singleFileBuilder{first, second} {
		wg.Add(1)
		go func(fork *singleFileBuilder) {
			defer wg.Done()
			fork.FunctionBuilder = nil
			fork.constMap["new"] = nil
			fork.fullTypeNameMap["name"][0] = "changed"
			fork.allImportPkgSlice[0][0] = "changed"
			fork.selfPkgPath[0] = "changed"
		}(fork)
	}
	wg.Wait()
	require.Same(t, owner, original.FunctionBuilder)
	require.NotContains(t, original.constMap, "new")
	require.Equal(t, "original", original.fullTypeNameMap["name"][0])
	require.Equal(t, "java", original.allImportPkgSlice[0][0])
	require.Equal(t, "package", original.selfPkgPath[0])
}
