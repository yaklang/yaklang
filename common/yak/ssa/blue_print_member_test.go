package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func requireBlueprintMemberIDs(t *testing.T, values []Value, want ...int64) {
	t.Helper()
	got := make([]int64, 0, len(values))
	for _, value := range values {
		require.NotNil(t, value)
		got = append(got, value.GetId())
	}
	require.Equal(t, want, got)
}

func TestBlueprintRegisterMemberPreservesLastAssignment(t *testing.T) {
	_, builder := newTestBuilder(t)
	first := builder.EmitUndefined("first")
	second := builder.EmitUndefined("second")

	blueprint := NewBlueprint("Demo")
	blueprint.RegisterNormalMember("field", first, false)
	blueprint.RegisterNormalMember("field", first, false)
	requireBlueprintMemberIDs(t, blueprint.GetNormalMembers("field"), first.GetId())

	blueprint.RegisterNormalMember("field", second, false)
	blueprint.RegisterNormalMember("field", first, false)
	requireBlueprintMemberIDs(t, blueprint.GetNormalMembers("field"), first.GetId(), second.GetId(), first.GetId())
	require.Equal(t, first.GetId(), blueprint.GetNormalMember("field").GetId())

	blueprint.RegisterStaticMember("field", first, false)
	blueprint.RegisterStaticMember("field", first, false)
	requireBlueprintMemberIDs(t, blueprint.GetStaticMembers("field"), first.GetId())

	blueprint.RegisterStaticMember("field", second, false)
	blueprint.RegisterStaticMember("field", first, false)
	requireBlueprintMemberIDs(t, blueprint.GetStaticMembers("field"), first.GetId(), second.GetId(), first.GetId())
	require.Equal(t, first.GetId(), blueprint.GetStaticMember("field").GetId())
}

func TestBlueprintRegisterMemberSkipsNil(t *testing.T) {
	var nilValue Value

	blueprint := NewBlueprint("Demo")
	blueprint.RegisterNormalMember("field", nilValue, false)
	blueprint.RegisterStaticMember("field", nilValue, false)
	blueprint.RegisterConstMember("field", nilValue, false)

	require.Empty(t, blueprint.GetNormalMembers("field"))
	require.Empty(t, blueprint.GetStaticMembers("field"))
	require.Nil(t, blueprint.GetConstMember("field"))
}

func TestBlueprintAddParentPreservesRepeatedLastValue(t *testing.T) {
	_, builder := newTestBuilder(t)
	first := builder.EmitUndefined("first")
	second := builder.EmitUndefined("second")

	parent := NewBlueprint("Parent")
	parent.RegisterNormalMember("field", first, false)
	parent.RegisterNormalMember("field", second, false)
	parent.RegisterNormalMember("field", first, false)
	parent.RegisterStaticMember("field", first, false)
	parent.RegisterStaticMember("field", second, false)
	parent.RegisterStaticMember("field", first, false)

	child := NewBlueprint("Child")
	child.AddParentBlueprint(parent)

	requireBlueprintMemberIDs(t, child.GetNormalMembers("field"), first.GetId(), second.GetId(), first.GetId())
	require.Equal(t, first.GetId(), child.GetNormalMember("field").GetId())
	requireBlueprintMemberIDs(t, child.GetStaticMembers("field"), first.GetId(), second.GetId(), first.GetId())
	require.Equal(t, first.GetId(), child.GetStaticMember("field").GetId())
}

func TestBlueprintAddParentDoesNotOverrideExistingChildMembers(t *testing.T) {
	_, builder := newTestBuilder(t)
	parentNormal := builder.EmitUndefined("parent-normal")
	parentStatic := builder.EmitUndefined("parent-static")
	parentConst := builder.EmitUndefined("parent-const")
	childNormal := builder.EmitUndefined("child-normal")
	childStatic := builder.EmitUndefined("child-static")
	childConst := builder.EmitUndefined("child-const")

	parent := NewBlueprint("Parent")
	parent.RegisterNormalMember("field", parentNormal, false)
	parent.RegisterStaticMember("field", parentStatic, false)
	parent.RegisterConstMember("field", parentConst, false)

	child := NewBlueprint("Child")
	child.RegisterNormalMember("field", childNormal, false)
	child.RegisterStaticMember("field", childStatic, false)
	child.RegisterConstMember("field", childConst, false)
	child.AddParentBlueprint(parent)

	requireBlueprintMemberIDs(t, child.GetNormalMembers("field"), childNormal.GetId())
	require.Equal(t, childNormal.GetId(), child.GetNormalMember("field").GetId())
	requireBlueprintMemberIDs(t, child.GetStaticMembers("field"), childStatic.GetId())
	require.Equal(t, childStatic.GetId(), child.GetStaticMember("field").GetId())
	require.Equal(t, childConst.GetId(), child.GetConstMember("field").GetId())
}

func TestBlueprintAddParentKeepsFirstInheritedMemberForSameName(t *testing.T) {
	_, builder := newTestBuilder(t)
	firstInherited := builder.EmitUndefined("first-parent")
	secondInherited := builder.EmitUndefined("second-parent")

	firstParent := NewBlueprint("FirstParent")
	firstParent.RegisterNormalMember("field", firstInherited, false)
	secondParent := NewBlueprint("SecondParent")
	secondParent.RegisterNormalMember("field", secondInherited, false)

	child := NewBlueprint("Child")
	child.AddParentBlueprint(firstParent)
	child.AddParentBlueprint(secondParent)

	requireBlueprintMemberIDs(t, child.GetNormalMembers("field"), firstInherited.GetId())
	require.Equal(t, firstInherited.GetId(), child.GetNormalMember("field").GetId())
}

func TestBlueprintAddParentDoesNotStoreCopiedMembersOnChildContainer(t *testing.T) {
	_, builder := newTestBuilder(t)
	parent := builder.CreateBlueprint("Parent")
	child := builder.CreateBlueprint("Child")

	normal := builder.EmitUndefined("normal")
	static := builder.EmitUndefined("static")
	constValue := builder.EmitUndefined("const")
	parent.RegisterNormalMember("field", normal)
	parent.RegisterStaticMember("field", static)
	parent.RegisterConstMember("field", constValue)

	child.AddParentBlueprint(parent)

	requireBlueprintMemberIDs(t, child.GetNormalMembers("field"), normal.GetId())
	requireBlueprintMemberIDs(t, child.GetStaticMembers("field"), static.GetId())
	require.Equal(t, constValue.GetId(), child.GetConstMember("field").GetId())

	for _, pair := range GetMemberPairs(child.Container()) {
		require.NotEqual(t, "field", pair.KeyString(), "inherited members should not be stored as direct child container fields")
	}
}

func TestBlueprintStaticMembersAreNotInstanceMembers(t *testing.T) {
	_, builder := newTestBuilder(t)
	class := builder.CreateBlueprint("Class")
	key := builder.EmitConstInst("field")
	staticValue := builder.EmitUndefined("static")
	normalValue := builder.EmitUndefined("normal")

	class.RegisterStaticMember("field", staticValue)
	require.Equal(t, staticValue.GetId(), builder.ReadMemberCallValue(class.Container(), key).GetId())

	instance := builder.EmitEmptyContainer()
	instance.SetType(class)
	instanceField := builder.ReadMemberCallValue(instance, key)
	require.NotEqual(t, staticValue.GetId(), instanceField.GetId())

	class.RegisterNormalMember("field", normalValue)
	instanceWithNormal := builder.EmitEmptyContainer()
	instanceWithNormal.SetType(class)
	require.Equal(t, normalValue.GetId(), builder.ReadMemberCallValue(instanceWithNormal, key).GetId())
}
