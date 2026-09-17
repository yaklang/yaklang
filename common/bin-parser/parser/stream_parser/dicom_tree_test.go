package stream_parser

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func dicomBridgeTestNode(t *testing.T, size int, types ...string) (*DefParser, *base.Node) {
	t.Helper()
	root, err := base.ParseRule("application-layer/dicom.yaml")
	require.NoError(t, err)
	parser := &DefParser{}
	require.NoError(t, parser.OnRoot(root))
	anchor := root.Ctx.GetItem(CfgRootMap).(map[string]*base.Node)["Package"]
	typeName := "DICOMPDVList"
	if len(types) != 0 {
		typeName = types[0]
	}
	node, err := NewNodeByType(anchor, typeName)
	require.NoError(t, err)
	node.Name = "Presentation Data Values"
	node.Cfg.SetItem(CfgLength, uint64(size)*8)
	return parser, node
}

// The bridge owns exactly one finalization of its caller's transaction. Use a
// real bit reader, writer and terminal parser here, including pending bits.
func TestDICOMPDVBridgeTransaction(t *testing.T) {
	body := []byte{0, 0, 0, 2, 1, 0xff, 0, 0, 0, 4, 1, 0x82, 0xab, 0xcd}
	for pending := uint64(0); pending < 8; pending++ {
		for _, outcome := range []string{"commit", "invalid tail", "short read", "callback error"} {
			t.Run(fmt.Sprintf("bits-%d/%s", pending, outcome), func(t *testing.T) {
				input := bytes.Clone(body)
				if outcome == "invalid tail" {
					input[10] = 2
				}
				if outcome == "short read" {
					input = input[:len(input)-1]
				}
				parser, node := dicomBridgeTestNode(t, len(body))
				originalCfg, originalAlias := node.Cfg, node.Children[0]
				var wire bytes.Buffer
				wireWriter := base.NewBitWriter(&wire)
				if pending > 0 {
					require.NoError(t, wireWriter.WriteBits([]byte{0x55}, pending))
					_, err := parser.write([]byte{0x55}, pending)
					require.NoError(t, err)
				}
				require.NoError(t, wireWriter.WriteBits(input, uint64(len(input))*8))
				// Complete the final physical octet without adding a full byte.
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
				handled, err := parseDICOMPDVList(node, func(raw *base.Node) (func(bool), error) {
					calls++
					require.NotContains(t, node.Children, raw)
					require.Same(t, node, raw.Cfg.GetItem(CfgParent))
					require.NoError(t, reader.Backup())
					parseErr := parser.Parse(reader, raw)
					if outcome == "callback error" {
						parseErr = errors.New("injected terminal callback failure")
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
				require.True(t, handled, "standard definition must actually enter the native path")
				require.Equal(t, 1, calls)
				require.Equal(t, 1, finalizations)
				require.Same(t, originalCfg, node.Cfg)
				if outcome != "commit" {
					require.Error(t, err)
					require.True(t, recovered)
					require.Len(t, node.Children, 1)
					require.Same(t, originalAlias, node.Children[0])
					require.False(t, node.Cfg.Has("template"))
					require.Equal(t, pending, node.Ctx.GetUint64("pointer"))
					require.Equal(t, before, writer.Snapshot())
					replayed, readErr := reader.ReadBits(uint64(len(input)) * 8)
					require.NoError(t, readErr)
					require.Equal(t, input, replayed)
				} else {
					require.NoError(t, err)
					require.False(t, recovered)
					require.Len(t, node.Children, 2)
					require.Equal(t, uint64(len(body))*8, CalcNodeConsumedLength(node))
					require.Equal(t, pending+uint64(len(body))*8, node.Ctx.GetUint64("pointer"))
					require.Same(t, node, node.Cfg.GetItem("template").(*base.Node).Cfg.GetItem(CfgParent))
					for i, element := range node.Children {
						require.Same(t, node, element.Cfg.GetItem(CfgParent))
						require.False(t, element.Cfg.Has(CfgElementIndex))
						require.Equal(t, [2]uint64{pending + uint64(i)*48, pending + uint64(i)*48 + 32}, GetNodeResultPos(element.Children[0]))
						for _, field := range element.Children {
							require.Same(t, element, field.Cfg.GetItem(CfgParent))
						}
					}
					require.False(t, node.Children[0].Children[5].Cfg.Has(CfgNodeResult))
					require.False(t, node.Children[0].Children[5].Cfg.Has(CfgLength))
					fragment, resultErr := getNodeResult(node.Children[1].Children[5], true)
					require.NoError(t, resultErr)
					require.Equal(t, []byte{0xab, 0xcd}, fragment)
				}
				require.ErrorContains(t, reader.Recovery(), "no backup")
			})
		}
	}
}

func TestDICOMPDVBridgeCustomDefinitionFallsBackBeforeRead(t *testing.T) {
	for name, modify := range map[string]func(*base.Node){
		"list unit":          func(n *base.Node) { n.Cfg.SetItem(CfgUnit, "bit") },
		"list parser":        func(n *base.Node) { n.Cfg.SetItem("parser", "custom") },
		"existing template":  func(n *base.Node) { n.Cfg.SetItem("template", n.Children[0]) },
		"alias operator":     func(n *base.Node) { n.Children[0].Cfg.SetItem(CfgOperator, "custom()") },
		"alias endian":       func(n *base.Node) { n.Children[0].Cfg.SetItem(CfgEndian, "little") },
		"alias length":       func(n *base.Node) { n.Children[0].Cfg.SetItem(CfgLength, uint64(48)) },
		"changed definition": func(n *base.Node) { n.Ctx.GetItem(CfgRootMap).(map[string]*base.Node)["DICOMPDV"].Origin = "raw" },
		"zero boundary":      func(n *base.Node) { n.Cfg.SetItem(CfgLength, uint64(0)) },
	} {
		t.Run(name, func(t *testing.T) {
			_, node := dicomBridgeTestNode(t, 6)
			modify(node)
			alias := node.Children[0]
			handled, err := parseDICOMPDVList(node, func(*base.Node) (func(bool), error) {
				t.Fatal("custom definition must fall back before reading")
				return nil, nil
			})
			require.NoError(t, err)
			require.False(t, handled)
			require.Len(t, node.Children, 1)
			require.Same(t, alias, node.Children[0])
		})
	}
}

func TestDICOMPDVBridgeGeneratorAndLegacyModes(t *testing.T) {
	for _, mode := range []string{"", "generator", GeneratorMode, ParserMode} {
		t.Run(mode, func(t *testing.T) {
			_, node := dicomBridgeTestNode(t, 6)
			if mode == ParserMode {
				node.Ctx.SetItem("dicomPDVLegacy", true)
			}
			var modes []string
			if mode != "" {
				modes = append(modes, mode)
			}
			err := ExecOperator(node, `handled, err = tryParseDICOMPDVList(); if err != nil { panic(err) }; if handled { panic("unexpected native parse") }`, func(*base.Node) (func(bool), error) {
				t.Fatal("non-parser and explicit legacy modes must not read")
				return nil, nil
			}, modes...)
			require.NoError(t, err)
		})
	}
}
