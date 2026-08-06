package ssa

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func newMemberRelationsTestProgram(t *testing.T, programName string, kind ProgramCacheKind, fs fi.FileSystem) *Program {
	t.Helper()
	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(programName))
	require.NoError(t, err)
	if fs == nil {
		fs = filesys.NewVirtualFs()
	}
	return NewProgram(cfg, kind, Application, fs, "", 0)
}

func TestMemberRelations_MultipleMembersSameKey(t *testing.T) {
	_, builder := newTestBuilder(t)

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
	_, builder := newTestBuilder(t)

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

func TestMemberRelations_KeyString(t *testing.T) {
	_, builder := newTestBuilder(t)

	obj := builder.EmitEmptyContainer()
	key := builder.EmitConstInst("cmd")
	member := builder.EmitUndefined("member")
	setMemberCallRelationship(obj, key, member)

	memberPairs := GetMemberPairs(obj)
	require.Len(t, memberPairs, 1)
	require.Equal(t, "cmd", memberPairs[0].KeyString())

	ownerPairs := GetObjectKeyPairs(member)
	require.Len(t, ownerPairs, 1)
	require.Equal(t, "cmd", ownerPairs[0].KeyString())
}

func TestMemberRelations_ReadLegacyObjectMembers(t *testing.T) {
	prog, builder := newTestBuilder(t)

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
	prog, builder := newTestBuilder(t)

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

	prog := newMemberRelationsTestProgram(t, programName, ProgramCacheDBWrite, filesys.NewVirtualFs())
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
