package omap

import (
	"github.com/davecgh/go-spew/spew"
	"testing"
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
