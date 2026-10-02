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
)

const cassandraFieldsTestRule = "application-layer.cassandra_fields"

var cassandraFieldsTestEntries = map[int]string{4: "CQLOptions4Fields", 6: "CQLSupported4Fields", 8: "CQLStartup4Fields", 12: "CQLOptions5InitialFields", 14: "CQLSupported5InitialFields", 16: "CQLStartup5InitialFields", 20: "CassandraInternodeInitiateFields"}

func TestProtocolCorpusCassandraFieldsOriginalRecords(t *testing.T) {
	path := "testdata/protocol-corpus/captures/ndpi/ndpi-cassandra.pcap"
	raw, e := os.ReadFile(path)
	require.NoError(t, e)
	require.Equal(t, "5992c6bbbd1f84fafb84052520e710adec4144fbf5f71b75a8a6d74bad57d155", tlsCertificateTestSHA(raw))
	records := protocolCorpusAuditPackets(t, path)
	require.Len(t, records, 20)
	lengths := []int{0, 0, 0, 9, 0, 61, 0, 31, 0, 0, 0, 9, 0, 111, 0, 92, 0, 0, 0, 19}
	seqs := []uint32{3182721335, 3972075230, 3182721336, 3182721336, 3972075231, 3972075231, 3182721345, 3182721345, 901705491, 2497051354, 901705492, 901705492, 2497051355, 2497051355, 901705501, 901705501, 1058319756, 3406588783, 1058319757, 1058319757}
	acks := []uint32{0, 3182721336, 3972075231, 3972075231, 3182721345, 3182721345, 3972075292, 3972075292, 0, 901705492, 2497051355, 2497051355, 901705501, 901705501, 2497051466, 2497051466, 0, 1058319757, 3406588784, 3406588784}
	syn := map[int]bool{1: true, 2: true, 9: true, 10: true, 17: true, 18: true}
	first := map[int]bool{1: true, 9: true, 17: true}
	controls, payloads, total, cql, internode := 0, 0, 0, 0, 0
	flows := map[string]int{}
	for i, record := range records {
		frame := i + 1
		p := gopacket.NewPacket(record, layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, p.ErrorLayer())
		ip := p.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
		tcp := p.Layer(layers.LayerTypeTCP).(*layers.TCP)
		a, b := fmt.Sprintf("%s:%d", ip.SrcIP, tcp.SrcPort), fmt.Sprintf("%s:%d", ip.DstIP, tcp.DstPort)
		if a > b {
			a, b = b, a
		}
		flows[a+"-"+b]++
		require.Equal(t, seqs[i], tcp.Seq)
		require.Equal(t, acks[i], tcp.Ack)
		require.Equal(t, syn[frame], tcp.SYN)
		require.Equal(t, !first[frame], tcp.ACK)
		require.False(t, tcp.FIN)
		require.False(t, tcp.RST)
		offset := 14 + int(ip.IHL)*4 + int(tcp.DataOffset)*4
		w := tcp.Payload
		require.Equal(t, record[offset:14+int(ip.Length)], w)
		require.Len(t, w, lengths[i])
		if len(w) == 0 {
			controls++
			require.Empty(t, cassandraFieldsTestEntries[frame])
			continue
		}
		payloads++
		total += len(w)
		if frame == 20 {
			internode++
		} else {
			cql++
		}
		entry := cassandraFieldsTestEntries[frame]
		require.NotEmpty(t, entry)
		t.Run(fmt.Sprintf("frame-%d-%s", frame, entry), func(t *testing.T) {
			n := protocolCorpusRequireBoundedRuleParse(t, w, cassandraFieldsTestRule, entry)
			tlsCertificateTestTree(t, n, w, 0)
			smtpFieldsTestWhole(t, record, offset, len(w), "application-layer/cassandra_fields.yaml", entry)
			m := n.Cfg.GetItem("additionInfo").(map[string]any)
			require.Equal(t, false, m["Session State Validated"])
			require.Equal(t, false, m["Query Executed"])
			require.Equal(t, false, m["TCP Reassembly Performed"])
			cassandraFieldsTestMetadataBytes(t, w, m)
			if frame == 20 {
				protocolCorpusRequireValue(t, n, "Protocol Magic", uint64(0xca552dfa))
				protocolCorpusRequireValue(t, n, "Connection Flags", uint64(0x0c0a0c11))
				protocolCorpusRequireValue(t, n, "Message CRC32", uint64(0xf3cb19b3))
				require.Equal(t, true, m["CRC32 Verified"])
				require.Equal(t, [2]int{0, 15}, m["CRC32 Covered Byte Range"])
				require.Equal(t, [2]int{15, 19}, m["CRC32 Field Byte Range"])
				require.Equal(t, uint64(12), m["Requested Messaging Version"])
				require.Equal(t, uint64(10), m["Minimum Messaging Version"])
				require.Equal(t, uint64(12), m["Maximum Messaging Version"])
				require.Equal(t, "urgent messages", m["Connection Type"])
				require.Equal(t, "crc", m["Framing"])
				require.Equal(t, false, m["Streaming Mode"])
				require.Equal(t, uint64(0), m["Reserved Connection Bits"])
				require.Equal(t, "198.18.0.2", m["Endpoint Address"].(map[string]any)["Text"])
				require.Equal(t, uint64(7000), m["Endpoint Port"])
				_, e := parser.ParseBinary(newProtocolCorpusBoundedReader(w), cassandraFieldsTestRule, "CQLOptions4Fields")
				require.Error(t, e)
				return
			}
			protocolCorpusRequireValue(t, n, "Version", uint64(w[0]))
			protocolCorpusRequireValue(t, n, "Body Length", uint64(len(w)-9))
			protocolCorpusRequireValue(t, n, "Opcode", uint64(w[4]))
			stream := uint64(0)
			if frame == 8 || frame == 16 {
				stream = 1
			}
			require.Equal(t, stream, m["Stream ID"])
			require.Equal(t, frame == 6 || frame == 14, m["Response"])
			if frame >= 12 {
				require.Equal(t, uint64(5), m["Version"])
				require.Contains(t, m["Framing Context"], "initial exchange")
			}
			if frame == 4 || frame == 12 {
				require.NotContains(t, m, "Options")
				return
			}
			options := m["Options"].([]map[string]any)
			require.Equal(t, true, m["CQL Version Key Present"])
			var got []string
			for _, opt := range options {
				key := opt["Key"].(map[string]any)["Text"].(string)
				if value, ok := opt["Value"].(map[string]any); ok {
					got = append(got, key+"="+value["Text"].(string))
				} else {
					for _, v := range opt["Values"].([]map[string]any) {
						got = append(got, key+"="+v["Text"].(string))
					}
				}
			}
			want := map[int][]string{
				6:  {"COMPRESSION=snappy", "COMPRESSION=lz4", "CQL_VERSION=3.3.1"},
				8:  {"CQL_VERSION=3.3.1"},
				14: {"PROTOCOL_VERSIONS=3/v3", "PROTOCOL_VERSIONS=4/v4", "PROTOCOL_VERSIONS=5/v5", "PROTOCOL_VERSIONS=6/v6-beta", "COMPRESSION=snappy", "COMPRESSION=lz4", "CQL_VERSION=3.4.6"},
				16: {"DRIVER_NAME=DataStax Python Driver", "DRIVER_VERSION=3.25.0", "CQL_VERSION=3.4.6"},
			}[frame]
			require.Equal(t, want, got)
			// Every original map/list item is present in the public tree.
			require.Len(t, protocolCorpusNodesNamed(n, "Option"), len(options))
			require.Len(t, protocolCorpusNodesNamed(n, "Option Value"), len(want))
		})
	}
	require.Equal(t, 13, controls)
	require.Equal(t, 7, payloads)
	require.Equal(t, 332, total)
	require.Equal(t, 6, cql)
	require.Equal(t, 1, internode)
	require.Equal(t, map[string]int{"127.0.0.1:46536-127.0.0.1:9042": 8, "198.18.0.2:9042-198.18.0.3:37892": 8, "198.18.0.2:37184-198.18.0.3:7000": 4}, flows)
}

func cassandraFieldsTestMetadataBytes(t *testing.T, w []byte, x any) {
	t.Helper()
	switch v := x.(type) {
	case map[string]any:
		if b, ok := v["Bytes"].([]byte); ok {
			span, ok := v["Byte Range"].([2]int)
			require.True(t, ok)
			require.GreaterOrEqual(t, span[0], 0)
			require.GreaterOrEqual(t, span[1], span[0])
			require.LessOrEqual(t, span[1], len(w))
			require.Equal(t, w[span[0]:span[1]], b)
		}
		for _, item := range v {
			cassandraFieldsTestMetadataBytes(t, w, item)
		}
	case []map[string]any:
		for _, item := range v {
			cassandraFieldsTestMetadataBytes(t, w, item)
		}
	}
}

func TestProtocolCorpusCassandraFieldsIndependentLists(t *testing.T) {
	text := func(s string) []byte {
		b := []byte{byte(len(s) >> 8), byte(len(s))}
		return append(b, []byte(s)...)
	}
	for _, version := range []byte{4, 5} {
		for _, multi := range []bool{false, true} {
			for _, empty := range []bool{false, true} {
				body := []byte{0, 0}
				if !empty {
					body[1] = 2
					for _, value := range []string{"名🔍", ""} {
						body = append(body, text("SAME")...)
						if multi {
							body = append(body, 0, 1)
						}
						body = append(body, text(value)...)
					}
				}
				v, opcode, kind := version, byte(1), "Startup"
				if multi {
					v |= 128
					opcode, kind = 6, "Supported"
				}
				w := append([]byte{v, 0, 0x7f, 0xff, opcode, 0, 0, 0, 0}, body...)
				binary.BigEndian.PutUint32(w[5:], uint32(len(body)))
				entry := fmt.Sprintf("CQL%s%d", kind, version)
				if version == 5 {
					entry += "Initial"
				}
				entry += "Fields"
				n := protocolCorpusRequireBoundedRuleParse(t, w, cassandraFieldsTestRule, entry)
				tlsCertificateTestTree(t, n, w, 0)
				m := n.Cfg.GetItem("additionInfo").(map[string]any)
				cassandraFieldsTestMetadataBytes(t, w, m)
				require.Equal(t, uint64(32767), m["Stream ID"])
				require.Equal(t, false, m["CQL Version Key Present"])
				require.Equal(t, false, m["Negotiation Validated"])
				if empty {
					require.Empty(t, m["Options"])
				} else {
					keys := protocolCorpusNodesNamed(n, "Option Key")
					require.Len(t, keys, 2)
					for _, key := range keys {
						result, e := key.Result()
						require.NoError(t, e)
						require.Equal(t, "SAME", result.Value)
					}
					bad := bytes.Clone(w)
					bad[len(w)-1] = 1 // final empty string now claims one missing byte
					_, e := parser.ParseBinary(newProtocolCorpusBoundedReader(bad), cassandraFieldsTestRule, entry)
					require.Error(t, e)
				}
			}
		}
	}
}

func TestProtocolCorpusCassandraFieldsBoundariesAndFallback(t *testing.T) {
	records := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/ndpi/ndpi-cassandra.pcap")
	for frame, entry := range cassandraFieldsTestEntries {
		wire := records[frame-1][66:]
		t.Run(entry, func(t *testing.T) {
			for _, name := range []string{entry, entry + "Carrier"} {
				_, e := parser.ParseBinary(bytes.NewReader(wire), cassandraFieldsTestRule, name)
				require.ErrorContains(t, e, "explicit")
				_, e = parser.GenerateBinary(map[string]any{}, cassandraFieldsTestRule, name)
				require.Error(t, e)
				for _, bits := range []uint64{0, 1, 7, (1<<20)*8 + 1, (1<<20 + 1) * 8} {
					r := &tlsSHTestHeldReader{bits: bits}
					_, e := parser.ParseBinary(r, cassandraFieldsTestRule, name)
					require.Error(t, e)
					require.Zero(t, r.reads)
				}
			}
			for cut := 0; cut < len(wire); cut++ {
				_, e := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), cassandraFieldsTestRule, entry)
				require.Error(t, e, "cut %d", cut)
			}
			joined := append(bytes.Clone(wire), wire...)
			smtpFieldsTestWhole(t, joined, 0, len(wire), "application-layer/cassandra_fields.yaml", entry)
			_, e := parser.ParseBinary(newProtocolCorpusBoundedReader(joined), cassandraFieldsTestRule, entry)
			require.Error(t, e)
			for offset := uint64(0); offset < 8; offset++ {
				for _, good := range []bool{true, false} {
					w := bytes.Clone(wire)
					if !good {
						if frame == 20 {
							w[len(w)-1] ^= 1
						} else {
							w = append(w, 0)
							binary.BigEndian.PutUint32(w[5:], uint32(len(w)-9))
						}
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
					root := latTestInline(t, fmt.Sprintf("endian: little\nPackage:\n  Wrapped:\n    operator: |\n      if %d > 0 { this.ProcessSubNode(\"Prefix\") }\n      this.GetSubNode(\"Message\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Message\")\n      this.ProcessSubNode(\"Sentinel\")\n      if %d > 0 { this.ProcessSubNode(\"Padding\") }\n    Prefix: uint8,%dbit\n    Message: \"import:application-layer/cassandra_fields.yaml;node:%sCarrier\"\n    Sentinel: uint8\n    Padding: uint8,%dbit\n", offset, len(w), offset, offset, entry, 8-offset))
					root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
					root.Ctx.SetItem("marker", 123)
					r := base.NewBitReader(bytes.NewReader(packed.Bytes()))
					require.NoError(t, r.Backup())
					require.NoError(t, root.ParseSubNode(r, "Wrapped"))
					n := protocolCorpusFindNode(root, "Message")
					tlsCertificateTestTree(t, n, w, offset)
					if good {
						require.Nil(t, protocolCorpusFindNode(n, "Unparsed Cassandra Wire"))
						if frame == 20 {
							protocolCorpusRequireValue(t, n, "Message CRC32", uint64(0xf3cb19b3))
						} else {
							protocolCorpusRequireValue(t, n, "Body Length", uint64(len(w)-9))
						}
					} else {
						protocolCorpusRequireValue(t, n, "Unparsed Cassandra Wire", w)
						require.Nil(t, protocolCorpusFindNode(n, "Version"))
						require.Nil(t, protocolCorpusFindNode(n, "Protocol Magic"))
					}
					protocolCorpusRequireValue(t, root, "Sentinel", uint64(0xd3))
					require.Equal(t, 123, root.Ctx.GetItem("marker"))
					require.Equal(t, packed.Bytes(), NodeToBytes(base.GetNodeByPath(root, "@Wrapped")))
					require.NoError(t, r.Recovery())
					got, e := r.ReadBits(uint64(packed.Len()) * 8)
					require.NoError(t, e)
					require.Equal(t, packed.Bytes(), got)
					require.ErrorContains(t, r.PopBackup(), "no backup")
					_, e = r.ReadBits(8)
					require.ErrorIs(t, e, io.EOF)
				}
			}
		})
	}
	for worker := 0; worker < 4; worker++ {
		t.Run(fmt.Sprintf("cache-%d", worker), func(t *testing.T) {
			t.Parallel()
			w := records[13][66:]
			for i := 0; i < 5; i++ {
				n := protocolCorpusRequireBoundedRuleParse(t, w, cassandraFieldsTestRule, "CQLSupported5InitialFields")
				tlsCertificateTestTree(t, n, w, 0)
				protocolCorpusRequireValue(t, n, "Option Count", uint64(3))
			}
		})
	}
}
