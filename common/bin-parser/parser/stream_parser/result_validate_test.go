package stream_parser

import (
	"bytes"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func discardedOutcome(call func() error) (outcome string) {
	defer func() {
		if value := recover(); value != nil {
			outcome = fmt.Sprintf("panic %T: %v", value, value)
		}
	}()
	return fmt.Sprint(call())
}

func TestValidateResultLeafErrorsMatchProjection(t *testing.T) {
	for _, pending := range []uint8{0, 1, 7} {
		for _, typ := range []string{"raw", "bytes", "string", "uint8", "uint64", "int16", "bool", "unknown"} {
			for _, span := range [][2]uint64{{0, 0}, {0, 8}, {1, 7}, {1, 14}, {8, 16}, {8, 7}, {0, 25}, {math.MaxUint64, math.MaxUint64}} {
				for _, list := range []bool{false, true} {
					node := positionedResultNode([]byte{0xa5, 0x77}, pending)
					node.Cfg.SetItems(base.ConfigItem{Key: CfgNodeResult, Value: span}, base.ConfigItem{Key: CfgType, Value: typ}, base.ConfigItem{Key: CfgIsList, Value: list})
					d := &DefParser{ctx: node.Ctx}
					want := discardedOutcome(func() error { _, err := d.Result(node); return err })
					got := discardedOutcome(func() error { return d.ValidateResult(node) })
					require.Equal(t, want, got, "type=%s span=%v list=%t pending=%d", typ, span, list, pending)
				}
			}
		}
	}
}

type discardedCallbackParser struct {
	base.Parser
	result func(*base.Node) (*base.NodeValue, error)
}

func (p *discardedCallbackParser) Result(n *base.Node) (*base.NodeValue, error) { return p.result(n) }

func TestValidateResultCallbackOrderAndTreeMutation(t *testing.T) {
	const parserName = "discarded-result-callback-test"
	previous := base.ParserRegistration(parserName)
	t.Cleanup(func() { base.RegisterParser(parserName, previous) })
	for _, list := range []bool{false, true} {
		var traces [2][]string
		for mode := 0; mode < 2; mode++ {
			root := giopBridgeInlineRoot(t, "Package:\n  Parent:\n    Wrapper:\n      unpack: true\n      A: uint8\n    B: uint8\n")
			require.NoError(t, root.Parse(base.NewBitReader(bytes.NewReader([]byte{1, 2}))))
			n := root.Children[0].Children[0]
			n.Cfg.SetItem(CfgIsList, list)
			a, b := n.Children[0].Children[0], n.Children[1]
			replacement := b.Copy()
			replacement.Name = "Replacement"
			for _, child := range []*base.Node{a, b, replacement} {
				child.Cfg.SetItem("parser", parserName)
			}
			base.RegisterParser(parserName, &discardedCallbackParser{result: func(child *base.Node) (*base.NodeValue, error) {
				traces[mode] = append(traces[mode], child.Name)
				if child == a {
					n.Children[1] = replacement
					n.Children[0].Children = nil
				}
				// Nil results still count as a successful collection element.
				return nil, nil
			}})
			if mode == 0 {
				_, err := n.Result()
				require.NoError(t, err)
			} else {
				require.NoError(t, n.ValidateResult())
			}
		}
		require.Equal(t, traces[0], traces[1])
		if list {
			require.Equal(t, []string{"A", "Replacement"}, traces[1])
		} else {
			require.Equal(t, []string{"A", "B"}, traces[1])
		}
	}
}

func TestValidateResultOutAndFormatterEffects(t *testing.T) {
	for _, source := range []string{
		`node.Ctx.SetItem("seen", node.Ctx.GetItem("seen") + 1); return data.Value`,
		`node.Ctx.SetItem("seen", node.Ctx.GetItem("seen") + 1); panic("out failed")`,
	} {
		var outcomes [2]string
		for mode := 0; mode < 2; mode++ {
			root := giopBridgeInlineRoot(t, "Package:\n  A: uint8\n")
			require.NoError(t, root.Parse(base.NewBitReader(bytes.NewReader([]byte{1}))))
			n := root.Children[0].Children[0]
			n.Ctx.SetItem("seen", 0)
			n.Cfg.SetItem("out", source)
			outcomes[mode] = discardedOutcome(func() error {
				if mode == 0 {
					_, err := n.Result()
					return err
				}
				return n.ValidateResult()
			})
			require.EqualValues(t, 1, n.Ctx.GetItem("seen"))
			require.Equal(t, source, n.Cfg.GetString("out"))
		}
		require.Equal(t, outcomes[0], outcomes[1])
	}
	root := giopBridgeInlineRoot(t, "Package:\n  A: uint8\n")
	require.NoError(t, root.Parse(base.NewBitReader(bytes.NewReader([]byte{1}))))
	n := root.Children[0].Children[0]
	n.Ctx.SetItem("formatter", "discarded-formatter-test")
	calls := 0
	formatters["discarded-formatter-test"] = func(*base.Node) (*base.NodeValue, error) {
		calls++
		return nil, fmt.Errorf("custom formatter error")
	}
	t.Cleanup(func() { delete(formatters, "discarded-formatter-test") })
	want := discardedOutcome(func() error { _, err := n.Result(); return err })
	got := discardedOutcome(n.ValidateResult)
	require.Equal(t, want, got)
	require.Equal(t, 2, calls)
	delete(formatters, "discarded-formatter-test")
	require.Equal(t, discardedOutcome(func() error { _, err := n.Result(); return err }), discardedOutcome(n.ValidateResult))
}
