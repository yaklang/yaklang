package sfvm

import (
	"encoding/json"
	"fmt"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
)

// EncodeBuiltinRuleMetadata stores parser outputs, not DB state or source.
// Compaction only removes typed zero values. Map entries, parameter defaults,
// empty collections and instruction operands retain their exact meaning.
func EncodeBuiltinRuleMetadata(rule *schema.SyntaxFlowRule) (json.RawMessage, error) {
	raw, err := json.Marshal(rule)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	for _, key := range []string{"ID", "CreatedAt", "UpdatedAt", "DeletedAt", "Groups", "Content"} {
		delete(fields, key)
	}
	var opcodes struct {
		SchemaVersion int                          `json:"schema_version"`
		Opcode        []map[string]json.RawMessage `json:"opcode"`
	}
	if err := json.Unmarshal([]byte(rule.OpCodes), &opcodes); err != nil {
		return nil, err
	}
	for _, instruction := range opcodes.Opcode {
		omitZeroFields(instruction)
	}
	fields["OpCodes"], err = json.Marshal(opcodes)
	if err != nil {
		return nil, err
	}
	omitZeroFields(fields)
	return json.Marshal(fields)
}

func omitZeroFields(fields map[string]json.RawMessage) {
	for key, value := range fields {
		switch string(value) {
		case "null", "false", "0", `""`:
			delete(fields, key)
		}
	}
}

// The build generator binds this payload to the compiler sources in its recipe.
// Engine release labels are supplied later by ldflags, so apply the running
// version only to this verified, build-generated cache. Ordinary DB opcodes keep
// ToOpCodes' strict engine-version check and dynamic compilation fallback.
func DecodeBuiltinRuleMetadata(raw json.RawMessage, content string) (*schema.SyntaxFlowRule, error) {
	rule := new(schema.SyntaxFlowRule)
	var wire struct {
		*schema.SyntaxFlowRule
		OpCodes json.RawMessage
	}
	wire.SyntaxFlowRule = rule
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, err
	}
	var opcodes struct {
		Version       string          `json:"version"`
		SchemaVersion int             `json:"schema_version"`
		Opcode        json.RawMessage `json:"opcode"`
	}
	if err := json.Unmarshal(wire.OpCodes, &opcodes); err != nil {
		return nil, err
	}
	if opcodes.SchemaVersion != CurrentOpcodeSchemaVersion || len(opcodes.Opcode) == 0 || opcodes.Opcode[0] != '[' {
		return nil, fmt.Errorf("incompatible builtin rule opcodes")
	}
	opcodes.Version = consts.GetYakVersion()
	encoded, err := json.Marshal(opcodes)
	if err != nil {
		return nil, err
	}
	rule.OpCodes, rule.Content = string(encoded), content
	return rule, nil
}
