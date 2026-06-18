package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func newTestBuilder(t *testing.T) (*Program, *FunctionBuilder) {
	t.Helper()
	cfg, _ := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(t.Name()))
	prog := NewProgram(cfg, ProgramCacheMemory, Application, nil, "", 0)
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

func TestReplaceMemberCallMergesMemberWithoutResolvedOwner(t *testing.T) {
	_, builder := newTestBuilder(t)

	holder := emitObject(builder, "holder")
	replacement := emitObject(builder, "replacement")
	key := builder.EmitConstInst("field")
	existing := builder.ReadMemberCallValue(replacement, key)
	member := builder.EmitConstInst("payload")

	require.NotNil(t, existing)
	holder.AddMember(key, member)

	name := checkCanMemberCallExist(replacement, key).name
	result := ReplaceMemberCall(holder, replacement)

	require.Contains(t, result, name)
	updated := result[name]
	_, ok := ToPhi(updated)
	require.True(t, ok, "replacement should merge existing and moved members")

	latest, ok := GetLatestMemberByKey(replacement, key)
	require.True(t, ok)
	require.Equal(t, updated.GetId(), latest.GetId())
}

func TestReplaceMemberCallMergesExistingMemberPairBeforeScopeLookup(t *testing.T) {
	_, builder := newTestBuilder(t)

	holder := emitObject(builder, "holder")
	replacement := emitObject(builder, "replacement")
	key := builder.EmitConstInst("field")
	existing := builder.EmitConstInst("old")
	member := builder.EmitConstInst("new")

	setMemberCallRelationship(replacement, key, existing)
	holder.AddMember(key, member)

	name := checkCanMemberCallExist(replacement, key).name
	result := ReplaceMemberCall(holder, replacement)

	updated := result[name]
	phi, ok := ToPhi(updated)
	require.True(t, ok, "replacement should merge moved and existing members")
	require.Len(t, phi.Edge, 2)

	first, ok := phi.GetValueById(phi.Edge[0])
	require.True(t, ok)
	require.Equal(t, member.GetId(), first.GetId())
	second, ok := phi.GetValueById(phi.Edge[1])
	require.True(t, ok)
	require.Equal(t, existing.GetId(), second.GetId())
}

func TestReplaceMemberCallMergesTargetWhenMovedMemberBelongsToOtherKey(t *testing.T) {
	_, builder := newTestBuilder(t)

	holder := emitObject(builder, "holder")
	replacement := emitObject(builder, "replacement")
	targetKey := builder.EmitConstInst("field")
	sourceKey := builder.EmitConstInst("source")
	existing := builder.EmitConstInst("old")
	member := builder.EmitConstInst("new")

	setMemberCallRelationship(replacement, targetKey, existing)
	setMemberCallRelationship(replacement, sourceKey, member)
	holder.AddMember(targetKey, member)

	name := checkCanMemberCallExist(replacement, targetKey).name
	result := ReplaceMemberCall(holder, replacement)

	updated := result[name]
	phi, ok := ToPhi(updated)
	require.True(t, ok, "target key should merge moved source member with existing target member")
	require.Len(t, phi.Edge, 2)

	first, ok := phi.GetValueById(phi.Edge[0])
	require.True(t, ok)
	require.Equal(t, member.GetId(), first.GetId())
	second, ok := phi.GetValueById(phi.Edge[1])
	require.True(t, ok)
	require.Equal(t, existing.GetId(), second.GetId())
}

func TestReplaceMemberCallResolvesUndefinedSourceMemberBeforeMergingTarget(t *testing.T) {
	_, builder := newTestBuilder(t)

	holder := emitObject(builder, "holder")
	replacement := emitObject(builder, "replacement")
	targetKey := builder.EmitConstInst("field")
	sourceKey := builder.EmitConstInst("source")
	existing := builder.EmitConstInst("old")
	source := builder.EmitConstInst("new")
	placeholder := builder.EmitValueOnlyDeclare("placeholder")

	setMemberCallRelationship(replacement, targetKey, existing)
	setMemberCallRelationship(replacement, sourceKey, source)
	setMemberCallRelationship(holder, sourceKey, placeholder)
	holder.AddMember(targetKey, placeholder)
	AddObjectKeyPair(placeholder, holder, targetKey)

	name := checkCanMemberCallExist(replacement, targetKey).name
	result := ReplaceMemberCall(holder, replacement)

	updated := result[name]
	phi, ok := ToPhi(updated)
	require.True(t, ok, "undefined source placeholder should resolve through replacement source key")
	require.Len(t, phi.Edge, 2)

	first, ok := phi.GetValueById(phi.Edge[0])
	require.True(t, ok)
	require.Equal(t, source.GetId(), first.GetId())
	second, ok := phi.GetValueById(phi.Edge[1])
	require.True(t, ok)
	require.Equal(t, existing.GetId(), second.GetId())
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
