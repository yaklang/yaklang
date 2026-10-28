package ssa

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func TestFastPathCloseWithoutSaveIsIdempotent(t *testing.T) {
	prog := newResidentDeltaProgram(t)
	defer closeUnsavedFlushFixture(t, prog)()
	var callers sync.WaitGroup
	callers.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer callers.Done()
			prog.Cache.instructions.CloseWithoutSave()
		}()
	}
	done := make(chan struct{})
	go func() {
		callers.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("repeated reload-cache closure must complete")
	}
}

func TestFastPathDirtyRelationsAndReleasedInstruction(t *testing.T) {
	prog := newResidentDeltaProgram(t)
	b := prog.GetAndCreateFunctionBuilder("", "private")
	left := b.EmitUndefined("left")
	right := b.EmitUndefined("right")
	value := b.EmitBinOp(OpAdd, left, right)
	b.Finish()
	store := prog.Cache.instructions
	require.NoError(t, store.residentCache.Flush(false))
	require.Zero(t, store.residentCache.DirtyCount())
	_, resident := store.resident.Get(value.GetId())
	require.False(t, resident, "saved unnamed body instructions should release clean cache residency")
	require.Greater(t, store.PersistenceStats().ResidentReleased, int64(0))

	// A held concrete instruction can still publish a late mutation after its
	// cache entry was released. Relation-only updates must also queue writes.
	user, ok := ToUser(value)
	require.True(t, ok)
	user.ReplaceValue(left, right)
	left.RemoveUser(user)
	right.AddMember(left, value)
	value.AddObjectKeyPair(right, left)
	require.Greater(t, store.residentCache.DirtyCount(), 0)
	require.NoError(t, store.Close(nil))
	reloaded := ssadb.GetIrCodeItemById(ssadb.GetDB(), prog.Name, value.GetId())
	require.NotNil(t, reloaded)
	decoded, err := NewLazyInstructionFromIrCode(reloaded, prog)
	require.NoError(t, err)
	bin, ok := ToBinOp(decoded)
	require.True(t, ok)
	require.Equal(t, right.GetId(), bin.X)
	leftRow := ssadb.GetIrCodeItemById(ssadb.GetDB(), prog.Name, left.GetId())
	require.Empty(t, leftRow.Users)
	before := store.PersistenceStats().WriteOperations
	require.NoError(t, store.Close(nil))
	require.Equal(t, before, store.PersistenceStats().WriteOperations)
	// Clean DB reloads must remain queryable and must not create dirty work.
	require.NotNil(t, prog.Cache.GetInstruction(value.GetId()))
	require.Zero(t, store.residentCache.DirtyCount())
}

func TestFastPathDirtyReloadedSetter(t *testing.T) {
	prog := newResidentDeltaProgram(t)
	b := prog.GetAndCreateFunctionBuilder("", "private")
	value := b.EmitBinOp(OpAdd, b.EmitUndefined("a"), b.EmitUndefined("b"))
	b.Finish()
	require.NoError(t, prog.Cache.instructions.residentCache.Flush(false))
	reloaded := prog.Cache.GetInstruction(value.GetId())
	require.True(t, reloaded.IsLazy())
	reloaded.SetExtern(true)
	require.NoError(t, prog.Cache.SaveToDatabase())
	row := ssadb.GetIrCodeItemById(ssadb.GetDB(), prog.Name, value.GetId())
	require.True(t, row.IsExternal)
}

func TestLazySaveTypeReleasesConstructionClosure(t *testing.T) {
	value := NewValue()
	calls := 0
	value.SetLazySaveType(func() { calls++ })
	value.LazySaveType()
	require.Nil(t, value.lazySaveType, "completed work must not retain its captured type graph")
	value.LazySaveType()
	require.Equal(t, 1, calls)
}

var dirtyValueAllocationSink *anValue

func TestFastPathDirtyKeepsNilGuards(t *testing.T) {
	var function *Function
	var parameter *Parameter
	var block *BasicBlock
	require.NotPanics(t, func() { function.Finish() })
	require.NotPanics(t, func() { function.SetType(CreateAnyType()) })
	require.NotPanics(t, func() { parameter.SetDefault(nil) })
	require.NotPanics(t, func() { block.SetConditionFromValue(nil, "nil") })
	require.NotPanics(t, func() { block.SetConditionInstID(1) })
}

func TestValueConstructionOwnsBaseWithoutClosure(t *testing.T) {
	require.LessOrEqual(t, testing.AllocsPerRun(100, func() {
		dirtyValueAllocationSink = NewValue()
	}), float64(1), "a value should allocate neither a separate base nor a deferred-type closure")
	require.Nil(t, dirtyValueAllocationSink.lazySaveType)
	dirtyValueAllocationSink = nil
}

func TestPendingTypeReleasedAfterRegister(t *testing.T) {
	prog := newResidentDeltaProgram(t)
	b := prog.GetAndCreateFunctionBuilder("", "type_owner")
	value := NewUndefined("pending")
	initial := CreateStringType()
	value.SetType(initial)
	require.Same(t, initial, value.persistState)
	b.EmitFirst(value)
	require.Same(t, value, value.persistState, "construction metadata should become the concrete dirty-publication owner")
	require.Same(t, initial, value.GetType())
	b.Finish()
}

func TestFastPathDirtyDeletionDoesNotLeavePendingKeys(t *testing.T) {
	prog := newResidentDeltaProgram(t)
	b := prog.GetAndCreateFunctionBuilder("", "deleted")
	value := b.EmitUndefined("removed")
	b.Finish()
	prog.Cache.DeleteInstruction(value)
	require.NoError(t, prog.Cache.instructions.residentCache.Flush(false))
	require.Zero(t, prog.Cache.instructions.residentCache.DirtyCount())
	require.NoError(t, prog.Cache.SaveToDatabase())
	require.Zero(t, prog.Cache.instructions.PersistenceStats().RemainingDirty)
}

func TestFastPathDirtyLateFunctionMetadata(t *testing.T) {
	prog := newResidentDeltaProgram(t)
	b := prog.GetAndCreateFunctionBuilder("", "metadata")
	param := b.NewParam("argument")
	value := b.EmitUndefined("default")
	variable := b.CreateVariable("captured")
	b.Finish()
	store := prog.Cache.instructions
	require.NoError(t, store.residentCache.Flush(false))

	// These updates do not emit body instructions or finish the function again.
	// Each must still change the successfully saved database snapshot.
	param.SetDefault(value)
	b.Function.AddThrow(value)
	b.Function.AddSideEffect(variable, value)
	b.HandlerEllipsis()
	lateParam := b.NewParam("late")
	require.NoError(t, store.residentCache.Flush(false))
	row := ssadb.GetIrCodeItemById(ssadb.GetDB(), prog.Name, b.Function.GetId())
	lazy, err := NewLazyInstructionFromIrCode(row, prog)
	require.NoError(t, err)
	function, ok := ToFunction(lazy)
	require.True(t, ok)
	require.Contains(t, function.Params, lateParam.GetId())
	require.Equal(t, []int64{value.GetId()}, function.Throws)
	require.True(t, function.hasEllipsis)
	require.Len(t, function.SideEffects, 1)
	require.Equal(t, value.GetId(), function.SideEffects[0].Modify)
	paramRow := ssadb.GetIrCodeItemById(ssadb.GetDB(), prog.Name, param.GetId())
	paramLazy, err := NewLazyInstructionFromIrCode(paramRow, prog)
	require.NoError(t, err)
	decodedParam, ok := ToParameter(paramLazy)
	require.True(t, ok)
	require.Equal(t, value.GetId(), decodedParam.defaultValue)

	// Function.SetType overrides the ordinary value setter.
	require.Zero(t, store.residentCache.DirtyCount())
	functionType := NewFunctionType("late_type", nil, CreateStringType(), false)
	b.Function.SetType(functionType)
	require.Positive(t, store.residentCache.DirtyCount())
	require.NoError(t, prog.Cache.SaveToDatabase())
	row = ssadb.GetIrCodeItemById(ssadb.GetDB(), prog.Name, b.Function.GetId())
	require.Equal(t, functionType.GetId(), row.TypeID)
	require.NotNil(t, ssadb.GetIrTypeItemById(ssadb.GetDB(), prog.Name, row.TypeID),
		"the final IR snapshot must not refer to an unsaved replacement type")
}

func TestFastPathDirtyExplicitFieldMutation(t *testing.T) {
	prog := newResidentDeltaProgram(t)
	b := prog.GetAndCreateFunctionBuilder("", "direct_field")
	value := b.EmitBinOp(OpAdd, b.EmitUndefined("a"), b.EmitUndefined("b"))
	b.Finish()
	store := prog.Cache.instructions
	require.NoError(t, store.residentCache.Flush(false))
	bin, ok := ToBinOp(value)
	require.True(t, ok)
	bin.Op = OpSub
	prog.Cache.MarkInstructionDirty(bin)
	require.NoError(t, store.residentCache.Flush(false))
	row := ssadb.GetIrCodeItemById(ssadb.GetDB(), prog.Name, value.GetId())
	lazy, err := NewLazyInstructionFromIrCode(row, prog)
	require.NoError(t, err)
	decoded, ok := ToBinOp(lazy)
	require.True(t, ok)
	require.Equal(t, BinaryOpcode(OpSub), decoded.Op)
}
