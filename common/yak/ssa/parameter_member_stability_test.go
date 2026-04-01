package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

func emitDynamicTestKey(t *testing.T, builder *FunctionBuilder, fileName, source string, start, end int, name string) Value {
	t.Helper()

	editor := memedit.NewMemEditor(source)
	editor.SetProgramName(builder.GetProgram().GetProgramName())
	editor.SetFileName(fileName)

	originalRange := builder.CurrentRange
	builder.CurrentRange = editor.GetRangeOffset(start, end)
	key := builder.EmitUndefined(name)
	builder.CurrentRange = originalRange
	return key
}

func TestReadValueByVariableReusesParameterMember(t *testing.T) {
	_, builder := newTestBuilder(t)

	thisParam := builder.NewParam("$this")
	memberVar := builder.CreateMemberCallVariable(thisParam, builder.EmitConstInst("BlockTypes"))
	require.NotNil(t, memberVar)

	first := builder.ReadValueByVariable(memberVar)
	memberVarAgain := builder.CreateMemberCallVariable(thisParam, builder.EmitConstInst("BlockTypes"))
	require.NotNil(t, memberVarAgain)
	second := builder.ReadValueByVariable(memberVarAgain)
	require.NotNil(t, first)
	require.NotNil(t, second)
	require.Equal(t, first.GetId(), second.GetId(), "repeated reads should reuse the same parameter member")
	require.Len(t, builder.Function.ParameterMembers, 1, "parameter member should not be duplicated on repeated reads")
}

func TestReadMemberCallValueReusesNestedParameterMember(t *testing.T) {
	_, builder := newTestBuilder(t)

	thisParam := builder.NewParam("$this")
	blockTypesValue := builder.ReadMemberCallValue(thisParam, builder.EmitConstInst("BlockTypes"))
	require.NotNil(t, blockTypesValue)

	first := builder.ReadMemberCallValue(blockTypesValue, builder.EmitConstInst(0))
	second := builder.ReadMemberCallValue(blockTypesValue, builder.EmitConstInst(0))
	require.NotNil(t, first)
	require.NotNil(t, second)
	require.Equal(t, first.GetId(), second.GetId(), "nested parameter member reads should reuse the same value")
	require.Len(t, builder.Function.ParameterMembers, 2, "expected one top-level and one nested parameter member")
}

func TestReadValueByVariableReusesDynamicParameterMemberFromSameSourceKey(t *testing.T) {
	_, builder := newTestBuilder(t)

	thisParam := builder.NewParam("$this")
	keySource := "$function"
	firstKey := emitDynamicTestKey(t, builder, "dynamic_same.php", keySource, 0, 8, "$function")
	secondKey := emitDynamicTestKey(t, builder, "dynamic_same.php", keySource, 0, 8, "$function")

	firstVar := builder.CreateMemberCallVariable(thisParam, firstKey)
	secondVar := builder.CreateMemberCallVariable(thisParam, secondKey)
	require.NotNil(t, firstVar)
	require.NotNil(t, secondVar)

	first := builder.ReadValueByVariable(firstVar)
	second := builder.ReadValueByVariable(secondVar)
	require.NotNil(t, first)
	require.NotNil(t, second)
	require.Equal(t, first.GetId(), second.GetId(), "same dynamic source key should reuse the same parameter member")
	require.Len(t, builder.Function.ParameterMembers, 1, "dynamic key reuse should not create duplicate parameter members")
}

func TestReadValueByVariableKeepsDifferentDynamicSourceKeysDistinct(t *testing.T) {
	_, builder := newTestBuilder(t)

	thisParam := builder.NewParam("$this")
	keySource := "$function|$function"
	firstKey := emitDynamicTestKey(t, builder, "dynamic_distinct.php", keySource, 0, 8, "$function")
	secondKey := emitDynamicTestKey(t, builder, "dynamic_distinct.php", keySource, 10, 18, "$function")

	firstVar := builder.CreateMemberCallVariable(thisParam, firstKey)
	secondVar := builder.CreateMemberCallVariable(thisParam, secondKey)
	require.NotNil(t, firstVar)
	require.NotNil(t, secondVar)

	first := builder.ReadValueByVariable(firstVar)
	second := builder.ReadValueByVariable(secondVar)
	require.NotNil(t, first)
	require.NotNil(t, second)
	require.NotEqual(t, first.GetId(), second.GetId(), "different dynamic source keys should remain distinct")
	require.Len(t, builder.Function.ParameterMembers, 2, "distinct dynamic keys should create separate parameter members")
}
