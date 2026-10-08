package bin_parser

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

func depthFieldValues(v any, key string) []any {
	var out []any
	switch x := v.(type) {
	case map[string]any:
		for k, value := range x {
			if k == key {
				out = append(out, value)
			}
			out = append(out, depthFieldValues(value, key)...)
		}
	case []any:
		for _, value := range x {
			out = append(out, depthFieldValues(value, key)...)
		}
	}
	return out
}

func TestProtocolMergeWebSocketProjection(t *testing.T) {
	wire := mustHex(t, "818537fa213d7f9f4d5158") // RFC 6455 section 5.7.
	n, err := parser.ParseBinary(bytes.NewReader(wire), "application-layer.websocket", "WebSocket")
	require.NoError(t, err)
	require.Equal(t, []any{"Hello"}, depthFieldValues(NodeToMap(n), "Text"))
	require.Equal(t, wire, NodeToBytes(n), "application projection must not rewrite captured bytes")
	// Non-UTF-8 binary/control payloads must remain bytes in JSON projections.
	wire = []byte{0x82, 0x82, 1, 2, 3, 4, 0xfe, 0x82}
	n, err = parser.ParseBinary(bytes.NewReader(wire), "application-layer.websocket", "WebSocket")
	require.NoError(t, err)
	first := depthFieldValues(NodeToMap(n), "Binary")[0].([]byte)
	require.Equal(t, []byte{0xff, 0x80}, first)
	first[0] = 0
	require.Equal(t, []any{[]byte{0xff, 0x80}}, depthFieldValues(NodeToMap(n), "Binary"))
	v, err := n.Result()
	require.NoError(t, err)
	v.Child("Binary").Value.([]byte)[0] = 0
	again, err := n.Result()
	require.NoError(t, err)
	require.Equal(t, []byte{0xff, 0x80}, again.Child("Binary").Value)
	require.Equal(t, []any{[]byte{0xff, 0x80}}, depthFieldValues(NodeToMap(n), "Binary"))
	require.Equal(t, wire, NodeToBytes(n))
}

func TestProtocolMergeWebSocketBounds(t *testing.T) {
	for name, wire := range map[string][]byte{
		"one-byte-close":     {0x88, 1, 0},
		"nonminimal16":       {0x81, 126, 0, 1, 'x'},
		"nonminimal64":       {0x81, 127, 0, 0, 0, 0, 0, 0, 0, 1, 'x'},
		"invalid-close-code": {0x88, 2, 3, 0xed}, // 1005 cannot appear on the wire.
		"invalid-close-utf8": {0x88, 3, 3, 0xe8, 0xff},
		"invalid-text-utf8":  {0x81, 1, 0xff},
	} {
		t.Run(name, func(t *testing.T) { parseMustFail(t, wire, "application-layer.websocket", "WebSocket") })
	}
	for _, size := range []int{0, 125, 126, 65535, 65536, 1 << 20} {
		header := []byte{0x82, byte(size)}
		if size >= 126 && size <= 65535 {
			header = []byte{0x82, 126, byte(size >> 8), byte(size)}
		}
		if size > 65535 {
			header = make([]byte, 10)
			header[0], header[1] = 0x82, 127
			binary.BigEndian.PutUint64(header[2:], uint64(size))
		}
		wire := append(header, bytes.Repeat([]byte{0xff}, size)...)
		r := bytes.NewReader(wire)
		n, err := parser.ParseBinary(r, "application-layer.websocket", "WebSocket")
		require.NoError(t, err)
		require.Zero(t, r.Len(), "size %d must consume its complete payload", size)
		if size > 0 {
			require.Equal(t, []any{bytes.Repeat([]byte{0xff}, size)}, depthFieldValues(NodeToMap(n), "Binary"))
		}
	}
}

func TestProtocolMergePostgreSQLAmbiguousDirections(t *testing.T) {
	for _, typ := range []byte{'C', 'D'} {
		wire := pgTyped(typ, []byte("Sstatement\x00"))
		n, err := parser.ParseBinary(bytes.NewReader(wire), "application-layer.postgresql", "PostgreSQL")
		require.NoError(t, err, "frontend Close/Describe must not be forced through backend grammar")
		require.Empty(t, depthFieldValues(NodeToMap(n), "PostgreSQLCommandComplete"))
		require.Empty(t, depthFieldValues(NodeToMap(n), "PostgreSQLDataRow"))
	}
}

func TestProtocolMergePostgreSQLRepeatedValues(t *testing.T) {
	wire := pgTyped('P', []byte{0, 'S', 'E', 'L', 'E', 'C', 'T', ' ', '$', '1', 0, 0, 2, 0, 0, 0, 23, 0, 0, 0, 25})
	n, err := parser.ParseBinary(bytes.NewReader(wire), "application-layer.postgresql", "PostgreSQL")
	require.NoError(t, err)
	require.ElementsMatch(t, []any{uint32(23), uint32(25)}, depthFieldValues(NodeToMap(n), "OID"))
}

func TestProtocolMergeLDAPUnsolicitedAndOptionalValue(t *testing.T) {
	// RFC 4511 sections 4.4/4.4.1: messageID 0, Notice of Disconnection OID.
	wire := mustHex(t, "3024020100781f0a0102040004008a16312e332e362e312e342e312e313436362e3230303336")
	n := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.ldap_fields", "LDAPExtendedResponseFields")
	require.Equal(t, uint64(0), n.Cfg.GetItem("additionInfo").(map[string]any)["Message ID"])
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(mustHex(t, "300c02010078070a010204000400")), "application-layer.ldap_fields", "LDAPExtendedResponseFields")
	require.Error(t, err, "an unsolicited response needs its notification OID")
	// StartTLS ExtendedRequest has no requestValue.
	parseRule(t, mustHex(t, "301d02010177188016312e332e362e312e342e312e313436362e3230303337"), "application-layer.ldap", "LDAPMessage")
}

func TestProtocolMergePostgreSQLRowsAndFormats(t *testing.T) {
	// Three distinct columns: binary octets, NULL, and an empty value.
	body := []byte{0, 3, 0, 0, 0, 2, 0xff, 0x80, 0xff, 0xff, 0xff, 0xff, 0, 0, 0, 0}
	wire := pgTyped('D', body)
	r := bytes.NewReader(wire)
	n, err := parser.ParseBinaryWithConfig(r, "application-layer.postgresql", map[string]any{"postgresqlDirection": "backend"}, "PostgreSQL")
	require.NoError(t, err)
	require.Zero(t, r.Len())
	columns := depthFieldValues(NodeToMap(n), "Columns")
	require.Len(t, columns, 1)
	require.Len(t, columns[0].([]any), 3)
	require.Equal(t, []any{[]byte{0xff, 0x80}}, depthFieldValues(NodeToMap(n), "Value"))
	require.Equal(t, wire, NodeToBytes(n))
	for name, wire := range map[string][]byte{
		"bind-format":          pgTyped('B', []byte{0, 0, 0, 1, 0, 2, 0, 0, 0, 0}),
		"bind-format-count":    pgTyped('B', []byte{0, 0, 0, 2, 0, 0, 0, 1, 0, 0, 0, 0}),
		"bind-negative-length": pgTyped('B', []byte{0, 0, 0, 0, 0, 1, 0xff, 0xff, 0xff, 0xfe, 0, 0}),
	} {
		t.Run(name, func(t *testing.T) { parseMustFail(t, wire, "application-layer.postgresql", "PostgreSQL") })
	}
	column := append([]byte{0, 1, 'x', 0}, make([]byte, 18)...)
	column[len(column)-1] = 2
	parseMustFail(t, pgTyped('T', column), "application-layer.postgresql", "PostgreSQL")
}
