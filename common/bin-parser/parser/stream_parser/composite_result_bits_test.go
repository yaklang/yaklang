package stream_parser

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func TestCompositeNodeResultBitSpans(t *testing.T) {
	for offset := uint64(0); offset < 8; offset++ {
		t.Run(fmt.Sprint(offset), func(t *testing.T) {
			var packed bytes.Buffer
			w := base.NewBitWriter(&packed)
			if offset != 0 {
				require.NoError(t, w.WriteBits([]byte{0x55}, offset))
			}
			require.NoError(t, w.WriteBits([]byte{0x1a}, 5))
			require.NoError(t, w.WriteBits([]byte{0x6b}, 7))
			if padding := (8 - (offset+12)%8) % 8; padding != 0 {
				require.NoError(t, w.WriteBits([]byte{0}, padding))
			}
			root := giopBridgeInlineRoot(t, fmt.Sprintf(`endian: big
Package:
  Message:
    operator: |
      if %d > 0 { this.ProcessSubNode("Prefix") }
      this.ProcessSubNode("Composite")
    Prefix: uint8,%dbit
    Composite:
      First: uint8,5bit
      Second: uint8,7bit
`, offset, offset))
			root.Cfg.SetItem(CfgLength, offset+12)
			require.NoError(t, root.ParseSubNode(base.NewBitReader(bytes.NewReader(packed.Bytes())), "Message"))
			n := base.GetNodeByPath(root, "@Message.Composite")
			require.NotNil(t, n)
			require.False(t, NodeHasResult(n))
			r := base.NewBitReader(bytes.NewReader(packed.Bytes()))
			if offset != 0 {
				_, err := r.ReadBits(offset)
				require.NoError(t, err)
			}
			want, err := r.ReadBits(12)
			require.NoError(t, err)
			for _, raw := range []bool{true, false} {
				got, err := getNodeResult(n, raw)
				require.NoError(t, err)
				require.Equal(t, want, got)
			}
			require.Equal(t, want, GetBytesByNode(n))
			result, err := n.Result()
			require.NoError(t, err)
			require.True(t, result.IsStruct())
			require.Len(t, result.Children(), 2)
			require.Equal(t, uint64(12), CalcNodeConsumedLength(n))
			last := n.Children[1]
			saved := last.Cfg.GetItem(CfgNodeResult)
			last.Cfg.SetItem(CfgNodeResult, [2]uint64{offset + 5, uint64(packed.Len()+1) * 8})
			_, err = getNodeResult(n, true)
			require.ErrorContains(t, err, "invalid result span")
			last.Cfg.SetItem(CfgNodeResult, saved)
		})
	}
}
