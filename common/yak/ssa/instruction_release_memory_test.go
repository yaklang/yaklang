package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func TestFastPathReleasesSavedAnonymousConstants(t *testing.T) {
	prog := newResidentDeltaProgram(t)
	defer closeUnsavedFlushFixture(t, prog)()
	b := prog.GetAndCreateFunctionBuilder("", "constants")
	anonymous := b.EmitConstInst("anonymous")
	named := b.EmitConstInst("named")
	v := b.CreateVariable("named")
	require.NoError(t, v.Assign(named))
	store := prog.Cache.instructions
	require.NoError(t, store.residentCache.Flush(false))
	require.True(t, prog.Cache.hasResidentInstruction(anonymous.GetId()), "unfinished function must keep its values")
	b.Finish()
	// The tail was already saved before Finish. A clean flush must still
	// release newly eligible values, without rewriting the saved IR row.
	before := store.PersistenceStats().WriteOperations
	require.NoError(t, store.residentCache.Flush(false))
	require.False(t, prog.Cache.hasResidentInstruction(anonymous.GetId()), "saved anonymous constants must not accumulate in the resident map")
	require.Same(t, named, v.GetValue(), "live lexical values keep their canonical identity")
	require.True(t, prog.Cache.hasResidentInstruction(named.GetId()))
	require.LessOrEqual(t, store.PersistenceStats().WriteOperations-before, int64(1), "Finish may update the function, not unchanged constants")
	loaded, ok := ToConstInst(prog.Cache.GetInstruction(anonymous.GetId()))
	require.True(t, ok)
	require.Equal(t, "anonymous", loaded.GetRawValue())
	// A concrete pointer held by a caller can publish a late mutation even
	// after eviction. The next save must persist it exactly once.
	anonymous.SetExtern(true)
	require.NoError(t, store.residentCache.Flush(false))
	row := ssadb.GetIrCodeItemById(ssadb.GetDB(), prog.Name, anonymous.GetId())
	require.NotNil(t, row)
	require.True(t, row.IsExternal)
}

func TestFastPathCloseWithoutSaveReleasesResidentObjects(t *testing.T) {
	prog := newResidentDeltaProgram(t)
	b := prog.GetAndCreateFunctionBuilder("", "close-residency")
	value := b.EmitUndefined("named")
	require.NoError(t, b.CreateVariable("named").Assign(value))
	b.Finish()
	require.NoError(t, prog.Cache.SaveToDatabase())
	store := prog.Cache.instructions
	require.Positive(t, store.Count())
	prog.Cache.CloseWithoutSave()
	prog.Cache.CloseWithoutSave()
	require.Zero(t, store.Count(), "explicit close must release the fast-path resident object graph")
	require.Empty(t, store.residentFingerprints, "close must release saved snapshot fingerprints")
	loaded := prog.Cache.GetInstruction(value.GetId())
	require.NotNil(t, loaded, "queries must remain available through DB reload after close")
	require.Equal(t, "named", loaded.GetName())
}

func TestFastPathReloadPreservesLibraryOwner(t *testing.T) {
	p := newResidentDeltaProgram(t)
	defer closeUnsavedFlushFixture(t, p)()
	lib, err := p.GetOrCreateLibrary("package-a")
	require.NoError(t, err)
	b := lib.GetAndCreateFunctionBuilder("", "owner")
	v := b.EmitUndefined("anonymous")
	require.Same(t, lib, v.GetProgram())
	b.Finish()
	require.NoError(t, p.Cache.instructions.residentCache.Flush(false))
	require.False(t, p.Cache.hasResidentInstruction(v.GetId()))
	loaded := p.Cache.GetInstruction(v.GetId())
	require.NotNil(t, loaded)
	require.True(t, lib == loaded.GetProgram(), "anonymous IR must retain its compilation package when reloaded")
}
