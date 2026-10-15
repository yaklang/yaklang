package sfvm

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
)

func TestBuiltinRuleMetadataPreservesOperandsAndReleaseCache(t *testing.T) {
	old := consts.GetYakVersion()
	defer consts.SetYakVersion(old)
	consts.SetYakVersion("dev")
	frame, err := NewSyntaxFlowVirtualMachine().Compile(`desc(title:"cached rule"); * as $value; alert $value;`)
	require.NoError(t, err)
	original := frame.GetRule()
	var want OpCodes
	require.NoError(t, json.Unmarshal([]byte(original.OpCodes), &want))
	// Empty maps/slices and their entries differ from nil. Compaction must not
	// recursively erase meaningful operands or user-provided values.
	want.Opcode = append(want.Opcode, &SFI{OpCode: OpPass, Values: []string{}, MultiOperator: []int{0}, FileFilterMethodItem: map[string]string{"empty": ""}})
	encoded, err := json.Marshal(want)
	require.NoError(t, err)
	original.OpCodes = string(encoded)
	raw, err := EncodeBuiltinRuleMetadata(original)
	require.NoError(t, err)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	for _, name := range []string{"ID", "CreatedAt", "UpdatedAt", "DeletedAt", "Groups", "Content"} {
		require.NotContains(t, fields, name)
	}
	require.Equal(t, byte('{'), fields["OpCodes"][0], "instructions must be an object, not escaped JSON")
	consts.SetYakVersion("release-test")
	got, err := DecodeBuiltinRuleMetadata(raw, original.Content)
	require.NoError(t, err)
	actual, hit := ToOpCodes(got.OpCodes)
	require.True(t, hit, "build-generated instructions must work with a release label")
	want.Version = "release-test"
	require.Equal(t, want, *actual)
	before, after := *original, *got
	before.OpCodes, after.OpCodes = "", ""
	require.Equal(t, before, after)
	_, compiled, err := NewSyntaxFlowVirtualMachine().Load(got)
	require.NoError(t, err)
	require.False(t, compiled, "verified metadata must avoid dynamic compilation")
	var wire map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &wire))
	wire["OpCodes"] = json.RawMessage(`{"schema_version":-1,"opcode":[]}`)
	broken, err := json.Marshal(wire)
	require.NoError(t, err)
	_, err = DecodeBuiltinRuleMetadata(broken, original.Content)
	require.Error(t, err, "incompatible instructions must fall back")
	// The separate, ordinary DB-cache policy remains strict.
	_, hit = ToOpCodes(original.OpCodes)
	require.False(t, hit)
}

func TestBuiltinMetadataHandlesRuleWithoutInstructions(t *testing.T) {
	frame, err := NewSyntaxFlowVirtualMachine().Compile(`desc(title:"metadata only");`)
	require.NoError(t, err)
	raw, err := EncodeBuiltinRuleMetadata(frame.GetRule())
	require.NoError(t, err)
	rule, err := DecodeBuiltinRuleMetadata(raw, frame.GetRule().Content)
	require.NoError(t, err)
	_, compiled, err := NewSyntaxFlowVirtualMachine().Load(rule)
	require.NoError(t, err)
	require.False(t, compiled)
}
