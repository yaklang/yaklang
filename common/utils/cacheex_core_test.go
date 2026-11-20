package utils

import (
	"testing"
	"time"
)

func TestCacheExCoreNilValuesAndExplicitDelete(t *testing.T) {
	ptrCache := NewCacheExWithKey[string, *int](WithCacheTTL(time.Minute))
	defer ptrCache.Close()

	ptrCache.Set("nil-ptr", nil)
	gotPtr, ok := ptrCache.Get("nil-ptr")
	if !ok || gotPtr != nil {
		t.Fatalf("expected (nil, true), got (%v, %v)", gotPtr, ok)
	}
	allPtr := ptrCache.GetAll()
	if len(allPtr) != 1 || allPtr["nil-ptr"] != nil {
		t.Fatalf("unexpected GetAll: %#v", allPtr)
	}

	var visited int
	ptrCache.ForEach(func(k string, v *int) {
		visited++
		if k != "nil-ptr" || v != nil {
			t.Fatalf("unexpected ForEach entry (%q, %v)", k, v)
		}
	})
	if visited != 1 {
		t.Fatalf("expected 1 visited entry, got %d", visited)
	}

	loaded, err := ptrCache.GetOrLoad("loaded-nil", func() (*int, error) {
		return nil, nil
	})
	if err != nil || loaded != nil {
		t.Fatalf("expected (nil, nil), got (%v, %v)", loaded, err)
	}
	if ptrCache.Count() != 2 {
		t.Fatalf("expected count=2, got %d", ptrCache.Count())
	}

	ptrCache.Delete("nil-ptr")
	if _, ok := ptrCache.Get("nil-ptr"); ok {
		t.Fatal("expected nil-ptr to be deleted")
	}

	ptrCache.DeleteAll()
	if ptrCache.Count() != 0 {
		t.Fatalf("expected count=0 after DeleteAll, got %d", ptrCache.Count())
	}

	anyCache := NewCacheExWithKey[string, any](WithCacheTTL(time.Minute))
	defer anyCache.Close()
	anyCache.Set("empty", nil)
	if v, ok := anyCache.Get("empty"); !ok || v != nil {
		t.Fatalf("expected (nil, true) for any cache, got (%v, %v)", v, ok)
	}
}
