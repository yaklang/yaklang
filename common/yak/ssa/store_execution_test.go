package ssa

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

func TestStoredBuilderExecutionDoesNotMutateDeclaration(t *testing.T) {
	prog := newDeferredBuildTestProgram(t, "builder-scope")
	decl := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	original := memedit.NewMemEditor("original")
	decl.SetEditor(original)
	stored := decl.StoreFunctionBuilder()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			view := stored.NewExecutionBuilder()
			view.SetEditor(memedit.NewMemEditor("private"))
			view.MarkedThisClassBlueprint = NewBlueprint("private")
		}()
	}
	wg.Wait()
	require.Same(t, original, decl.GetEditor())
	require.Nil(t, decl.MarkedThisClassBlueprint)
	require.Same(t, prog, decl.GetProgram())
}
