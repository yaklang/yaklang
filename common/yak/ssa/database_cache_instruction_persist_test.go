package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func TestInstructionPersistedInDB_lazySnapshot(t *testing.T) {
	lz := &LazyInstruction{
		id:     42,
		ir:     &ssadb.IrCode{CodeID: 42, Opcode: 1},
		Modify: false,
	}
	require.True(t, instructionPersistedInDB(lz))

	lz.Modify = true
	require.False(t, instructionPersistedInDB(lz))

	lz.Modify = false
	lz.ir.Opcode = 0
	require.False(t, instructionPersistedInDB(lz))
}

func TestInstructionPersistedInDB_nonLazy(t *testing.T) {
	prog := NewTmpProgram("persist-test")
	fn := prog.NewFunction("main")
	require.False(t, instructionPersistedInDB(fn))
}
