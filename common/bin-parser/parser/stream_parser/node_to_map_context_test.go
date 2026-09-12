package stream_parser

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func TestNodeToMapMixedAndReplacedContexts(t *testing.T) {
	left, right := positionedResultNode([]byte{0x11}, 0), positionedResultNode([]byte{0x22}, 0)
	leaf := func(name string, ctx *base.NodeContext) *base.Node {
		n := &base.Node{Name: name, Cfg: base.NewEmptyConfig(), Ctx: ctx}
		n.Cfg.SetItems(base.ConfigItem{Key: CfgNodeResult, Value: [2]uint64{0, 8}},
			base.ConfigItem{Key: CfgType, Value: "raw"}, base.ConfigItem{Key: CfgIsTerminal, Value: true})
		return n
	}
	root := &base.Node{Cfg: base.NewEmptyConfig(), Children: []*base.Node{leaf("A", left.Ctx), leaf("B", right.Ctx), leaf("C", left.Ctx)}}
	first := NodeToMap(root).(map[string]any)
	require.Equal(t, map[string]any{"A": []byte{0x11}, "B": []byte{0x22}, "C": []byte{0x11}}, first)
	buffer := bytes.NewBuffer([]byte{0x33})
	left.Ctx.SetItem("buffer", buffer)
	left.Ctx.SetItem("writer", base.NewBitWriter(buffer))
	second := NodeToMap(root).(map[string]any)
	require.Equal(t, map[string]any{"A": []byte{0x33}, "B": []byte{0x22}, "C": []byte{0x33}}, second)
	second["A"].([]byte)[0] = 0xff
	require.Equal(t, []byte{0x33}, second["C"])
	require.Equal(t, []byte{0x11}, first["A"])
}
