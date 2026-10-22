package ssa

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/dbcache"
)

// A batch release is a memory-only operation. Loading an already-spilled
// BasicBlock cannot recover its transient ScopeTable and instead resurrects IR
// and its member/type graph. Count loads, not time: two successive releases
// must perform no storage reads, even though an explicit value read still can.
func TestReleasePersistedScopesDoesNotReloadSpilledBlocks(t *testing.T) {
	fn, variable, value := assignLocal(t, "helper")
	persistFinished(t, fn)
	prog := fn.GetProgram()
	store := prog.Cache.instructions
	resident := store.snapshotResident()
	store.writer.CloseWithoutSave()

	var loads atomic.Int64
	store.writer = dbcache.NewCache[Instruction, *instructionPersistRecord](0, 0, nil, nil,
		func(id int64) (Instruction, error) {
			loads.Add(1)
			return store.loadInstruction(id)
		})
	t.Cleanup(store.writer.CloseWithoutSave)
	// Keep boundary identities, but model an earlier batch's spilled blocks.
	for _, inst := range resident {
		if _, block := inst.(*BasicBlock); !block {
			store.writer.Set(inst)
		}
	}
	require.NotEmpty(t, fn.Blocks)
	require.NotNil(t, fn.builder.CurrentBlock.ScopeTable)
	require.Greater(t, prog.ReleasePersistedOrdinaryValues(), 0)
	require.Nil(t, variable.Value)
	require.Zero(t, loads.Load(), "reclaiming scopes must not reload previously spilled blocks")
	require.Zero(t, prog.ReleasePersistedOrdinaryValues())
	require.Zero(t, loads.Load(), "a second batch release must stay memory-only")
	reloaded := variable.GetValue()
	require.NotNil(t, reloaded)
	require.Equal(t, value.GetId(), reloaded.GetId(), "explicit semantic lookup must still load the saved value")
}
