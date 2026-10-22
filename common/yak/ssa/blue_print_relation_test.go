package ssa

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

func TestBlueprintRelationsDoNotReportMissingInternalMembers(t *testing.T) {
	prog, builder := newTestBuilder(t)
	builder.SetLanguageConfig(
		WithLanguageConfigSupportClass(true),
		WithLanguageConfigIsSupportClassStaticModifier(true),
	)
	class := builder.CreateBlueprint("Class")
	parent := builder.CreateBlueprint("Parent")
	iface := builder.CreateInterface("Interface")

	// ObjectError is only recorded for instructions with a source range.
	// Give every container a range so this test exercises the diagnostic path
	// used by frontends rather than merely checking the relationship graph.
	rng := memedit.NewMemEditor("class Class : Parent, Interface {}").GetFullRange()
	class.Container().SetRange(rng)
	parent.Container().SetRange(rng)
	iface.Container().SetRange(rng)

	class.AddParentBlueprint(parent)
	class.AddInterfaceBlueprint(iface)

	require.Empty(t, prog.GetErrors(), prog.GetErrors().String())

	relations := []struct {
		container Value
		name      BlueprintRelationKind
		want      Value
	}{
		{class.Container(), BlueprintRelationParents, parent.Container()},
		{parent.Container(), BlueprintRelationInherit, class.Container()},
		{class.Container(), BlueprintRelationInterface, iface.Container()},
		{iface.Container(), BlueprintRelationImpl, class.Container()},
	}
	for _, relation := range relations {
		members := relation.container.GetMembersByKeyString(string(relation.name))
		require.Len(t, members, 1, "relation %s should be stored exactly once", relation.name)
		require.Equal(t, relation.want.GetId(), members[0].GetId())
	}
}

func TestBlueprintRelationOnlyDoesNotCreateNameOnlyMethodEdges(t *testing.T) {
	_, builder := newTestBuilder(t)
	parent := builder.CreateBlueprint("Parent")
	child := builder.CreateBlueprint("Child")

	parentMethod := builder.NewFunc("parent-int-overload")
	childMethod := builder.NewFunc("child-string-overload")
	parent.RegisterNormalMethodExact("M", parentMethod)
	child.RegisterNormalMethodExact("M", childMethod)

	child.AddParentBlueprintRelationOnly(parent)

	require.Equal(t, []*Blueprint{parent}, child.GetParentBlueprint())
	require.Same(t, childMethod, child.GetNormalMethod("M"))
	require.Nil(t, childMethod.GetReference(), "unrelated same-name methods must not become overrides")
	require.Empty(t, parentMethod.GetPointer(), "relation-only inheritance must not add a reverse dispatch edge")
	parentType, ok := ToFunctionType(parentMethod.GetType())
	require.True(t, ok)
	require.Same(t, parent, parentType.ObjectType, "the parent method owner must not be rewritten to the child")
}

func TestBlueprintRelationOnlyStillFindsInheritedMethods(t *testing.T) {
	_, builder := newTestBuilder(t)
	parent := builder.CreateBlueprint("Parent")
	child := builder.CreateBlueprint("Child")

	parentMethod := builder.NewFunc("parent-method")
	parent.RegisterNormalMethodExact("M", parentMethod)
	child.AddParentBlueprintRelationOnly(parent)

	require.Same(t, parentMethod, child.GetNormalMethod("M"))
	parentType, ok := ToFunctionType(parentMethod.GetType())
	require.True(t, ok)
	require.Same(t, parent, parentType.ObjectType, "inherited lookup must retain the declaring owner")
}

// Relation registration must not execute either declaration's lazy body or
// create assignments in their source scopes. Those bodies belong to unit workers.
func TestBlueprintRelationDoesNotBuildOrMutateDeclarationScope(t *testing.T) {
	_, builder := newTestBuilder(t)
	parent := builder.CreateBlueprint("Parent")
	child := builder.CreateBlueprint("Child")
	parentName, childName := parent.Container().GetVerboseName(), child.Container().GetVerboseName()
	insts := append([]int64(nil), builder.CurrentBlock.Insts...)
	built := false
	parent.AddLazyBuilder(func() { built = true })
	child.AddLazyBuilder(func() { built = true })

	child.AddParentBlueprintRelationOnly(parent)

	require.False(t, built)
	require.Equal(t, insts, builder.CurrentBlock.Insts)
	require.Equal(t, parentName, parent.Container().GetVerboseName())
	require.Equal(t, childName, child.Container().GetVerboseName())
	require.Len(t, parent.Container().GetMembersByKeyString(string(BlueprintRelationInherit)), 1)
	require.Len(t, child.Container().GetMembersByKeyString(string(BlueprintRelationParents)), 1)
}

func TestBlueprintConcurrentReverseRelations(t *testing.T) {
	_, builder := newTestBuilder(t)
	parent := builder.CreateBlueprint("Parent")
	const count = 32
	children := make([]*Blueprint, count)
	for i := range children {
		children[i] = builder.CreateBlueprint(fmt.Sprintf("Child%d", i))
	}
	insts := append([]int64(nil), builder.CurrentBlock.Insts...)
	var wg sync.WaitGroup
	for _, child := range children {
		wg.Add(1)
		go func(child *Blueprint) {
			defer wg.Done()
			child.AddParentBlueprintRelationOnly(parent)
		}(child)
	}
	wg.Wait()
	require.Equal(t, insts, builder.CurrentBlock.Insts)
	require.Len(t, parent.Container().GetMembersByKeyString(string(BlueprintRelationInherit)), count)
	for _, child := range children {
		require.Len(t, child.Container().GetMembersByKeyString(string(BlueprintRelationParents)), 1)
	}
}
