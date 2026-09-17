package bin_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

const mysqlFieldsTestRule = "application-layer.mysql_fields"

var mysqlFieldsTestOriginalEntries = map[int]string{
	4: "MySQLGreetingFields", 6: "MariaDBHandshakeResponse41Fields", 8: "MySQLOKSessionTrackFields", 9: "MySQLCommandFields", 10: "MariaDBTextResultSetFields", 12: "MySQLCommandFields", 19: "MySQLGreetingFields", 21: "MySQLSSLRequestFields",
}
var mysqlFieldsTestOriginalHashes = map[int]string{
	4:  "5a00686672a42a7417ef29e3b538ff325f3f6263e6f4472d7cea3f6eca47686b",
	6:  "7b10f7b1562b2fcdf283822d0dd05632044b7a7f9d9d7842667fef28d7d3d05a",
	8:  "4c43a3bfb39072b01e4a1e76f2b9ba35eebd596f119f50a28443bff04d46b3c2",
	9:  "b03f6eefda70d2a10b01a64bc7ce54b1f20a71c84ed46979bdeb39dcdada4974",
	10: "cf740e51e02fa5c8608d109c5d12bc02997c66d06beff587f329fcdb78d13354",
	12: "a1cb20470d89874f33383802c72d3c27a0668ebffd81934705ab0cfcbf1a1e3a",
	19: "a18c7347ea7641c85908c4b02d9f990383bf6607a58534956f378f6f09458787",
	21: "1e22091185c945b8a9f76638df90d5af599c71e170e65909b485273ae5e9c0c2",
	23: "b0e6a62192d99313aff5e8fa7cbbd5189854effcbee44a193af52e71c5dda23b",
	25: "3f91e20040ba5f90319957f949a4f61357aa7007e7226f2a4f6cb3bfe6f98002",
	27: "d3501fc325d2d0e1df16a88e9f160c94bcc4d3b602a6fa76531ef2fd53a64150",
	28: "b145166b1c5238948eaaae5f8baf3001d7e9e382a58e1dccb64bd0d0bcd498d1",
	29: "c950b1bad854141f606801f1bb587054d31253b2874fc284bd6cf350295ed89b",
	30: "ec7651ef8e94189b209dc6679cc52b25278d63e8346873a409b58a77141c4781",
	32: "1cee7dfa1279444704aea2dd6bb03b485f3e81de17aaed8d85b67c889bd5804f",
	33: "18edec6fb85f80035dce4087b66aa86f3c63efbc9d948731b703c3e30a8a98b4",
	34: "48d9c01e2267feb290d5a60858844b61fd179f4ea87499819bef01f51c88a976",
	35: "d62c2405958937bd38780e366c120592015e5be4e0cafb1af68b5071529514c7",
	36: "97d9251b9e6d59ec5e23304ff2fa193c668ed918031e87e9d157c4ba86ce3f9c",
	38: "5bb23c53177e34b6a4bafda4181c0cd3f5c5a10fde88db95428245dbe36d88c8",
}

func mysqlFieldsPublicFixtures(t *testing.T) map[string][]byte {
	t.Helper()
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/ndpi/ndpi-mysql.pcapng")
	fixtures := map[string][]byte{}
	for f, entry := range mysqlFieldsTestOriginalEntries {
		fixtures[entry] = append([]byte(nil), frames[f-1][66:]...)
	}
	w := append([]byte(nil), frames[5][66:]...)
	clear(w[32:36])
	fixtures["MySQLHandshakeResponse41Fields"] = w
	w = append([]byte(nil), frames[20][66:]...)
	w[32] = 0x1d
	fixtures["MariaDBSSLRequestFields"] = w
	fixtures["MySQLOK41Fields"] = mysqlPacket(2, []byte{0, 0xfc, 0x34, 0x12, 0, 2, 0, 0, 0})
	fixtures["MySQLError41Fields"] = mysqlPacket(1, []byte("\xff\x28\x04#HY000message"))
	fixtures["MySQLEOF41Fields"] = mysqlPacket(5, []byte{0xfe, 0, 0, 2, 0})
	col := []byte{3, 'd', 'e', 'f', 0, 0, 0, 1, 'x', 0, 12, 33, 0, 36, 0, 0, 0, 0xfd, 0, 0, 0x27, 0, 0}
	w = mysqlPacket(1, []byte{1})
	for i, p := range [][]byte{col, {0xfe, 0, 0, 2, 0}, {1, 'v'}, {0xfe, 0, 0, 2, 0}} {
		w = append(w, mysqlPacket(byte(i+2), p)...)
	}
	fixtures["MySQLTextResultSet41Fields"] = w
	require.Len(t, fixtures, 12)
	return fixtures
}

func TestProtocolCorpusMySQLFieldsBoundariesAndIsolation(t *testing.T) {
	fixtures := mysqlFieldsPublicFixtures(t)
	for entry, wire := range fixtures {
		for _, entry := range []string{entry, entry + "Carrier"} {
			_, err := parser.ParseBinary(bytes.NewReader(wire), mysqlFieldsTestRule, entry)
			require.ErrorContains(t, err, "explicit")
			_, err = parser.GenerateBinary(map[string]any{}, mysqlFieldsTestRule, entry)
			require.Error(t, err)
			for _, bits := range []uint64{0, 1, 7, (1<<20)*8 + 1, (1<<20 + 1) * 8} {
				r := &tlsSHTestHeldReader{bits: bits}
				_, err := parser.ParseBinary(r, mysqlFieldsTestRule, entry)
				require.Error(t, err)
				require.Zero(t, r.reads)
			}
		}
		for _, endian := range []string{"big", "little"} {
			for offset := uint64(0); offset < 8; offset++ {
				for _, good := range []bool{true, false} {
					w := append([]byte(nil), wire...)
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
					root := latTestInline(t, fmt.Sprintf("endian: %s\nPackage:\n  Wrapped:\n    operator: |\n      if %d > 0 { this.ProcessSubNode(\"Prefix\") }\n      this.GetSubNode(\"Message\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Message\")\n      this.ProcessSubNode(\"Sentinel\")\n      if %d > 0 { this.ProcessSubNode(\"Padding\") }\n    Prefix: uint8,%dbit\n    Message: \"import:application-layer/mysql_fields.yaml;node:%sCarrier\"\n    Sentinel: uint8\n    Padding: uint8,%dbit\n", endian, offset, len(w), offset, offset, entry, 8-offset))
					root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
					root.Ctx.SetItem("marker", 123)
					r := base.NewBitReader(bytes.NewReader(packed.Bytes()))
					require.NoError(t, r.Backup())
					require.NoError(t, root.ParseSubNode(r, "Wrapped"))
					n := protocolCorpusFindNode(root, "Message")
					tlsCertificateTestTree(t, n, w, offset)
					if good {
						require.Nil(t, protocolCorpusFindNode(n, "Unparsed MySQL Wire"))
						var check func(*base.Node)
						check = func(n *base.Node) {
							if stream_parser.NodeHasResult(n) {
								require.Equal(t, "little", n.Cfg.GetString(base.CfgEndian))
							}
							for _, c := range n.Children {
								check(c)
							}
						}
						check(n)
					} else {
						protocolCorpusRequireValue(t, n, "Unparsed MySQL Wire", w)
						require.Nil(t, protocolCorpusFindNode(n, "Sequence ID"))
					}
					protocolCorpusRequireValue(t, root, "Sentinel", uint64(0xd3))
					require.Equal(t, endian, root.Cfg.GetString(base.CfgEndian))
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
		}
		smtpFieldsTestWhole(t, append(append([]byte(nil), wire...), wire...), 0, len(wire), "application-layer/mysql_fields.yaml", entry)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(append(append([]byte(nil), wire...), wire...)), mysqlFieldsTestRule, entry)
		require.Error(t, err)
	}
	for worker := 0; worker < 4; worker++ {
		worker := worker
		t.Run(fmt.Sprintf("cache-%d", worker), func(t *testing.T) {
			t.Parallel()
			w := fixtures["MariaDBHandshakeResponse41Fields"]
			before := tlsCertificateTestSHA(w)
			cfg := map[string]any{"marker": worker, "Layout Context": "ssl-request"}
			n := protocolCorpusRequireBoundedRuleParseWithConfig(t, w, mysqlFieldsTestRule, "MariaDBHandshakeResponse41Fields", cfg)
			info := n.Cfg.GetItem("additionInfo").(map[string]any)
			require.Equal(t, "mariadb-response41", info["Layout Context"])
			a := info["Connection Attributes"].([]map[string]any)
			a[0]["Value"].([]byte)[0] = '!'
			a[0]["Key Relative Byte Range"] = [2]int{999, 999}
			require.Equal(t, before, tlsCertificateTestSHA(w))
			n = protocolCorpusRequireBoundedRuleParse(t, w, mysqlFieldsTestRule, "MariaDBHandshakeResponse41Fields")
			a = n.Cfg.GetItem("additionInfo").(map[string]any)["Connection Attributes"].([]map[string]any)
			require.Equal(t, []byte("Linux"), a[0]["Value"])
			require.Equal(t, map[string]any{"marker": worker, "Layout Context": "ssl-request"}, cfg)
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(w), mysqlFieldsTestRule, "MySQLHandshakeResponse41Fields")
			require.Error(t, err)
		})
	}
}

func TestProtocolCorpusMySQLFieldsAllOriginalRecords(t *testing.T) {
	specs := []struct {
		path, sha string
		count     int
	}{
		{"ndpi/ndpi-mysql.pcapng", "11e0988e75b471e25d4c1e3948b9357b77882e5710a492976f2c54d8cf2d98b7", 41},
		{"generated-local/gen-mariadb.pcap", "1d707377dffcda7fe2d1a5884f2b382780569f065a0727190e0865305ee7d1eb", 4},
		{"generated-pr5023/pr5023-gen-mariadb.pcap", "1d1dacf36e35116d762ea93300f447b83f2d6f586f5ebfebbe87b683f73ab677", 4},
	}
	recordCount, controlCount, appBytes, messages, classicPackets, tlsRecords, protected := 0, 0, 0, 0, 0, 0, 0
	for _, spec := range specs {
		path := "testdata/protocol-corpus/captures/" + spec.path
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, spec.sha, tlsCertificateTestSHA(raw))
		records := protocolCorpusAuditPackets(t, path)
		require.Len(t, records, spec.count)
		next := map[string]uint32{}
		for i, record := range records {
			frame := i + 1
			recordCount++
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
			}
			w := tcp.Payload
			if len(w) == 0 {
				controlCount++
				continue
			}
			require.Equal(t, next[key], tcp.Seq, "%s frame %d gap/overlap", spec.path, frame)
			next[key] += uint32(len(w))
			appBytes += len(w)
			entry := "MySQLGreetingFields"
			if spec.count == 41 {
				require.Equal(t, 66, offset)
				require.Equal(t, mysqlFieldsTestOriginalHashes[frame], tlsCertificateTestSHA(w))
				entry = mysqlFieldsTestOriginalEntries[frame]
			} else {
				require.Equal(t, 54, offset)
				require.Equal(t, "ffd2c017cc4a64fdf09a727273f674ba5b027e4625b07aeae82019679aac40bf", tlsCertificateTestSHA(w))
			}
			if entry != "" {
				n := smtpFieldsTestWhole(t, record, offset, len(w), "application-layer/mysql_fields.yaml", entry)
				messages++
				info := n.Cfg.GetItem("additionInfo").(map[string]any)
				require.Equal(t, false, info["Payload Decrypted"])
				require.Equal(t, false, info["Session State Validated"])
				for at := 0; at < len(w); {
					require.GreaterOrEqual(t, len(w)-at, 4)
					size := int(w[at]) | int(w[at+1])<<8 | int(w[at+2])<<16
					require.LessOrEqual(t, at+4+size, len(w))
					classicPackets++
					at += 4 + size
				}
				if entry == "MySQLGreetingFields" {
					wantID := uint64(3)
					version := "5.5.5-10.11.19-MariaDB-ubu2204"
					caps := uint64(0x81fff7fe)
					plugin := "mysql_native_password"
					if spec.count == 41 && frame == 4 {
						wantID = 32
						version = "5.5.5-10.6.12-MariaDB-0ubuntu0.22.04.1"
					}
					if spec.count == 41 && frame == 19 {
						wantID = 12
						version = "8.0.36"
						caps = 0xdfffffff
						plugin = "caching_sha2_password"
					}
					require.Equal(t, version, info["Server Version"])
					require.Equal(t, caps, info["Capabilities"])
					require.Equal(t, plugin, info["Plugin Name"])
					protocolCorpusRequireValue(t, n, "Connection ID", wantID)
					if frame != 19 {
						require.Equal(t, uint64(0x1d), info["MariaDB Extended Capabilities"])
					}
				}
				if spec.count == 41 {
					switch frame {
					case 6:
						require.Equal(t, uint64(0x20ffa684), info["Capabilities"])
						protocolCorpusRequireValue(t, n, "Maximum Packet Size", uint64(16777216))
						require.Equal(t, uint64(0x1d), info["MariaDB Extended Capabilities"])
						attrs := info["Connection Attributes"].([]map[string]any)
						require.Len(t, attrs, 7)
						for _, a := range attrs {
							for _, name := range []string{"Key", "Value"} {
								span := a[name+" Relative Byte Range"].([2]int)
								require.Equal(t, w[span[0]:span[1]], a[name])
							}
						}
						require.Equal(t, []byte("_client_name"), attrs[1]["Key"])
						require.Equal(t, []byte("libmariadb"), attrs[1]["Value"])
					case 8:
						require.Equal(t, uint64(0), info["Affected Rows"])
						protocolCorpusRequireValue(t, n, "Status Flags", uint64(2))
					case 9:
						protocolCorpusRequireValue(t, n, "Query Bytes", []byte("select @@version_comment limit 1"))
					case 10:
						require.Equal(t, uint64(1), info["Column Count"])
						require.Equal(t, 1, info["Row Count"])
						col := info["Columns"].([]map[string]any)[0]
						require.Equal(t, []byte("@@version_comment"), col["Column Alias"])
						require.Equal(t, uint64(33), col["Character Set"])
						require.Equal(t, uint64(36), col["Maximum Column Length"])
						require.Equal(t, uint64(0xfd), col["Column Type"])
						row := info["Rows"].([][]map[string]any)[0][0]
						require.Equal(t, false, row["NULL"])
						require.Equal(t, []byte("Ubuntu 22.04"), row["Value"])
						require.Equal(t, [2]int{64, 76}, row["Relative Byte Range"])
						for _, name := range []string{"Catalog", "Schema", "Table Alias", "Table Name", "Column Alias", "Column Name"} {
							span := col[name+" Relative Byte Range"].([2]int)
							require.Equal(t, w[span[0]:span[1]], col[name])
						}
					case 21:
						require.Equal(t, true, info["TLS Requested"])
						require.Equal(t, uint64(0x19ffae85), info["Capabilities"])
					}
				}
				continue
			}
			// SSLRequest is at frame 21 in this fixture; independently walk every
			// subsequent record length. Protected bytes are never sent to MySQL.
			require.GreaterOrEqual(t, frame, 23)
			fallback := smtpFieldsTestWhole(t, record, offset, len(w), "application-layer/mysql_fields.yaml", "MySQLCommandFieldsCarrier")
			protocolCorpusRequireValue(t, fallback, "Unparsed MySQL Wire", w)
			for at := 0; at < len(w); {
				require.GreaterOrEqual(t, len(w)-at, 5)
				size := int(binary.BigEndian.Uint16(w[at+3 : at+5]))
				end := at + 5 + size
				require.LessOrEqual(t, end, len(w))
				tlsRecords++
				switch w[at] {
				case 22:
					if frame == 23 {
						require.Equal(t, byte(1), w[at+5])
						n := protocolCorpusRequireBoundedRuleParse(t, w[at+5:end], "application-layer.tls_hello", "TLSClientHello")
						tlsCertificateTestTree(t, n, w[at+5:end], 0)
					} else {
						require.Equal(t, 25, frame)
						smtpFieldsTestWhole(t, record, offset+at, 5+size, "application-layer/tls_server_hello.yaml", "TLSServerHelloRecord")
					}
				case 20:
					smtpFieldsTestWhole(t, record, offset+at, 5+size, "application-layer/tls_change_cipher_spec.yaml", "TLSChangeCipherSpecRecord")
				case 23:
					protected++
					root, err := parser.ParseBinary(newProtocolCorpusBoundedReader(w[at:end]), "application-layer.tls")
					require.NoError(t, err)
					require.Equal(t, w[at:end], NodeToBytes(root))
					protocolCorpusRequireValue(t, root, "Payload", w[at+5:end])
					require.Nil(t, protocolCorpusFindNode(root, "Query Bytes"))
				default:
					t.Fatalf("unexpected TLS type %d", w[at])
				}
				at = end
			}
		}
	}
	require.Equal(t, 49, recordCount)
	require.Equal(t, 27, controlCount)
	require.Equal(t, 4475, appBytes)
	require.Equal(t, 10, messages)
	require.Equal(t, 14, classicPackets)
	require.Equal(t, 20, tlsRecords)
	require.Equal(t, 16, protected)
}

func TestProtocolCorpusMySQLFieldsNestedFallback(t *testing.T) {
	fixtures := mysqlFieldsPublicFixtures(t)
	for entry, wire := range fixtures {
		if entry == "MySQLOK41Fields" {
			continue
		} // trailing info is allowed in this explicit layout
		w := append([]byte(nil), wire...)
		// All lengths remain intact: fail within the phase-specific payload.
		switch entry {
		case "MySQLGreetingFields":
			w[len(w)-1] = 1
		case "MySQLHandshakeResponse41Fields", "MariaDBHandshakeResponse41Fields", "MySQLSSLRequestFields", "MariaDBSSLRequestFields":
			w[13] = 1
		case "MySQLCommandFields", "MySQLOKSessionTrackFields", "MySQLError41Fields", "MySQLEOF41Fields":
			w[4] = 0xf1
		case "MySQLTextResultSet41Fields", "MariaDBTextResultSetFields":
			w[len(w)-5] = 0xff
		}
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(w), mysqlFieldsTestRule, entry)
		require.Error(t, err, entry)
		n := smtpFieldsTestWhole(t, append(append([]byte{0x78}, w...), 0xd3), 1, len(w), "application-layer/mysql_fields.yaml", entry+"Carrier")
		protocolCorpusRequireValue(t, n, "Unparsed MySQL Wire", w)
		for _, name := range []string{"Sequence ID", "Columns", "Client Capabilities", "Connection ID"} {
			require.Nil(t, protocolCorpusFindNode(n, name))
		}
	}
}

func TestProtocolCorpusMySQLFieldsWideIntegers(t *testing.T) {
	for _, tc := range []struct {
		encoded []byte
		want    uint64
	}{
		{[]byte{250}, 250}, {[]byte{0xfc, 0x34, 0x12}, 0x1234},
		{[]byte{0xfd, 0x56, 0x34, 0x12}, 0x123456},
		{[]byte{0xfe, 8, 7, 6, 5, 4, 3, 2, 0xf1}, 0xf102030405060708},
	} {
		payload := append([]byte{0}, tc.encoded...)
		payload = append(payload, 0, 2, 0, 0, 0)
		wire := mysqlPacket(2, payload)
		n := smtpFieldsTestWhole(t, append([]byte{0xaa}, wire...), 1, len(wire), "application-layer/mysql_fields.yaml", "MySQLOK41Fields")
		require.Equal(t, tc.want, n.Cfg.GetItem("additionInfo").(map[string]any)["Affected Rows"])
		name := "Affected Rows"
		if len(tc.encoded) == 1 {
			name += " Prefix"
		}
		protocolCorpusRequireValue(t, n, name, tc.want)
	}
}
