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
