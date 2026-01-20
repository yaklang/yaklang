package tests

import (
	"context"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/compiler"
)

func TestEmptyCompilation(t *testing.T) {
	// Manually construct a simple dummy program
	// In the future, we might want to use a real SSA builder or parser
	prog := &ssa.Program{
		Name: "test_prog_empty",
	}

	ctx := context.Background()
	c := compiler.NewCompiler(ctx, prog)
	if c == nil {
		t.Fatal("Failed to create compiler")
	}
	defer c.Dispose()

	// For Phase 1, we just verify that we can initialize the context
	// and get a valid (empty) module representation.
	ir := c.Compile()

	t.Logf("Generated IR:\n%s", ir)

	if ir == "" {
		t.Fatal("Generated IR is empty")
	}

	// Basic check to see if the module name is in the output
	// LLVM IR usually starts with "; ModuleID = '...'"
	// but exact string depends on version.
	// We can check if it's not empty, which we did.
}
