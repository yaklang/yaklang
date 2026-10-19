package ssautil

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// BenchmarkAssignVariableCaptureCheck measures the capture check in
// ScopedVersionedTable.AssignVariable: assigning a non-local variable
// repeatedly under the same name in a sub scope. Rescanning the whole
// version chain on every assignment (LinkNode.All) costs O(n^2) for n
// assignments; the per-name local index set keeps it O(n).
func BenchmarkAssignVariableCaptureCheck(b *testing.B) {
	for _, n := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("assigns=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				root := NewRootVersionedTable[value]("", NewVersioned[value])
				sub := root.CreateSubScope()
				for j := 0; j < n; j++ {
					writeVariable(sub, "x", NewConsts(j))
				}
			}
		})
	}
}

// TestAssignVariableCaptureCheckLocalIndexEquivalence pins the capture
// check in AssignVariable: a non-local assignment skips capture
// registration only when its captured target is itself a local version
// of the same name in the current scope; every other case falls through
// to tryRegisterCapturedVariable.
func TestAssignVariableCaptureCheckLocalIndexEquivalence(t *testing.T) {
	t.Run("captured target is a local version in current scope: no capture", func(t *testing.T) {
		test := assert.New(t)
		root := NewRootVersionedTable[value]("", NewVersioned[value])
		writeVariable(root, "a", one)

		sub := root.CreateSubScope()
		writeLocalVariable(sub, "a", two)
		local := sub.ReadVariable("a")
		test.True(local.GetLocal())

		v := sub.CreateVariable("a", false)
		v.SetCaptured(local)
		sub.AssignVariable(v, two)

		sub.ForEachCapturedVariable(func(name string, ver VersionedIF[value]) {
			test.NotEqual("a", name)
		})
	})

	t.Run("captured target is a local version with another index: register capture", func(t *testing.T) {
		test := assert.New(t)
		root := NewRootVersionedTable[value]("", NewVersioned[value])
		writeVariable(root, "a", one)

		sub := root.CreateSubScope()
		writeLocalVariable(sub, "a", two)
		shadow := sub.CreateVariable("a", true)

		v := sub.CreateVariable("a", false)
		v.SetCaptured(shadow)
		sub.AssignVariable(v, two)

		var captured []string
		sub.ForEachCapturedVariable(func(name string, ver VersionedIF[value]) {
			captured = append(captured, name)
		})
		test.Equal([]string{"a"}, captured)
		test.Equal(v, sub.ReadVariable("a"))
	})

	t.Run("fresh non-local variable with parent version: rebind capture to parent", func(t *testing.T) {
		test := assert.New(t)
		root := NewRootVersionedTable[value]("", NewVersioned[value])
		writeVariable(root, "a", one)
		parentVar := root.ReadVariable("a")

		sub := root.CreateSubScope()
		v := sub.CreateVariable("a", false)
		sub.AssignVariable(v, one)

		var found VersionedIF[value]
		sub.ForEachCapturedVariable(func(name string, ver VersionedIF[value]) {
			if name == "a" {
				found = ver
			}
		})
		test.NotNil(found)
		test.Equal(parentVar.GetGlobalIndex(), v.GetCaptured().GetGlobalIndex())
	})

	t.Run("fresh non-local variable unknown to parent: no capture", func(t *testing.T) {
		test := assert.New(t)
		root := NewRootVersionedTable[value]("", NewVersioned[value])
		sub := root.CreateSubScope()

		v := sub.CreateVariable("b", false)
		sub.AssignVariable(v, one)

		sub.ForEachCapturedVariable(func(name string, ver VersionedIF[value]) {
			test.NotEqual("b", name)
		})
		test.Equal(v.GetGlobalIndex(), v.GetCaptured().GetGlobalIndex())
	})
}
