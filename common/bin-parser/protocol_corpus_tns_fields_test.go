package bin_parser

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
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

const tnsFieldsTestRule = "application-layer.tns_fields"

var tnsFieldsTestEntries = map[int]string{
	4: "Connect315", 6: "Resend16", 8: "Connect315", 10: "Accept315", 11: "Services32", 13: "Services32", 14: "ProtocolRequest32", 16: "ProtocolResponse32", 17: "TypesRequestNative32", 19: "TypesResponseNative32", 20: "ParametersNativeLE6432",
}

func TestProtocolCorpusTNSFieldsOriginalRecords(t *testing.T) {
	path := "testdata/protocol-corpus/captures/ndpi/ndpi-oracle.pcapng"
	raw, e := os.ReadFile(path)
	require.NoError(t, e)
	require.Equal(t, "842255a04c47c1dabe18d96bc5296ff705dc167ddcc6785651a14d0e27ffa79b", tlsCertificateTestSHA(raw))
	records := protocolCorpusAuditPackets(t, path)
	require.Len(t, records, 20)
	lengths := []int{0, 0, 0, 212, 0, 8, 0, 212, 0, 41, 164, 0, 127, 38, 0, 239, 82, 0, 26, 233}
	seqs := []uint32{2085889274, 7872001, 2085889275, 2085889275, 7872002, 7872002, 2085889487, 2085889487, 7872010, 7872010, 2085889699, 7872051, 7872051, 2085889863, 7872178, 7872178, 2085889901, 7872417, 7872417, 2085889983}
	acks := []uint32{0, 2085889275, 7872002, 7872002, 2085889487, 2085889487, 7872010, 7872010, 2085889699, 2085889699, 7872051, 2085889863, 2085889863, 7872178, 2085889901, 2085889901, 7872417, 2085889983, 2085889983, 7872443}
	controls, payloads, total := 0, 0, 0
	for index, record := range records {
		frame := index + 1
		p := gopacket.NewPacket(record, layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, p.ErrorLayer())
		ip := p.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
		tcp := p.Layer(layers.LayerTypeTCP).(*layers.TCP)
		source, dest := fmt.Sprintf("%s:%d", ip.SrcIP, tcp.SrcPort), fmt.Sprintf("%s:%d", ip.DstIP, tcp.DstPort)
		require.Contains(t, []string{"10.0.2.15:40226", "10.0.72.139:1521"}, source)
		require.Contains(t, []string{"10.0.2.15:40226", "10.0.72.139:1521"}, dest)
		require.NotEqual(t, source, dest)
		require.Equal(t, seqs[index], tcp.Seq)
		require.Equal(t, acks[index], tcp.Ack)
		require.Equal(t, frame <= 2, tcp.SYN)
		require.Equal(t, frame != 1, tcp.ACK)
		require.False(t, tcp.FIN)
		require.False(t, tcp.RST)
		offset := 14 + int(ip.IHL)*4 + int(tcp.DataOffset)*4
		w := tcp.Payload
		require.Equal(t, record[offset:14+int(ip.Length)], w)
		require.Len(t, w, lengths[index])
		if len(w) == 0 {
			controls++
			require.Empty(t, tnsFieldsTestEntries[frame])
			continue
		}
		payloads++
		total += len(w)
		entry := "TNS" + tnsFieldsTestEntries[frame] + "Fields"
		t.Run(fmt.Sprintf("frame-%d-%s", frame, entry), func(t *testing.T) {
			n := protocolCorpusRequireBoundedRuleParse(t, w, tnsFieldsTestRule, entry)
			tlsCertificateTestTree(t, n, w, 0)
			smtpFieldsTestWhole(t, record, offset, len(w), "application-layer/tns_fields.yaml", entry)
			protocolCorpusRequireValue(t, n, "Packet Length", uint64(len(w)))
			protocolCorpusRequireValue(t, n, "Packet Type", uint64(w[4]))
			m := n.Cfg.GetItem("additionInfo").(map[string]any)
			require.Equal(t, false, m["Session State Validated"])
			require.Equal(t, false, m["TCP Reassembly Performed"])
			require.Equal(t, false, m["Connection Opened"])
			tnsFieldsTestMetadataBytes(t, w, m)
			switch frame {
			case 4, 8:
				require.Equal(t, records[3][54:], w)
				protocolCorpusRequireValue(t, n, "Connect Data Length", uint64(142))
				protocolCorpusRequireValue(t, n, "Connect Data Offset", uint64(70))
				require.Equal(t, uint64(8192), m["SDU32"])
				require.Equal(t, uint64(2097152), m["TDU32"])
				desc := m["Connect Descriptor"].(map[string]any)
				require.Equal(t, "DESCRIPTION", desc["Key"])
				var pairs []string
				var visit func(map[string]any)
				visit = func(d map[string]any) {
					if value, ok := d["Value"].(map[string]any); ok {
						pairs = append(pairs, fmt.Sprintf("%s=%s", d["Key"], value["Bytes"]))
					}
					if children, ok := d["Children"].([]map[string]any); ok {
						for _, c := range children {
							visit(c)
						}
					}
				}
				visit(desc)
				require.Equal(t, []string{"SID=igor", "PROGRAM=sqlplus@kali", "HOST=kali", "USER=root", "PROTOCOL=TCP", "HOST=10.0.72.139", "PORT=1521"}, pairs)
			case 10:
				require.Equal(t, uint64(8192), m["SDU32"])
				require.Equal(t, uint64(2097152), m["TDU32"])
				protocolCorpusRequireValue(t, n, "Connect Data Length", uint64(0))
				require.NotContains(t, m, "Connect Descriptor")
			case 11, 13:
				services := m["Services"].([]map[string]any)
				require.Len(t, services, 4)
				counts := []int{3, 3, 3, 2}
				array := []uint64{4, 1, 1, 2}
				if frame == 13 {
					counts = []int{3, 2, 2, 2}
					array = []uint64{4, 1}
				}
				for i, service := range services {
					require.Equal(t, []uint64{4, 1, 2, 3}[i], service["Type"])
					require.Equal(t, uint64(0), service["Error"])
					parts := service["Subpackets"].([]map[string]any)
					require.Len(t, parts, counts[i])
					require.Equal(t, uint64(0x0c100200), parts[0]["Value"])
				}
				require.Equal(t, array, services[0]["Subpackets"].([]map[string]any)[2]["Values"])
				if frame == 11 {
					require.Equal(t, uint64(0xe0e1), services[1]["Subpackets"].([]map[string]any)[1]["Value"])
					require.Equal(t, uint64(0xfcff), services[1]["Subpackets"].([]map[string]any)[2]["Value"])
					require.Equal(t, "001106100c0f0a0b08020103", hex.EncodeToString(services[2]["Subpackets"].([]map[string]any)[1]["Data"].(map[string]any)["Bytes"].([]byte)))
				} else {
					require.Equal(t, uint64(0xfbff), services[1]["Subpackets"].([]map[string]any)[1]["Value"])
					for _, i := range []int{2, 3} {
						require.Equal(t, uint64(0), services[i]["Subpackets"].([]map[string]any)[1]["Value"])
					}
				}
			case 14:
				require.Equal(t, []uint64{6, 5, 4, 3, 2, 1}, m["Protocol Versions"])
				require.Equal(t, []byte("x86_64/Linux 2.4.xx"), m["Platform"].(map[string]any)["Bytes"])
			case 16:
				require.Equal(t, []uint64{6}, m["Protocol Versions"])
				require.Equal(t, uint64(873), m["Character Set"])
				require.Equal(t, uint64(2000), m["National Character Set"])
				require.Len(t, m["Character Conversions"], 10)
				require.Equal(t, "99e8bbd31af34133b6a780205681c1c8da28542df265d3dca42b49b616feca41", tlsCertificateTestSHA(m["Representation Descriptor"].([]byte)))
				require.Equal(t, [2]int{150, 152}, m["National Character Set Byte Range"])
				require.Equal(t, "060101012f010107010101010101017fff030b03030101ff01ffff010a0101ff01060960017f0400", hex.EncodeToString(m["Compile Capabilities"].(map[string]any)["Bytes"].([]byte)))
				require.Equal(t, []byte{2, 1, 0, 1, 24, 0, 3}, m["Runtime Capabilities"].(map[string]any)["Bytes"])
			case 17, 19:
				require.Equal(t, int64(0), m["Time Zone Offset Seconds"])
				require.Equal(t, uint64(18), m["Time Zone Data Version"])
				if frame == 17 {
					require.Equal(t, uint64(2000), m["National Character Set"])
					require.Equal(t, []byte{2, 1, 0, 0, 24, 0, 7}, m["Runtime Capabilities"].(map[string]any)["Bytes"])
				}
			case 20:
				protocolCorpusRequireValue(t, n, "User Pointer", uint64(0xfffffffffffffffe))
				protocolCorpusRequireValue(t, n, "User Conversion Capacity", uint64(9))
				require.Equal(t, []byte("sys"), m["User"].(map[string]any)["Bytes"])
				require.Equal(t, uint64(33), m["Mode"])
				pairs := m["Parameters"].([]map[string]any)
				require.Len(t, pairs, 5)
				for i, pair := range pairs {
					key := pair["Key"].(map[string]any)["Bytes"].([]byte)
					value := pair["Value"].(map[string]any)["Bytes"].([]byte)
					require.Equal(t, []string{"AUTH_TERMINAL", "AUTH_PROGRAM_NM", "AUTH_MACHINE", "AUTH_PID", "AUTH_SID"}[i], string(key))
					require.Equal(t, []string{"pts/0", "sqlplus@kali (TNS V1-V3)", "kali", "19033", "root"}[i], string(value))
					require.Equal(t, uint64(3*len(key)), pair["Key Conversion Capacity"])
					require.Equal(t, uint64(3*len(value)), pair["Value Conversion Capacity"])
					require.Equal(t, uint64(0), pair["Flags"])
				}
			}
		})
	}
	require.Equal(t, 9, controls)
	require.Equal(t, 11, payloads)
	require.Equal(t, 1382, total)
}

func tnsFieldsTestMetadataBytes(t *testing.T, wire []byte, value any) {
	t.Helper()
	switch x := value.(type) {
	case map[string]any:
		if b, ok := x["Bytes"].([]byte); ok {
			span, ok := x["Byte Range"].([2]int)
			require.True(t, ok)
			require.GreaterOrEqual(t, span[0], 0)
			require.GreaterOrEqual(t, span[1], span[0])
			require.LessOrEqual(t, span[1], len(wire))
			require.Equal(t, wire[span[0]:span[1]], b)
		}
		for _, v := range x {
			tnsFieldsTestMetadataBytes(t, wire, v)
		}
	case []map[string]any:
		for _, v := range x {
			tnsFieldsTestMetadataBytes(t, wire, v)
		}
	}
}

func TestProtocolCorpusTNSFieldsBoundariesAndFallback(t *testing.T) {
	records := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/ndpi/ndpi-oracle.pcapng")
	for frame, suffix := range tnsFieldsTestEntries {
		if frame == 8 || frame == 13 {
			continue
		}
		entry := "TNS" + suffix + "Fields"
		wire := records[frame-1][54:]
		t.Run(entry, func(t *testing.T) {
			for _, name := range []string{entry, entry + "Carrier"} {
				_, e := parser.ParseBinary(bytes.NewReader(wire), tnsFieldsTestRule, name)
				require.ErrorContains(t, e, "explicit")
				_, e = parser.GenerateBinary(map[string]any{}, tnsFieldsTestRule, name)
				require.Error(t, e)
				for _, bits := range []uint64{0, 1, 7, (1<<20)*8 + 1, (1<<20 + 1) * 8} {
					r := &tlsSHTestHeldReader{bits: bits}
					_, e := parser.ParseBinary(r, tnsFieldsTestRule, name)
					require.Error(t, e)
					require.Zero(t, r.reads)
				}
			}
			for cut := 0; cut < len(wire); cut++ {
				_, e := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), tnsFieldsTestRule, entry)
				require.Error(t, e, "cut %d", cut)
			}
			joined := append(bytes.Clone(wire), wire...)
			smtpFieldsTestWhole(t, joined, 0, len(wire), "application-layer/tns_fields.yaml", entry)
			_, e := parser.ParseBinary(newProtocolCorpusBoundedReader(joined), tnsFieldsTestRule, entry)
			require.Error(t, e)
			for offset := uint64(0); offset < 8; offset++ {
				for _, good := range []bool{true, false} {
					w := bytes.Clone(wire)
					if !good {
						w = append(w, 0)
						if frame <= 10 {
							binary.BigEndian.PutUint16(w, uint16(len(w)))
						} else {
							binary.BigEndian.PutUint32(w, uint32(len(w)))
						}
						if frame == 4 {
							binary.BigEndian.PutUint16(w[24:], uint16(len(w)-70))
						}
						if frame == 11 {
							binary.BigEndian.PutUint16(w[14:], uint16(len(w)-10))
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
					root := latTestInline(t, fmt.Sprintf("endian: little\nPackage:\n  Wrapped:\n    operator: |\n      if %d > 0 { this.ProcessSubNode(\"Prefix\") }\n      this.GetSubNode(\"Message\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Message\")\n      this.ProcessSubNode(\"Sentinel\")\n      if %d > 0 { this.ProcessSubNode(\"Padding\") }\n    Prefix: uint8,%dbit\n    Message: \"import:application-layer/tns_fields.yaml;node:%sCarrier\"\n    Sentinel: uint8\n    Padding: uint8,%dbit\n", offset, len(w), offset, offset, entry, 8-offset))
					root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
					root.Ctx.SetItem("marker", 123)
					r := base.NewBitReader(bytes.NewReader(packed.Bytes()))
					require.NoError(t, r.Backup())
					require.NoError(t, root.ParseSubNode(r, "Wrapped"))
					n := protocolCorpusFindNode(root, "Message")
					tlsCertificateTestTree(t, n, w, offset)
					if good {
						require.Nil(t, protocolCorpusFindNode(n, "Unparsed TNS Wire"))
						protocolCorpusRequireValue(t, n, "Packet Length", uint64(len(w)))
						if frame == 20 {
							protocolCorpusRequireValue(t, n, "User Pointer", uint64(0xfffffffffffffffe))
							protocolCorpusRequireValue(t, n, "Mode", uint64(33))
						}
					} else {
						protocolCorpusRequireValue(t, n, "Unparsed TNS Wire", w)
						require.Nil(t, protocolCorpusFindNode(n, "Packet Length"))
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
			w := records[19][54:]
			for i := 0; i < 5; i++ {
				n := protocolCorpusRequireBoundedRuleParse(t, w, tnsFieldsTestRule, "TNSParametersNativeLE6432Fields")
				tlsCertificateTestTree(t, n, w, 0)
				protocolCorpusRequireValue(t, n, "Parameter Count", uint64(5))
			}
		})
	}
}
