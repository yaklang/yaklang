package sfvm

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
)

func TestToOpCodesRejectLegacySchema(t *testing.T) {
	legacy := `{"version":"dev","opcode":[{"op_code":1}]}`
	_, ok := ToOpCodes(legacy)
	require.False(t, ok)
}

func TestToOpCodesKeepsDevPayloadOnDevRuntime(t *testing.T) {
	previous := consts.GetYakVersion()
	consts.SetYakVersion("dev")
	t.Cleanup(func() { consts.SetYakVersion(previous) })

	current := fmt.Sprintf(
		`{"version":"dev","schema_version":%d,"opcode":[{"op_code":%d}]}`,
		CurrentOpcodeSchemaVersion,
		OpEnterStatement,
	)
	opcodes, ok := ToOpCodes(current)
	require.True(t, ok)
	require.NotNil(t, opcodes)
	require.Len(t, opcodes.Opcode, 1)
	require.Equal(t, OpEnterStatement, opcodes.Opcode[0].OpCode)
}

func TestToOpCodesRejectsDevPayloadOnStableRuntime(t *testing.T) {
	previous := consts.GetYakVersion()
	consts.SetYakVersion("v1.0.0-test")
	t.Cleanup(func() { consts.SetYakVersion(previous) })

	current := fmt.Sprintf(
		`{"version":"dev","schema_version":%d,"opcode":[{"op_code":%d}]}`,
		CurrentOpcodeSchemaVersion,
		OpEnterStatement,
	)
	_, ok := ToOpCodes(current)
	require.False(t, ok)
}

func TestToOpCodesVersionPolicy(t *testing.T) {
	runtimeVersion := consts.GetYakVersion()
	current := fmt.Sprintf(
		`{"version":"%s","schema_version":%d,"opcode":[{"op_code":%d}]}`,
		runtimeVersion,
		CurrentOpcodeSchemaVersion,
		OpEnterStatement,
	)
	opcodes, ok := ToOpCodes(current)

	expect := runtimeVersion != ""
	require.Equal(t, expect, ok)
	if expect {
		require.NotNil(t, opcodes)
		require.Len(t, opcodes.Opcode, 1)
		require.Equal(t, OpEnterStatement, opcodes.Opcode[0].OpCode)
	}
}

func TestVMLoadFallbackCompileWhenLegacyOpcodePayload(t *testing.T) {
	legacy := fmt.Sprintf(`{"version":"dev","opcode":[{"op_code":%d,"unary_str":""}]}`, OpRemoveRef)
	rule := &schema.SyntaxFlowRule{
		Content: `* as $sink`,
		OpCodes: legacy,
	}

	vm := NewSyntaxFlowVirtualMachine()
	frame, compiledFromContent, err := vm.Load(rule)
	require.NoError(t, err)
	require.True(t, compiledFromContent)
	require.NotNil(t, frame)
	require.Greater(t, len(frame.Codes), 1)
	require.Equal(t, OpEnterStatement, frame.Codes[0].OpCode)

	result, err := frame.Feed(NewEmptyValues())
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestVMLoadUsesDevPayloadWhenRuntimeIsDev(t *testing.T) {
	previous := consts.GetYakVersion()
	consts.SetYakVersion("dev")
	t.Cleanup(func() { consts.SetYakVersion(previous) })

	devPayload := fmt.Sprintf(
		`{"version":"dev","schema_version":%d,"opcode":[{"op_code":%d}]}`,
		CurrentOpcodeSchemaVersion,
		OpEnterStatement,
	)
	rule := &schema.SyntaxFlowRule{
		Content: `* as $sink`,
		OpCodes: devPayload,
	}

	vm := NewSyntaxFlowVirtualMachine()
	frame, compiledFromContent, err := vm.Load(rule)
	require.NoError(t, err)
	require.False(t, compiledFromContent)
	require.NotNil(t, frame)
	require.Len(t, frame.Codes, 1)
	require.Equal(t, OpEnterStatement, frame.Codes[0].OpCode)
}
