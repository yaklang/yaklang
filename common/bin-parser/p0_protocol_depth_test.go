package bin_parser

import (
	"bytes"
	"os"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

func TestLDAPWriteAndExtendedOperations(t *testing.T) {
	for _, tc := range []struct {
		entry, yamlChild, yamlField, wire string
		yamlValue, fieldValue             any
	}{
		{"LDAPModifyRequestFields", "ModifyRequest", "Object Name", "301d02010166180404636e3d783010300e0a010030090402636e3103040179", "cn=x", "cn=x"},
		{"LDAPAddRequestFields", "AddRequest", "Entry", "301802010168130404636e3d78300b30090402636e3103040178", "cn=x", "cn=x"},
		{"LDAPDelRequestFields", "DelRequest", "Entry", "30090201014a04636e3d78", "cn=x", "cn=x"},
		{"LDAPModifyDNRequestFields", "ModifyDNRequest", "New RDN", "30180201016c130406636e3d6f6c640406636e3d6e65770101ff", "cn=new", "cn=new"},
		{"LDAPCompareRequestFields", "CompareRequest", "Entry", "30140201016e0f0404636e3d7830070402636e040178", "cn=x", "cn=x"},
		{"LDAPAbandonRequestFields", "AbandonRequest", "Abandoned Message ID", "3006020101500102", []byte{2}, uint64(2)},
		{"LDAPExtendedRequestFields", "ExtendedRequest", "Request Name", "3020020101771b8016312e332e362e312e342e312e313436362e32303033378101ff", "1.3.6.1.4.1.1466.20037", "1.3.6.1.4.1.1466.20037"},
		{"LDAPModifyResponseFields", "LDAPResult", "Result Code", "300c02010167070a010004000400", []byte{0}, uint64(0)},
		{"LDAPExtendedResponseFields", "LDAPResult", "Result Code", "3024020101781f0a0100040004008a16312e332e362e312e342e312e313436362e3230303337", []byte{0}, uint64(0)},
	} {
		t.Run(tc.entry, func(t *testing.T) {
			wire := mustHex(t, tc.wire)
			hist := parseRule(t, wire, "application-layer.ldap", "LDAPMessage")
			require.Equal(t, []byte{1}, bytesVal(t, mustChild(t, hist, "Body", "MessageID")))
			got := mustChild(t, hist, "Body", "ProtocolOp", tc.yamlChild, tc.yamlField)
			switch want := tc.yamlValue.(type) {
			case string:
				require.Equal(t, want, strVal(t, got))
			case []byte:
				require.Equal(t, want, bytesVal(t, got))
			}
			eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 389, wire))
			require.NotNil(t, mustChild(t, eth, "IP", "TCP", "LDAPMessage", "Body", "ProtocolOp", tc.yamlChild, tc.yamlField))

			n := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.ldap_fields", tc.entry)
			protocolCorpusRequireValue(t, n, tc.yamlField, tc.fieldValue)
			require.Equal(t, wire, NodeToBytes(n))
			for cut := 0; cut < len(wire); cut++ {
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), "application-layer.ldap_fields", tc.entry)
				require.Error(t, err, "prefix %d", cut)
			}
		})
	}

	parseMustFail(t, mustHex(t, "3007020101770280"), "application-layer.ldap", "LDAPMessage")
	parseMustFail(t, mustHex(t, "301d02010166180404636e3d783010300e0a010030090402636e3103040179")[:8], "application-layer.ldap", "LDAPMessage")
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(mustHex(t, "3006020101500100")), "application-layer.ldap_fields", "LDAPAbandonRequestFields")
	require.Error(t, err)
	_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(mustHex(t, "300c02010167070a010004000400")), "application-layer.ldap_fields", "LDAPModifyRequestFields")
	require.Error(t, err)
}

func TestPostgreSQLParseBindExecuteRowMessages(t *testing.T) {
	parseMsg := pgTyped('P', []byte("stmt\x00SELECT $1\x00\x00\x01\x00\x00\x00\x17"))
	n := parseRule(t, parseMsg, "application-layer.postgresql", "PostgreSQL")
	require.Equal(t, uint64('P'), uintVal(t, n.Child("First")))
	require.Equal(t, "stmt", strVal(t, mustChild(t, n, "Payload", "PostgreSQLParse").Child("Statement")))
	require.Equal(t, "SELECT $1", strVal(t, mustChild(t, n, "Payload", "PostgreSQLParse").Child("Query")))
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, n, "Payload", "PostgreSQLParse").Child("Parameter Type Count")))
	require.Equal(t, uint64(23), uintVal(t, mustChild(t, n, "Payload", "PostgreSQLParse", "Type OID").Child("OID")))
	eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 5432, parseMsg))
	require.Equal(t, "SELECT $1", strVal(t, mustChild(t, eth, "IP", "TCP", "PostgreSQL", "Payload", "PostgreSQLParse").Child("Query")))

	bindBody := []byte{0, 's', 't', 'm', 't', 0, 0, 1, 0, 0, 0, 1, 0, 0, 0, 1, 'x', 0, 1, 0, 0}
	bindMsg := pgTyped('B', bindBody)
	b := parseRule(t, bindMsg, "application-layer.postgresql", "PostgreSQL")
	require.Equal(t, "", strVal(t, mustChild(t, b, "Payload", "PostgreSQLBind").Child("Portal")))
	require.Equal(t, "stmt", strVal(t, mustChild(t, b, "Payload", "PostgreSQLBind").Child("Statement")))
	require.Equal(t, "x", strVal(t, mustChild(t, b, "Payload", "PostgreSQLBind").Child("Parameter").Child("Value")))

	rowDesc := pgTyped('T', []byte{0, 1, 'i', 'd', 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 23, 0, 4, 0xff, 0xff, 0xff, 0xff, 0, 1})
	td := parseRule(t, rowDesc, "application-layer.postgresql", "PostgreSQL")
	col := mustChild(t, td, "Payload", "PostgreSQLRowDescription").Child("Column")
	require.Equal(t, "id", strVal(t, col.Child("Column Name")))
	require.Equal(t, uint64(23), uintVal(t, col.Child("Type OID")))
	require.Equal(t, uint64(1), uintVal(t, col.Child("Format Code")))

	dataRow := pgTyped('D', []byte{0, 1, 0, 0, 0, 4, 0, 0, 0, 3})
	dr := parseRule(t, dataRow, "application-layer.postgresql", "PostgreSQL")
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, dr, "Payload", "PostgreSQLDataRow").Child("Column Count")))
	require.Equal(t, "\x00\x00\x00\x03", strVal(t, mustChild(t, dr, "Payload", "PostgreSQLDataRow").Child("Column").Child("Value")))

	complete := pgTyped('C', append([]byte("SELECT 1"), 0))
	cc := parseRule(t, complete, "application-layer.postgresql", "PostgreSQL")
	require.Equal(t, "SELECT 1", strVal(t, mustChild(t, cc, "Payload", "PostgreSQLCommandComplete").Child("Tag")))
	ethC := parseEthernet(t, ipv4TCPFrame(t, 5432, 50000, complete))
	require.Equal(t, "SELECT 1", strVal(t, mustChild(t, ethC, "IP", "TCP", "PostgreSQL", "Payload", "PostgreSQLCommandComplete").Child("Tag")))

	execMsg := pgTyped('E', []byte{0, 0, 0, 0, 1})
	ex := protocolCorpusRequireBoundedRuleParse(t, execMsg, "application-layer.postgresql_fields", "PostgreSQLFrontendFields")
	protocolCorpusRequireValue(t, ex, "Portal Name", []byte{})
	info := ex.Cfg.GetItem("additionInfo").(map[string]any)
	require.Equal(t, "Execute", info["Message Name"])
	require.Equal(t, int64(1), info["Maximum Rows"])
	require.Equal(t, execMsg, NodeToBytes(ex))
	ethE := parseEthernet(t, ipv4TCPFrame(t, 50000, 5432, execMsg))
	require.Equal(t, uint64('E'), uintVal(t, mustChild(t, ethE, "IP", "TCP", "PostgreSQL").Child("First")))

	parseMustFail(t, parseMsg[:5], "application-layer.postgresql", "PostgreSQL")
	parseMustFail(t, []byte{0x00, 0x00, 0x00, 0x08, 0x00, 0x03, 0x00, 0x03}, "application-layer.postgresql", "Startup")
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(execMsg[:4]), "application-layer.postgresql_fields", "PostgreSQLFrontendFields")
	require.Error(t, err)

	frontend := protocolCorpusRequireBoundedRuleParse(t, parseMsg, "application-layer.postgresql_fields", "PostgreSQLFrontendFields")
	protocolCorpusRequireValue(t, frontend, "Statement Name", []byte("stmt"))
	protocolCorpusRequireValue(t, frontend, "Query Bytes", []byte("SELECT $1"))
	backend := protocolCorpusRequireBoundedRuleParse(t, rowDesc, "application-layer.postgresql_fields", "PostgreSQLBackendFields")
	protocolCorpusRequireValue(t, backend, "Column Name", []byte("id"))
	row := protocolCorpusRequireBoundedRuleParse(t, dataRow, "application-layer.postgresql_fields", "PostgreSQLBackendFields")
	protocolCorpusRequireValue(t, row, "Value", []byte{0, 0, 0, 3})
	tag := protocolCorpusRequireBoundedRuleParse(t, complete, "application-layer.postgresql_fields", "PostgreSQLBackendFields")
	protocolCorpusRequireValue(t, tag, "Command Tag", []byte("SELECT 1"))
}

func TestPostgreSQLNDPICaptureNamedMessages(t *testing.T) {
	records := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/ndpi/ndpi-postgresql.pcap")
	require.Len(t, records, 88)
	seen := map[string]int{}
	for frame, record := range records {
		p := gopacket.NewPacket(record, layers.LayerTypeEthernet, gopacket.Default)
		tcp := p.Layer(layers.LayerTypeTCP).(*layers.TCP)
		if len(tcp.Payload) == 0 {
			continue
		}
		entry := postgresqlFieldsOriginalEntry(frame+1, tcp.DstPort == 5432)
		n := protocolCorpusRequireBoundedRuleParse(t, tcp.Payload, "application-layer.postgresql_fields", entry)
		info := n.Cfg.GetItem("additionInfo").(map[string]any)
		list := []map[string]any{info}
		if msgs, ok := info["Messages"].([]map[string]any); ok {
			list = msgs
		}
		for _, m := range list {
			if name, ok := m["Message Name"].(string); ok {
				seen[name]++
			}
		}
	}
	for _, name := range []string{"Parse", "Bind", "Execute", "RowDescription", "DataRow", "CommandComplete"} {
		require.Greater(t, seen[name], 0, "ndpi-postgresql missing %s", name)
	}
}

func TestWebSocketNamedFramesAndUnmask(t *testing.T) {
	text := []byte{0x81, 0x05, 'H', 'e', 'l', 'l', 'o'}
	w := parseRule(t, text, "application-layer.websocket", "WebSocket")
	require.Equal(t, "Hello", strVal(t, w.Child("Text")))

	binaryFrame := []byte{0x82, 0x03, 1, 2, 3}
	b := parseRule(t, binaryFrame, "application-layer.websocket", "WebSocket")
	require.Equal(t, uint64(2), uintVal(t, b.Child("Opcode")))
	require.Equal(t, []byte{1, 2, 3}, bytesVal(t, b.Child("Binary")))

	cont := []byte{0x00, 0x03, 'a', 'b', 'c'}
	c := parseRule(t, cont, "application-layer.websocket", "WebSocket")
	require.Equal(t, uint64(0), uintVal(t, c.Child("Opcode")))
	require.Equal(t, "abc", strVal(t, c.Child("Continuation")))

	ping := []byte{0x89, 0x04, 'p', 'i', 'n', 'g'}
	p := parseRule(t, ping, "application-layer.websocket", "WebSocket")
	require.Equal(t, "ping", strVal(t, p.Child("Ping")))
	pong := []byte{0x8a, 0x04, 'p', 'o', 'n', 'g'}
	require.Equal(t, "pong", strVal(t, parseRule(t, pong, "application-layer.websocket", "WebSocket").Child("Pong")))

	closeF := []byte{0x88, 0x05, 0x03, 0xe8, 'b', 'y', 'e'}
	cl := parseRule(t, closeF, "application-layer.websocket", "WebSocket")
	require.Equal(t, uint64(1000), uintVal(t, cl.Child("Close Code")))
	require.Equal(t, "bye", strVal(t, cl.Child("Reason")))

	// RFC 6455 §5.7 masked "Hello".
	masked := []byte{0x81, 0x85, 0x37, 0xfa, 0x21, 0x3d, 0x7f, 0x9f, 0x4d, 0x51, 0x58}
	m := parseRule(t, masked, "application-layer.websocket", "WebSocket")
	require.Equal(t, []byte{0x37, 0xfa, 0x21, 0x3d}, bytesVal(t, m.Child("Masking Key")))
	require.Equal(t, "Hello", strVal(t, m.Child("Text")))
	eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 8080, masked))
	require.Equal(t, "Hello", strVal(t, mustChild(t, eth, "IP", "TCP", "WebSocket").Child("Text")))

	key := []byte{1, 2, 3, 4}
	maskClose := []byte{0x88, 0x82, key[0], key[1], key[2], key[3], 0x03 ^ key[0], 0xe8 ^ key[1]}
	mc := parseRule(t, maskClose, "application-layer.websocket", "WebSocket")
	require.Equal(t, uint64(1000), uintVal(t, mc.Child("Close Code")))

	maskPing := []byte{0x89, 0x85, key[0], key[1], key[2], key[3], 'h' ^ key[0], 'e' ^ key[1], 'l' ^ key[2], 'l' ^ key[3], 'o' ^ key[0]}
	require.Equal(t, "hello", strVal(t, parseRule(t, maskPing, "application-layer.websocket", "WebSocket").Child("Ping")))

	parseMustFail(t, []byte{0x83, 0x00}, "application-layer.websocket", "WebSocket")
	parseMustFail(t, []byte{0xc1, 0x00}, "application-layer.websocket", "WebSocket")
	parseMustFail(t, []byte{0x81, 0x05, 'h'}, "application-layer.websocket", "WebSocket")
	parseMustFail(t, []byte{0x09, 0x00}, "application-layer.websocket", "WebSocket")
	parseMustFail(t, []byte{0x89, 126, 0, 10}, "application-layer.websocket", "WebSocket")
}

func TestWebSocketNDPICaptureFrames(t *testing.T) {
	raw, err := os.ReadFile("testdata/protocol-corpus/captures/ndpi/ndpi-websocket.pcap")
	require.NoError(t, err)
	require.Greater(t, len(raw), 24)
	records := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/ndpi/ndpi-websocket.pcap")
	require.NotEmpty(t, records)
	parsed := 0
	for _, record := range records {
		p := gopacket.NewPacket(record, layers.LayerTypeEthernet, gopacket.Default)
		tcpL := p.Layer(layers.LayerTypeTCP)
		if tcpL == nil {
			continue
		}
		payload := tcpL.(*layers.TCP).Payload
		if len(payload) < 2 {
			continue
		}
		op := payload[0] & 0x0f
		if op > 0x0a || op >= 3 && op <= 7 {
			continue
		}
		n, err := parser.ParseBinary(bytes.NewReader(payload), "application-layer.websocket", "WebSocket")
		if err != nil {
			continue
		}
		val, err := n.Result()
		require.NoError(t, err)
		require.NotNil(t, val.Child("Opcode"))
		parsed++
	}
	require.Greater(t, parsed, 0)
}

func TestPostgreSQLBindParameterLayout(t *testing.T) {
	portal := []byte("p1")
	stmt := []byte("s1")
	body := append(append(portal, 0), append(stmt, 0)...)
	body = append(body, 0, 1, 0, 0)             // one text format
	body = append(body, 0, 1, 0, 0, 0, 4, 1, 2, 3, 4) // one 4-byte value
	body = append(body, 0, 0)                   // no result formats
	msg := pgTyped('B', body)
	n := parseRule(t, msg, "application-layer.postgresql", "PostgreSQL")
	bind := mustChild(t, n, "Payload", "PostgreSQLBind")
	require.Equal(t, "p1", strVal(t, bind.Child("Portal")))
	require.Equal(t, "s1", strVal(t, bind.Child("Statement")))
	require.Equal(t, []byte{1, 2, 3, 4}, bytesVal(t, bind.Child("Parameter").Child("Value")))
	front := protocolCorpusRequireBoundedRuleParse(t, msg, "application-layer.postgresql_fields", "PostgreSQLFrontendFields")
	protocolCorpusRequireValue(t, front, "Portal Name", []byte("p1"))
	protocolCorpusRequireValue(t, front, "Statement Name", []byte("s1"))
}
