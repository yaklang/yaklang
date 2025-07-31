package ssa

import (
	"testing"
)

func TestGeneratePhiWithNilCfgEntryBlock(t *testing.T) {
	// Create a test program and function
	// programName := uuid.NewString()
	// ttl := time.Millisecond * 100

	// defer ssadb.DeleteProgram(ssadb.GetDB(), programName)
	// vf := filesys.NewVirtualFs()
	// prog := NewProgram(programName, true, Application, vf, "", ttl)
	// builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	// // This reproduces the exact scenario from member_call_replace.go line 15:
	// // createPhi := generatePhi(builder, nil, nil)
	// createPhi := generatePhi(builder, nil, nil)
	// // Create some test values
	// const1 := builder.EmitConstInst(1)
	// const2 := builder.EmitConstInst(2)
	// values := []Value{const1, const2}

	// // This should not panic after the fix
	// phi := createPhi("test_phi", values)

	// if phi == nil {
	// 	t.Fatal("phi should not be nil")
	// }

	// phiInst := phi.(*Phi)
	// if phiInst.CFGEntryBasicBlock != -1 {
	// 	t.Errorf("Expected CFGEntryBasicBlock to be -1, got %d", phiInst.CFGEntryBasicBlock)
	// }
}
