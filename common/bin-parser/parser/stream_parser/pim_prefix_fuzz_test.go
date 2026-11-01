package stream_parser

import (
	"bytes"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

// Check field contents against original bytes, not a second parser result.
func pimCheckPrefixOwnership(t *testing.T, wire []byte, value any) {
	t.Helper()
	switch row := value.(type) {
	case map[string]any:
		if ranges, ok := row["Wire Field Ranges"].(map[string]any); ok {
			for key, raw := range ranges {
				span, ok := raw.([]any)
				require.True(t, ok)
				require.Len(t, span, 2)
				start, end := span[0].(uint64), span[1].(uint64)
				require.LessOrEqual(t, start, end)
				require.LessOrEqual(t, end, uint64(len(wire)))
				field, present := row[key]
				require.True(t, present, key)
				require.NotNil(t, field, key)
				b := wire[start:end]
				switch v := field.(type) {
				case []byte:
					require.Equal(t, b, v, key)
				case string:
					address, err := netip.ParseAddr(v)
					require.NoError(t, err, key)
					require.Equal(t, b, address.AsSlice(), key)
				case uint64:
					require.LessOrEqual(t, len(b), 8)
					var word uint64
					for _, octet := range b {
						word = word<<8 | uint64(octet)
					}
					switch key {
					case "Version", "IP Version":
						word >>= 4
					case "Type":
						word &= 15
					case "Metric Preference":
						word &= 0x7fffffff
					case "Propagation Delay":
						word &= 0x7fff
					}
					require.Equal(t, word, v, key)
				case bool:
					var word uint64
					for _, octet := range b {
						word = word<<8 | uint64(octet)
					}
					mask := map[string]uint64{"Border": 1 << 31, "Null Register": 1 << 30,
						"Prune Indicator": 128, "Prune Now": 64, "Assert Override": 32,
						"Tracking Support": 1 << 15, "Bidirectional": 128, "Admin Scope Zone": 1,
						"Sparse": 4, "Wildcard": 2}[key]
					if key == "RPT" {
						mask = 1
						if len(b) == 4 {
							mask = 1 << 31
						}
					}
					require.NotZero(t, mask, key)
					require.Equal(t, word&mask != 0, v, key)
				default:
					t.Fatalf("non-scalar wire field %s: %T", key, field)
				}
			}
		}
		for key, child := range row {
			if key != "Wire Field Ranges" {
				pimCheckPrefixOwnership(t, wire, child)
			}
		}
	case []map[string]any:
		for _, child := range row {
			pimCheckPrefixOwnership(t, wire, child)
		}
	}
}

func FuzzPIMCapturedPrefixOwnership(f *testing.F) {
	for _, wire := range pimTestProgressiveMessages() {
		f.Add(wire, uint16(len(wire)), uint8(0))
		f.Add(wire, uint16(len(wire)-1), uint8(1))
	}
	f.Fuzz(func(t *testing.T, wire []byte, cutWord uint16, mode uint8) {
		if len(wire) > 65535 {
			return
		}
		original := bytes.Clone(wire)
		cut := int(cutWord) % (len(wire) + 1)
		context := PIMWireContext{}
		switch mode % 4 {
		case 1:
			context.DeclaredBytes = -1
		case 2:
			context.DeclaredBytes = len(wire) + 1
		case 3:
			context.DeclaredBytes = cut
		}
		fields, err := DecodePIMControl(wire[:cut], context, 64)
		require.Equal(t, original, wire)
		pimCheckPrefixOwnership(t, wire[:cut], fields)
		if err == nil {
			require.True(t, context.DeclaredBytes == 0 || context.DeclaredBytes == cut)
		}
	})
}
