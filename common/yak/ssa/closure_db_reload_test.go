package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

// Regression tests for the frappe-project panics:
//
//  1. "lazy builder panic: nil pointer dereference" (closure.go:88):
//     a Function rebuilt from DB (compile-unit spill + lazy reload) has
//     builder == nil; AddSideEffect dereferenced f.builder.parentBuilder.
//
//  2. "replace value panic: assignment to entry in nil map":
//     CreateInstruction rebuilt Call/Function with nil maps
//     (SideEffectValue/Binding/FreeValues); later writes panicked.

func newTestEditor(t *testing.T, prog *Program) *memedit.MemEditor {
	t.Helper()
	editor := memedit.NewMemEditor("")
	editor.SetFileName("closure_reload_test.yak")
	prog.PushEditor(editor)
	return editor
}

func newTestBuilderFunc(t *testing.T, prog *Program, name string, parent *FunctionBuilder) (*Function, *FunctionBuilder) {
	t.Helper()
	f := NewFunctionWithType(name, nil)
	f.SetProgram(prog)
	b := NewBuilder(nil, f, parent)
	if parent == nil {
		// root builder needs an entry block with a scope to walk
		block := f.NewBasicBlock("entry")
		block.SetScope(NewScope(f, prog.GetProgramName()))
		b.CurrentBlock = block
	}
	return f, b
}

// TestFunction_AddSideEffect_NilBuilder_NoPanic verifies AddSideEffect on a
// function without a builder (DB-reloaded) records the side effect and binds
// to the original variable instead of panicking on f.builder.parentBuilder.
func TestFunction_AddSideEffect_NilBuilder_NoPanic(t *testing.T) {
	f := NewFunctionWithType("reloaded-func", nil)
	require.NotNil(t, f)
	require.Nil(t, f.builder, "NewFunctionWithType creates a function without builder")

	variable := NewVariable(1, "x", true, nil).(*Variable)
	v := NewUndefined("value")

	require.NotPanics(t, func() {
		f.AddSideEffect(variable, v)
	})

	require.Len(t, f.SideEffects, 1)
	se := f.SideEffects[0]
	require.Equal(t, "x", se.Name)
	require.Equal(t, v.GetId(), se.Modify)
	require.Same(t, variable, se.Variable,
		"without a builder, bind falls back to the original variable")
}

// TestFunction_AddSideEffect_WithBuilder_WalksParents verifies the nil guard
// did not change the compiler path: with a builder whose parent scope holds a
// same-name variable, AddSideEffect binds to the parent variable.
func TestFunction_AddSideEffect_WithBuilder_WalksParents(t *testing.T) {
	prog := NewTmpProgram("add-side-effect-parent-walk")
	newTestEditor(t, prog)

	// parent builder with a scope holding variable "x"
	_, parent := newTestBuilderFunc(t, prog, "parent-func", nil)
	parentVar := parent.CreateVariable("x")
	require.NotNil(t, parentVar)
	parent.AssignVariable(parentVar, NewUndefined("parent-x"))

	// child function whose builder chain points at parent
	childFunc, _ := newTestBuilderFunc(t, prog, "child-func", parent)
	require.NotNil(t, childFunc.builder)
	require.Equal(t, parent, childFunc.builder.parentBuilder)

	variable := NewVariable(1, "x", true, nil).(*Variable)
	v := NewUndefined("value")

	childFunc.AddSideEffect(variable, v)
	require.Len(t, childFunc.SideEffects, 1)
	se := childFunc.SideEffects[0]
	require.Same(t, parentVar, se.Variable,
		"with a builder, bind should resolve the parent-scope variable")
}

// TestCreateInstruction_CallMapsInitialized verifies the CreateInstruction fix:
// Call/Function rebuilt via CreateInstruction (the LazyInstruction reload
// path) must have non-nil maps, since ReplaceValue trees can still write to
// them after a compile-unit spill.
func TestCreateInstruction_CallMapsInitialized(t *testing.T) {
	t.Run("call", func(t *testing.T) {
		inst := CreateInstruction(SSAOpcodeCall)
		require.NotNil(t, inst)
		c, ok := ToCall(inst)
		require.True(t, ok)

		require.NotNil(t, c.Binding,
			"DB-reloaded Call must have non-nil Binding (HandleFreeValue writes it)")
		require.NotNil(t, c.SideEffectValue,
			"DB-reloaded Call must have non-nil SideEffectValue (handleSideEffect writes it)")

		// writing must not panic
		require.NotPanics(t, func() {
			c.Binding["x"] = 1
			c.SideEffectValue["x"] = 1
		})
	})

	t.Run("function", func(t *testing.T) {
		inst := CreateInstruction(SSAOpcodeFunction)
		require.NotNil(t, inst)
		f, ok := ToFunction(inst)
		require.True(t, ok)

		require.NotNil(t, f.FreeValues,
			"DB-reloaded Function must have non-nil FreeValues (BuildFreeValue writes it)")

		require.NotPanics(t, func() {
			f.FreeValues[&Variable{}] = 1
		})
	})
}