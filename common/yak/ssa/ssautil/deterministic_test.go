package ssautil

import (
	"reflect"
	"testing"
)

func newTestVersioned(name string, scope ScopedVersionedTableIF[value]) VersionedIF[value] {
	variable := scope.CreateVariable(name, false)
	scope.AssignVariable(variable, NewConsts(name))
	return variable
}

func TestLinkNodeMapForEachStableOrder(t *testing.T) {
	root := NewRootVersionedTable[value]("test", NewVersioned[value])
	links := newLinkNodeMap[value]()
	links.Append("gamma", newTestVersioned("gamma", root))
	links.Append("alpha", newTestVersioned("alpha", root))
	links.Append("beta", newTestVersioned("beta", root))

	want := []string{"alpha", "beta", "gamma"}
	for i := 0; i < 32; i++ {
		var got []string
		links.ForEach(func(name string, _ VersionedIF[value]) {
			got = append(got, name)
		})
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("stable order violated on iteration %d: got %v want %v", i, got, want)
		}
	}
}

func TestForEachCapturedVariableStableOrder(t *testing.T) {
	root := NewRootVersionedTable[value]("test", NewVersioned[value])
	sub := root.CreateSubScope()
	sub.SetCapturedVariable("gamma", newTestVersioned("gamma", root))
	sub.SetCapturedVariable("alpha", newTestVersioned("alpha", root))
	sub.SetCapturedVariable("beta", newTestVersioned("beta", root))

	want := []string{"alpha", "beta", "gamma"}
	for i := 0; i < 32; i++ {
		var got []string
		sub.ForEachCapturedVariable(func(name string, _ VersionedIF[value]) {
			got = append(got, name)
		})
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("captured variable order violated on iteration %d: got %v want %v", i, got, want)
		}
	}
}

func TestForEachCapturedSideEffectStableOrder(t *testing.T) {
	root := NewRootVersionedTable[value]("test", NewVersioned[value])
	sub := root.CreateSubScope()
	bind := newTestVersioned("bind", root)
	sub.SetCapturedSideEffect("gamma", newTestVersioned("gamma", root), bind)
	sub.SetCapturedSideEffect("alpha", newTestVersioned("alpha", root), bind)
	sub.SetCapturedSideEffect("beta", newTestVersioned("beta", root), bind)

	want := []string{"alpha", "beta", "gamma"}
	for i := 0; i < 32; i++ {
		var got []string
		sub.ForEachCapturedSideEffect(func(name string, _ []VersionedIF[value]) {
			got = append(got, name)
		})
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("captured side-effect order violated on iteration %d: got %v want %v", i, got, want)
		}
	}
}
