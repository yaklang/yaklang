package ssa

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

// Equal types must share one id. Before interning, every created primitive got
// its own id: Hue stored 3.47M type ids for 106K distinct bodies, and the top
// duplicates were string (1.39M ids), number (888K) and any (421K).
func TestTypeStoreInternsEqualTypes(t *testing.T) {
	db, store := newTypeSnapshotStore(t, "intern")
	first := CreateStringType()
	first.AddFullTypeName("pkg.Str")
	second := CreateStringType()
	second.AddFullTypeName("pkg.Str")

	store.remember(first)
	store.remember(second)

	require.Greater(t, first.GetId(), int64(0))
	require.Equal(t, first.GetId(), second.GetId(), "equal types must reuse one id")
	require.Equal(t, 1, store.resident.Count(), "only the canonical object stays resident")
	require.NoError(t, store.flush())

	var rows int64
	require.NoError(t, db.Model(&ssadb.IrType{}).Count(&rows).Error)
	require.Equal(t, int64(1), rows)
}

func TestTypeStoreKeepsDifferentBodiesOnDifferentIDs(t *testing.T) {
	_, store := newTypeSnapshotStore(t, "intern-diff")
	number, text := CreateNumberType(), CreateStringType()
	store.remember(number)
	store.remember(text)
	require.NotEqual(t, number.GetId(), text.GetId())
}

// Blueprints are excluded: a class gains parents, interfaces and a container
// after its first reference, so two classes that start equal must keep
// separate ids.
func TestTypeStoreDoesNotInternBlueprints(t *testing.T) {
	_, store := newTypeSnapshotStore(t, "intern-blueprint")
	first, second := NewBlueprint("Same"), NewBlueprint("Same")
	store.remember(first)
	store.remember(second)
	require.NotEqual(t, first.GetId(), second.GetId(), "blueprints must keep independent identities")
	require.Equal(t, 2, store.resident.Count())
}

// The interning map is shared by parallel compile units, so the same body
// created by several goroutines must still resolve to one id.
func TestTypeStoreInternsConcurrently(t *testing.T) {
	_, store := newTypeSnapshotStore(t, "intern-parallel")
	const workers = 32
	ids := make([]int64, workers)
	var start, wg sync.WaitGroup
	start.Add(1)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			start.Wait()
			typ := CreateStringType()
			typ.AddFullTypeName("pkg.Str")
			ids[i] = store.remember(typ).GetId()
		}(i)
	}
	start.Done()
	wg.Wait()

	for i, id := range ids {
		require.Equal(t, ids[0], id, "worker %d saw a different id", i)
	}
	require.Equal(t, 1, store.resident.Count())
}

// A type that already carries an id keeps it. That id may already be
// referenced by persisted instructions, so rewriting it to a shared id would
// break those rows.
func TestTypeStoreKeepsPreassignedID(t *testing.T) {
	db, store := newTypeSnapshotStore(t, "preassigned")
	first := CreateStringType()
	store.remember(first)
	require.NoError(t, store.flush())
	assigned := first.GetId()

	imported := CreateStringType()
	imported.SetId(assigned + 41)
	store.remember(imported)
	require.Equal(t, assigned+41, imported.GetId(), "an assigned id is never rewritten")
	require.NoError(t, store.flush())

	require.Equal(t, int64(2), countIrTypeRowsForProgram(t, db, "preassigned"),
		"two ids that already exist cannot be merged after the fact")
}

// The other order is the one that used to leak a duplicate row: a shared
// global such as defaultAnyType arrives already numbered, then a fresh equal
// type is created. The numbered body must still teach the intern table.
func TestTypeStoreAdoptsNumberedTypeForLaterMatches(t *testing.T) {
	db, store := newTypeSnapshotStore(t, "numbered-first")
	preset := CreateAnyType()
	preset.SetId(1)
	store.remember(preset)

	fresh := CreateAnyType()
	store.remember(fresh)
	require.Equal(t, int64(1), preset.GetId(), "a preset id is never rewritten")
	require.Equal(t, int64(1), fresh.GetId(), "a later equal type must reuse the preset id")
	require.NoError(t, store.flush())
	require.Equal(t, int64(1), countIrTypeRowsForProgram(t, db, "numbered-first"))
}

// remember runs on every SetType, and most of those types already carry an id.
// Adopting such a type must cost one map lookup after the first time, not a
// snapshot per call.
func BenchmarkTypeRememberNumbered(b *testing.B) {
	store := &typeStore{
		mode:        ProgramCacheDBWrite,
		programName: "bench",
		nextID:      newTestTypeStore().nextID,
		resident:    utils.NewSafeMapWithKey[int64, Type](),
	}
	typ := CreateStringType()
	typ.SetId(1)
	store.remember(typ)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.remember(typ)
	}
}

// A function type is filled in place by Finish() after its first use: it
// starts as "() -> " and later becomes "(number) -> any". Sharing an id at the
// empty stage would bind unrelated signatures to one row, so function types
// must never intern. Probing a real compile showed 176 such rewrites, 55 of
// them after another type had already reused the id.
func TestTypeStoreDoesNotInternFunctionTypes(t *testing.T) {
	_, store := newTypeSnapshotStore(t, "intern-func")
	first := NewFunctionType("", nil, nil, false)
	second := NewFunctionType("", nil, nil, false)
	store.remember(first)
	store.remember(second)
	require.NotEqual(t, first.GetId(), second.GetId())
	require.Equal(t, 2, store.resident.Count())

	// The bodies diverge after the ids were handed out; each keeps its own.
	first.ReturnType, second.ReturnType = CreateNumberType(), CreateStringType()
	require.NotEqual(t, first.GetId(), second.GetId())
	require.NotEqual(t, first.String(), second.String())
}

// Slice and map wrappers are rewritten too: coverType copies FieldType/KeyTyp
// in, ApplyGenericType substitutes type parameters, and a slice that becomes
// []byte is replaced wholesale. Two equal-looking wrappers can therefore stop
// being equal, so they must not share an id.
func TestTypeStoreDoesNotInternSliceOrMapWrappers(t *testing.T) {
	_, store := newTypeSnapshotStore(t, "intern-wrapper")
	sliceA, sliceB := NewSliceType(CreateAnyType()), NewSliceType(CreateAnyType())
	store.remember(sliceA)
	store.remember(sliceB)
	require.NotEqual(t, sliceA.GetId(), sliceB.GetId(), "slice wrappers are mutated after construction")

	sliceA.FieldType = CreateStringType()
	require.NotEqual(t, sliceA.String(), sliceB.String())

	mapA, mapB := NewMapType(CreateStringType(), CreateAnyType()), NewMapType(CreateStringType(), CreateAnyType())
	store.remember(mapA)
	store.remember(mapB)
	require.NotEqual(t, mapA.GetId(), mapB.GetId(), "map wrappers are mutated after construction")
}
