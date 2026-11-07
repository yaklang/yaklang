package ssa

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func allocations(fn func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	fn()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// Growing an object literal used to scan every stored key on each insert to
// answer "is this key absent", and format each key text while doing it. The
// work is quadratic in the number of members, so doubling the literal size
// much more than doubled the allocations. dotCMS ace-builds has a 291k byte
// array literal that held a CPU window for minutes.
func TestObjectLiteralInsertStaysLinear(t *testing.T) {
	build := func(size int) uint64 {
		obj := NewObjectType()
		return allocations(func() {
			for i := 0; i < size; i++ {
				key := NewConst(int64(i))
				if field := obj.GetField(key); field != nil {
					t.Fatalf("fresh key %d must be absent", i)
				}
				obj.AddField(key, CreateNumberType())
			}
		})
	}

	const small, large = 4000, 8000
	smallAlloc := build(small)
	largeAlloc := build(large)

	// Linear growth doubles the work; quadratic growth quadruples it. Allow a
	// wide margin so GC noise cannot make this flaky, while still failing the
	// previous quadratic behaviour.
	require.Less(t, largeAlloc, 3*smallAlloc,
		"inserting %d members allocated %d bytes, %d members allocated %d bytes",
		small, smallAlloc, large, largeAlloc)
}

// A presence check must see keys added through AddField, and a rebuild must
// recover an index for a type whose keys were replaced wholesale.
func TestObjectTypeKeyIndexStaysConsistent(t *testing.T) {
	obj := NewObjectType()
	obj.AddField(NewConst("alpha"), CreateNumberType())
	obj.AddField(NewConst(int64(1)), CreateStringType())

	require.NotNil(t, obj.GetField(NewConst("alpha")))
	require.NotNil(t, obj.GetField(NewConst(int64(1))))
	require.Nil(t, obj.GetField(NewConst("missing")))

	// Same shape as the truncated variadic tuple in call.go: Keys replaced
	// directly, so the index must be rebuilt from them.
	obj.Keys = obj.Keys[:1]
	obj.FieldTypes = obj.FieldTypes[:1]
	obj.rebuildKeyIndex()
	require.NotNil(t, obj.GetField(NewConst("alpha")))
	require.Nil(t, obj.GetField(NewConst(int64(1))), "truncated key must no longer resolve")

	// A type assembled without AddField still resolves through the fallback.
	raw := NewObjectType()
	raw.keymap = nil
	raw.Keys = append(raw.Keys, NewConst("beta"))
	raw.FieldTypes = append(raw.FieldTypes, CreateStringType())
	require.NotNil(t, raw.GetField(NewConst("beta")))
	require.Nil(t, raw.GetField(NewConst("alpha")))
	require.NotNil(t, raw.keymap, "fallback must rebuild the index")
	_ = fmt.Sprintf("%v", raw.keymap)
}
