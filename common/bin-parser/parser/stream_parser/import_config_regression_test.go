package stream_parser_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

func TestImportedRuleKeepsCallerSettingsAndLocalRuntimeScope(t *testing.T) {
	var document yaml.MapSlice
	require.NoError(t, yaml.Unmarshal([]byte(`
Package:
  Outer:
    operator: |
      setCtx("transient-parent-value", 123)
      this.ProcessSubNode("Message")
    Message: "import:application-layer/hislip.yaml;node:HiSLIP"
`), &document))
	root, err := base.NewNodeTree(document)
	require.NoError(t, err)
	state := map[string]any{"count": 1}
	root.Ctx.SetItem(base.CtxInputConfig, map[string]any{
		"hislipPayloadLimit": 4, "callerState": state, "explicitFalse": false,
		"root": "not a node", "rootNodeMap": "not a type map", "buffer": "not a byte buffer", "inList": true,
	})
	wire := append([]byte{'H', 'S', 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 4}, []byte("inst")...)
	root.Cfg.SetItem(base.CfgLength, uint64(len(wire)*8))
	reader := bytes.NewReader(wire)
	require.NoError(t, root.Parse(base.NewBitReader(reader)))
	require.Zero(t, reader.Len())
	message := base.GetNodeByPath(root, "@Outer.Message")
	require.NotNil(t, message)
	require.Equal(t, 4, message.Ctx.GetItem("hislipPayloadLimit"))
	require.True(t, message.Ctx.Has("explicitFalse"))
	require.Equal(t, false, message.Ctx.GetItem("explicitFalse"))
	require.False(t, message.Ctx.Has("transient-parent-value"))
	require.False(t, message.Ctx.GetBool("inList"))
	require.IsType(t, &base.Node{}, message.Ctx.GetItem("root"))
	require.IsType(t, map[string]*base.Node{}, message.Ctx.GetItem("rootNodeMap"))
	require.IsType(t, &bytes.Buffer{}, message.Ctx.GetItem("buffer"))
	shared := message.Ctx.GetItem("callerState").(map[string]any)
	shared["count"] = 2
	require.Equal(t, 2, state["count"], "caller-owned state tables retain their identity")
}
