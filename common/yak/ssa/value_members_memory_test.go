package ssa

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func TestMemoryMemberResolutionDoesNotReadPersistedProgram(t *testing.T) {
	name := uuid.NewString()
	db := ssadb.GetDB()
	t.Cleanup(func() { ssadb.DeleteProgram(db, name) })
	const missingID = int64(99999)
	record := &ssadb.IrCode{ProgramName: name, CodeID: missingID, Opcode: int64(SSAOpcodeConstInst), String: "unrelated-persisted-value"}
	require.NoError(t, db.Create(record).Error)
	require.NotNil(t, ssadb.GetIrCodeByIdFast(db, name, missingID))
	prog := newMemberRelationsTestProgram(t, name, ProgramCacheMemory, nil)
	value := NewValue()
	value.SetProgram(prog)
	resolved, ok := value.resolveLinkedValue(missingID)
	require.False(t, ok, "a missing memory-mode relation must not use a persisted program with the same name")
	require.Nil(t, resolved)
}
