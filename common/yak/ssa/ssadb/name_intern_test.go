package ssadb

import (
	"strings"
	"sync"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
)

func TestNameCacheInternOwnsAndSharesStrings(t *testing.T) {
	cache := NewNameCache("intern-only", false)
	input := strings.Clone(strings.Repeat("prefix", 10000) + "qualified.Type")
	name := input[len(input)-len("qualified.Type"):]
	first := cache.Intern(name)
	require.Equal(t, name, first)
	require.True(t, unsafe.StringData(name) != unsafe.StringData(first), "pool must not retain the input buffer via a substring")
	keys := cache.strings.Keys()
	require.Len(t, keys, 1)
	require.True(t, unsafe.StringData(keys[0]) == unsafe.StringData(first), "map key and value must share the owned string")
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if unsafe.StringData(cache.Intern(strings.Clone(name))) != unsafe.StringData(first) {
				t.Error("concurrent intern returned different string storage")
			}
		}()
	}
	wg.Wait()
	require.Zero(t, cache.nextID, "interning type strings must not consume name IDs")
	require.Empty(t, cache.nameToId)
	require.Empty(t, cache.idToName)
	id := cache.GetID(first)
	require.Equal(t, id, cache.GetID(strings.Clone(name)))
	require.Equal(t, first, cache.GetName(id))
}
