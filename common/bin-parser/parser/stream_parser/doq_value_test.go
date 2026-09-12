package stream_parser

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDoQIndependentMappingsAndBounds(t *testing.T) {
	decodeHex := func(s string) []byte { b, e := hex.DecodeString(s); require.NoError(t, e); return b }
	query := decodeHex("000000000001000000000001076578616d706c6503636f6d0000010001000029ffff000000000000")
	response := decodeHex("000080800001000100000001076578616d706c6503636f6d0000010001c00c000100010000542c00045db8d8220000290200000000000000")
	prefix := func(msg []byte) []byte {
		wire := make([]byte, 2, len(msg)+2)
		binary.BigEndian.PutUint16(wire, uint16(len(msg)))
		return append(wire, msg...)
	}
	for _, tc := range []struct {
		wire     []byte
		mode     string
		response bool
	}{{query, "draft-i00", false}, {response, "draft-i00", true}, {prefix(query), "rfc9250", false}, {prefix(response), "rfc9250", true}, {append(prefix(response), prefix(response)...), "rfc9250", true}} {
		fields, _, err := decodeDoQStream(tc.wire, tc.mode, tc.response)
		require.NoError(t, err)
		var walk func([]doqField, int) int
		walk = func(fields []doqField, pos int) int {
			for _, f := range fields {
				require.Equal(t, pos, f.Start, f.Name)
				require.GreaterOrEqual(t, f.End, f.Start)
				require.LessOrEqual(t, f.End, len(tc.wire))
				if f.Type == "" {
					require.Equal(t, f.End, walk(f.Children, f.Start), f.Name)
				}
				pos = f.End
			}
			return pos
		}
		require.Equal(t, len(tc.wire), walk(fields, 0))
		for cut := 0; cut < len(tc.wire); cut++ {
			_, _, err := decodeDoQStream(tc.wire[:cut], tc.mode, tc.response)
			// A prefix ending after a complete response message is itself a
			// syntactically complete response stream; FIN remains external.
			if tc.mode == "rfc9250" && tc.response && cut == len(response)+2 {
				require.NoError(t, err)
			} else {
				require.Error(t, err, "prefix %d", cut)
			}
		}
	}
	for _, msg := range [][]byte{query, response} {
		_, _, err := decodeDoQStream(msg, "rfc9250", msg[2]&0x80 != 0)
		require.ErrorContains(t, err, "message length must be at least 12")
	}
	bad := func(msg []byte, pos int, b byte) []byte { out := bytes.Clone(msg); out[pos] = b; return out }
	for _, tc := range []struct {
		wire     []byte
		mode     string
		response bool
	}{
		{bad(query, 0, 1), "draft-i00", false}, {query, "draft-i00", true}, {response, "draft-i00", false},
		{bad(query, 2, 0x30), "draft-i00", false}, {bad(query, 3, 0x40), "draft-i00", false},
		{bad(query, 4, 5), "draft-i00", false}, {bad(query, 12, 64), "draft-i00", false},
		{bad(response, 30, 29), "draft-i00", true}, // self-reference pointer
		{bad(response, 30, 0), "draft-i00", true},  // header is not a name
		{bad(response, 40, 5), "draft-i00", true},  // A RData length
		{append(bytes.Clone(query), 0), "draft-i00", false},
		{append(prefix(query), prefix(query)...), "rfc9250", false},
		{append(prefix(response), 0), "rfc9250", true},
		{query, "unknown", false},
	} {
		_, _, err := decodeDoQStream(tc.wire, tc.mode, tc.response)
		require.Error(t, err)
	}
	// Existing OPT is empty; insert an independently serialized option.
	for _, code := range []uint16{11, 12, 65000} {
		wire := bytes.Clone(query)
		wire[len(wire)-1] = 5
		wire = append(wire, byte(code>>8), byte(code), 0, 1, 0)
		_, _, err := decodeDoQStream(wire, "draft-i00", false)
		if code == 11 {
			require.ErrorContains(t, err, "edns-tcp-keepalive")
		} else {
			require.NoError(t, err)
		}
		wire[len(wire)-2] = 2
		_, _, err = decodeDoQStream(wire, "draft-i00", false)
		require.Error(t, err)
	}
	for _, count := range []int{256, 257} {
		_, _, err := decodeDoQStream(bytes.Repeat(prefix(response), count), "rfc9250", true)
		if count == 256 {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "message count")
		}
	}
}
