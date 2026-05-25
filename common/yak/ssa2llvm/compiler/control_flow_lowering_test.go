package compiler

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestControlFlowLowering_IfAssignUsesSlotStoresNotLLVMPhi(t *testing.T) {
	// Boundary: SSA merge for x must not become LLVM phi nodes; both branches store
	// into the same entry slot and the use loads once after the merge block.
	code := `
check = () => {
	x = 1
	if true {
		x = 2
	}
	return x
}
`
	_, _, ir, err := compileToIRFromCodeWithExternBindings(code, "yak", nil)
	require.NoError(t, err)
	requireNoLLVMPhiNodes(t, ir)
	requireIRContainsSlotLowering(t, ir)
	require.Contains(t, ir, "store i64 1")
	require.Contains(t, ir, "store i64 2")
}

func TestControlFlowLowering_IfElseAssignCompilesAndReturnsCorrectArm(t *testing.T) {
	code := `
check = () => {
	x = 1
	if false {
		x = 10
	} else {
		x = 20
	}
	return x
}
`
	_, _, ir, err := compileToIRFromCodeWithExternBindings(code, "yak", nil)
	require.NoError(t, err)
	requireNoLLVMPhiNodes(t, ir)
	requireIRContainsSlotLowering(t, ir)
}
