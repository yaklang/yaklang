package bin_parser

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

type giopCarrierTestBitReader struct {
	*bytes.Reader
	bits uint64
}

func (r *giopCarrierTestBitReader) InputBitLength() uint64 { return r.bits }

// Independent GIOP 1.2 big-endian CancelRequest: message size 4, request ID 7.
func giopCarrierTestWire(valid bool) []byte {
	wire := []byte{'G', 'I', 'O', 'P', 1, 2, 0, 2, 0, 0, 0, 4, 0, 0, 0, 7}
	if !valid {
		wire[0] = 'X'
	}
	return wire
}

func giopCarrierTestInline(t *testing.T, source string) *base.Node {
	t.Helper()
	var document yaml.MapSlice
	require.NoError(t, yaml.Unmarshal([]byte(source), &document))
	root, err := base.NewNodeTree(document)
	require.NoError(t, err)
	return root
}

func TestProtocolCorpusGIOPCarrierNonByteDirectBoundary(t *testing.T) {
	for extra := uint64(1); extra < 8; extra++ {
		for _, valid := range []bool{true, false} {
			for _, entry := range []string{"GIOP", "GIOPCarrier"} {
				t.Run(fmt.Sprintf("%s/extra-%d/valid-%t", entry, extra, valid), func(t *testing.T) {
					wire := append(giopCarrierTestWire(valid), 0xa5)
					reader := &giopCarrierTestBitReader{Reader: bytes.NewReader(wire), bits: 128 + extra}
					node, err := parser.ParseBinary(reader, "application-layer.iiop", entry)
					if entry == "GIOPCarrier" {
						require.ErrorContains(t, err, "giop: carrier requires a byte boundary")
					} else {
						require.ErrorContains(t, err, "giop: incomplete or non-byte message boundary")
					}
					require.Nil(t, node)
					require.Equal(t, len(wire), reader.Len(), "the rejected boundary must not read the header or fallback bytes")
				})
			}
		}
	}
}

func TestProtocolCorpusGIOPCarrierNonByteImportedBoundary(t *testing.T) {
	for offset := uint64(0); offset < 8; offset++ {
		for extra := uint64(1); extra < 8; extra++ {
			for _, valid := range []bool{true, false} {
				t.Run(fmt.Sprintf("offset-%d/extra-%d/valid-%t", offset, extra, valid), func(t *testing.T) {
					wire := giopCarrierTestWire(valid)
					var packed, expectedBuffer bytes.Buffer
					writer, expectedWriter := base.NewBitWriter(&packed), base.NewBitWriter(&expectedBuffer)
					if offset > 0 {
						require.NoError(t, writer.WriteBits([]byte{0x55}, offset))
						require.NoError(t, expectedWriter.WriteBits([]byte{0x55}, offset))
					}
					require.NoError(t, writer.WriteBits(wire, 128))
					require.NoError(t, writer.WriteBits([]byte{0x55}, extra))
					require.NoError(t, writer.WriteBits([]byte{0xd3}, 8))
					if padding := (8 - (offset+extra)%8) % 8; padding > 0 {
						require.NoError(t, writer.WriteBits([]byte{0}, padding))
					}
					root := giopCarrierTestInline(t, fmt.Sprintf(`
endian: big
unit: byte
Package:
  Wrapped:
    operator: |
      if %d > 0 { this.ProcessSubNode("Prefix") }
      this.ProcessSubNode("Carrier")
      this.ProcessSubNode("Sentinel")
    Prefix: uint8,%dbit
    Carrier:
      import: application-layer/iiop.yaml
      node: GIOPCarrier
      length: %d
    Sentinel: uint8
`, offset, offset, 128+extra))
					root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
					root.Ctx.SetItem("giop-carrier-test-marker", "outer")
					reader := base.NewBitReader(bytes.NewReader(packed.Bytes()))
					err := root.ParseSubNode(reader, "Wrapped")
					require.ErrorContains(t, err, "giop: carrier requires a byte boundary")
					wrapped := base.GetNodeByPath(root, "@Wrapped")
					for _, name := range []string{"Magic", "Request ID", "GIOP Payload", "Sentinel"} {
						require.Nil(t, protocolCorpusFindNode(wrapped, name), name)
					}
					if offset > 0 {
						protocolCorpusRequireValue(t, wrapped, "Prefix", uint64(0x55)&(1<<offset-1))
						require.Equal(t, [2]uint64{0, offset}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(wrapped, "Prefix")))
					}
					require.Equal(t, offset, root.Ctx.GetUint64("pointer"))
					require.Equal(t, expectedBuffer.Bytes(), root.Ctx.GetItem("buffer").(*bytes.Buffer).Bytes())
					require.Equal(t, expectedWriter.Snapshot(), root.Ctx.GetItem("writer").(*base.BitWriter).Snapshot())
					require.Equal(t, "outer", root.Ctx.GetItem("giop-carrier-test-marker"))
					require.ErrorContains(t, reader.Recovery(), "no backup")
					require.ErrorContains(t, reader.PopBackup(), "no backup")
					remaining, err := reader.ReadBits(128)
					require.NoError(t, err)
					require.Equal(t, wire, remaining)
					residue, err := reader.ReadBits(extra)
					require.NoError(t, err)
					require.Equal(t, []byte{0x55 & byte(1<<extra-1)}, residue)
					sentinel, err := reader.ReadBits(8)
					require.NoError(t, err)
					require.Equal(t, []byte{0xd3}, sentinel)
				})
			}
		}
	}
}

func TestProtocolCorpusGIOPCarrierByteBoundaryControls(t *testing.T) {
	for _, valid := range []bool{true, false} {
		wire := giopCarrierTestWire(valid)
		node := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.iiop", "GIOPCarrier")
		require.Equal(t, wire, NodeToBytes(node))
		if valid {
			protocolCorpusRequireValue(t, node, "Request ID", uint64(7))
		} else {
			protocolCorpusRequireValue(t, node, "GIOP Payload", wire)
		}
		for offset := uint64(0); offset < 8; offset++ {
			t.Run(fmt.Sprintf("offset-%d/valid-%t", offset, valid), func(t *testing.T) {
				var packed bytes.Buffer
				writer := base.NewBitWriter(&packed)
				if offset > 0 {
					require.NoError(t, writer.WriteBits([]byte{0x55}, offset))
				}
				require.NoError(t, writer.WriteBits(wire, 128))
				require.NoError(t, writer.WriteBits([]byte{0x55}, 8-offset))
				root := giopCarrierTestInline(t, fmt.Sprintf(`
endian: big
unit: byte
Package:
  Wrapped:
    operator: |
      if %d > 0 { this.ProcessSubNode("Prefix") }
      this.ProcessSubNode("Carrier")
      this.ProcessSubNode("Sentinel")
    Prefix: uint8,%dbit
    Carrier:
      import: application-layer/iiop.yaml
      node: GIOPCarrier
      length: 128
    Sentinel: uint8,%dbit
`, offset, offset, 8-offset))
				root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
				reader := base.NewBitReader(bytes.NewReader(packed.Bytes()))
				require.NoError(t, root.ParseSubNode(reader, "Wrapped"))
				wrapped := base.GetNodeByPath(root, "@Wrapped")
				require.Equal(t, packed.Bytes(), NodeToBytes(wrapped))
				protocolCorpusRequireValue(t, wrapped, "Sentinel", uint64(0x55)&(1<<(8-offset)-1))
				if valid {
					protocolCorpusRequireValue(t, wrapped, "Request ID", uint64(7))
					require.Equal(t, [2]uint64{offset, offset + 32}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(wrapped, "Magic")))
				} else {
					protocolCorpusRequireValue(t, wrapped, "GIOP Payload", wire)
					require.Equal(t, [2]uint64{offset, offset + 128}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(wrapped, "GIOP Payload")))
				}
				require.Equal(t, uint64(packed.Len())*8, root.Ctx.GetUint64("pointer"))
				require.ErrorContains(t, reader.Recovery(), "no backup")
				require.ErrorContains(t, reader.PopBackup(), "no backup")
			})
		}
	}
}
