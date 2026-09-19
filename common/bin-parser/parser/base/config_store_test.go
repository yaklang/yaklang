package base

import (
	"fmt"
	"math/rand"
	"reflect"
	"sync"
	"testing"

	"github.com/yaklang/yaklang/common/utils/omap"
)

type configTestStore interface {
	Get(string) (any, bool)
	Set(string, any)
	Delete(string)
	ForEach(func(string, any) bool)
}

func configStoreSnapshot(s configTestStore) []configEntry {
	var result []configEntry
	s.ForEach(func(k string, v any) bool {
		result = append(result, configEntry{k, v})
		return true
	})
	return result
}

func TestConfigStoreMatchesOrderedMap(t *testing.T) {
	old := omap.NewEmptyOrderedMap[string, any]()
	current := &configStore{}
	rng := rand.New(rand.NewSource(20260907))
	keys := []string{"", "nil", "endian", "parser", "unit", CfgOptionFuns}
	for i := 0; i < 50; i++ {
		keys = append(keys, fmt.Sprintf("key-%d", i))
	}
	shared := map[string]any{"retained": true}
	values := []any{nil, false, uint64(0), "", "little", shared, []byte{0, 1}}
	for step := 0; step < 4000; step++ {
		key := keys[rng.Intn(len(keys))]
		if rng.Intn(4) == 0 {
			old.Delete(key)
			current.Delete(key)
		} else {
			value := values[rng.Intn(len(values))]
			old.Set(key, value)
			current.Set(key, value)
		}
		if !reflect.DeepEqual(configStoreSnapshot(old), configStoreSnapshot(current)) {
			t.Fatalf("ordered values diverged at step %d", step)
		}
		for _, key := range keys {
			a, aOK := old.Get(key)
			b, bOK := current.Get(key)
			if aOK != bOK || !reflect.DeepEqual(a, b) {
				t.Fatalf("lookup diverged at step %d, key %q", step, key)
			}
		}
	}
	if current.index == nil {
		t.Fatal("test did not exercise larger contexts")
	}
	for _, key := range keys {
		old.Delete(key)
		current.Delete(key)
	}
	current.Set("shared", shared)
	shared["retained"] = false
	v, _ := current.Get("shared")
	if v.(map[string]any)["retained"] != false {
		t.Fatal("store must preserve shallow value sharing")
	}
	current.Delete("shared")
	current.Set("after-empty", nil)
	if v, ok := current.Get("after-empty"); !ok || v != nil {
		t.Fatal("present nil changed after deleting all entries")
	}
}

func TestConfigStoreSnapshotReentrantAndEarlyStop(t *testing.T) {
	for _, makeStore := range []func() configTestStore{
		func() configTestStore { return omap.NewEmptyOrderedMap[string, any]() },
		func() configTestStore { return &configStore{} },
	} {
		store := makeStore()
		store.Set("a", 1)
		store.Set("b", 2)
		var seen []configEntry
		store.ForEach(func(k string, v any) bool {
			seen = append(seen, configEntry{k, v})
			store.Delete("b")
			store.Set("b", 9)
			store.Set("c", 3)
			return true
		})
		if !reflect.DeepEqual(seen, []configEntry{{"a", 1}, {"b", 2}}) {
			t.Fatalf("iteration must snapshot keys AND values: %v", seen)
		}
		count := 0
		store.ForEach(func(string, any) bool { count++; return false })
		if count != 1 {
			t.Fatal("early termination was ignored")
		}
	}
	var empty *configStore
	empty.Set("ignored", true)
	empty.Delete("ignored")
	empty.ForEach(nil)
	if v, ok := empty.Get("ignored"); ok || v != nil {
		t.Fatal("nil receiver behavior differs from OrderedMap")
	}
}

func TestConfigStoreSpillReleasesInlineValues(t *testing.T) {
	store := &configStore{}
	for i := 0; i < 9; i++ {
		store.Set(fmt.Sprint(i), &struct{ value int }{i})
	}
	for _, entry := range store.inline {
		if entry.key != "" || entry.value != nil {
			t.Fatal("stale inline references retained after spill")
		}
	}
	for i := 0; i < 9; i++ {
		store.Delete(fmt.Sprint(i))
	}
	for _, entry := range store.entries[:cap(store.entries)] {
		if entry.key != "" || entry.value != nil {
			t.Fatal("deleted entries retain references")
		}
	}
}

func TestConfigStoreConcurrentAccess(t *testing.T) {
	store := &configStore{}
	var wg sync.WaitGroup
	for worker := 0; worker < 12; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 300; i++ {
				key := fmt.Sprintf("%d/%d", worker, i%4)
				store.Set(key, i)
				if v, ok := store.Get(key); !ok || v != i {
					t.Errorf("worker-owned key lost: %s", key)
				}
				if i%11 == 0 {
					store.ForEach(func(string, any) bool { return true })
					store.Delete(key)
				}
			}
		}(worker)
	}
	wg.Wait()
}

func BenchmarkConfigStoreConstruction(b *testing.B) {
	for _, size := range []int{4, 8, 16, 32} {
		keys := make([]string, size)
		for i := range keys {
			keys[i] = fmt.Sprintf("field-%d", i)
		}
		for _, implementation := range []string{"ordered", "compact"} {
			b.Run(fmt.Sprintf("%s/%d", implementation, size), func(b *testing.B) {
				create := func() configTestStore { return omap.NewEmptyOrderedMap[string, any]() }
				if implementation == "compact" {
					create = func() configTestStore { return &configStore{} }
				}
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					store := create()
					for _, key := range keys {
						store.Set(key, true)
						store.Set("options", nil)
					}
					for _, key := range keys {
						if _, ok := store.Get(key); !ok {
							b.Fatal("missing field")
						}
					}
				}
			})
		}
	}
}
