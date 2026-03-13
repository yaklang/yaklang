package ssa

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func newTestBuilder(t *testing.T) (*Program, *FunctionBuilder) {
	t.Helper()
	prog := NewProgram(context.Background(), t.Name(), ProgramCacheMemory, Application, nil, "", 0)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	require.NotNil(t, builder)
	return prog, builder
}

func emitObject(builder *FunctionBuilder, name string) Value {
	obj := builder.EmitEmptyContainer()
	obj.SetName(name)
	return obj
}

func TestReplaceMemberCallRebindsMemberObject(t *testing.T) {
	_, builder := newTestBuilder(t)

	holder := emitObject(builder, "holder")
	replacement := emitObject(builder, "replacement")
	member := emitObject(builder, "member-value")
	key := builder.EmitConstInst("field")

	setMemberCallRelationship(holder, key, member)
	owner, ok := GetLatestObjectKeyPair(member)
	require.True(t, ok, "precondition: member should belong to holder")
	require.Equal(t, holder.GetId(), owner.Object.GetId(), "precondition: member should belong to holder")

	expectedName := checkCanMemberCallExist(replacement, key).name
	result := ReplaceMemberCall(holder, replacement)

	require.Contains(t, result, expectedName, "returned mapping should include updated member name")
	updated := result[expectedName]
	updatedOwner, ok := GetLatestObjectKeyPair(updated)
	require.True(t, ok)
	require.Equal(t, replacement.GetId(), updatedOwner.Object.GetId(), "updated member should point to replacement object")

	got, ok := GetLatestMemberByKey(replacement, key)
	require.True(t, ok, "replacement should expose member for key")
	require.Equal(t, updated.GetId(), got.GetId(), "member relationship should persist after replacement")
}

func TestReplaceMemberCallPropagatesNestedMembers(t *testing.T) {
	_, builder := newTestBuilder(t)

	holder := emitObject(builder, "holder")
	replacement := emitObject(builder, "replacement")
	parentKey := builder.EmitConstInst("parent")
	childKey := builder.EmitConstInst("child")

	parentMember := emitObject(builder, "parent-member")
	childMember := emitObject(builder, "child-member")

	setMemberCallRelationship(holder, parentKey, parentMember)
	setMemberCallRelationship(parentMember, childKey, childMember)

	result := ReplaceMemberCall(holder, replacement)

	parentName := checkCanMemberCallExist(replacement, parentKey).name
	require.Contains(t, result, parentName)
	parentUpdated := result[parentName]
	parentOwner, ok := GetLatestObjectKeyPair(parentUpdated)
	require.True(t, ok)
	require.Equal(t, replacement.GetId(), parentOwner.Object.GetId())

	childName := checkCanMemberCallExist(parentUpdated, childKey).name
	require.Contains(t, result, childName)
	childUpdated := result[childName]
	childOwner, ok := GetLatestObjectKeyPair(childUpdated)
	require.True(t, ok)
	require.Equal(t, parentUpdated.GetId(), childOwner.Object.GetId())

	parentValue, ok := GetLatestMemberByKey(replacement, parentKey)
	require.True(t, ok)
	require.Equal(t, parentUpdated.GetId(), parentValue.GetId())

	childValue, ok := GetLatestMemberByKey(parentValue, childKey)
	require.True(t, ok)
	require.Equal(t, childUpdated.GetId(), childValue.GetId())
}

func TestReplaceMemberCallKeepsUndefinedMembers(t *testing.T) {
	_, builder := newTestBuilder(t)

	holder := emitObject(builder, "holder")
	replacement := emitObject(builder, "replacement")
	key := builder.EmitConstInst("missing")

	undefinedMember := builder.EmitValueOnlyDeclare("undefined-member")
	setMemberCallRelationship(holder, key, undefinedMember)

	result := ReplaceMemberCall(holder, replacement)

	name := checkCanMemberCallExist(replacement, key).name
	require.Contains(t, result, name)
	updated := result[name]
	_, ok := ToUndefined(updated)
	require.True(t, ok, "undefined member should remain undefined after replacement")
}

func TestReplaceMemberCallOnExternLib(t *testing.T) {
	prog, builder := newTestBuilder(t)

	libName := "lib"
	libTable := map[string]any{
		"field": map[string]any{
			"value": "payload",
		},
	}
	prog.ExternLib[libName] = libTable
	entry := NewExternLib(libName, builder, libTable)

	holder := emitObject(builder, "holder")
	key := builder.EmitConstInst("field")
	setMemberCallRelationship(holder, key, emitObject(builder, "member-from-lib"))

	replacement := builder.ReadMemberCallValue(entry, key)
	result := ReplaceMemberCall(holder, replacement)

	name := checkCanMemberCallExist(replacement, key).name
	require.Contains(t, result, name)
	updated := result[name]
	updatedOwner, ok := GetLatestObjectKeyPair(updated)
	require.True(t, ok)
	require.Equal(t, replacement.GetId(), updatedOwner.Object.GetId())
}
