package bin_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

const postgresqlFieldsTestRule = "application-layer.postgresql_fields"

func postgresqlFieldsOriginalEntry(frame int, client bool) string {
	switch frame {
	case 7, 9, 55, 80:
		return "PostgreSQLStartupFields"
	case 43, 66:
		return "PostgreSQLGSSRequestFields"
	case 45, 68:
		return "PostgreSQLGSSResponseFields"
	case 51, 74:
		return "PostgreSQLSSLRequestFields"
	case 53, 78:
		return "PostgreSQLSSLResponseFields"
	case 15, 16:
		return "PostgreSQLPasswordFields"
	case 83:
		return "PostgreSQLSASLInitialFields"
	case 84, 87:
		return "PostgreSQLBackendSCRAMBlockFields"
	case 86:
		return "PostgreSQLSASLSCRAMResponseFields"
	}
	if client {
		return "PostgreSQLFrontendBlockFields"
	}
	return "PostgreSQLBackendBlockFields"
}
func postgresqlFieldsPublicFixtures(t *testing.T) map[string][]byte {
	t.Helper()
	records := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/ndpi/ndpi-postgresql.pcap")
	m := map[string][]byte{}
	for _, frame := range []int{7, 43, 45, 51, 53, 15, 83, 87, 86, 21, 25} {
		p := gopacket.NewPacket(records[frame-1], layers.LayerTypeEthernet, gopacket.Default).Layer(layers.LayerTypeTCP).(*layers.TCP)
		m[postgresqlFieldsOriginalEntry(frame, p.DstPort == 5432)] = bytes.Clone(p.Payload)
	}
	m["PostgreSQLFrontendFields"] = bytes.Clone(records[20][66:98])
	m["PostgreSQLBackendFields"] = bytes.Clone(records[10][66:])
	m["PostgreSQLBackendSCRAMFields"] = bytes.Clone(records[83][66:])
	m["PostgreSQLSASLResponseFields"] = bytes.Clone(records[85][66:])
	w := make([]byte, 16)
	binary.BigEndian.PutUint32(w, 16)
	binary.BigEndian.PutUint32(w[4:], 80877102)
	binary.BigEndian.PutUint32(w[8:], 0x12345678)
	m["PostgreSQLCancelFields"] = w
	require.Len(t, m, 16)
	return m
}
func TestProtocolCorpusPostgreSQLFieldsBoundariesAndIsolation(t *testing.T) {
	fixtures := postgresqlFieldsPublicFixtures(t)
	for entry, wire := range fixtures {
		for _, entry := range []string{entry, entry + "Carrier"} {
			_, err := parser.ParseBinary(bytes.NewReader(wire), postgresqlFieldsTestRule, entry)
			require.ErrorContains(t, err, "explicit")
			_, err = parser.GenerateBinary(map[string]any{}, postgresqlFieldsTestRule, entry)
			require.Error(t, err)
			for _, bits := range []uint64{0, 1, 7, (1<<20)*8 + 1, (1<<20 + 1) * 8} {
				r := &tlsSHTestHeldReader{bits: bits}
				_, err := parser.ParseBinary(r, postgresqlFieldsTestRule, entry)
				require.Error(t, err)
				require.Zero(t, r.reads)
			}
		}
		for offset := uint64(0); offset < 8; offset++ {
			for _, good := range []bool{true, false} {
				w := bytes.Clone(wire)
				if !good {
					w[0] = 0xff
				}
				var packed bytes.Buffer
				bw := base.NewBitWriter(&packed)
				if offset > 0 {
					require.NoError(t, bw.WriteBits([]byte{0x55}, offset))
				}
				require.NoError(t, bw.WriteBits(w, uint64(len(w))*8))
				require.NoError(t, bw.WriteBits([]byte{0xd3}, 8))
				if offset > 0 {
					require.NoError(t, bw.WriteBits([]byte{0}, 8-offset))
				}
				root := latTestInline(t, fmt.Sprintf("endian: little\nPackage:\n  Wrapped:\n    operator: |\n      if %d > 0 { this.ProcessSubNode(\"Prefix\") }\n      this.GetSubNode(\"Message\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Message\")\n      this.ProcessSubNode(\"Sentinel\")\n      if %d > 0 { this.ProcessSubNode(\"Padding\") }\n    Prefix: uint8,%dbit\n    Message: \"import:application-layer/postgresql_fields.yaml;node:%sCarrier\"\n    Sentinel: uint8\n    Padding: uint8,%dbit\n", offset, len(w), offset, offset, entry, 8-offset))
				root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
				root.Ctx.SetItem("marker", 123)
				r := base.NewBitReader(bytes.NewReader(packed.Bytes()))
				require.NoError(t, r.Backup())
				require.NoError(t, root.ParseSubNode(r, "Wrapped"))
				n := protocolCorpusFindNode(root, "Message")
				tlsCertificateTestTree(t, n, w, offset)
				if good {
					require.Nil(t, protocolCorpusFindNode(n, "Unparsed PostgreSQL Wire"))
					var walk func(*base.Node)
					walk = func(n *base.Node) {
						if stream_parser.NodeHasResult(n) {
							require.Equal(t, "big", n.Cfg.GetString(base.CfgEndian))
						}
						for _, c := range n.Children {
							walk(c)
						}
					}
					walk(n)
				} else {
					protocolCorpusRequireValue(t, n, "Unparsed PostgreSQL Wire", w)
					require.Nil(t, protocolCorpusFindNode(n, "Messages"))
				}
				protocolCorpusRequireValue(t, root, "Sentinel", uint64(0xd3))
				require.Equal(t, 123, root.Ctx.GetItem("marker"))
				require.Equal(t, packed.Bytes(), NodeToBytes(base.GetNodeByPath(root, "@Wrapped")))
				require.NoError(t, r.Recovery())
				got, err := r.ReadBits(uint64(packed.Len()) * 8)
				require.NoError(t, err)
				require.Equal(t, packed.Bytes(), got)
				require.ErrorContains(t, r.PopBackup(), "no backup")
				_, err = r.ReadBits(8)
				require.ErrorIs(t, err, io.EOF)
			}
		}
		double := append(bytes.Clone(wire), wire...)
		smtpFieldsTestWhole(t, double, 0, len(wire), "application-layer/postgresql_fields.yaml", entry)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(double), postgresqlFieldsTestRule, entry)
		if strings.Contains(entry, "Block") {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
	for worker := 0; worker < 4; worker++ {
		worker := worker
		t.Run(fmt.Sprintf("cache-%d", worker), func(t *testing.T) {
			t.Parallel()
			w := fixtures["PostgreSQLBackendBlockFields"]
			hash := tlsCertificateTestSHA(w)
			cfg := map[string]any{"marker": worker, "Layout Context": "frontend"}
			n := protocolCorpusRequireBoundedRuleParseWithConfig(t, w, postgresqlFieldsTestRule, "PostgreSQLBackendBlockFields", cfg)
			msgs := n.Cfg.GetItem("additionInfo").(map[string]any)["Messages"].([]map[string]any)
			msgs[5]["Columns"].([]map[string]any)[0]["Name"].([]byte)[0] = '!'
			msgs[6]["Values"].([]map[string]any)[0]["Value"].([]byte)[0] = 255
			require.Equal(t, hash, tlsCertificateTestSHA(w))
			n = protocolCorpusRequireBoundedRuleParse(t, w, postgresqlFieldsTestRule, "PostgreSQLBackendBlockFields")
			msgs = n.Cfg.GetItem("additionInfo").(map[string]any)["Messages"].([]map[string]any)
			require.Equal(t, []byte("revision"), msgs[5]["Columns"].([]map[string]any)[0]["Name"])
			require.Equal(t, int64(3), msgs[6]["Values"].([]map[string]any)[0]["Decoded int4"])
			row := w[65:80]
			single := protocolCorpusRequireBoundedRuleParse(t, row, postgresqlFieldsTestRule, "PostgreSQLBackendFields")
			require.Equal(t, false, single.Cfg.GetItem("additionInfo").(map[string]any)["Row Metadata Applied"])
			require.Equal(t, map[string]any{"marker": worker, "Layout Context": "frontend"}, cfg)
		})
	}
}

func TestProtocolCorpusPostgreSQLFieldsAllOriginalRecords(t *testing.T) {
	path := "testdata/protocol-corpus/captures/ndpi/ndpi-postgresql.pcap"
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "1bbd7586c3d43cb43cc19a0361cf3363cb4b3645df54431828f4c94120302163", tlsCertificateTestSHA(raw))
	hashes := map[int]string{
		7: "57c848a178b7754268ca5fc6da72de9fb4c95c2f32c1b588beb10a8de9cc5e2b", 9: "57c848a178b7754268ca5fc6da72de9fb4c95c2f32c1b588beb10a8de9cc5e2b", 11: "eca692416bd86afbdfc7b05d4c843d2438a60c02380f73db8e73bfe2d8f4c7f4", 13: "a07b943c0bb644208778c6ddf649a251d48582b7496fac41784698d76351d31d", 15: "05156d3688e83001b409bb1b42b565c3b504ed9eacd683a232b67b8a3b19f510", 16: "98c4eb545e32ac783779ffc7aefc4fbbc75260fb3cd34a752dc5a638e59fc6aa",
		19: "8bab245d655034191bae2cad2d478bb07140c6e5db4335d335ee12e2a14aae42", 20: "205660f0b801d27ae094cd48ccc58765460adb77a5d8bce3efc5c232d887dd51", 21: "1f7fa050c268c712dcf13564e8994227066479356ab6a30431e84dbcc525dcc2", 23: "ff5cce5a4d26e53fd7e0dcbc412a778a7806bcc1a4ce4e5fbe62541c4e68aad7", 25: "98fc96d276a5e614e0a6473224c4f0c1e393371a58056c2f70e4c1f63ab40f05", 26: "ac1cf5828bf50753037091ebc0f619e57af9d9d76c8f6dd2a578fa4a07a4adbd", 27: "6f70253b4e7e5d3a8b82ac9f0600b25b5bb0ed226ff508ac97d89a564c72f489", 28: "87bfd69fdf6c97deccafe40ff3e881b6ee10f859fe23e93db743674ae47d6934", 29: "1a15594157b890e5fe08dfd2ad10a76030bf9de80ccc453ac496c18cafdb17a5", 32: "65301945424ec44a0eebd31cdcc2091e81e84389ff37fe462e4d877cc650cb65", 34: "a8242ef937678d3de23041f797feb6b18b04204c1e72e41e3ff7b2e159fdd717", 36: "ab80a0ee6d68d02c26aef80b08d04fbb47c8b682403eaf2ac9fbf589cca881dd", 38: "5b5980edd953c5cc3b7f3f1b49c9e4fe579b978251bd70abdd0eba6bdac9f59d",
		43: "8bfd1501894ec4c8787d9d22637dd7d1d2424303ef50b1c84e3d92117be171ac", 45: "333e0a1e27815d0ceee55c473fe3dc93d56c63e3bee2b3b4aee8eed6d70191a3", 51: "694bec2754742b999eb8e010ccd19ce861d58b3639a7645f3d650a582ce4173d", 53: "8ce86a6ae65d3692e7305e2c58ac62eebd97d3d943e093f577da25c36988246b", 55: "7bd1c474a112073957ac353482e4c6f31c5fdd71dafcffd280646a10dc5d05a3", 58: "b0fc5122bf9d156b3a595eab798f7768bdae50a87a2f05b3f6d793b387b84558", 66: "8bfd1501894ec4c8787d9d22637dd7d1d2424303ef50b1c84e3d92117be171ac", 68: "333e0a1e27815d0ceee55c473fe3dc93d56c63e3bee2b3b4aee8eed6d70191a3", 74: "694bec2754742b999eb8e010ccd19ce861d58b3639a7645f3d650a582ce4173d", 78: "8ce86a6ae65d3692e7305e2c58ac62eebd97d3d943e093f577da25c36988246b", 80: "7bd1c474a112073957ac353482e4c6f31c5fdd71dafcffd280646a10dc5d05a3", 81: "b0fc5122bf9d156b3a595eab798f7768bdae50a87a2f05b3f6d793b387b84558", 83: "f986fbb4a54881ca3833857ddde220d740f15c12252138de2d59dcb69430803b", 84: "7aa8b05ff944a9454b884981ee8bd3c27842b128eb4ea655165c7b2368b2d049", 86: "fdccd6bf831446941197cb02601b163907c4f1139d01927f12b40bc091e376b9", 87: "8e53dc1da71b0e145eb6af16d0334c31c0645ea855d2a46607f0fb91dfe59da4",
	}
	records := protocolCorpusAuditPackets(t, path)
	require.Len(t, records, 88)
	next := map[string]uint32{}
	syns := map[string]bool{}
	counts := map[string]int{}
	controls, appBytes, messages, columns, values, scramMessages := 0, 0, 0, 0, 0, 0
	for i, record := range records {
		frame := i + 1
		p := gopacket.NewPacket(record, layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, p.ErrorLayer())
		ip := p.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
		tcp := p.Layer(layers.LayerTypeTCP).(*layers.TCP)
		offset := 14 + int(ip.IHL)*4 + int(tcp.DataOffset)*4
		require.Equal(t, int(ip.Length)+14, len(record))
		require.Equal(t, tcp.Payload, record[offset:])
		key := fmt.Sprint(p.NetworkLayer().NetworkFlow(), tcp.TransportFlow())
		if tcp.SYN {
			next[key] = tcp.Seq + 1
			syns[key] = true
		}
		w := tcp.Payload
		if len(w) == 0 {
			controls++
			continue
		}
		require.Equal(t, 66, offset)
		require.Equal(t, next[key], tcp.Seq, "frame %d gap/overlap", frame)
		next[key] += uint32(len(w))
		appBytes += len(w)
		require.Equal(t, hashes[frame], tlsCertificateTestSHA(w))
		client := tcp.DstPort == 5432
		if !client {
			require.Equal(t, layers.TCPPort(5432), tcp.SrcPort)
		}
		entry := postgresqlFieldsOriginalEntry(frame, client)
		n := smtpFieldsTestWhole(t, record, offset, len(w), "application-layer/postgresql_fields.yaml", entry)
		info := n.Cfg.GetItem("additionInfo").(map[string]any)
		require.Equal(t, false, info["Session State Validated"])
		list := []map[string]any{info}
		if strings.Contains(entry, "Block") {
			list = info["Messages"].([]map[string]any)
		}
		for _, m := range list {
			messages++
			name, ok := m["Message Name"].(string)
			if !ok {
				name = entry
			}
			direction := "backend/"
			if client {
				direction = "frontend/"
			}
			counts[direction+name]++
			if cs, ok := m["Columns"].([]map[string]any); ok {
				columns += len(cs)
				for _, c := range cs {
					span := c["Name Relative Byte Range"].([2]int)
					require.Equal(t, w[span[0]:span[1]], c["Name"])
				}
			}
			if vs, ok := m["Values"].([]map[string]any); ok {
				values += len(vs)
				for _, v := range vs {
					span := v["Relative Byte Range"].([2]int)
					require.Equal(t, w[span[0]:span[1]], v["Value"])
				}
			}
			if attrs, ok := m["SCRAM Attributes"].([]map[string]any); ok {
				scramMessages++
				require.Equal(t, false, m["SCRAM Proof Verified"])
				for _, a := range attrs {
					span := a["Relative Byte Range"].([2]int)
					require.Equal(t, w[span[0]:span[1]], a["Value"])
				}
			}
		}
		switch frame {
		case 7, 9, 55, 80:
			params := info["Parameters"].([]map[string]any)
			want := 2
			if frame > 9 {
				want = 4
			}
			require.Len(t, params, want)
			for _, p := range params {
				for _, name := range []string{"Name", "Value"} {
					span := p[name+" Relative Byte Range"].([2]int)
					require.Equal(t, w[span[0]:span[1]], p[name])
				}
			}
		case 25:
			require.Len(t, list, 9)
			c := list[5]["Columns"].([]map[string]any)[0]
			require.Equal(t, []byte("revision"), c["Name"])
			require.Equal(t, uint64(17215), c["Table OID"])
			require.Equal(t, int64(1), c["Attribute Number"])
			require.Equal(t, uint64(23), c["Type OID"])
			require.Equal(t, int64(4), c["Type Size"])
			require.Equal(t, int64(-1), c["Type Modifier"])
			require.Equal(t, uint64(1), c["Format Code"])
			v := list[6]["Values"].([]map[string]any)[0]
			require.Equal(t, int64(3), v["Decoded int4"])
			require.Equal(t, [2]int{76, 80}, v["Relative Byte Range"])
			require.Equal(t, [2]int{31, 65}, list[6]["Column Metadata Relative Byte Range"])
		case 32, 36:
			index := 0
			span := [2]int{18, 22}
			want := []byte("arnt")
			if frame == 32 {
				index = 1
				span = [2]int{232, 235}
				want = []byte("ams")
			}
			params := list[index]["Parameters"].([]map[string]any)
			require.Len(t, params, 1)
			require.Equal(t, want, params[0]["Value"])
			require.Equal(t, span, params[0]["Relative Byte Range"])
			require.Equal(t, uint64(0), params[0]["Format Code"])
		case 34, 38:
			idx := 1
			if frame == 34 {
				idx = 2
			}
			cols := list[idx]["Columns"].([]map[string]any)
			require.Len(t, cols, 10)
			require.Equal(t, int64(-1), cols[3]["Type Size"])
			require.Equal(t, int64(132), cols[3]["Type Modifier"])
		case 45, 68:
			require.Equal(t, "G", info["Response Code"])
			require.Equal(t, true, info["Positive Response Observed"])
			require.Equal(t, false, info["Payload Decrypted"])
		case 53, 78:
			require.Equal(t, "N", info["Response Code"])
			require.Equal(t, false, info["Positive Response Observed"])
		case 83:
			require.Equal(t, "SCRAM-SHA-256", info["Mechanism"])
			require.Equal(t, true, info["Empty SCRAM Username Observed"])
		case 84:
			attrs := list[0]["SCRAM Attributes"].([]map[string]any)
			require.Len(t, attrs, 3)
			require.Equal(t, uint64(4096), attrs[2]["Integer"])
			require.Equal(t, [2]int{89, 93}, attrs[2]["Relative Byte Range"])
		case 86:
			require.Equal(t, 32, info["SCRAM Attributes"].([]map[string]any)[2]["Decoded Octet Count"])
		case 87:
			require.Len(t, list, 15)
			require.Equal(t, 32, list[0]["SCRAM Attributes"].([]map[string]any)[0]["Decoded Octet Count"])
			require.Equal(t, "I", list[14]["Transaction Status"])
		}
	}
	require.Len(t, syns, 12)
	require.Equal(t, 53, controls)
	require.Equal(t, 2993, appBytes)
	require.Equal(t, 117, messages)
	require.Equal(t, 29, columns)
	require.Equal(t, 1, values)
	require.Equal(t, 4, scramMessages)
	require.Equal(t, map[string]int{
		"frontend/PostgreSQLStartupFields": 4, "frontend/PostgreSQLGSSRequestFields": 2, "backend/PostgreSQLGSSResponseFields": 2, "frontend/PostgreSQLSSLRequestFields": 2, "backend/PostgreSQLSSLResponseFields": 2,
		"frontend/PasswordMessage": 2, "frontend/SASLInitialResponse": 1, "frontend/SASLResponse": 1, "frontend/Parse": 6, "frontend/Bind": 7, "frontend/Describe": 6, "frontend/Execute": 7, "frontend/Sync": 6,
		"backend/Authentication": 9, "backend/ParameterStatus": 21, "backend/BackendKeyData": 3, "backend/ReadyForQuery": 9, "backend/ParseComplete": 6, "backend/BindComplete": 7, "backend/CommandComplete": 7, "backend/RowDescription": 5, "backend/DataRow": 1, "backend/NoData": 1,
	}, counts)
}

func TestProtocolCorpusPostgreSQLFieldsNestedFallback(t *testing.T) {
	for entry, wire := range postgresqlFieldsPublicFixtures(t) {
		w := bytes.Clone(wire)
		// Preserve outer lengths and fail inside the selected grammar.
		switch entry {
		case "PostgreSQLStartupFields", "PostgreSQLPasswordFields":
			w[len(w)-1] = 1 // required NUL
		case "PostgreSQLFrontendFields", "PostgreSQLFrontendBlockFields":
			w[30], w[31] = 0xff, 0xff // count inside first Parse
		case "PostgreSQLBackendFields":
			w[8] = 0xff // unknown method code
		case "PostgreSQLBackendBlockFields", "PostgreSQLBackendSCRAMBlockFields":
			w[len(w)-1] = '?' // invalid final transaction status
		case "PostgreSQLBackendSCRAMFields", "PostgreSQLSASLSCRAMResponseFields", "PostgreSQLSASLInitialFields":
			w[len(w)-1] = ',' // invalid final SCRAM attribute
		default:
			continue // fixed indicators and uninterpreted SASL bytes have no nested grammar
		}
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(w), postgresqlFieldsTestRule, entry)
		require.Error(t, err, entry)
		n := smtpFieldsTestWhole(t, append(append([]byte{0x78}, w...), 0xd3), 1, len(w), "application-layer/postgresql_fields.yaml", entry+"Carrier")
		protocolCorpusRequireValue(t, n, "Unparsed PostgreSQL Wire", w)
		for _, name := range []string{"Message Type", "Messages", "Parameters", "Columns", "SCRAM Fields", "Protocol Code"} {
			require.Nil(t, protocolCorpusFindNode(n, name), entry)
		}
	}
}

func TestProtocolCorpusPostgreSQLFieldsSignedValues(t *testing.T) {
	typed := func(typ byte, body []byte) []byte {
		w := make([]byte, 5)
		w[0] = typ
		binary.BigEndian.PutUint32(w[1:], uint32(4+len(body)))
		return append(w, body...)
	}
	for _, tc := range []struct{ attribute, size, modifier int64 }{{-32768, -1, -2147483648}, {32767, 4, 2147483647}} {
		body := []byte{0, 1, 'x', 0, 0x89, 0xab, 0xcd, 0xef, 0, 0, 0, 0, 0, 25, 0, 0, 0, 0, 0, 0, 0, 0}
		binary.BigEndian.PutUint16(body[8:], uint16(tc.attribute))
		binary.BigEndian.PutUint16(body[14:], uint16(tc.size))
		binary.BigEndian.PutUint32(body[16:], uint32(tc.modifier))
		w := typed('T', body)
		n := smtpFieldsTestWhole(t, append([]byte{0x78}, w...), 1, len(w), "application-layer/postgresql_fields.yaml", "PostgreSQLBackendFields")
		for name, want := range map[string]int64{"Attribute Number": tc.attribute, "Type Size": tc.size, "Type Modifier": tc.modifier} {
			leaf := protocolCorpusFindNode(n, name)
			require.NotNil(t, leaf)
			value, err := leaf.Result()
			require.NoError(t, err)
			require.Equal(t, want, intVal(t, value), name)
		}
		protocolCorpusRequireValue(t, n, "Table OID", uint64(0x89abcdef))
	}
	w := typed('D', []byte{0, 2, 0xff, 0xff, 0xff, 0xff, 0, 0, 0, 0})
	n := smtpFieldsTestWhole(t, append([]byte{0x78}, w...), 1, len(w), "application-layer/postgresql_fields.yaml", "PostgreSQLBackendFields")
	values := n.Cfg.GetItem("additionInfo").(map[string]any)["Values"].([]map[string]any)
	require.Equal(t, true, values[0]["NULL"])
	require.NotContains(t, values[0], "Value")
	require.Equal(t, false, values[1]["NULL"])
	require.Empty(t, values[1]["Value"])
	var lengths []int64
	var walk func(*base.Node)
	walk = func(n *base.Node) {
		if n.Name == "Value Length" {
			value, err := n.Result()
			require.NoError(t, err)
			lengths = append(lengths, intVal(t, value))
		}
		for _, child := range n.Children {
			walk(child)
		}
	}
	walk(n)
	require.Equal(t, []int64{-1, 0}, lengths)
}
