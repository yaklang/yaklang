package stream_parser

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func TestWSMPNativeScalarExpressions(t *testing.T) {
	definitions, err := wsmpDefinitions()
	require.NoError(t, err)
	require.Equal(t, wsmpPSIDOut, definitions["WSMPPSID"].Cfg.GetString("out"))
	require.Equal(t, wsmpLengthOut, definitions["WSMPLength"].Cfg.GetString("out"))
	for encoded, want := range map[string]int{"00": 0, "7f": 127, "8000": 128, "bfff": 16511, "c00000": 16512, "dfffff": 2113663, "e0000000": 2113664, "efffffff": 270549119} {
		wire, err := hex.DecodeString(encoded)
		require.NoError(t, err)
		data := &base.NodeValue{Value: []*base.NodeValue{{Value: wire}}}
		got, handled := evalWSMPScalarOut(wsmpPSIDOut, data)
		require.True(t, handled)
		require.IsType(t, int(0), got)
		require.Equal(t, want, got)
		_, handled = evalWSMPScalarOut(wsmpPSIDOut+"return 17", data)
		require.False(t, handled)
	}
	for _, value := range []int{0, 1, 127, 128, 255, 256, 16383} {
		children := []*base.NodeValue{{Value: uint8(value)}}
		if value >= 128 {
			children = []*base.NodeValue{{Value: uint8(128 | value>>8)}, {Value: uint8(value)}}
		}
		got, handled := evalWSMPScalarOut(wsmpLengthOut, &base.NodeValue{Value: children})
		require.True(t, handled)
		require.IsType(t, int(0), got)
		require.Equal(t, value, got)
	}
	for _, data := range []*base.NodeValue{nil, {}, {Value: []any{1}}, {Value: []*base.NodeValue{nil}}, {Value: []*base.NodeValue{{Value: uint16(2)}}}} {
		for _, code := range []string{wsmpPSIDOut, wsmpLengthOut} {
			_, handled := evalWSMPScalarOut(code, data)
			require.False(t, handled)
		}
	}
}

// Deterministic mutations exercise acceptance parity without deriving the
// expectations from the native decoder. Complete accepted trees must agree.
func TestWSMPNativeMutationDifferential(t *testing.T) {
	for fixture, wire := range wsmpNativeWires(t)[:8] {
		for offset := range wire {
			if offset >= len(wire)-2 {
				break
			}
			for _, mask := range []byte{1, 0x80} {
				t.Run(fmt.Sprintf("record-%d/byte-%d/mask-%02x", fixture, offset, mask), func(t *testing.T) {
					input := bytes.Clone(wire)
					input[offset] ^= mask
					parse := func(legacy bool) (*base.Node, error) {
						root, err := base.ParseRule("wsmp.yaml")
						require.NoError(t, err)
						root.Cfg.SetItem(CfgLength, uint64(len(input))*8)
						root.Ctx.SetItem("wsmpLegacy", legacy)
						root.Ctx.SetItem("outScalarLegacy", legacy)
						err = root.ParseSubNode(base.NewBitReader(bytes.NewReader(input)), "WSMP")
						return base.GetNodeByPath(root, "@WSMP"), err
					}
					fast, fe := parse(false)
					legacy, le := parse(true)
					require.Equal(t, le == nil, fe == nil, "native %v; YAML %v", fe, le)
					if fe == nil {
						wsmpCompareTrees(t, fast, legacy)
					}
				})
			}
		}
	}
}
