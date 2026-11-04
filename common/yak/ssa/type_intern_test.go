package ssa

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
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
// break those rows. Interning is therefore best effort: it covers types whose
// id this store assigns, not types that arrive pre-numbered.
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
		"a pre-numbered equal type still gets its own row")
}
