package ssa

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func TestMemberRelations_MultipleMembersSameKey(t *testing.T) {
	prog := NewProgram(context.Background(), t.Name(), ProgramCacheMemory, Application, nil, "", 0)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	obj := builder.EmitEmptyContainer()
	key := builder.EmitConstInst("cmd")
	member1 := builder.EmitUndefined("member1")
	member2 := builder.EmitUndefined("member2")

	setMemberCallRelationship(obj, key, member1)
	setMemberCallRelationship(obj, key, member2)

	members := GetMembersByKey(obj, key)
	require.Len(t, members, 2)
	require.ElementsMatch(t, []int64{member1.GetId(), member2.GetId()}, []int64{members[0].GetId(), members[1].GetId()})

	pairs := GetMemberPairs(obj)
	matched := make([]int64, 0, 2)
	for _, pair := range pairs {
		if pair.Key.GetId() == key.GetId() {
			matched = append(matched, pair.Member.GetId())
		}
	}
	require.ElementsMatch(t, []int64{member1.GetId(), member2.GetId()}, matched)
}

func TestMemberRelations_SharedMemberAcrossObjects(t *testing.T) {
	prog := NewProgram(context.Background(), t.Name(), ProgramCacheMemory, Application, nil, "", 0)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	obj1 := builder.EmitEmptyContainer()
	obj2 := builder.EmitEmptyContainer()
	key := builder.EmitConstInst("cmd")
	shared := builder.EmitUndefined("shared")

	setMemberCallRelationship(obj1, key, shared)
	setMemberCallRelationship(obj2, key, shared)

	pairs := GetObjectKeyPairs(shared)
	require.Len(t, pairs, 2)
	require.ElementsMatch(t, []int64{obj1.GetId(), obj2.GetId()}, []int64{pairs[0].Object.GetId(), pairs[1].Object.GetId()})
	for _, pair := range pairs {
		require.Equal(t, key.GetId(), pair.Key.GetId())
	}
}

func TestMemberRelations_ReadLegacyObjectMembers(t *testing.T) {
	prog := NewProgram(context.Background(), t.Name(), ProgramCacheMemory, Application, nil, "", 0)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	key := builder.EmitConstInst("cmd")
	member := builder.EmitUndefined("legacy")
	loaded := builder.EmitEmptyContainer()

	ir := ssadb.EmptyIrCode(prog.Name, loaded.GetId())
	ir.ObjectMembers = make(ssadb.Int64Map, 0, 1)
	ir.ObjectMembers.Append(key.GetId(), member.GetId())

	prog.Cache.valueFromIrCode(prog.Cache, loaded, ir)

	members := GetMembersByKey(loaded, key)
	require.Len(t, members, 1)
	require.Equal(t, member.GetId(), members[0].GetId())
}

func TestMemberRelations_ReadLegacyObjectOwner(t *testing.T) {
	prog := NewProgram(context.Background(), t.Name(), ProgramCacheMemory, Application, nil, "", 0)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	obj := builder.EmitEmptyContainer()
	key := builder.EmitConstInst("cmd")
	member := builder.EmitUndefined("legacy-owner")

	ir := ssadb.EmptyIrCode(prog.Name, member.GetId())
	ir.IsObjectMember = true
	ir.ObjectParent = obj.GetId()
	ir.ObjectKey = key.GetId()

	prog.Cache.valueFromIrCode(prog.Cache, member, ir)

	pairs := GetObjectKeyPairs(member)
	require.Len(t, pairs, 1)
	require.Equal(t, obj.GetId(), pairs[0].Object.GetId())
	require.Equal(t, key.GetId(), pairs[0].Key.GetId())
}

func TestMemberRelations_PersistPairsToDatabase(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	prog := NewProgram(context.Background(), programName, ProgramCacheDBWrite, Application, filesys.NewVirtualFs(), "", 0, time.Millisecond*100)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	obj := builder.EmitEmptyContainer()
	key := builder.EmitConstInst("cmd")
	member1 := builder.EmitUndefined("member1")
	member2 := builder.EmitUndefined("member2")
	setMemberCallRelationship(obj, key, member1)
	setMemberCallRelationship(obj, key, member2)

	owner1 := builder.EmitEmptyContainer()
	owner2 := builder.EmitEmptyContainer()
	shared := builder.EmitUndefined("shared")
	setMemberCallRelationship(owner1, key, shared)
	setMemberCallRelationship(owner2, key, shared)

	prog.Finish()
	prog.UpdateToDatabase()
	prog.Cache.SaveToDatabase()

	objIR := ssadb.GetIrCodeItemById(ssadb.GetDB(), programName, obj.GetId())
	require.NotNil(t, objIR)
	require.Len(t, objIR.ObjectMemberPairs, 2)
	require.Len(t, objIR.ObjectMembers, 0)

	sharedIR := ssadb.GetIrCodeItemById(ssadb.GetDB(), programName, shared.GetId())
	require.NotNil(t, sharedIR)
	require.Len(t, sharedIR.ObjectOwnerPairs, 2)
	require.Zero(t, sharedIR.ObjectParent)
	require.Zero(t, sharedIR.ObjectKey)
}
