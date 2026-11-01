package bin_parser

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func TestICMPv4CanonicalHeaderUnionAndBounds(t *testing.T) {
	for _, tc := range []struct {
		name string
		wire []byte
		want map[string]any
	}{
		{"MTU", []byte{3, 4, 0, 0, 0x12, 0x34, 5, 0xd4}, map[string]any{"Type": byte(3), "Code": byte(4), "MTU": uint16(1492), "Rest of Header": []byte{0x12, 0x34, 5, 0xd4}, "Quoted Packet": []byte{}}},
		{"gateway", []byte{5, 1, 0, 0, 192, 0, 2, 9}, map[string]any{"Type": byte(5), "Code": byte(1), "Gateway": "192.0.2.9", "Rest of Header": []byte{192, 0, 2, 9}, "Quoted Packet": []byte{}}},
		{"pointer", []byte{12, 0, 0, 0, 19, 0, 0, 0}, map[string]any{"Type": byte(12), "Code": byte(0), "Pointer": byte(19), "Rest of Header": []byte{19, 0, 0, 0}, "Quoted Packet": []byte{}}},
		{"timestamps", []byte{14, 0, 0, 0, 0x12, 0x34, 0x56, 0x78, 1, 2, 3, 4, 0x80, 0, 0, 0, 0xff, 0xff, 0xff, 0xff}, map[string]any{"Type": byte(14), "Code": byte(0), "Identifier": uint16(0x1234), "Sequence": uint16(0x5678), "Originate Timestamp": uint32(0x01020304), "Receive Timestamp": uint32(0x80000000), "Transmit Timestamp": uint32(0xffffffff)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v, err := ParseStructured(tc.wire, "network_icmp", "ICMPv4HeaderFields")
			require.NoError(t, err)
			fields := v["fields"].(map[string]any)["Message"].(map[string]any)
			require.Equal(t, tc.want, fields)
			shared, err := stream_parser.DecodeICMPv4HeaderWire(tc.wire, len(tc.wire))
			require.NoError(t, err)
			require.Equal(t, fields, shared)
			if tc.wire[0] == 14 {
				for n := 0; n < len(tc.wire); n++ {
					_, err := ParseStructured(tc.wire[:n], "network_icmp", "ICMPv4HeaderFields")
					require.Error(t, err, "short timestamp length%d", n)
				}
			}
		})
	}
	_, err := stream_parser.DecodeICMPv4HeaderWire(make([]byte, 8), 7)
	require.ErrorIs(t, err, stream_parser.ErrObservedWireBudget)
	for n := 0; n < 8; n++ {
		_, err := ParseStructured(make([]byte, n), "network_icmp", "ICMPv4HeaderFields")
		require.Error(t, err)
	}
}
