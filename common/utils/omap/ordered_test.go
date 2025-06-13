package omap

import (
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/davecgh/go-spew/spew"
)

func TestWalk(t *testing.T) {
	var a = NewGeneralOrderedMap()
	var b = NewGeneralOrderedMap()
	a.Set("BBB", "CCC")
	b.Set("CCC", "DDD")
	a.Set("b", b)
	Walk(a, func(parent any, key any, value any) bool {
		t.Logf("%v %v %v", parent, key, value)
		return true
	})
}

func TestWalkSearch(t *testing.T) {
	var a = NewGeneralOrderedMap()
	var b = NewGeneralOrderedMap()
	a.Set("BBBD", "CCC")
	b.Set("CCCD", "DDD")
	a.Set("bD", b)
	vars, err := a.WalkSearchGlobKey("*D")
	if err != nil {
		t.Fatal(err)
	}
	spew.Dump(vars)
}

func TestMarshalJSON(t *testing.T) {
	m := NewGeneralOrderedMap()
	m2 := NewGeneralOrderedMap()
	m2.Set("B", 1)
	m.Set("D", '1')

	m.Set("A", m2)
	m3 := NewGeneralOrderedMap()
	m3.Add("111")
	m3.Add("112")
	m3.Add("113")
	m.Set("C", m3)
	raw, err := m.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"D":49,"A":{"B":1},"C":["111","112","113"]}` {
		t.Fatal(string(raw) + ": not right")
	}
	spew.Dump(raw)
}

func TestSetOnNilMap(t *testing.T) {
	// a map created with new() will have a nil inner map
	var m = new(OrderedMap[string, any])
	m.Set("a", 1)
	v, ok := m.Get("a")
	if !ok {
		t.Fatal("expected to get a value, but got none")
	}
	if v.(int) != 1 {
		t.Fatalf("expected value to be 1, but got %v", v)
	}
}

func TestInitEdgeCases_Concurrent(t *testing.T) {
	var m OrderedMap[string, int]
	var wg sync.WaitGroup
	numGoroutines := 100

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", i)
			m.Set(key, i)
		}(i)
	}
	wg.Wait()

	if m.Len() != numGoroutines {
		t.Fatalf("expected len %d, got %d", numGoroutines, m.Len())
	}

	// Verify all values are present
	for i := 0; i < numGoroutines; i++ {
		key := fmt.Sprintf("key-%d", i)
		val, ok := m.Get(key)
		if !ok || val != i {
			t.Fatalf("expected to find key %s with value %d, found %d (ok: %v)", key, i, val, ok)
		}
	}
}

func TestInitEdgeCases_NilReceiver(t *testing.T) {
	var m *OrderedMap[string, int]
	// m is nil
	// Calling any method should not panic because of the nil check in init()
	if m.Len() != 0 {
		t.Fatal("Len on nil map should be 0")
	}
	m.Set("a", 1)
	v, ok := m.Get("a")
	if ok || v != 0 {
		t.Fatal("expected Get on nil map to return zero value and false")
	}
}

func TestKeyOrderAndDeletion(t *testing.T) {
	m := NewEmptyOrderedMap[string, int]()
	m.Set("first", 1)
	m.Set("second", 2)
	m.Set("third", 3)

	keys := m.Keys()
	expectedKeys := []string{"first", "second", "third"}
	if !reflect.DeepEqual(keys, expectedKeys) {
		t.Fatalf("expected keys %v, got %v", expectedKeys, keys)
	}

	// Delete middle one
	m.Delete("second")
	keys = m.Keys()
	expectedKeys = []string{"first", "third"}
	if !reflect.DeepEqual(keys, expectedKeys) {
		t.Fatalf("expected keys %v after delete, got %v", expectedKeys, keys)
	}
	if m.Len() != 2 {
		t.Fatalf("expected len 2 after delete, got %d", m.Len())
	}

	// Delete non-existent key
	m.Delete("non-existent")
	if m.Len() != 2 {
		t.Fatalf("expected len 2 after deleting non-existent key, got %d", m.Len())
	}
}

func TestCopy(t *testing.T) {
	m1 := NewEmptyOrderedMap[string, int]()
	m1.Set("a", 1)
	m1.Set("b", 2)

	m2 := m1.Copy()
	if !reflect.DeepEqual(m1.Keys(), m2.Keys()) {
		t.Fatal("copy should have the same keys in the same order")
	}
	if !reflect.DeepEqual(m1.Values(), m2.Values()) {
		t.Fatal("copy should have the same values in the same order")
	}

	// Modify copy
	m2.Set("c", 3)
	if m1.Len() == m2.Len() {
		t.Fatal("modifying copy should not affect original map's length")
	}

	// Modify original
	m1.Delete("a")
	val, ok := m2.Get("a")
	if !ok || val != 1 {
		t.Fatal("deleting from original should not affect copy")
	}
}

func TestBringKeyToLastOne(t *testing.T) {
	m := NewEmptyOrderedMap[string, int]()
	m.Set("a", 1)
	m.Set("b", 2)
	m.Set("c", 3)

	m.BringKeyToLastOne("a")
	expectedKeys := []string{"b", "c", "a"}
	if !reflect.DeepEqual(m.Keys(), expectedKeys) {
		t.Fatalf("expected keys %v, got %v", expectedKeys, m.Keys())
	}
}
