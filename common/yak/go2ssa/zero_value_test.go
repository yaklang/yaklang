package go2ssa

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func zeroValueTestBuilder(t *testing.T) *astbuilder {
	t.Helper()
	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(t.Name()))
	require.NoError(t, err)
	prog := ssa.NewProgram(cfg, ssa.ProgramCacheMemory, ssa.Application, nil, "", 0)
	builder := prog.GetAndCreateFunctionBuilder("", string(ssa.MainFunctionName))
	require.NotNil(t, builder)
	return &astbuilder{FunctionBuilder: builder}
}

// Match the four repeated type identities in ARTEX's pre-fix goroutine dump.
// Malformed/incomplete type graphs must not allocate an unbounded stream of IR.
func TestGoDefaultValueTypeCycle(t *testing.T) {
	b := zeroValueTestBuilder(t)
	types := make([]*ssa.ObjectType, 4)
	for i := range types {
		types[i] = ssa.NewStructType()
	}
	for i, typ := range types {
		typ.AddField(b.EmitConstInst("Next"), types[(i+1)%len(types)])
	}
	root := b.GetDefaultValue(types[0])
	value := root
	for range types {
		value = b.ReadMemberCallValueByName(value, "Next")
		require.NotNil(t, value)
	}
	require.True(t, value.IsUndefined(), "a malformed by-value cycle needs a typed unknown, not infinite materialization")
	require.Same(t, types[0], value.GetType())
	var instructions int
	for _, id := range b.Function.Blocks {
		block, ok := b.Function.GetBasicBlockByID(id)
		require.True(t, ok)
		instructions += len(block.Insts)
	}
	require.Less(t, instructions, 100, "four types must generate bounded IR")
}

func TestGoDefaultValueDoesNotShareSiblingObjects(t *testing.T) {
	b := zeroValueTestBuilder(t)
	leaf := ssa.NewStructType()
	leaf.AddField(b.EmitConstInst("Count"), ssa.CreateNumberType())
	pair := ssa.NewStructType()
	pair.AddField(b.EmitConstInst("Left"), leaf)
	pair.AddField(b.EmitConstInst("Right"), leaf)
	value := b.GetDefaultValue(pair)
	left := b.ReadMemberCallValueByName(value, "Left")
	right := b.ReadMemberCallValueByName(value, "Right")
	require.NotEqual(t, left.GetId(), right.GetId(), "zero structs are independent mutable values")
	require.Equal(t, "0", b.ReadMemberCallValueByName(left, "Count").String())
	require.Equal(t, "0", b.ReadMemberCallValueByName(right, "Count").String())
	b.AssignVariable(b.CreateMemberCallVariable(left, b.EmitConstInst("Count")), b.EmitConstInst(7))
	require.Equal(t, "7", b.ReadMemberCallValueByName(left, "Count").String())
	require.Equal(t, "0", b.ReadMemberCallValueByName(right, "Count").String())
}

func TestGoDefaultValueDeepAcyclicStruct(t *testing.T) {
	b := zeroValueTestBuilder(t)
	var typ ssa.Type = ssa.CreateNumberType()
	const depth = 64
	for i := 0; i < depth; i++ {
		parent := ssa.NewStructType()
		parent.AddField(b.EmitConstInst("Child"), typ)
		typ = ssa.NewAliasType(fmt.Sprintf("Level%d", i), "", parent)
	}
	value := b.GetDefaultValue(typ)
	for i := 0; i < depth; i++ {
		value = b.ReadMemberCallValueByName(value, "Child")
		require.NotNil(t, value)
	}
	require.Equal(t, "0", value.String(), "a guard must not truncate valid deep structs or aliases")
}
