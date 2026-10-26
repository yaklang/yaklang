package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

// ReleaseSettled may trim the resident name/id membership. It must not trim
// the index or offset dedup keys: those are the only record that a
// relationship reached the saver, and an old id can still gain a new name.
// ir_indices_v1 has no UNIQUE key (a forgotten key inserts a duplicate row)
// and ir_offsets has one (a forgotten key fails the insert).
func TestReleaseSettledDropsResidentMembershipOnly(t *testing.T) {
	prog := newResidentDeltaProgram(t)
	defer closeUnsavedFlushFixture(t, prog)()
	builder := prog.GetAndCreateFunctionBuilder("", "settled-index")
	oldValue := builder.EmitUndefined("old")
	store := prog.Cache.indexes
	store.AddVariable("old", oldValue)
	store.AddVariable("object.old", oldValue)
	store.AddClassInstance("OldClass", oldValue)
	store.AddConst(oldValue)
	oldOffset := ssadb.CreateOffset(memedit.NewRangeFromOffsets(memedit.NewMemEditor("old"), 0, 3), prog.Name)
	oldOffset.ValueID = oldValue.GetId()
	oldOffset.VariableName = "old"
	store.saveOffsetDedup(oldOffset)
	require.NoError(t, store.Flush())

	newer := builder.EmitUndefined("newer")
	store.AddVariable("newer", newer)
	store.AddClassInstance("NewClass", newer)
	newOffset := ssadb.CreateOffset(memedit.NewRangeFromOffsets(memedit.NewMemEditor("newer"), 0, 5), prog.Name)
	newOffset.ValueID = newer.GetId()
	newOffset.VariableName = "newer"
	store.saveOffsetDedup(newOffset)
	require.NoError(t, store.Flush())

	var programName string
	store.indexSaved.ForEach(func(key indexRelationshipKey, _ struct{}) bool {
		if key.value == oldValue.GetId() {
			programName = key.program
			return false
		}
		return true
	})
	require.NotEmpty(t, programName)
	indexBefore := store.indexSaved.Count()
	offsetBefore := store.offsetSaved.Count()
	residentBefore := store.residentKeys.Count()
	require.Greater(t, indexBefore, 0)
	require.Greater(t, offsetBefore, 0)
	require.Greater(t, residentBefore, 0)

	require.Zero(t, store.ReleaseSettled("other-program"), "another program must not be released")

	released := store.ReleaseSettled(programName)
	require.Greater(t, released, 0)
	require.Zero(t, store.residentKeys.Count(), "the resident membership of a released batch is redundant")
	require.Equal(t, indexBefore, store.indexSaved.Count(), "index dedup keys must survive a release")
	require.Equal(t, offsetBefore, store.offsetSaved.Count(), "offset dedup keys must survive a release")

	var rowsBefore int64
	require.NoError(t, ssadb.GetDB().Model(&ssadb.IrIndex{}).Where("program_name = ?", programName).Count(&rowsBefore).Error)

	// Re-registering the exact relationships that were already written must
	// still be deduped, otherwise ir_indices_v1 gains duplicate rows.
	store.AddVariable("old", oldValue)
	store.AddClassInstance("OldClass", oldValue)
	store.saveOffsetDedup(oldOffset)
	require.NoError(t, store.Flush())
	var rowsAfter int64
	require.NoError(t, ssadb.GetDB().Model(&ssadb.IrIndex{}).Where("program_name = ?", programName).Count(&rowsAfter).Error)
	require.Equal(t, rowsBefore, rowsAfter, "a repeated relationship must not be inserted twice")
	var offsetRows int64
	require.NoError(t, ssadb.GetDB().Model(&ssadb.IrOffset{}).Where("program_name = ?", programName).Count(&offsetRows).Error)
	require.Greater(t, offsetRows, int64(0), "the repeated offset must stay deduped instead of failing the UNIQUE insert")

	// An id below the released batch can still gain a name it never had. That
	// name needs both the resident lookup entry and its own row.
	store.AddVariable("late", oldValue)
	found := store.FindByVariableName(ssadb.NameMatch, "late", func(id int64) Instruction {
		if id == oldValue.GetId() {
			return oldValue
		}
		return nil
	})
	require.Len(t, found, 1, "a name registered after the release must stay findable")
	require.NoError(t, store.Flush())
	found = store.FindByVariableName(ssadb.NameMatch, "late", func(id int64) Instruction {
		if id == oldValue.GetId() {
			return oldValue
		}
		return nil
	})
	require.Len(t, found, 1)
	builder.Finish()
}

func TestReleaseSettledKeepsUnflushedIndexWritable(t *testing.T) {
	prog := newResidentDeltaProgram(t)
	defer closeUnsavedFlushFixture(t, prog)()
	builder := prog.GetAndCreateFunctionBuilder("", "unflushed-index")
	first := builder.EmitUndefined("first")
	store := prog.Cache.indexes
	store.AddVariable("first", first)
	require.NoError(t, store.Flush())

	// Allocated but not yet written: its index row still has to reach the DB.
	second := builder.EmitUndefined("second")
	require.Greater(t, second.GetId(), first.GetId())
	store.appendIndex(store.variable, "second", second.GetId())

	var programName string
	store.indexSaved.ForEach(func(key indexRelationshipKey, _ struct{}) bool {
		programName = key.program
		return false
	})
	require.NotEmpty(t, programName)
	require.Greater(t, store.ReleaseSettled(programName), 0)

	store.AddVariable("second", second)
	require.NoError(t, store.Flush())
	var rows int64
	require.NoError(t, ssadb.GetDB().Model(&ssadb.IrIndex{}).Where("program_name = ? AND value_id = ?", programName, second.GetId()).Count(&rows).Error)
	require.Greater(t, rows, int64(0), "an instruction allocated but not yet flushed must still be able to write its index row")
	builder.Finish()
}

// A relationship created after ReleaseSettled still needs its resident lookup
// entry. Java pre-handlers allocate a local early and register its name while
// the AST is built, which can happen after a mid-batch release.
func TestAddVariableAfterSettledReleaseStaysFindable(t *testing.T) {
	prog := newResidentDeltaProgram(t)
	defer closeUnsavedFlushFixture(t, prog)()
	builder := prog.GetAndCreateFunctionBuilder("", "late-name")
	value := builder.EmitUndefined("late")
	store := prog.Cache.indexes
	store.AddVariable("early", value)
	require.NoError(t, store.Flush())

	var programName string
	store.indexSaved.ForEach(func(key indexRelationshipKey, _ struct{}) bool {
		programName = key.program
		return false
	})
	require.NotEmpty(t, programName)
	require.Greater(t, store.ReleaseSettled(programName), 0)

	// This name was never indexed before, even though the value id is old.
	store.AddVariable("res3", value)
	found := store.FindByVariableName(ssadb.NameMatch, "res3", func(id int64) Instruction {
		if id == value.GetId() {
			return value
		}
		return nil
	})
	require.Len(t, found, 1, "a name registered after the release must stay findable")
	require.Equal(t, value.GetId(), found[0].GetId())
	builder.Finish()
}

func TestFindByVariableExSurvivesSettledRelease(t *testing.T) {
	prog := newResidentDeltaProgram(t)
	defer closeUnsavedFlushFixture(t, prog)()
	builder := prog.GetAndCreateFunctionBuilder("", "find-settled")
	value := builder.EmitUndefined("kept")
	store := prog.Cache.indexes
	store.AddVariable("kept", value)
	require.NoError(t, store.Flush())
	var programName string
	store.indexSaved.ForEach(func(key indexRelationshipKey, _ struct{}) bool {
		programName = key.program
		return false
	})
	require.Greater(t, store.ReleaseSettled(programName), 0)

	found := store.FindByVariableEx(ssadb.NameMatch, func(name string) bool { return name == "kept" }, func(id int64) Instruction {
		if id == value.GetId() {
			return value
		}
		return nil
	})
	require.Len(t, found, 1)
	require.Equal(t, value.GetId(), found[0].GetId(), "the resident name index must survive a dedup-key trim")
	builder.Finish()
}

// A name registered after a release must also reach the database. A compile
// that keeps the row only in the resident map looks correct in DBWrite mode
// and returns nothing when the program is reopened.
func TestVariableRegisteredAfterSettledReleaseIsSaved(t *testing.T) {
	prog := newResidentDeltaProgram(t)
	builder := prog.GetAndCreateFunctionBuilder("", "late-save")
	value := builder.EmitUndefined("late")
	store := prog.Cache.indexes
	store.AddVariable("early", value)
	require.NoError(t, store.Flush())

	var programName string
	var variableID int64
	store.indexSaved.ForEach(func(key indexRelationshipKey, _ struct{}) bool {
		programName = key.program
		return false
	})
	require.NotEmpty(t, programName)
	require.Greater(t, store.ReleaseSettled(programName), 0)

	store.AddVariable("res3", value)
	require.NoError(t, store.Flush())
	if id := prog.NameCache.GetID("res3"); id > 0 {
		variableID = id
	}

	var rows int64
	require.NoError(t, ssadb.GetDB().Model(&ssadb.IrIndex{}).
		Where("program_name = ? AND value_id = ?", programName, value.GetId()).Count(&rows).Error)
	require.Greater(t, rows, int64(0), "a late name needs its own index row")
	if variableID != 0 {
		var lateRows int64
		require.NoError(t, ssadb.GetDB().Model(&ssadb.IrIndex{}).
			Where("program_name = ? AND value_id = ? AND variable_id = ?", programName, value.GetId(), variableID).
			Count(&lateRows).Error)
		require.Equal(t, int64(1), lateRows, "the late name must be written exactly once")
	}
	builder.Finish()
	prog.Cache.CloseWithoutSave()
}
