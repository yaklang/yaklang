package ssautil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Embed only the existing interface, so this scope deliberately has no fast
// FindVariableByName method. Third-party scope implementations stay compatible.
type snapshotOnlyScope struct {
	ScopedVersionedTableIF[*dummySSAValue]
}

func TestFindVariableByNameScopeOrderAndFallback(t *testing.T) {
	root := NewRootVersionedTable[*dummySSAValue]("", NewVersioned[*dummySSAValue])
	parent := root.CreateVariable("x", true)
	root.AssignVariable(parent, &dummySSAValue{id: 1})
	child := root.CreateSubScope()
	local := child.CreateVariable("x", true)
	child.AssignVariable(local, &dummySSAValue{id: 2})
	latest := child.CreateVariable("x", false)
	child.AssignVariable(latest, &dummySSAValue{id: 3})

	for _, scope := range []ScopedVersionedTableIF[*dummySSAValue]{child, snapshotOnlyScope{child}} {
		var visits []VersionedIF[*dummySSAValue]
		found := FindVariableByName(scope, "x", false, func(v VersionedIF[*dummySSAValue]) bool {
			visits = append(visits, v)
			return v == local
		})
		require.Same(t, local, found)
		require.Equal(t, []VersionedIF[*dummySSAValue]{latest, local}, visits)
		require.Nil(t, FindVariableByName(scope, "x", true, func(v VersionedIF[*dummySSAValue]) bool { return v == parent }))
		require.Same(t, parent, FindVariableByName(scope, "x", false, func(v VersionedIF[*dummySSAValue]) bool { return v == parent }))
		require.Nil(t, FindVariableByName(scope, "missing", false, func(VersionedIF[*dummySSAValue]) bool {
			t.Fatal("missing names must not invoke the predicate")
			return true
		}))
	}
}
