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
