package stream_parser

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

func giopBridgeWire(minor, flags, typ byte, body []byte) []byte {
	wire := append([]byte{'G', 'I', 'O', 'P', 1, minor, flags, typ, 0, 0, 0, 0}, body...)
	var order binary.ByteOrder = binary.BigEndian
	if flags&1 != 0 {
		order = binary.LittleEndian
	}
	order.PutUint32(wire[8:12], uint32(len(body)))
	return wire
}

func giopBridgeInlineRoot(t *testing.T, rule string) *base.Node {
	t.Helper()
	var doc yaml.MapSlice
	require.NoError(t, yaml.Unmarshal([]byte(rule), &doc))
	root, err := base.NewNodeTree(doc)
	require.NoError(t, err)
	return root
}

func giopBridgeFind(t *testing.T, n *base.Node, name string) *base.Node {
	t.Helper()
	var find func(*base.Node) *base.Node
	find = func(n *base.Node) *base.Node {
		if n.Name == name {
			return n
		}
		for _, child := range n.Children {
			if result := find(child); result != nil {
				return result
			}
		}
		return nil
	}
	result := find(n)
	require.NotNil(t, result, "missing node %s", name)
	return result
}

func giopBridgeAssertTree(t *testing.T, n *base.Node, start, end uint64) {
	t.Helper()
	result, err := n.Result()
	if n.Cfg.GetBool(CfgIsList) && len(n.Children) == 0 && !NodeHasResult(n) {
		// Existing result formatting omits empty containers, but the original
		// zero-length list node must remain available in the parse tree.
		require.ErrorIs(t, err, noResultError)
	} else {
		require.NoError(t, err)
		require.Same(t, n, result.Origin)
		require.Equal(t, n.Cfg.GetBool(CfgIsList), result.ListValue)
	}
	require.Equal(t, end-start, CalcNodeConsumedLength(n), "consumed length of %s", n.Name)
	if NodeHasResult(n) {
		require.Equal(t, [2]uint64{start, end}, GetNodeResultPos(n), n.Name)
		return
	}
	pos := start
	for i, child := range n.Children {
		require.Same(t, n, child.Cfg.GetItem(CfgParent), child.Name)
		require.Same(t, n.Ctx, child.Ctx, child.Name)
		if n.Cfg.GetBool(CfgIsList) {
			require.Equal(t, i, child.Cfg.GetItem(CfgElementIndex), child.Name)
		}
		length := child.Cfg.GetUint64(CfgLength)
		giopBridgeAssertTree(t, child, pos, pos+length)
		pos += length
	}
	require.Equal(t, end, pos, "unrepresented span in %s", n.Name)
}

func TestGIOPBridgeStagedTransactionsAndBitSpans(t *testing.T) {
	w := giopTestRequest(2, true, 2, 2, nil, []byte{0xde, 0xad, 0xbe, 0xef})
	valid := giopBridgeWire(2, 1, 0, w.b)
	for pending := uint64(0); pending < 8; pending++ {
		for _, outcome := range []string{"commit", "header invalid", "body invalid", "header short", "body short", "header callback", "body callback"} {
			t.Run(fmt.Sprintf("offset-%d/%s", pending, outcome), func(t *testing.T) {
				input := bytes.Clone(valid)
				switch outcome {
				case "header invalid":
					input[0] = 'X'
				case "body invalid":
					input[12+w.marks["Response Flags"]] = 2
				case "header short":
					input = input[:11]
				case "body short":
					input = input[:len(input)-1]
				}
				root := giopBridgeInlineRoot(t, "endian: big\nunit: byte\nPackage:\n  Message: {}\n")
				root.Children[0].Children[0].Cfg.SetItem(CfgIsTerminal, false)
				// Bind the tree's public result runtime before the direct parser
				// fixture installs its shared writer/context.
				require.NoError(t, root.Parse(base.NewBitReader(bytes.NewReader(nil))))
				root.Cfg.SetItem(CfgLength, pending+uint64(len(valid))*8)
				p := &DefParser{}
				require.NoError(t, p.OnRoot(root))
				n := root.Children[0].Children[0]
				n.Cfg.SetItem(CfgLength, uint64(len(valid))*8)
				n.Cfg.SetItem("caller-marker", "unchanged")
				alias := &base.Node{Name: "existing unparsed alias", Origin: yaml.MapSlice{}, Cfg: base.NewConfig(n.Cfg), Ctx: n.Ctx}
				alias.Cfg.SetItem(CfgParent, n)
				n.Children = []*base.Node{alias}
				cfg := n.Cfg
				var wire bytes.Buffer
				ww := base.NewBitWriter(&wire)
				if pending > 0 {
					require.NoError(t, ww.WriteBits([]byte{0x55}, pending))
					_, err := p.write([]byte{0x55}, pending)
					require.NoError(t, err)
				}
				require.NoError(t, ww.WriteBits(input, uint64(len(input))*8))
				if pending > 0 {
					require.NoError(t, ww.WriteBits([]byte{0}, 8-pending))
				}
				reader := base.NewBitReader(bytes.NewReader(wire.Bytes()))
				if pending > 0 {
					_, err := reader.ReadBits(pending)
					require.NoError(t, err)
				}
				writer := n.Ctx.GetItem("writer").(*base.BitWriter)
				buffer := n.Ctx.GetItem("buffer").(*bytes.Buffer)
				before := writer.Snapshot()
				calls := 0
				var finishOrder []int
				err := parseGIOPMessage(n, func(raw *base.Node) (func(bool), error) {
					calls++
					call := calls
					require.NotContains(t, n.Children, raw)
					require.Same(t, n, raw.Cfg.GetItem(CfgParent))
					require.NoError(t, reader.Backup())
					position := n.Ctx.GetUint64("pointer")
					state := writer.Snapshot()
					e := p.Parse(reader, raw)
					if (outcome == "header callback" && call == 1) || (outcome == "body callback" && call == 2) {
						e = errors.New("injected callback error")
					}
					return func(rollback bool) {
						finishOrder = append(finishOrder, call)
						if rollback {
							n.Ctx.SetItem("pointer", position)
							buffer.Truncate(int(position / 8))
							require.NoError(t, writer.Restore(state))
							require.NoError(t, reader.Recovery())
						} else {
							require.NoError(t, reader.PopBackup())
						}
					}, e
				}, true)
				require.Same(t, cfg, n.Cfg)
				require.Equal(t, "unchanged", n.Cfg.GetItem("caller-marker"))
				if calls == 1 {
					require.Equal(t, []int{1}, finishOrder)
				} else {
					require.Equal(t, []int{2, 1}, finishOrder)
				}
				if outcome == "commit" {
					require.NoError(t, err)
					require.Equal(t, 2, calls)
					giopBridgeAssertTree(t, n, pending, pending+uint64(len(valid))*8)
					id := giopBridgeFind(t, n, "Request ID")
					result, e := id.Result()
					require.NoError(t, e)
					require.Equal(t, uint32(0x10203040), result.Value)
					stub := giopBridgeFind(t, n, "Stub Data")
					require.Equal(t, [2]uint64{pending + uint64(12+w.marks["Stub"])*8, pending + uint64(len(valid))*8}, GetNodeResultPos(stub))
				} else {
					require.Error(t, err)
					require.Len(t, n.Children, 1)
					require.Same(t, alias, n.Children[0])
					require.False(t, n.Cfg.Has("additionInfo"))
					require.Equal(t, pending, n.Ctx.GetUint64("pointer"))
					require.Equal(t, before, writer.Snapshot())
					got, e := reader.ReadBits(uint64(len(input)) * 8)
					require.NoError(t, e)
					require.Equal(t, input, got)
				}
				require.ErrorContains(t, reader.Recovery(), "no backup")
				require.ErrorContains(t, reader.PopBackup(), "no backup")
			})
		}
	}
}

func TestGIOPBridgeImportedStreamBoundaryAndCallerScope(t *testing.T) {
	first := giopBridgeWire(2, 0, 2, []byte{0x10, 0x20, 0x30, 0x40})
	second := giopBridgeWire(2, 1, 2, []byte{0x44, 0x33, 0x22, 0x11})
	input := append(append([]byte{0xab}, first...), second...)
	input = append(input, 0xcd)
	root := giopBridgeInlineRoot(t, fmt.Sprintf(`
endian: little
unit: byte
Package:
  Outer:
    operator: |
      setCtx("parent-temporary", true)
      this.ProcessSubNode("Prefix")
      this.ProcessSubNode("Messages")
      this.ProcessSubNode("Suffix")
    Prefix: uint8
    Messages:
      import: application-layer/iiop.yaml
      node: GIOPStream
      length: %d
    Suffix: uint8
`, (len(first)+len(second))*8))
	state := map[string]any{"calls": 1}
	root.Ctx.SetItem(base.CtxInputConfig, map[string]any{
		"callerState": state, "explicitFalse": false, "root": "invalid", "rootNodeMap": "invalid", "buffer": "invalid", "writer": "invalid", "inList": true,
	})
	root.Cfg.SetItem(CfgLength, uint64(len(input))*8)
	reader := base.NewBitReader(bytes.NewReader(input))
	require.NoError(t, root.Parse(reader))
	messages := giopBridgeFind(t, root, "Messages")
	require.Len(t, messages.Children, 2)
	require.Equal(t, uint64(len(first)+len(second))*8, CalcNodeConsumedLength(messages))
	for i, message := range messages.Children {
		require.Same(t, messages, message.Cfg.GetItem(CfgParent))
		require.Equal(t, uint64(16*8), CalcNodeConsumedLength(message))
		magic := giopBridgeFind(t, message, "Magic")
		require.Equal(t, [2]uint64{uint64(1+i*16) * 8, uint64(5+i*16) * 8}, GetNodeResultPos(magic))
		result, err := giopBridgeFind(t, message, "Request ID").Result()
		require.NoError(t, err)
		require.Equal(t, []uint32{0x10203040, 0x11223344}[i], result.Value)
	}
	result, err := messages.Result()
	require.NoError(t, err)
	require.True(t, result.ListValue)
	require.Same(t, messages, result.Origin)
	require.Len(t, result.Children(), 2)
	for i, value := range result.Children() {
		require.Same(t, messages.Children[i], value.Origin)
	}
	suffix, err := giopBridgeFind(t, root, "Suffix").Result()
	require.NoError(t, err)
	require.Equal(t, uint8(0xcd), suffix.Value)
	require.Equal(t, uint64(len(input))*8, root.Ctx.GetUint64("pointer"))
	require.False(t, messages.Ctx.Has("parent-temporary"))
	require.Equal(t, false, messages.Ctx.GetItem("explicitFalse"))
	require.False(t, messages.Ctx.GetBool("inList"))
	require.IsType(t, &base.Node{}, messages.Ctx.GetItem("root"))
	require.IsType(t, map[string]*base.Node{}, messages.Ctx.GetItem(CfgRootMap))
	require.IsType(t, &bytes.Buffer{}, messages.Ctx.GetItem("buffer"))
	messages.Ctx.GetItem("callerState").(map[string]any)["calls"] = 2
	require.Equal(t, 2, state["calls"])
	require.ErrorContains(t, reader.Recovery(), "no backup")
	require.ErrorContains(t, reader.PopBackup(), "no backup")
}

func TestGIOPBridgeImportedCarrierRollbackWithinParent(t *testing.T) {
	good := giopBridgeWire(2, 0, 2, []byte{0, 0, 0, 7})
	for name, payload := range map[string][]byte{
		"exact valid":       good,
		"extra bytes":       append(bytes.Clone(good), 0x88, 0x99),
		"invalid body":      giopBridgeWire(2, 0, 2, []byte{0, 0, 0}),
		"invalid signature": append([]byte{'X'}, good[1:]...),
	} {
		t.Run(name, func(t *testing.T) {
			root := giopBridgeInlineRoot(t, fmt.Sprintf(`
unit: byte
Package:
  Prefix: uint8,3bit
  Carrier:
    import: application-layer/iiop.yaml
    node: GIOPCarrier
    length: %d
  Suffix: uint8,5bit
`, len(payload)*8))
			var wire bytes.Buffer
			writer := base.NewBitWriter(&wire)
			require.NoError(t, writer.WriteBits([]byte{5}, 3))
			require.NoError(t, writer.WriteBits(payload, uint64(len(payload))*8))
			require.NoError(t, writer.WriteBits([]byte{21}, 5))
			root.Cfg.SetItem(CfgLength, uint64(wire.Len())*8)
			reader := base.NewBitReader(bytes.NewReader(wire.Bytes()))
			require.NoError(t, root.Parse(reader))
			carrier := giopBridgeFind(t, root, "Carrier")
			require.Len(t, carrier.Children, 1)
			require.Equal(t, uint64(len(payload))*8, CalcNodeConsumedLength(carrier))
			if name == "exact valid" {
				require.Equal(t, "GIOP", carrier.Children[0].Name)
				magic := giopBridgeFind(t, carrier, "Magic")
				require.Equal(t, [2]uint64{3, 35}, GetNodeResultPos(magic))
			} else {
				raw := carrier.Children[0]
				require.Equal(t, "GIOP Payload", raw.Name)
				require.Equal(t, [2]uint64{3, 3 + uint64(len(payload))*8}, GetNodeResultPos(raw))
				value, err := raw.Result()
				require.NoError(t, err)
				require.Equal(t, payload, value.Value)
				require.False(t, carrier.Cfg.Has("additionInfo"))
			}
			suffix, err := giopBridgeFind(t, root, "Suffix").Result()
			require.NoError(t, err)
			require.Equal(t, uint8(21), suffix.Value)
			require.Equal(t, uint64(wire.Len())*8, root.Ctx.GetUint64("pointer"))
			require.ErrorContains(t, reader.Recovery(), "no backup")
			require.ErrorContains(t, reader.PopBackup(), "no backup")
		})
	}
}

func TestGIOPBridgeStreamRejectsSecondMessageCrossingParent(t *testing.T) {
	first := giopBridgeWire(2, 0, 2, []byte{0, 0, 0, 1})
	second := giopBridgeWire(2, 0, 2, []byte{0, 0, 0, 2})
	// The physical reader has this extra byte, but it belongs to the outer
	// structure, not to the explicitly bounded stream.
	binary.BigEndian.PutUint32(second[8:12], 5)
	input := append(append(bytes.Clone(first), second...), 0xee)
	root := giopBridgeInlineRoot(t, fmt.Sprintf(`
unit: byte
Package:
  Messages:
    import: application-layer/iiop.yaml
    node: GIOPStream
    length: %d
  Suffix: uint8
`, (len(first)+len(second))*8))
	root.Cfg.SetItem(CfgLength, uint64(len(input))*8)
	reader := base.NewBitReader(bytes.NewReader(input))
	err := root.Parse(reader)
	require.ErrorContains(t, err, "declared size does not fit message boundary")
	require.Zero(t, root.Ctx.GetUint64("pointer"))
	require.Zero(t, root.Ctx.GetItem("buffer").(*bytes.Buffer).Len())
	got, err := reader.ReadBits(uint64(len(input)) * 8)
	require.NoError(t, err)
	require.Equal(t, input, got)
	require.ErrorContains(t, reader.Recovery(), "no backup")
	require.ErrorContains(t, reader.PopBackup(), "no backup")
}

func TestGIOPBridgeModesAndImpossibleBoundariesDoNotRead(t *testing.T) {
	for _, bits := range []uint64{0, 8, 95, 97, 127} {
		root := giopBridgeInlineRoot(t, "Package:\n  Message: {}\n")
		p := &DefParser{}
		require.NoError(t, p.OnRoot(root))
		root.Cfg.SetItem(CfgLength, bits)
		n := root.Children[0].Children[0]
		err := parseGIOPMessage(n, func(*base.Node) (func(bool), error) {
			t.Fatal("invalid boundary must not read")
			return nil, nil
		}, true)
		require.Error(t, err)
	}
	for _, mode := range []string{"", GeneratorMode} {
		root := giopBridgeInlineRoot(t, "Package:\n  Message: {}\n")
		p := &DefParser{}
		require.NoError(t, p.OnRoot(root))
		n := root.Children[0].Children[0]
		var modes []string
		if mode != "" {
			modes = append(modes, mode)
		}
		require.NoError(t, ExecOperator(n, `err = parseGIOPMessage(true); if err == nil { panic("unexpected generation") }`, func(*base.Node) (func(bool), error) {
			t.Fatal("generation must not read")
			return nil, nil
		}, modes...))
	}
}
