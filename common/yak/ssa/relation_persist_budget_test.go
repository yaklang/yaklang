package ssa

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func newRelationBudgetFixture(t *testing.T) (*Program, *FunctionBuilder) {
	t.Helper()
	prog := newSplitWriteProgram(t)
	prog.Cache.DisableInstructionSpill()
	t.Cleanup(closeUnsavedFlushFixture(t, prog))
	return prog, prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
}

func relationRecord(t *testing.T, store *instructionStore, inst Instruction) *instructionPersistRecord {
	t.Helper()
	record, err := store.marshalInstructionRecord(inst, utils.EvictionReasonDeleted)
	require.NoError(t, err)
	require.NotNil(t, record)
	return record
}

// A new leaf must not serialize a saved, resident object graph again just
// because its reverse edge points into it. Check rows, not timing or GC.
func TestRelationPersistSkipsSavedClosure(t *testing.T) {
	prog, builder := newRelationBudgetFixture(t)
	store := prog.Cache.instructions
	owner := builder.EmitEmptyContainer()
	for i := 0; i < 32; i++ {
		key := builder.EmitConstInst(fmt.Sprintf("field%d", i))
		member := builder.EmitUndefined(fmt.Sprintf("value%d", i))
		setMemberCallRelationship(owner, key, member)
	}
	require.NoError(t, store.saveInstructionPersistRecords([]*instructionPersistRecord{relationRecord(t, store, owner)}))
	require.True(t, store.isPersisted(owner.GetId()))
	lateKey := builder.EmitConstInst("late")
	late := builder.EmitUndefined("late-value")
	setMemberCallRelationship(owner, lateKey, late)
	root := relationRecord(t, store, late)
	expanded := store.expandRelationPersistRecords([]*instructionPersistRecord{root})
	t.Cleanup(func() {
		for _, r := range expanded {
			if r != nil {
				releaseIrCode(r.IrCode)
			}
		}
	})
	for _, r := range expanded {
		require.False(t, store.isPersisted(r.CodeID), "relation-only expansion must not re-marshal a settled graph")
	}
	require.Len(t, expanded, 2, "only the new leaf and its new key are needed; explicit dirty owner is saved separately")
}

// The explicit dirty snapshot must win over the insert-only snapshot discovered
// from another root. Otherwise filtering settled INSERTs drops the real update.
func TestRelationPersistKeepsExplicitUpdate(t *testing.T) {
	for _, ownerFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("owner-first-%v", ownerFirst), func(t *testing.T) {
			prog, builder := newRelationBudgetFixture(t)
			store := prog.Cache.instructions
			owner := builder.EmitEmptyContainer()
			require.NoError(t, store.saveInstructionPersistRecords([]*instructionPersistRecord{relationRecord(t, store, owner)}))
			key := builder.EmitConstInst("late")
			leaf := builder.EmitUndefined("new-member")
			setMemberCallRelationship(owner, key, leaf)
			owner.SetExtern(true)
			update := relationRecord(t, store, owner)
			require.True(t, update.UpdateExisting)
			insert := relationRecord(t, store, leaf)
			records := []*instructionPersistRecord{insert, update}
			if ownerFirst {
				records = []*instructionPersistRecord{update, insert}
			}
			require.NoError(t, store.saveInstructionPersistRecords(records))
			row := ssadb.GetIrCodeItemById(ssadb.GetDB(), prog.Name, owner.GetId())
			require.NotNil(t, row)
			require.True(t, row.IsExternal, "relation expansion must preserve the explicit UPDATE")
			require.Len(t, row.ObjectMemberPairs, 1)
			memberRow := ssadb.GetIrCodeItemById(ssadb.GetDB(), prog.Name, leaf.GetId())
			require.NotNil(t, memberRow)
			require.Len(t, memberRow.ObjectOwnerPairs, 1, "keep both relation directions")
			require.NotNil(t, ssadb.GetIrCodeItemById(ssadb.GetDB(), prog.Name, key.GetId()))
		})
	}
}
