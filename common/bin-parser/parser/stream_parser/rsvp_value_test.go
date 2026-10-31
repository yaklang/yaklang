package stream_parser

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"math/big"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func rsvpTestObject(class, ctype byte, value []byte) []byte {
	w := []byte{byte((4 + len(value)) >> 8), byte(4 + len(value)), class, ctype}
	return append(w, value...)
}
func rsvpTestMessage(typ byte, body []byte) []byte {
	w := append([]byte{0x10, typ, 0, 0, 64, 0, 0, 0}, body...)
	binary.BigEndian.PutUint16(w[6:], uint16(len(w)))
	sum := ospfv2Sum(w)
	if sum == 0 {
		sum = 0xffff
	}
	binary.BigEndian.PutUint16(w[2:], sum)
	return w
}
func rsvpFirstBody(t *testing.T, f map[string]any) map[string]any {
	t.Helper()
	return f["Objects"].([]map[string]any)[0]["Body"].(map[string]any)
}

func TestRSVPHeaderBudgetChecksumAndPartialPrimitiveEvidence(t *testing.T) {
	header := rsvpTestMessage(1, nil)
	f, err := DecodeRSVPWire(header, 0)
	require.NoError(t, err)
	require.True(t, f["Checksum Verified"].(bool))
	require.Empty(t, f["Objects"])
	header[2], header[3] = 0, 0
	f, err = DecodeRSVPWire(header, 0)
	require.NoError(t, err)
	require.True(t, f["Checksum Omitted"].(bool))
	require.False(t, f["Checksum Verified"].(bool))
	require.False(t, f["Authentication Verified"].(bool))
	w := rsvpTestMessage(1, rsvpTestObject(5, 1, []byte{0, 0, 0x75, 0x30}))
	for _, cut := range []int{0, 1, 2, 3, 7, 9, 14} {
		f, err = DecodeRSVPWire(w[:cut], 64)
		require.Error(t, err)
		require.False(t, errors.Is(err, ErrRSVPWireFormat))
		require.False(t, f["Wire Grammar Complete"].(bool))
		if cut == 9 {
			require.Empty(t, f["Objects"], "a single partial object-length byte is not an observed object")
		}
		if cut == 14 {
			require.NotContains(t, rsvpFirstBody(t, f), "Refresh Period Milliseconds")
		}
	}
	bad := bytes.Clone(w)
	binary.BigEndian.PutUint16(bad[6:], 4)
	f, err = DecodeRSVPWire(bad, 64)
	require.ErrorIs(t, err, ErrRSVPWireFormat)
	require.Equal(t, uint64(4), f["Header"].(map[string]any)["Message Length"])
	require.NotContains(t, f, "Objects")
	w[len(w)-1] ^= 1
	f, err = DecodeRSVPWire(w, 0)
	require.ErrorIs(t, err, ErrRSVPWireIntegrity)
	require.ErrorIs(t, err, ErrObservedWireBudget)
	require.Empty(t, f["Objects"])
}

func TestRSVPUnknownObjectDoesNotConsumeFollowingKnownObject(t *testing.T) {
	body := append(rsvpTestObject(250, 99, []byte("ABCD")), rsvpTestObject(5, 1, []byte{0, 0, 0x75, 0x30})...)
	w := rsvpTestMessage(1, body)
	f, err := DecodeRSVPWire(w, 64)
	require.NoError(t, err)
	require.True(t, f["Wire Grammar Complete"].(bool))
	require.False(t, f["Field Layouts Complete"].(bool))
	rows := f["Objects"].([]map[string]any)
	require.Len(t, rows, 2)
	require.Equal(t, []byte("ABCD"), rows[0]["Body"].(map[string]any)["Opaque Body"])
	known := rows[1]["Body"].(map[string]any)
	require.Equal(t, uint64(30000), known["Refresh Period Milliseconds"])
	require.Equal(t, []any{uint64(20), uint64(24)}, known["Wire Field Ranges"].(map[string]any)["Refresh Period Milliseconds"])
	w[12] = 'X'
	require.Equal(t, []byte("ABCD"), rows[0]["Body"].(map[string]any)["Opaque Body"])
	f, err = DecodeRSVPWire(rsvpTestMessage(12, body), 64)
	require.ErrorIs(t, err, ErrObservedWireContext)
	require.NotContains(t, f, "Objects")
	require.Equal(t, body, f["Unparsed Body"])
	f, err = DecodeRSVPWire([]byte{0x20, 1, 0, 0, 64, 0, 0, 8}, 64)
	require.ErrorIs(t, err, ErrObservedWireContext)
	h := f["Header"].(map[string]any)
	require.Equal(t, uint64(2), h["Version"])
	require.NotContains(t, h, "Message Type")
	require.NotContains(t, h, "Flags")
}

func TestRSVPIPv6BaseObjectsAndUnselectedTypes(t *testing.T) {
	address := []byte{0x20, 1, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
	for _, tc := range []struct {
		class byte
		key   string
		tail  []byte
	}{
		{1, "Destination Address", []byte{17, 0, 0x40, 4}}, {3, "Hop Address", []byte{1, 2, 3, 4}}, {6, "Error Node Address", []byte{0, 0, 0, 0}},
		{10, "Source Address", []byte{0, 0, 0x40, 4}}, {11, "Source Address", []byte{0, 0, 0x40, 4}}, {15, "Receiver Address", nil},
	} {
		f, err := DecodeRSVPWire(rsvpTestMessage(1, rsvpTestObject(tc.class, 2, append(bytes.Clone(address), tc.tail...))), 64)
		require.NoError(t, err)
		b := rsvpFirstBody(t, f)
		require.Equal(t, "2001:db8::1", b[tc.key])
		require.Equal(t, []any{uint64(12), uint64(28)}, b["Wire Field Ranges"].(map[string]any)[tc.key])
		require.False(t, f["Reservation State Inferred"].(bool))
	}
	for _, class := range []byte{5, 8} {
		f, err := DecodeRSVPWire(rsvpTestMessage(1, rsvpTestObject(class, 2, []byte{1, 2, 3, 4})), 64)
		require.NoError(t, err)
		require.False(t, f["Field Layouts Complete"].(bool))
		require.Equal(t, []byte{1, 2, 3, 4}, rsvpFirstBody(t, f)["Opaque Body"])
	}
}

func TestRSVPScopeBudgetAndPartialLastAddress(t *testing.T) {
	w := rsvpTestMessage(1, rsvpTestObject(7, 1, []byte{192, 0, 2, 1, 192, 0, 2, 2}))
	f, err := DecodeRSVPWire(w, 2)
	require.ErrorIs(t, err, ErrObservedWireBudget)
	require.True(t, f["Checksum Verified"].(bool))
	addresses := rsvpFirstBody(t, f)["Addresses"].([]map[string]any)
	require.Len(t, addresses, 1)
	require.Equal(t, "192.0.2.1", addresses[0]["Address"])
	partial := []byte{0x20, 1, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0x20, 1, 0x0d, 0xb8}
	f, err = DecodeRSVPWire(rsvpTestMessage(1, rsvpTestObject(7, 2, partial)), 64)
	require.ErrorIs(t, err, ErrRSVPWireFormat)
	b := rsvpFirstBody(t, f)
	require.Len(t, b["Addresses"], 1)
	require.Equal(t, partial[16:], b["Unselected Address Tail"])
}

func TestRSVPWrongLengthRetainsCapturedPrefixAndOwnTrailingRange(t *testing.T) {
	w := rsvpTestMessage(1, []byte{0, 12, 1, 1, 203, 0, 113, 9})
	f, err := DecodeRSVPWire(w, 64)
	require.ErrorIs(t, err, ErrRSVPWireFormat)
	b := rsvpFirstBody(t, f)
	require.Equal(t, "203.0.113.9", b["Destination Address"])
	require.Equal(t, []any{uint64(12), uint64(16)}, b["Wire Field Ranges"].(map[string]any)["Destination Address"])
	require.NotContains(t, b, "Protocol ID")
	require.NotContains(t, b, "Flags")
	require.NotContains(t, b, "Destination Port")
	require.NotContains(t, f, "Unparsed Body")
	header := rsvpTestMessage(1, nil)
	f, err = DecodeRSVPWire(append(header, 1, 2, 3, 4), 64)
	require.ErrorIs(t, err, ErrRSVPWireFormat)
	require.True(t, f["Checksum Verified"].(bool))
	require.Equal(t, []byte{1, 2, 3, 4}, f["Trailing Bytes"])
	require.Equal(t, []any{uint64(8), uint64(12)}, f["Wire Field Ranges"].(map[string]any)["Trailing Bytes"])
}

func FuzzRSVPBoundedOccurrences(f *testing.F) {
	f.Add(rsvpTestMessage(1, rsvpTestObject(5, 1, []byte{0, 0, 0x75, 0x30})), uint8(2))
	f.Add(rsvpTestMessage(1, rsvpTestObject(7, 1, []byte{192, 0, 2, 1, 192, 0, 2, 2})), uint8(2))
	f.Add(rsvpTestMessage(1, rsvpTestObject(12, 2, rsvpIntservTestBody(rsvpIntservTestWords(1, 0, rsvpIntservTestToken())))), uint8(3))
	f.Add(rsvpTestMessage(1, rsvpTestObject(13, 2, rsvpIntservTestBody(rsvpIntservTestWords(5, 0x80, nil)))), uint8(2))
	f.Fuzz(func(t *testing.T, w []byte, budget uint8) {
		if len(w) > 65535 {
			return
		}
		before := bytes.Clone(w)
		fields, err := DecodeRSVPWire(w, int(budget))
		require.Equal(t, before, w)
		if fields != nil {
			require.Equal(t, err == nil, fields["Wire Grammar Complete"])
		}
		var walk func(any)
		walk = func(value any) {
			switch v := value.(type) {
			case map[string]any:
				if ranges, ok := v["Wire Field Ranges"].(map[string]any); ok {
					for key, raw := range ranges {
						span := raw.([]any)
						a, b := span[0].(uint64), span[1].(uint64)
						require.LessOrEqual(t, a, b)
						require.LessOrEqual(t, b, uint64(len(w)))
						require.Contains(t, v, key)
						switch literal := v[key].(type) {
						case uint64:
							require.LessOrEqual(t, b-a, uint64(8))
							var actual uint64
							for _, octet := range w[a:b] {
								actual = actual<<8 | uint64(octet)
							}
							require.Equal(t, actual, literal)
						case string:
							if key == "Value" && v["IEEE754 Bits"] != nil {
								bits := binary.BigEndian.Uint32(w[a:b])
								if bits&0x7f800000 != 0x7f800000 {
									got, ok := new(big.Rat).SetString(literal)
									require.True(t, ok)
									want := new(big.Rat).SetFloat64(float64(math.Float32frombits(bits)))
									require.Zero(t, got.Cmp(want))
								}
								continue
							}
							require.Contains(t, []uint64{4, 16}, b-a)
							require.Equal(t, net.IP(w[a:b]).String(), literal)
						case []byte:
							require.Equal(t, w[a:b], literal)
						}
					}
				}
				for key, c := range v {
					if key != "Wire Field Ranges" {
						walk(c)
					}
				}
			case []map[string]any:
				for _, row := range v {
					require.NotNil(t, row)
					walk(row)
				}
			}
		}
		walk(fields)
	})
}
