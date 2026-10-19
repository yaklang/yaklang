package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConcurrentUnitMethodLookupsDoNotBuildForeignBodies(t *testing.T) {
	app, _ := newTestBuilder(t)
	lib := app.NewLibrary("dependency", []string{"dependency"})
	owner := lib.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	method := owner.NewFunc("method")
	runs := 0
	method.AddLazyBuilder(func() { runs++ })
	class := owner.CreateBlueprint("Dependency")
	class.RegisterNormalMethod("method", method)
	typ := NewBasicType(ObjectTypeKind, "Dependency")
	typ.AddMethod("method", method)

	app.SetConcurrentUnitBuild(true)
	app.SetDeferMethodBuildOnLookup(true)
	require.Same(t, method, class.GetNormalMethod("method"))
	require.Same(t, method, GetMethod(typ, "method"))
	require.Zero(t, runs, "lookups must leave the body for its owning unit")

	lib.LazyBuildCurrentProgram()
	require.Equal(t, 1, runs)

	method.AddLazyBuilder(func() { runs++ })
	app.SetConcurrentUnitBuild(false)
	require.Same(t, method, class.GetNormalMethod("method"))
	require.Equal(t, 2, runs, "serial lookups retain eager-build behavior")
}
