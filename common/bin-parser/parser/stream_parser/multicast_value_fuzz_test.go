package stream_parser

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

func FuzzMulticastMembershipWireBounds(f *testing.F) {
	for _, text := range []string{
		"220012340000000201010002ef010203c0000201c0000202deadbeef06000000ef040506",
		"1182000000000000ff950001c0000201",
		"8f0012340000000201010002ff3e000000000000000000000000123420010db800000000000000000000000100000000000000000000ffffc0000201deadbeef06000000ff0200000000000000000000000000fb",
		"82001234ffff0000ff3e000000000000000000000000abcdffff0000",
		"22", "8f0000", "", "110000000000000000",
	} {
		wire, err := hex.DecodeString(text)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(wire, uint8(0), uint16(4096))
	}
	f.Fuzz(func(t *testing.T, wire []byte, contextKind uint8, elements uint16) {
		if len(wire) > 65536 {
			return
		}
		original := bytes.Clone(wire)
		declared := len(wire)
		switch contextKind % 4 {
		case 1:
			declared = -1
		case 2:
			declared += 256
		case 3:
			if declared > 0 {
				declared--
			}
		}
		budget := int(elements)%513 + 1
		igmp, igmpErr := DecodeIGMPWire(wire, 65536, budget, IGMPWireContext{DeclaredBytes: declared})
		mld, mldErr := DecodeMLDWire(wire, 65536, budget, MLDWireContext{DeclaredBytes: declared})
		require.Equal(t, original, wire, "decoding must not mutate captured input")
		if igmpErr == nil {
			require.Equal(t, len(wire), declared, "unconfirmed/truncated enclosing extent cannot be a complete message")
			require.NotEmpty(t, wire)
			require.True(t, IGMPMembershipType(wire[0]))
		}
		if mldErr == nil {
			require.Equal(t, len(wire), declared)
			require.GreaterOrEqual(t, len(wire), 4)
			require.Contains(t, []byte{130, 131, 132, 143}, wire[0])
		}
		for _, value := range igmp.Values {
			require.GreaterOrEqual(t, value.Offset, 0)
			require.Positive(t, value.Length)
			require.LessOrEqual(t, value.Offset+value.Length, len(wire))
		}
		for _, value := range mld.Values {
			require.GreaterOrEqual(t, value.Offset, 0)
			require.Positive(t, value.Length)
			require.LessOrEqual(t, value.Offset+value.Length, len(wire))
		}
	})
}
