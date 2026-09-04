package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBlueprintAddParentKeepsHistoricalParentEffectiveMagicMethods(t *testing.T) {
	_, builder := newTestBuilder(t)
	parent := builder.CreateBlueprint("Parent")
	child := builder.CreateBlueprint("Child")

	parentConstructor := builder.NewFunc("parent-constructor")
	parentDestructor := builder.NewFunc("parent-destructor")
	childConstructor := builder.NewFunc("child-constructor")
	childDestructor := builder.NewFunc("child-destructor")
	parent.RegisterMagicMethod(Constructor, parentConstructor)
	parent.RegisterMagicMethod(Destructor, parentDestructor)
	child.RegisterMagicMethod(Constructor, childConstructor)
	child.RegisterMagicMethod(Destructor, childDestructor)

	child.AddParentBlueprint(parent)

	require.Equal(t, parentConstructor.GetId(), child.Constructor.GetId())
	require.Equal(t, parentDestructor.GetId(), child.Destructor.GetId())
	require.Equal(t, childConstructor.GetId(), child.MagicMethod[Constructor].GetId())
	require.Equal(t, childDestructor.GetId(), child.MagicMethod[Destructor].GetId())
	require.Equal(t, childConstructor.GetId(), parentConstructor.GetReference().GetId())
	require.Equal(t, childDestructor.GetId(), parentDestructor.GetReference().GetId())
	require.Contains(t, childConstructor.GetPointer(), Value(parentConstructor))
	require.Contains(t, childDestructor.GetPointer(), Value(parentDestructor))
}

func TestBlueprintAddParentInheritsMagicMethodsWhenChildHasNone(t *testing.T) {
	_, builder := newTestBuilder(t)
	parent := builder.CreateBlueprint("Parent")
	child := builder.CreateBlueprint("Child")

	parentConstructor := builder.NewFunc("parent-constructor")
	parentDestructor := builder.NewFunc("parent-destructor")
	parent.RegisterMagicMethod(Constructor, parentConstructor)
	parent.RegisterMagicMethod(Destructor, parentDestructor)

	child.AddParentBlueprint(parent)

	require.Equal(t, parentConstructor.GetId(), child.Constructor.GetId())
	require.Equal(t, parentDestructor.GetId(), child.Destructor.GetId())
}

func TestBlueprintAddParentReplacesUndefinedMagicPlaceholder(t *testing.T) {
	_, builder := newTestBuilder(t)
	parent := builder.CreateBlueprint("Parent")
	child := builder.CreateBlueprint("Child")

	parentConstructor := builder.NewFunc("parent-constructor")
	parent.RegisterMagicMethod(Constructor, parentConstructor)
	placeholder := builder.EmitUndefined("child-constructor-placeholder")
	child.Constructor = placeholder
	child.MagicMethod[Constructor] = placeholder

	child.AddParentBlueprint(parent)

	require.Equal(t, parentConstructor.GetId(), child.Constructor.GetId())
	require.Equal(t, placeholder.GetId(), child.MagicMethod[Constructor].GetId())
	require.Equal(t, placeholder.GetId(), parentConstructor.GetReference().GetId())
}

func TestBlueprintAddParentUsesParentEffectiveMagicAfterChildPlaceholder(t *testing.T) {
	_, builder := newTestBuilder(t)
	parent := builder.CreateBlueprint("Parent")
	child := builder.CreateBlueprint("Child")

	parentConstructor := builder.NewFunc("parent-constructor")
	parent.RegisterMagicMethod(Constructor, parentConstructor)
	placeholder := builder.EmitUndefined("child-constructor-placeholder")
	child.Constructor = placeholder
	child.MagicMethod[Constructor] = placeholder
	childConstructor := builder.NewFunc("child-constructor")
	child.RegisterMagicMethod(Constructor, childConstructor)

	child.AddParentBlueprint(parent)

	require.Equal(t, parentConstructor.GetId(), child.Constructor.GetId())
	require.Equal(t, placeholder.GetId(), child.MagicMethod[Constructor].GetId())
	require.Equal(t, placeholder.GetId(), childConstructor.GetReference().GetId())
	require.Equal(t, placeholder.GetId(), parentConstructor.GetReference().GetId())
	require.Contains(t, placeholder.GetPointer(), Value(childConstructor))
	require.Contains(t, placeholder.GetPointer(), Value(parentConstructor))
}

func TestBlueprintMagicRegisteredAfterParentBecomesEffective(t *testing.T) {
	_, builder := newTestBuilder(t)
	parent := builder.CreateBlueprint("Parent")
	child := builder.CreateBlueprint("Child")

	parentConstructor := builder.NewFunc("parent-constructor")
	childConstructor := builder.NewFunc("child-constructor")
	parent.RegisterMagicMethod(Constructor, parentConstructor)
	child.AddParentBlueprint(parent)
	child.RegisterMagicMethod(Constructor, childConstructor)

	require.Equal(t, childConstructor.GetId(), child.Constructor.GetId())
	require.Equal(t, parentConstructor.GetId(), child.MagicMethod[Constructor].GetId())
	require.Equal(t, parentConstructor.GetId(), childConstructor.GetReference().GetId())
	require.Contains(t, parentConstructor.GetPointer(), Value(childConstructor))
}

func TestBlueprintGetMagicMethodReturnsDestructorSlot(t *testing.T) {
	_, builder := newTestBuilder(t)
	class := builder.CreateBlueprint("Class")

	constructor := builder.NewFunc("constructor")
	destructor := builder.NewFunc("destructor")
	class.RegisterMagicMethod(Constructor, constructor)
	class.RegisterMagicMethod(Destructor, destructor)

	got := class.GetMagicMethod(Destructor, builder)
	require.NotNil(t, got)
	require.Equal(t, destructor.GetId(), got.GetId())
}

func TestBlueprintRegisterMagicMethodGuardsNilSelfAndDuplicatePointers(t *testing.T) {
	_, builder := newTestBuilder(t)
	class := builder.CreateBlueprint("Class")
	method := builder.NewFunc("method")

	require.NotPanics(t, func() {
		var nilBlueprint *Blueprint
		nilBlueprint.RegisterMagicMethod(Constructor, method)
		class.RegisterMagicMethod(Constructor, nil)
	})

	class.MagicMethod[Constructor] = nil
	class.RegisterMagicMethod(Constructor, method)
	require.Equal(t, method.GetId(), class.Constructor.GetId())
	require.Equal(t, method.GetId(), class.MagicMethod[Constructor].GetId())

	class.RegisterMagicMethod(Constructor, method)
	require.Nil(t, method.GetReference(), "registering the same function must not create a self-reference")
	require.Empty(t, method.GetPointer(), "registering the same function must not create a self-pointer")

	inherited := builder.NewFunc("inherited")
	class.RegisterMagicMethod(Constructor, inherited)
	class.RegisterMagicMethod(Constructor, inherited)
	require.Equal(t, method.GetId(), inherited.GetReference().GetId())
	require.Len(t, method.GetPointer(), 1, "registering the same pointer edge twice must not append a duplicate")
}

func TestBlueprintAddParentInheritsConcreteMagicThroughPlaceholderParent(t *testing.T) {
	_, builder := newTestBuilder(t)
	base := builder.CreateBlueprint("Base")
	child := builder.CreateBlueprint("Child")
	grandchild := builder.CreateBlueprint("Grandchild")

	baseConstructor := builder.NewFunc("base-constructor")
	base.RegisterMagicMethod(Constructor, baseConstructor)
	for _, blueprint := range []*Blueprint{child, grandchild} {
		placeholder := builder.EmitUndefined(blueprint.Name + "-constructor-placeholder")
		blueprint.Constructor = placeholder
		blueprint.MagicMethod[Constructor] = placeholder
	}

	child.AddParentBlueprint(base)
	grandchild.AddParentBlueprint(child)

	require.Equal(t, baseConstructor.GetId(), child.Constructor.GetId())
	require.Equal(t, baseConstructor.GetId(), grandchild.Constructor.GetId())
}
