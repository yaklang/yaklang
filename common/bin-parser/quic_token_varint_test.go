package bin_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func TestQUICTokenLengthAllWireWidthsAndLiteralBinary(t *testing.T) {
	for _, version := range []uint32{1, 0x6b3343cf} {
		for _, width := range []int{1, 2, 4, 8} {
			for _, length := range []int{0, 1, 63, 64, 300} {
				if width == 1 && length >= 64 {
					continue
				}
				first := byte(0xc0)
				if version != 1 {
					first = 0xd0
				}
				wire := binary.BigEndian.AppendUint32([]byte{first}, version)
				wire = append(wire, 0, 0)
				encoded := make([]byte, 8)
				binary.BigEndian.PutUint64(encoded, uint64(length))
				encoded = encoded[8-width:]
				prefix := map[int]byte{1: 0, 2: 1, 4: 2, 8: 3}[width]
				encoded[0] |= prefix << 6
				token := bytes.Repeat([]byte{0xff, 0, 0xc3}, (length+2)/3)[:length]
				wire = append(wire, encoded...)
				wire = append(wire, token...)
				wire = append(wire, 2, 0, 1)
				got, err := ParseStructured(wire, "application-layer.quic", "QUIC")
				require.NoError(t, err)
				fields := got["fields"].(map[string]any)
				tl, ok := fields["QUICToken"].(map[string]any)
				require.True(t, ok, "version %x width %d length %d must not fall back to opaque payload", version, width, length)
				if width == 1 {
					require.Equal(t, uint8(length), tl["Token Length"])
					require.NotContains(t, tl, "Length Encoding")
				} else {
					require.Equal(t, uint64(length), tl["Token Length"])
					require.Equal(t, prefix, tl["Length Encoding"])
				}
				if length > 0 {
					require.Equal(t, token, tl["Token"])
				} else {
					require.NotContains(t, tl, "Token")
				}
				require.Equal(t, []any{uint8(2), uint8(0), uint8(1)}, fields["Protected Payload"])
				reference, err := structuredNodeReference(wire, "application-layer.quic", "QUIC")
				require.NoError(t, err)
				require.Equal(t, reference, got, "node tree and structured paths agree")
			}
		}
	}
}

func TestQUICTokenVarintPreservesBitOffsetsAndFollowingField(t *testing.T) {
	for _, width := range []int{1, 2, 4, 8} {
		for offset := uint64(0); offset < 8; offset++ {
			length := binary.BigEndian.AppendUint64(nil, 3)[8-width:]
			length[0] |= map[int]byte{1: 0, 2: 1, 4: 2, 8: 3}[width] << 6
			wire := append(length, 0xff, 0, 0xc3)
			var packed bytes.Buffer
			writer := base.NewBitWriter(&packed)
			if offset > 0 {
				require.NoError(t, writer.WriteBits([]byte{0x55}, offset))
			}
			require.NoError(t, writer.WriteBits(wire, uint64(len(wire))*8))
			require.NoError(t, writer.WriteBits([]byte{0xd3}, 8))
			if offset > 0 {
				require.NoError(t, writer.WriteBits([]byte{0}, 8-offset))
			}
			source := fmt.Sprintf("endian: little\nPackage:\n  Wrapped:\n    operator: |\n      if %d > 0 { this.ProcessSubNode(\"Prefix\") }\n      this.GetSubNode(\"Token Fields\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Token Fields\")\n      this.ProcessSubNode(\"Sentinel\")\n      if %d > 0 { this.ProcessSubNode(\"Padding\") }\n    Prefix: uint8,%dbit\n    Token Fields: \"import:application-layer/quic.yaml;node:QUICInitialTokenFields\"\n    Sentinel: uint8\n    Padding: uint8,%dbit\n", offset, len(wire), offset, offset, 8-offset)
			root := latTestInline(t, source)
			root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
			require.NoError(t, root.ParseSubNode(base.NewBitReader(bytes.NewReader(packed.Bytes())), "Wrapped"))
			n := base.GetNodeByPath(root, "@Wrapped")
			body := protocolCorpusFindNode(n, "Token Fields")
			protocolCorpusRequireValue(t, body, "Token Length", uint64(3))
			protocolCorpusRequireValue(t, body, "Token", []byte{0xff, 0, 0xc3})
			protocolCorpusRequireValue(t, n, "Sentinel", uint64(0xd3))
			require.Equal(t, packed.Bytes(), NodeToBytes(n), "byte re-emission preserves prefix, token and following field")
		}
	}
}

func TestQUICTokenExplicitEntryRejectsIncompleteLengthsAndBodies(t *testing.T) {
	valid, err := ParseStructured([]byte{2, 0xff, 0}, "application-layer.quic", "QUICInitialTokenFields")
	require.NoError(t, err, "a valid explicit entry must work before checking rejection controls")
	require.Equal(t, []byte{0xff, 0}, valid["fields"].(map[string]any)["Token"])
	for _, bad := range [][]byte{nil, {0x40}, {0x80, 0, 0}, {0xc0, 0, 0, 0, 0, 0, 0}, {0x40, 3, 1, 2}, {0x80, 0, 0, 3, 1, 2}, {0xc0, 0, 0, 0, 0, 0, 0, 3, 1, 2}, {0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}} {
		_, err := ParseStructured(bad, "application-layer.quic", "QUICInitialTokenFields")
		require.Error(t, err, "explicit token fields require their complete declared input")
	}
}
