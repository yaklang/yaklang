package ssa

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func newResidentDeltaProgram(t *testing.T) *Program {
	t.Helper()
	name := uuid.NewString()
	t.Cleanup(func() { ssadb.DeleteProgram(ssadb.GetDB(), name) })
	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(name))
	require.NoError(t, err)
	cfg.SetCompileProjectBytes(fastPathProjectByteThreshold / 2)
	prog := NewProgram(cfg, ProgramCacheDBWrite, Application, filesys.NewVirtualFs(), "", 1)
	require.Equal(t, "resident-fast-path", prog.Cache.InstructionCacheMode())
	return prog
}

func TestFastPathFinalFlushSkipsUnchangedInstructions(t *testing.T) {
	prog := newResidentDeltaProgram(t)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	inst := builder.EmitUndefined("unchanged")
	builder.Finish()
	store := prog.Cache.instructions
	require.NoError(t, store.residentCache.Flush(false))
	before := store.PersistenceStats()
	require.Greater(t, before.WriteOperations, int64(0))

	// Reject a real SQL UPDATE, rather than trusting only the save counters.
	trigger := "reject_unchanged_ir_" + strings.ReplaceAll(prog.Name, "-", "_")
	db := ssadb.GetDB()
	require.NoError(t, db.Exec(fmt.Sprintf(`CREATE TRIGGER %s BEFORE UPDATE ON ir_codes
		WHEN OLD.program_name = '%s' AND OLD.code_id = %d
		BEGIN SELECT RAISE(ABORT, 'unchanged IR must not be updated'); END`, trigger, prog.Name, inst.GetId())).Error)
	t.Cleanup(func() { require.NoError(t, db.Exec("DROP TRIGGER IF EXISTS "+trigger).Error) })

	require.NoError(t, store.Close(nil))
	require.NoError(t, store.Close(nil))
	require.Equal(t, before.WriteOperations, store.PersistenceStats().WriteOperations,
		"unchanged snapshots must not issue any additional database writes")
	require.False(t, prog.Cache.hasResidentInstruction(inst.GetId()), "saved anonymous values should be released")
	loaded := prog.Cache.GetInstruction(inst.GetId())
	require.NotNil(t, loaded)
	require.Equal(t, inst.GetId(), loaded.GetId())
	require.Equal(t, inst.GetOpcode(), loaded.GetOpcode())
	require.Equal(t, inst.GetName(), loaded.GetName())
	require.Equal(t, before.WriteOperations, store.PersistenceStats().WriteOperations, "clean reload must not requeue a database write")
}

func TestFastPathFinalFlushPersistsChangedAndNewInstructions(t *testing.T) {
	prog := newResidentDeltaProgram(t)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	left := builder.EmitUndefined("left")
	right := builder.EmitUndefined("right")
	builder.EmitBinOp(OpAdd, left, right)
	otherUser := builder.EmitBinOp(OpAdd, right, right)
	left.SetExtern(true)
	builder.Finish()
	store := prog.Cache.instructions
	require.NoError(t, store.residentCache.Flush(false))

	// Exercise zero-valued flags and an in-place slice edit without Set/MarkDirty.
	left.SetExtern(false)
	require.Len(t, left.userList, 1)
	left.userList[0] = otherUser.GetId()
	lateBuilder := prog.GetAndCreateFunctionBuilder("", "late")
	late := lateBuilder.EmitUndefined("new_after_flush")
	lateBuilder.Finish()
	require.NoError(t, store.Close(nil))

	var got ssadb.IrCode
	require.NoError(t, ssadb.GetDB().Where("program_name = ? AND code_id = ?", prog.Name, left.GetId()).First(&got).Error)
	require.False(t, got.IsExternal)
	require.Equal(t, ssadb.Int64Slice{otherUser.GetId()}, got.Users)
	require.NotNil(t, ssadb.GetIrCodeItemById(ssadb.GetDB(), prog.Name, late.GetId()))
	before := store.PersistenceStats().WriteOperations
	require.NoError(t, store.Close(nil))
	require.Equal(t, before, store.PersistenceStats().WriteOperations,
		"a successfully saved new snapshot must be skipped on repeated final flush")
}

func TestFastPathFinalFlushRetriesFailedChanges(t *testing.T) {
	prog := newResidentDeltaProgram(t)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	inst := builder.EmitUndefined("retry")
	builder.Finish()
	store := prog.Cache.instructions
	require.NoError(t, store.residentCache.Flush(false))
	before := store.PersistenceStats().WriteOperations
	inst.SetExtern(true)

	trigger := "reject_changed_ir_" + strings.ReplaceAll(prog.Name, "-", "_")
	db := ssadb.GetDB()
	require.NoError(t, db.Exec(fmt.Sprintf(`CREATE TRIGGER %s BEFORE UPDATE ON ir_codes
		WHEN NEW.program_name = '%s' AND NEW.code_id = %d AND NEW.is_external = 1
		BEGIN SELECT RAISE(ABORT, 'reject changed IR'); END`, trigger, prog.Name, inst.GetId())).Error)
	t.Cleanup(func() { require.NoError(t, db.Exec("DROP TRIGGER IF EXISTS "+trigger).Error) })
	require.ErrorContains(t, store.Close(nil), "reject changed IR")
	require.Equal(t, before, store.PersistenceStats().WriteOperations)
	var got ssadb.IrCode
	require.NoError(t, db.Where("program_name = ? AND code_id = ?", prog.Name, inst.GetId()).First(&got).Error)
	require.False(t, got.IsExternal)

	require.NoError(t, db.Exec("DROP TRIGGER "+trigger).Error)
	require.NoError(t, store.Close(nil))
	got = ssadb.IrCode{}
	require.NoError(t, db.Where("program_name = ? AND code_id = ?", prog.Name, inst.GetId()).First(&got).Error)
	require.True(t, got.IsExternal, "a failed save must not advance the persisted fingerprint")
	after := store.PersistenceStats().WriteOperations
	require.NoError(t, store.Close(nil))
	require.Equal(t, after, store.PersistenceStats().WriteOperations)
}

func TestResidentIRFingerprintUsesPersistedStrings(t *testing.T) {
	raw := &ssadb.IrCode{Name: "name\xff", Variable: ssadb.StringSlice{"var\x00"}}
	canonical := &ssadb.IrCode{Name: "name", Variable: ssadb.StringSlice{"var"}}
	rawFingerprint, err := irCodePersistenceFingerprint(raw)
	require.NoError(t, err)
	canonicalFingerprint, err := irCodePersistenceFingerprint(canonical)
	require.NoError(t, err)
	require.Equal(t, canonicalFingerprint, rawFingerprint,
		"equivalent payloads after the database write hook must not cause another UPDATE")
	require.Equal(t, "name\xff", raw.Name)
	require.Equal(t, ssadb.StringSlice{"var\x00"}, raw.Variable)

	literalReplacement := &ssadb.IrCode{Name: "name\ufffd", Variable: canonical.Variable}
	literalFingerprint, err := irCodePersistenceFingerprint(literalReplacement)
	require.NoError(t, err)
	require.NotEqual(t, rawFingerprint, literalFingerprint,
		"JSON replaces invalid UTF-8 with U+FFFD, but the database drops it; those writes differ")
}

func TestResidentIRFingerprintIncludesRelationPairs(t *testing.T) {
	for _, field := range []string{"members", "member_pairs", "owner_pairs"} {
		t.Run(field, func(t *testing.T) {
			makeRow := func(value int64) *ssadb.IrCode {
				var pairs ssadb.Int64Map
				pairs.Append(7, value)
				ir := &ssadb.IrCode{}
				switch field {
				case "members":
					ir.ObjectMembers = pairs
				case "member_pairs":
					ir.ObjectMemberPairs = pairs
				case "owner_pairs":
					ir.ObjectOwnerPairs = pairs
				}
				return ir
			}
			before, err := irCodePersistenceFingerprint(makeRow(11))
			require.NoError(t, err)
			after, err := irCodePersistenceFingerprint(makeRow(19))
			require.NoError(t, err)
			require.NotEqual(t, before, after,
				"Int64Map items have private fields; their SQL values, not empty JSON objects, must be compared")
		})
	}
}

func TestResidentIRFingerprintTreatsEmptySQLSlicesEqually(t *testing.T) {
	before, err := irCodePersistenceFingerprint(&ssadb.IrCode{})
	require.NoError(t, err)
	after, err := irCodePersistenceFingerprint(&ssadb.IrCode{
		Users: ssadb.Int64Slice{}, Variable: ssadb.StringSlice{}, ObjectMemberPairs: ssadb.Int64Map{},
	})
	require.NoError(t, err)
	require.Equal(t, before, after, "nil and empty custom slices both persist as an empty SQL string")
}
