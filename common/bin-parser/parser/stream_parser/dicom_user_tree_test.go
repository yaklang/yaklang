package stream_parser

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func TestDICOMUserBridgeTransactionAndCounter(t *testing.T) {
	body := []byte{81, 0xff, 0, 4, 0, 0, 0x10, 0, 82, 1, 0, 3, '1', '.', '2', 0xee, 0x80, 0, 0, 85, 0, 0, 3, 'A', 'B', 'C'}
	for pending := uint64(0); pending < 8; pending++ {
		for _, outcome := range []string{"commit", "invalid tail", "short read", "callback error"} {
			t.Run(fmt.Sprintf("bits-%d/%s", pending, outcome), func(t *testing.T) {
				input := bytes.Clone(body)
				if outcome == "invalid tail" {
					input[len(input)-1] = 127
				}
				if outcome == "short read" {
					input = input[:len(input)-1]
				}
				parser, node := dicomBridgeTestNode(t, len(body), "DICOMUserInformation")
				node.Ctx.SetItem("dicomItemCount", 5)
				originalCfg, originalAlias := node.Cfg, node.Children[0]
				var wire bytes.Buffer
				wireWriter := base.NewBitWriter(&wire)
				if pending > 0 {
					require.NoError(t, wireWriter.WriteBits([]byte{0x55}, pending))
					_, err := parser.write([]byte{0x55}, pending)
					require.NoError(t, err)
				}
				require.NoError(t, wireWriter.WriteBits(input, uint64(len(input))*8))
				if pending > 0 {
					require.NoError(t, wireWriter.WriteBits([]byte{0}, 8-pending))
				}
				reader := base.NewBitReader(bytes.NewReader(wire.Bytes()))
				if pending > 0 {
					_, err := reader.ReadBits(pending)
					require.NoError(t, err)
				}
				writer := node.Ctx.GetItem("writer").(*base.BitWriter)
				buffer := node.Ctx.GetItem("buffer").(*bytes.Buffer)
				before := writer.Snapshot()
				calls, finalizations := 0, 0
				recovered := false
				handled, err := parseDICOMUserInformation(node, func(raw *base.Node) (func(bool), error) {
					calls++
					require.NotContains(t, node.Children, raw)
					require.Same(t, node, raw.Cfg.GetItem(CfgParent))
					require.NoError(t, reader.Backup())
					parseErr := parser.Parse(reader, raw)
					if outcome == "callback error" {
						parseErr = errors.New("injected user-item callback failure")
					}
					return func(rollback bool) {
						finalizations++
						recovered = rollback
						if rollback {
							node.Ctx.SetItem("pointer", pending)
							buffer.Truncate(0)
							require.NoError(t, writer.Restore(before))
							require.NoError(t, reader.Recovery())
						} else {
							require.NoError(t, reader.PopBackup())
						}
					}, parseErr
				})
				require.True(t, handled, "the standard definition must actually execute native parsing")
				require.Equal(t, 1, calls)
				require.Equal(t, 1, finalizations)
				require.Same(t, originalCfg, node.Cfg)
				if outcome != "commit" {
					require.Error(t, err)
					require.True(t, recovered)
					require.Len(t, node.Children, 1)
					require.Same(t, originalAlias, node.Children[0])
					require.False(t, node.Cfg.Has("template"))
					require.Equal(t, 5, node.Ctx.GetItem("dicomItemCount"))
					require.Equal(t, pending, node.Ctx.GetUint64("pointer"))
					require.Equal(t, before, writer.Snapshot())
					replayed, readErr := reader.ReadBits(uint64(len(input)) * 8)
					require.NoError(t, readErr)
					require.Equal(t, input, replayed)
				} else {
					require.NoError(t, err)
					require.False(t, recovered)
					require.Len(t, node.Children, 4)
					require.Equal(t, 9, node.Ctx.GetItem("dicomItemCount"))
					require.Equal(t, uint64(len(body))*8, CalcNodeConsumedLength(node))
					require.Equal(t, pending+uint64(len(body))*8, node.Ctx.GetUint64("pointer"))
					require.Same(t, node, node.Cfg.GetItem("template").(*base.Node).Cfg.GetItem(CfgParent))
					for i, offset := range []uint64{0, 8, 15, 19} {
						element := node.Children[i]
						require.Same(t, node, element.Cfg.GetItem(CfgParent))
						require.Equal(t, [2]uint64{pending + offset*8, pending + offset*8 + 8}, GetNodeResultPos(element.Children[0]))
						for _, field := range element.Children {
							require.Same(t, element, field.Cfg.GetItem(CfgParent))
						}
					}
					require.False(t, node.Children[2].Children[5].Cfg.Has(CfgNodeResult))
					require.False(t, node.Children[2].Children[5].Cfg.Has(CfgLength))
					uid := node.Children[1].Children[6]
					require.Equal(t, "Implementation Class", uid.Name)
					require.False(t, uid.Cfg.Has(CfgLength))
					require.Same(t, uid, uid.Children[0].Cfg.GetItem(CfgParent))
					value, resultErr := getNodeResult(uid.Children[0], false)
					require.NoError(t, resultErr)
					require.Equal(t, "1.2", value)
				}
				require.ErrorContains(t, reader.Recovery(), "no backup")
			})
		}
	}
}

func TestDICOMUserBridgeNonstandardConfigFallsBack(t *testing.T) {
	for name, change := range map[string]func(*base.Node){
		"bit unit":          func(n *base.Node) { n.Cfg.SetItem(CfgUnit, "bit") },
		"alias unit":        func(n *base.Node) { n.Children[0].Cfg.SetItem(CfgUnit, "byte") },
		"existing template": func(n *base.Node) { n.Cfg.SetItem("template", n.Children[0]) },
		"alias operator":    func(n *base.Node) { n.Children[0].Cfg.SetItem(CfgOperator, "custom()") },
		"custom UID":        func(n *base.Node) { n.Ctx.GetItem(CfgRootMap).(map[string]*base.Node)["DICOMUID"].Origin = "raw" },
		"counter missing":   func(n *base.Node) { n.Ctx.DeleteItem("dicomItemCount") },
		"counter negative":  func(n *base.Node) { n.Ctx.SetItem("dicomItemCount", -1) },
		"counter float":     func(n *base.Node) { n.Ctx.SetItem("dicomItemCount", 5.0) },
		"counter uint":      func(n *base.Node) { n.Ctx.SetItem("dicomItemCount", uint64(5)) },
	} {
		t.Run(name, func(t *testing.T) {
			_, node := dicomBridgeTestNode(t, 16, "DICOMUserInformation")
			node.Ctx.SetItem("dicomItemCount", 5)
			change(node)
			beforeCount := node.Ctx.GetItem("dicomItemCount")
			alias := node.Children[0]
			handled, err := parseDICOMUserInformation(node, func(*base.Node) (func(bool), error) {
				t.Fatal("nonstandard tree or context must fall back before wire read")
				return nil, nil
			})
			require.NoError(t, err)
			require.False(t, handled)
			require.Same(t, alias, node.Children[0])
			require.Equal(t, beforeCount, node.Ctx.GetItem("dicomItemCount"))
		})
	}
}

func TestDICOMUserBridgeGeneratorAndLegacyModes(t *testing.T) {
	for _, mode := range []string{"", "generator", GeneratorMode, ParserMode} {
		t.Run(mode, func(t *testing.T) {
			_, node := dicomBridgeTestNode(t, 16, "DICOMUserInformation")
			node.Ctx.SetItem("dicomItemCount", 5)
			if mode == ParserMode {
				node.Ctx.SetItem("dicomUserLegacy", true)
			}
			var modes []string
			if mode != "" {
				modes = append(modes, mode)
			}
			err := ExecOperator(node, `handled, err = tryParseDICOMUserInformation(); if err != nil { panic(err) }; if handled { panic("unexpected native user list") }`, func(*base.Node) (func(bool), error) {
				t.Fatal("non-parser and explicit legacy modes must not read")
				return nil, nil
			}, modes...)
			require.NoError(t, err)
		})
	}
}
