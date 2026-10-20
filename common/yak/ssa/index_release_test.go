package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func TestIndexStoreReleaseSettledDropsDedupBelowWatermark(t *testing.T) {
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
	_, err := store.Flush()
	require.NoError(t, err)

	beforeID := oldValue.GetId() + 1
	newer := builder.EmitUndefined("newer")
	require.GreaterOrEqual(t, newer.GetId(), beforeID)
	store.AddVariable("newer", newer)
	store.AddVariable("object.newer", newer)
	store.AddClassInstance("NewClass", newer)
	store.AddConst(newer)
	newOffset := ssadb.CreateOffset(memedit.NewRangeFromOffsets(memedit.NewMemEditor("newer"), 0, 5), prog.Name)
	newOffset.ValueID = newer.GetId()
	newOffset.VariableName = "newer"
	store.saveOffsetDedup(newOffset)
	_, err = store.Flush()
	require.NoError(t, err)

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

	require.Zero(t, store.ReleaseSettled("other-program", beforeID), "another program must keep this store's dedup keys")
	require.Equal(t, indexBefore, store.indexSaved.Count())

	released := store.ReleaseSettled(programName, beforeID)
	require.Greater(t, released, 0)
	require.Less(t, store.indexSaved.Count(), indexBefore)
	require.Less(t, store.offsetSaved.Count(), offsetBefore)
	require.Less(t, store.residentKeys.Count(), residentBefore)
	assertNoSettledDedup(t, store, programName, beforeID)

	var beforeRows int64
	require.NoError(t, ssadb.GetDB().Model(&ssadb.IrIndex{}).Where("program_name = ?", programName).Count(&beforeRows).Error)
	store.AddVariable("old", oldValue)
	store.AddVariable("newer", newer)
	store.AddClassInstance("OldClass", oldValue)
	store.AddClassInstance("NewClass", newer)
	repeatOffset := ssadb.CreateOffset(memedit.NewRangeFromOffsets(memedit.NewMemEditor("old"), 0, 3), prog.Name)
	repeatOffset.ValueID = oldValue.GetId()
	repeatOffset.VariableName = "old"
	store.saveOffsetDedup(repeatOffset)
	store.saveOffsetDedup(newOffset)
	_, err = store.Flush()
	require.NoError(t, err)
	var afterRows int64
	require.NoError(t, ssadb.GetDB().Model(&ssadb.IrIndex{}).Where("program_name = ?", programName).Count(&afterRows).Error)
	require.Equal(t, beforeRows, afterRows, "settled and still-open relationships must not be inserted twice")
	assertNoSettledDedup(t, store, programName, beforeID)

	fresh := builder.EmitUndefined("fresh")
	require.GreaterOrEqual(t, fresh.GetId(), beforeID)
	store.AddVariable("fresh", fresh)
	_, err = store.Flush()
	require.NoError(t, err)
	var freshRows int64
	require.NoError(t, ssadb.GetDB().Model(&ssadb.IrIndex{}).Where("program_name = ? AND value_id = ?", programName, fresh.GetId()).Count(&freshRows).Error)
	require.Greater(t, freshRows, int64(0), "an instruction allocated after the watermark must still be indexed")
	builder.Finish()
}

func TestReleaseSettledKeepsUnflushedIndexWritable(t *testing.T) {
	prog := newResidentDeltaProgram(t)
	defer closeUnsavedFlushFixture(t, prog)()
	builder := prog.GetAndCreateFunctionBuilder("", "unflushed-index")
	first := builder.EmitUndefined("first")
	store := prog.Cache.indexes
	store.AddVariable("first", first)
	_, err := store.Flush()
	require.NoError(t, err)

	second := builder.EmitUndefined("second")
	require.Greater(t, second.GetId(), first.GetId())
	store.appendIndex(store.variable, "second", second.GetId())

	var programName string
	store.indexSaved.ForEach(func(key indexRelationshipKey, _ struct{}) bool {
		programName = key.program
		return false
	})
	require.NotEmpty(t, programName)
	require.Greater(t, store.ReleaseSettled(programName, second.GetId()+1), 0)

	store.AddVariable("second", second)
	_, err = store.Flush()
	require.NoError(t, err)
	var rows int64
	require.NoError(t, ssadb.GetDB().Model(&ssadb.IrIndex{}).Where("program_name = ? AND value_id = ?", programName, second.GetId()).Count(&rows).Error)
	require.Greater(t, rows, int64(0), "an instruction allocated but not yet flushed must still be able to write its index row")
	builder.Finish()
}

func TestFindByVariableExSurvivesSettledRelease(t *testing.T) {
	prog := newResidentDeltaProgram(t)
	defer closeUnsavedFlushFixture(t, prog)()
	builder := prog.GetAndCreateFunctionBuilder("", "find-settled")
	value := builder.EmitUndefined("kept")
	store := prog.Cache.indexes
	store.AddVariable("kept", value)
	_, err := store.Flush()
	require.NoError(t, err)
	var programName string
	store.indexSaved.ForEach(func(key indexRelationshipKey, _ struct{}) bool {
		programName = key.program
		return false
	})
	require.Greater(t, store.ReleaseSettled(programName, value.GetId()+1), 0)

	found := store.FindByVariableEx(ssadb.NameMatch, func(name string) bool { return name == "kept" }, func(id int64) Instruction {
		if id == value.GetId() {
			return value
		}
		return nil
	})
	require.Len(t, found, 1)
	require.Equal(t, value.GetId(), found[0].GetId(), "settled dedup keys must not erase the resident name index")
	builder.Finish()
}

func assertNoSettledDedup(t *testing.T, store *indexStore, programName string, beforeID int64) {
	t.Helper()
	store.indexSaved.ForEach(func(key indexRelationshipKey, _ struct{}) bool {
		require.False(t, key.program == programName && key.value > 0 && key.value < beforeID)
		return true
	})
	store.offsetSaved.ForEach(func(key offsetLocationKey, _ struct{}) bool {
		require.False(t, key.value > 0 && key.value < beforeID)
		return true
	})
	store.residentKeys.ForEach(func(key residentIndexKey, _ struct{}) bool {
		require.False(t, key.id > 0 && key.id < beforeID)
		return true
	})
}
