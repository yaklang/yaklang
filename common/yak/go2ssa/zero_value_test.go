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

func TestGoDefaultValueUnknownTypeDoesNotInventNumber(t *testing.T) {
	for _, typ := range []ssa.Type{ssa.CreateAnyType(), ssa.NewBlueprint("ExternalState")} {
		t.Run(typ.String(), func(t *testing.T) {
			b := zeroValueTestBuilder(t)
			value := b.GetDefaultValue(typ)
			require.True(t, value.IsUndefined(), "an unresolved zero must remain unknown, never a known numeric zero")
			require.Same(t, typ, value.GetType(), "unknown zero values must preserve the available type identity")
		})
	}
}

// Match the four repeated type identities in ARTEX's pre-fix goroutine dump.
// Malformed/incomplete type graphs must not allocate an unbounded stream of IR.
// The expansion guard degrades a type already on the active path to a typed
// placeholder, so the contract is boundedness plus typing, not one concrete
// value kind.
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
	require.NotNil(t, value.GetType(), "a degraded cycle node must stay typed")
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
	// The expansion guard counts recursion per level, so a legal acyclic chain
	// within the budget must materialize completely instead of degrading.
	const depth = 16
	value := b.GetDefaultValue(aliasWrappedChain(t, b, depth))
	for i := 0; i < depth; i++ {
		value = b.ReadMemberCallValueByName(value, "Child")
		require.NotNil(t, value)
	}
	require.Equal(t, "0", value.String(), "a guard must not truncate valid deep structs or aliases")
}

// Past the budget the expansion degrades to a typed placeholder: it must still
// terminate with bounded IR instead of walking the whole chain.
func TestGoDefaultValueDeepAcyclicStructDegradesBoundedly(t *testing.T) {
	b := zeroValueTestBuilder(t)
	const depth = 200
	value := b.GetDefaultValue(aliasWrappedChain(t, b, depth))
	require.NotNil(t, value)
	require.NotNil(t, value.GetType())
	var instructions int
	for _, id := range b.Function.Blocks {
		block, ok := b.Function.GetBasicBlockByID(id)
		require.True(t, ok)
		instructions += len(block.Insts)
	}
	require.Less(t, instructions, 4*defaultValueNodeLimit, "the node budget must bound the expansion")
}

func aliasWrappedChain(t *testing.T, b *astbuilder, depth int) ssa.Type {
	t.Helper()
	var typ ssa.Type = ssa.CreateNumberType()
	for i := 0; i < depth; i++ {
		parent := ssa.NewStructType()
		parent.AddField(b.EmitConstInst("Child"), typ)
		typ = ssa.NewAliasType(fmt.Sprintf("Level%d", i), "", parent)
	}
	return typ
}
