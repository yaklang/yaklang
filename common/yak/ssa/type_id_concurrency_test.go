package ssa

import (
	"sync"
	"testing"

	"github.com/yaklang/yaklang/common/utils"
	"go.uber.org/atomic"
)

// newTestTypeStore builds the slice of typeStore that id assignment touches:
// the shared counter plus the resident index, without a DB or Program.
func newTestTypeStore() *typeStore {
	return &typeStore{
		nextID:   atomic.NewInt64(0),
		resident: utils.NewSafeMapWithKey[int64, Type](),
	}
}

// TestSharedTypeIdIsAssignedOnce pins the invariant that broke in CI: types are
// shared across instructions, and dbcache persist workers marshal them in
// parallel, so several workers can reach typeStore.remember for the same object
// at once. Exactly one id must win. Before the fix every worker could assign its
// own id, so one type could be written to the DB under several ids and read back
// inconsistently.
func TestSharedTypeIdIsAssignedOnce(t *testing.T) {
	store := newTestTypeStore()
	shared := NewInterfaceType("shared", "example/pkg")

	const workers = 32
	var start, wg sync.WaitGroup
	start.Add(1)
	observed := make([]int64, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			start.Wait()
			observed[i] = store.remember(shared).GetId()
		}(i)
	}
	start.Done()
	wg.Wait()

	if observed[0] <= 0 {
		t.Fatalf("type kept its unassigned id: %d", observed[0])
	}
	for i, id := range observed {
		if id != observed[0] {
			t.Fatalf("worker %d saw id %d but worker 0 saw %d: one type got several ids", i, id, observed[0])
		}
	}
	if _, ok := store.resident.Get(observed[0]); !ok {
		t.Fatalf("type is not indexed under its assigned id %d", observed[0])
	}
}

// TestConcurrentTypeIdReadDuringAssign reproduces the reported race pair: an IR
// marshal reading a shared type's id while another worker assigns it. Run with
// -race it fails if baseType.id stops being atomic.
func TestConcurrentTypeIdReadDuringAssign(t *testing.T) {
	const rounds = 2000
	shared := NewInterfaceType("shared", "example/pkg")
	store := newTestTypeStore()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { // reader, like value2IrCode calling typ.GetId()
		defer wg.Done()
		for i := 0; i < rounds; i++ {
			_ = shared.GetId()
		}
	}()
	go func() { // writer, like saveTypeWithValue calling remember
		defer wg.Done()
		for i := 0; i < rounds; i++ {
			store.remember(shared)
		}
	}()
	wg.Wait()

	if id := shared.GetId(); id <= 0 {
		t.Fatalf("type never got an id: %d", id)
	}
}
