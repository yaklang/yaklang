package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBlueprintAddParentPreservesChildMagicMethods(t *testing.T) {
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

	require.Equal(t, childConstructor.GetId(), child.Constructor.GetId())
	require.Equal(t, childDestructor.GetId(), child.Destructor.GetId())
	require.Equal(t, childConstructor.GetId(), child.MagicMethod[Constructor].GetId())
	require.Equal(t, childDestructor.GetId(), child.MagicMethod[Destructor].GetId())
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

func TestBlueprintAddParentPreservesConcreteMagicAfterPlaceholder(t *testing.T) {
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

	require.Equal(t, childConstructor.GetId(), child.Constructor.GetId())
	require.Equal(t, placeholder.GetId(), child.MagicMethod[Constructor].GetId())
	require.Equal(t, placeholder.GetId(), childConstructor.GetReference().GetId())
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
