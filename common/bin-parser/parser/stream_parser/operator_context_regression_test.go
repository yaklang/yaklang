package stream_parser_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

func TestOperatorContextPresencePreservesCallerOwnedEmptyValues(t *testing.T) {
	const source = `
Package:
  Message:
    operator: |
      for _, key = range ["state", "nil", "zero", "false"] {
        if !hasCtx(key) { panic("present context value reported absent") }
      }
      if hasCtx("absent") { panic("absent context value reported present") }
      state = getCtx("state")
      state[7] = 19
      setCtx("created", 0)
      if !hasCtx("created") { panic("new context value lost") }
      deleteCtx("created")
      if hasCtx("created") { panic("deleted context value retained") }
      this.ProcessSubNode("Byte")
    Byte: uint8
`
	var document yaml.MapSlice
	require.NoError(t, yaml.Unmarshal([]byte(source), &document))
	root, err := base.NewNodeTree(document)
	require.NoError(t, err)
	state := map[int]any{}
	for key, value := range map[string]any{"state": state, "nil": nil, "zero": 0, "false": false} {
		root.Ctx.SetItem(key, value)
	}
	root.Cfg.SetItem(base.CfgLength, uint64(8))
	require.NoError(t, root.Parse(base.NewBitReader(bytes.NewReader([]byte{0x21}))))
	require.Equal(t, map[int]any{7: 19}, state, "do not replace a caller-owned empty map")
}

func TestOperatorTypeMethodsAcceptAnExplicitNodeName(t *testing.T) {
	for name, operator := range map[string]string{
		"process": `this.ProcessByType("Record", "Alias")`,
		"new":     `this.NewSubNode("Record", "Alias").Process()`,
		"try-save": `value, operation = this.TryProcessByType("Record", "Alias")
      if !operation.OK { panic(operation.Message) }
      operation.Save()`,
		"try-recover": `value, operation = this.TryProcessByType("Record", "Discarded")
      if !operation.OK { panic(operation.Message) }
      operation.Recovery()
      this.ProcessByType("Record", "Alias")`,
	} {
		t.Run(name, func(t *testing.T) {
			source := fmt.Sprintf(`
Package:
  Message:
    operator: |
      %s
Record:
  Value: uint8
`, operator)
			root, _, reader := parseInlineRule(t, source, []byte{0x73})
			require.Zero(t, reader.Len())
			node := base.GetNodeByPath(root, "@Message.Alias.Value")
			require.NotNil(t, node)
			result, err := node.Result()
			require.NoError(t, err)
			require.Equal(t, uint8(0x73), result.Value)
			require.Nil(t, base.GetNodeByPath(root, "@Message.Discarded"))
		})
	}
}
