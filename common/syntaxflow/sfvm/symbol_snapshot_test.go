package sfvm

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/omap"
)

type snapshotIDValue struct {
	stubValueOperator
	id int64
}

func (v *snapshotIDValue) GetId() int64 { return v.id }

func snapshotTable(kv map[string][]int64) *omap.OrderedMap[string, Values] {
	table := omap.NewEmptyOrderedMap[string, Values]()
	for key, ids := range kv {
		vals := make(Values, 0, len(ids))
		for _, id := range ids {
			vals = append(vals, &snapshotIDValue{id: id})
		}
		table.Set(key, vals)
	}
	return table
}

func TestTakeSymbolSnapshot_HasNewNamedValue(t *testing.T) {
	snapshot := TakeSymbolSnapshot(snapshotTable(map[string][]int64{
		"a":   {1},
		"__m": {99}, // magic keys never count
	}))

	result := &SFFrameResult{
		SymbolTable: snapshotTable(map[string][]int64{
			"a": {1}, // same value -> not new
		}),
	}
	require.False(t, snapshot.HasNewNamedValue(result), "same named value must not be new")

	result.SymbolTable.Set("b", Values{&snapshotIDValue{id: 2}})
	require.True(t, snapshot.HasNewNamedValue(result), "new named key must be new")

	result.SymbolTable.Delete("b")
	result.SymbolTable.Set("a", Values{&snapshotIDValue{id: 2}})
	require.True(t, snapshot.HasNewNamedValue(result), "new value for existing key must be new")

	result.SymbolTable.Set("__new", Values{&snapshotIDValue{id: 3}})
	result.SymbolTable.Set("a", Values{&snapshotIDValue{id: 1}})
	require.False(t, snapshot.HasNewNamedValue(result), "only magic key changes must not be new")
}

// TestTakeSymbolSnapshot_EmptyTableShortCircuit verifies that a table with only
// magic keys (or no keys) returns the shared empty snapshot instead of a fresh
// one, and that its semantics match a manual empty snapshot (every child named
// key/value is new → merge).
func TestTakeSymbolSnapshot_EmptyTableShortCircuit(t *testing.T) {
	// nil table → shared empty snapshot
	if s := TakeSymbolSnapshot(nil); s != emptySymbolSnapshot {
		t.Fatalf("nil table should return the shared empty snapshot")
	}
	// empty table → shared empty snapshot
	empty := omap.NewEmptyOrderedMap[string, Values]()
	if s := TakeSymbolSnapshot(empty); s != emptySymbolSnapshot {
		t.Fatalf("empty table should return the shared empty snapshot")
	}
	// magic-only table → shared empty snapshot
	magic := snapshotTable(map[string][]int64{"__m": {1}, "": {2}})
	if s := TakeSymbolSnapshot(magic); s != emptySymbolSnapshot {
		t.Fatalf("magic-only table should return the shared empty snapshot")
	}

	// Semantics of the empty snapshot: a child with any named key/value is new.
	res := &SFFrameResult{
		SymbolTable: snapshotTable(map[string][]int64{"a": {1}}),
	}
	if !emptySymbolSnapshot.HasNewNamedValue(res) {
		t.Fatalf("empty snapshot must report a named child as new")
	}
	// A child with only magic vars is NOT new (keyOrValuesIsNew skips them).
	res.SymbolTable = snapshotTable(map[string][]int64{"__m": {1}})
	if emptySymbolSnapshot.HasNewNamedValue(res) {
		t.Fatalf("empty snapshot must NOT report magic-only child as new")
	}
}

// TestTakeSymbolSnapshot_ReuseNotRebuild verifies that the shared empty snapshot
// is returned repeatedly (no per-call rebuild) for a magic-only table, and that
// a named table still gets a distinct, non-shared snapshot.
func TestTakeSymbolSnapshot_ReuseNotRebuild(t *testing.T) {
	magic := snapshotTable(map[string][]int64{"__m": {1}})
	s1 := TakeSymbolSnapshot(magic)
	s2 := TakeSymbolSnapshot(magic)
	if s1 != s2 {
		t.Fatalf("magic-only snapshots should be the shared instance")
	}
	if s1 != emptySymbolSnapshot {
		t.Fatalf("magic-only snapshot should equal emptySymbolSnapshot")
	}

	named := snapshotTable(map[string][]int64{"a": {1}})
	ns := TakeSymbolSnapshot(named)
	if ns == emptySymbolSnapshot {
		t.Fatalf("named table must get a distinct snapshot")
	}
	if ns2 := TakeSymbolSnapshot(named); ns2 == ns {
		t.Fatalf("named snapshots are per-call fresh instances")
	}
}
