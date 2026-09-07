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

const tdsFieldsTestRule = "application-layer.tds_fields"

func TestProtocolCorpusTDSFieldsOriginalRecords(t *testing.T) {
	path := "testdata/protocol-corpus/captures/ndpi/ndpi-mssql.pcap"
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "8de89c7fa4ae159e6afe2c12a1460685f0941656278710e8caee30e2ba26ae7e", tlsCertificateTestSHA(raw))
	records := protocolCorpusAuditPackets(t, path)
	require.Len(t, records, 38)
	lengths := []int{190, 34, 292, 358, 44, 17, 185, 1082, 190, 34, 253, 57, 233, 57, 61, 80, 0, 109, 438, 0, 45, 420, 0, 103, 57, 1460, 1460, 1460, 1460, 1460, 700, 339, 371, 88, 218, 199, 268, 320}
	seqs := []uint32{1055141622, 1495355538, 1055141812, 1495355572, 4291453894, 4281945683, 2824465682, 2368157286, 2826578938, 1732190691, 2826579128, 1732190725, 2826579381, 1732190782, 2826579614, 1732190839, 0, 2826579675, 1732190919, 0, 2826579784, 1732191357, 0, 2826579829, 1732191777, 4101409879, 4101411339, 4101412799, 4101414259, 4101415719, 4101417179, 4101417879, 2824488388, 2824515315, 1169662543, 1169662960, 4262422388, 2020142839}
	var long []byte
	controls, payloads, messages, totalBytes := 0, 0, 0, 0
	allValues, allPackets := 0, 0
	flows := map[string]bool{}
	typeCounts := map[byte]int{}
	valueCounts := map[int]int{3: 5, 4: 5, 7: 3, 8: 3, 11: 3, 12: 1, 13: 3, 14: 1, 15: 2, 18: 3, 19: 10, 21: 1, 22: 9, 24: 3, 25: 1, 32: 2, 33: 8, 35: 8, 36: 8, 37: 20, 38: 6}
	for index, record := range records {
		frame := index + 1
		p := gopacket.NewPacket(record, layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, p.ErrorLayer())
		ip := p.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
		tcp := p.Layer(layers.LayerTypeTCP).(*layers.TCP)
		source, destination := fmt.Sprintf("%s:%d", ip.SrcIP, tcp.SrcPort), fmt.Sprintf("%s:%d", ip.DstIP, tcp.DstPort)
		if source > destination {
			source, destination = destination, source
		}
		flows[source+"-"+destination] = true
		offset := 14 + int(ip.IHL)*4 + int(tcp.DataOffset)*4
		require.Equal(t, record[offset:14+int(ip.Length)], tcp.Payload)
		require.Len(t, tcp.Payload, lengths[index], "frame %d", frame)
		if lengths[index] == 0 {
			controls++
			require.False(t, tcp.SYN)
			require.True(t, tcp.ACK)
			require.False(t, tcp.FIN)
			require.False(t, tcp.RST)
			require.Equal(t, map[int]uint32{17: 2826579675, 20: 2826579784, 23: 2826579829}[frame], tcp.Seq)
			require.Equal(t, map[int]uint32{17: 1732190919, 20: 1732191357, 23: 1732191777}[frame], tcp.Ack)
			require.NotEmpty(t, record[14+int(ip.Length):])
			continue
		}
		require.Equal(t, seqs[index], tcp.Seq, "frame %d", frame)
		payloads++
		totalBytes += len(tcp.Payload)
		wire := tcp.Payload
		if frame >= 26 && frame <= 32 {
			require.Equal(t, layers.TCPPort(6666), tcp.SrcPort)
			require.Equal(t, layers.TCPPort(1433), tcp.DstPort)
			long = append(long, wire...)
			if frame != 32 {
				continue
			}
			wire = long
		}
		version := "72"
		if frame == 5 || frame == 6 || frame == 8 || frame >= 35 && frame <= 37 {
			version = "71"
		}
		kind := map[byte]string{1: "Batch", 3: "RPC", 4: "Response"}[wire[0]]
		typeCounts[wire[0]]++
		require.NotEmpty(t, kind)
		entry := "TDS" + kind + version + "Fields"
		t.Run(fmt.Sprintf("frame-%d-%s", frame, entry), func(t *testing.T) {
			n := protocolCorpusRequireBoundedRuleParse(t, wire, tdsFieldsTestRule, entry)
			tlsCertificateTestTree(t, n, wire, 0)
			if frame != 32 {
				smtpFieldsTestWhole(t, record, offset, len(wire), "application-layer/tds_fields.yaml", entry)
			}
			info := n.Cfg.GetItem("additionInfo").(map[string]any)
			require.Equal(t, false, info["Session State Validated"])
			require.Equal(t, false, info["Query Executed"])
			require.Equal(t, valueCounts[frame], info["Value Count"])
			allValues += info["Value Count"].(int)
			allPackets += info["TDS Packet Count"].(int)
			protocolCorpusRequireValue(t, n, "SPID", uint64(wire[4])<<8|uint64(wire[5]))
			tdsFieldsTestAllSignedLeaves(t, n)
			tdsFieldsTestOriginalValues(t, frame, wire, info)
			if frame == 32 {
				require.Equal(t, 2, info["TDS Packet Count"])
				require.Len(t, wire, 8339)
			}
			t.Logf("frame %d: kind=%s values=%v", frame, kind, info["Value Count"])
			// Every proper prefix must fail: packet framing supplies the exact
			// message end, including the first complete non-EOM packet at 8000.
			for cut := 1; cut < len(wire); cut++ {
				// Public checks at every framing boundary; native tests exercise
				// every byte of independent grammar fixtures below.
				if cut > 9 && cut != len(wire)-1 && cut != 8000 && cut != 8008 {
					continue
				}
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), tdsFieldsTestRule, entry)
				require.Error(t, err, "prefix %d", cut)
			}
		})
		messages++
	}
	require.Equal(t, 3, controls)
	require.Equal(t, 35, payloads)
	require.Equal(t, 29, messages)
	require.Equal(t, 30, allPackets)
	require.Equal(t, 105, allValues)
	require.Equal(t, 14142, totalBytes)
	require.Len(t, flows, 12)
	require.Equal(t, map[byte]int{1: 3, 3: 16, 4: 10}, typeCounts)
	require.Equal(t, long, bytes.Join(func() [][]byte {
		var segments [][]byte
		for _, record := range records[25:32] {
			segments = append(segments, record[54:])
		}
		return segments
	}(), nil))
	t.Logf("TDS audit: %d records, %d payloads, %d controls, %d messages, %d TCP payload bytes", len(records), payloads, controls, messages, totalBytes)
}

func tdsFieldsTestAllSignedLeaves(t *testing.T, n *base.Node) {
	t.Helper()
	if stream_parser.NodeHasResult(n) && strings.HasPrefix(n.Cfg.GetString(base.CfgType), "int") {
		b := stream_parser.GetBytesByNode(n)
		require.NotEmpty(t, b)
		var v uint64
		for i := len(b) - 1; i >= 0; i-- {
			v = v<<8 | uint64(b[i])
		}
		shift := uint(64 - len(b)*8)
		got, err := n.Result()
		require.NoError(t, err)
		require.Equal(t, int64(v<<shift)>>shift, intVal(t, got))
	}
	for _, child := range n.Children {
		tdsFieldsTestAllSignedLeaves(t, child)
	}
}

func tdsFieldsTestOriginalValues(t *testing.T, frame int, wire []byte, info map[string]any) {
	t.Helper()
	tdsFieldsTestMetadataSpans(t, wire, info)
	wantTypes := map[int][]string{3: {"26e7e7e726"}, 7: {"262626"}, 8: {"e7e726"}, 11: {"26e7e7"}, 13: {"26e7e7"}, 15: {"26", "26"}, 18: {"26e7e7"}, 21: {"26"}, 24: {"26e7e7"}, 32: {"e726"}, 33: {"2626e7e7e7e7e726"}, 34: {""}, 35: {"241fe7a72626a526"}, 36: {"241fe71f2626a526"}, 37: {"24e7e7686f262626246826a5262626262668e726"}, 38: {"24e724e72626"}}
	if want, ok := wantTypes[frame]; ok {
		calls := info["RPC Requests"].([]map[string]any)
		require.Len(t, calls, len(want))
		for i, call := range calls {
			var types []byte
			for _, p := range call["Parameters"].([]map[string]any) {
				types = append(types, p["Type Info"].(map[string]any)["Type"].(byte))
			}
			require.Equal(t, want[i], fmt.Sprintf("%x", types))
		}
	}
	if frame == 1 || frame == 9 {
		require.Equal(t, " set transaction isolation level  read committed  set implicit_transactions off ", info["SQL Text"].(map[string]any)["Text"])
	}
	if frame == 5 {
		require.Equal(t, "COMMIT TRANSACTION", info["SQL Text"].(map[string]any)["Text"])
	}
	if frame == 6 {
		token := info["Response Tokens"].([]map[string]any)[0]
		require.Equal(t, "DONE", token["Name"])
		require.Equal(t, uint64(213), token["Current Command"])
		require.Equal(t, [2]int{0, 9}, token["Logical Byte Range"])
	}
	if frame == 4 || frame == 19 || frame == 22 {
		tokens := info["Response Tokens"].([]map[string]any)
		cols := tokens[0]["Columns"].([]map[string]any)
		wantNames := []string{"column1", "column2", "column3"}
		if frame == 4 {
			wantNames = []string{"name", "surname", "city", "id"}
		}
		require.Len(t, cols, len(wantNames))
		for i, col := range cols {
			require.Equal(t, wantNames[i], col["Name"].(map[string]any)["Text"])
		}
		rows := 0
		for _, token := range tokens {
			if token["Name"] == "ROW" {
				rows++
				vals := token["Values"].([]map[string]any)
				if frame == 4 {
					for i, want := range []string{"zzz" + strings.Repeat(" ", 27), "bbb" + strings.Repeat(" ", 27), "cxxx" + strings.Repeat(" ", 36)} {
						require.Equal(t, want, vals[i]["Text"])
					}
					require.Equal(t, int64(2), vals[3]["Signed Integer"])
				} else {
					for i, want := range []string{"first", "second", "third"} {
						require.Equal(t, []byte(want+strings.Repeat(" ", 30-len(want))), vals[i]["Bytes"])
						require.NotContains(t, vals[i], "Text")
					}
				}
			}
		}
		if frame == 4 {
			require.Equal(t, 1, rows)
		} else {
			require.Equal(t, 3, rows)
		}
	}
	if frame == 32 || frame == 37 {
		params := info["RPC Requests"].([]map[string]any)[0]["Parameters"].([]map[string]any)
		value := func(i int) map[string]any { return params[i]["Value"].(map[string]any) }
		if frame == 32 {
			require.Equal(t, "7249441180bdef1565f60b9f0559737a60b0f8b01ec141de44f4d61dc627171f", tlsCertificateTestSHA(wire))
			v := value(0)
			require.Equal(t, uint64(8196), v["PLP Total Length"])
			require.Equal(t, 1, v["PLP Chunk Count"])
			require.Equal(t, [][2]int{{102, 8000}, {8008, 8306}}, v["Data Wire Byte Ranges"])
			require.Equal(t, "f3d7240b3367153b77a279a3e58f149e4fa18430ed8a36066657573031141a73", tlsCertificateTestSHA(v["Bytes"].([]byte)))
			require.Contains(t, v["Text"], "Przewodników")
			require.True(t, strings.HasSuffix(v["Text"].(string), "\"Tylko dla Twoich oczu\""))
			require.Equal(t, int64(1), value(1)["Signed Integer"])
		} else {
			require.Equal(t, "67452301-ab89-efcd-0123-456789abcdef", value(0)["GUID"])
			require.Equal(t, "", value(1)["Text"])
			require.Equal(t, false, value(1)["Null"])
			require.Equal(t, "BOGUS", value(2)["Text"])
			require.Equal(t, false, value(3)["Boolean"])
			require.Equal(t, int64(-2), value(4)["Days"])
			require.Equal(t, uint32(0), value(4)["Ticks At 300 Hz"])
			for _, i := range []int{7, 8, 9, 12, 18, 19} {
				require.Equal(t, true, value(i)["Null"])
			}
			require.Equal(t, int64(5242880), value(14)["Signed Integer"])
			require.Equal(t, int64(45), value(15)["Signed Integer"])
		}
	}
}

func tdsFieldsTestMetadataSpans(t *testing.T, wire []byte, value any) {
	t.Helper()
	var body []byte
	for at := 0; at < len(wire); {
		n := int(binary.BigEndian.Uint16(wire[at+2 : at+4]))
		body = append(body, wire[at+8:at+n]...)
		at += n
	}
	read := func(spans [][2]int) []byte {
		var b []byte
		for _, span := range spans {
			require.LessOrEqual(t, span[0], span[1])
			require.GreaterOrEqual(t, span[0], 0)
			require.LessOrEqual(t, span[1], len(wire))
			b = append(b, wire[span[0]:span[1]]...)
		}
		return b
	}
	var walk func(any)
	walk = func(value any) {
		switch v := value.(type) {
		case []map[string]any:
			for _, m := range v {
				walk(m)
			}
		case map[string]any:
			if span, ok := v["Logical Byte Range"].([2]int); ok {
				require.GreaterOrEqual(t, span[0], 0)
				require.LessOrEqual(t, span[0], span[1])
				require.LessOrEqual(t, span[1], len(body))
				require.True(t, bytes.Equal(body[span[0]:span[1]], read(v["Wire Byte Ranges"].([][2]int))))
				if b, ok := v["Bytes"].([]byte); ok {
					spans := v["Wire Byte Ranges"].([][2]int)
					if data, ok := v["Data Wire Byte Ranges"].([][2]int); ok {
						spans = data
					}
					require.True(t, bytes.Equal(b, read(spans)))
				}
			}
			for _, child := range v {
				walk(child)
			}
		}
	}
	walk(value)
}

func tdsFieldsPublicFixtures(t *testing.T) map[string][]byte {
	wrap := func(kind byte, body []byte) []byte {
		h := []byte{kind, 1, 0, 0, 0x12, 0x34, 1, 0}
		binary.BigEndian.PutUint16(h[2:4], uint16(len(body)+8))
		return append(h, body...)
	}
	headers := mustHex(t, "16000000120000000200080706050403020101000000")
	query := mustHex(t, "41004200")
	rpc := mustHex(t, "ffff0a0000000000260404feffffff")
	return map[string][]byte{
		"TDSBatch71Fields": wrap(1, query), "TDSBatch72Fields": wrap(1, append(bytes.Clone(headers), query...)),
		"TDSRPC71Fields": wrap(3, rpc), "TDSRPC72Fields": wrap(3, append(bytes.Clone(headers), rpc...)),
		"TDSResponse71Fields": wrap(4, mustHex(t, "fd1000c10001000000")), "TDSResponse72Fields": wrap(4, mustHex(t, "fd1000c1000100000000000000")),
	}
}

func TestProtocolCorpusTDSFieldsBoundariesFallbackAndIsolation(t *testing.T) {
	fixtures := tdsFieldsPublicFixtures(t)
	for entry, wire := range fixtures {
		for _, name := range []string{entry, entry + "Carrier"} {
			_, err := parser.ParseBinary(bytes.NewReader(wire), tdsFieldsTestRule, name)
			require.ErrorContains(t, err, "explicit")
			_, err = parser.GenerateBinary(map[string]any{}, tdsFieldsTestRule, name)
			require.Error(t, err)
			for _, bits := range []uint64{0, 1, 7, (1<<20)*8 + 1, (1<<20 + 1) * 8} {
				r := &tlsSHTestHeldReader{bits: bits}
				_, err := parser.ParseBinary(r, tdsFieldsTestRule, name)
				require.Error(t, err)
				require.Zero(t, r.reads)
			}
		}
		for cut := 0; cut < len(wire); cut++ {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), tdsFieldsTestRule, entry)
			require.Error(t, err)
		}
		for offset := uint64(0); offset < 8; offset++ {
			for _, good := range []bool{true, false} {
				w := bytes.Clone(wire)
				if !good {
					if strings.Contains(entry, "RPC") {
						w[len(w)-5] = 3
					} else if strings.Contains(entry, "Response") {
						w[8] = 0xac
					} else {
						w[0] = 255
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
				root := latTestInline(t, fmt.Sprintf("endian: little\nPackage:\n  Wrapped:\n    operator: |\n      if %d > 0 { this.ProcessSubNode(\"Prefix\") }\n      this.GetSubNode(\"Message\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Message\")\n      this.ProcessSubNode(\"Sentinel\")\n      if %d > 0 { this.ProcessSubNode(\"Padding\") }\n    Prefix: uint8,%dbit\n    Message: \"import:application-layer/tds_fields.yaml;node:%sCarrier\"\n    Sentinel: uint8\n    Padding: uint8,%dbit\n", offset, len(w), offset, offset, entry, 8-offset))
				root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
				root.Ctx.SetItem("marker", 123)
				r := base.NewBitReader(bytes.NewReader(packed.Bytes()))
				require.NoError(t, r.Backup())
				require.NoError(t, root.ParseSubNode(r, "Wrapped"))
				n := protocolCorpusFindNode(root, "Message")
				tlsCertificateTestTree(t, n, w, offset)
				if good {
					require.Nil(t, protocolCorpusFindNode(n, "Unparsed TDS Wire"))
					protocolCorpusRequireValue(t, n, "SPID", uint64(0x1234))
					protocolCorpusRequireValue(t, n, "Length", uint64(len(w)))
					tdsFieldsTestAllSignedLeaves(t, n)
					if strings.Contains(entry, "72") && !strings.Contains(entry, "Response") {
						protocolCorpusRequireValue(t, n, "Transaction Descriptor", uint64(0x0102030405060708))
					}
					if strings.Contains(entry, "RPC") {
						protocolCorpusRequireValue(t, n, "Procedure ID", uint64(10))
						got, err := protocolCorpusFindNode(n, "Value").Result()
						require.NoError(t, err)
						require.Equal(t, int64(-2), intVal(t, got))
					}
					if strings.Contains(entry, "Response") {
						if strings.Contains(entry, "71") {
							got, err := protocolCorpusFindNode(n, "Done Row Count").Result()
							require.NoError(t, err)
							require.Equal(t, int64(1), intVal(t, got))
						} else {
							protocolCorpusRequireValue(t, n, "Done Row Count", uint64(1))
						}
					}
				} else {
					protocolCorpusRequireValue(t, n, "Unparsed TDS Wire", w)
					require.Nil(t, protocolCorpusFindNode(n, "TDS Packets"))
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
		joined := append(bytes.Clone(wire), wire...)
		smtpFieldsTestWhole(t, joined, 0, len(wire), "application-layer/tds_fields.yaml", entry)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(joined), tdsFieldsTestRule, entry)
		require.Error(t, err)
	}
	for worker := 0; worker < 4; worker++ {
		worker := worker
		t.Run(fmt.Sprintf("cache-%d", worker), func(t *testing.T) {
			t.Parallel()
			wire := fixtures["TDSRPC72Fields"]
			hash := tlsCertificateTestSHA(wire)
			cfg := map[string]any{"marker": worker, "Layout Context": "response71"}
			n := protocolCorpusRequireBoundedRuleParseWithConfig(t, wire, tdsFieldsTestRule, "TDSRPC72Fields", cfg)
			value := n.Cfg.GetItem("additionInfo").(map[string]any)["RPC Requests"].([]map[string]any)[0]["Parameters"].([]map[string]any)[0]["Value"].(map[string]any)
			value["Bytes"].([]byte)[0] = 0
			value["Data Wire Byte Ranges"].([][2]int)[0][0] = 0
			n = protocolCorpusRequireBoundedRuleParse(t, wire, tdsFieldsTestRule, "TDSRPC72Fields")
			value = n.Cfg.GetItem("additionInfo").(map[string]any)["RPC Requests"].([]map[string]any)[0]["Parameters"].([]map[string]any)[0]["Value"].(map[string]any)
			require.Equal(t, []byte{0xfe, 0xff, 0xff, 0xff}, value["Bytes"])
			require.Equal(t, [][2]int{{41, 45}}, value["Data Wire Byte Ranges"])
			require.Equal(t, hash, tlsCertificateTestSHA(wire))
			require.Equal(t, map[string]any{"marker": worker, "Layout Context": "response71"}, cfg)
		})
	}
}
