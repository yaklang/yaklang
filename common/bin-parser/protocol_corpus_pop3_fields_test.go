package bin_parser

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

const pop3FieldsTestRule = "application-layer.pop3_fields"

func TestProtocolCorpusPOP3FieldsBoundariesAndIsolation(t *testing.T) {
	fixtures := map[string]struct {
		wire  string
		limit uint64
	}{
		"command": {"TOP 7 0\r\n", 255}, "status": {"+OK example\r\n", 512}, "stat": {"+OK 3 19191\r\n", 512},
		"list": {"+OK\r\n1 123\r\n2 456 extra\r\n.\r\n", 1 << 20}, "uidl": {"+OK\r\n1 sample-id\r\n2 sample-id\r\n.\r\n", 1 << 20},
		"capa": {"+OK\r\nTOP\r\nSASL PLAIN\r\n.\r\n", 1 << 20}, "message": {"+OK\r\nSubject: example\r\n\tfold\r\n\r\n..text\r\n.\r\n", 1 << 20},
		"challenge": {"+ \r\n", 65536}, "client-continuation": {"YWJj\r\n", 65536}, "list-one": {"+OK 1 123\r\n", 512}, "uidl-one": {"+OK 1 sample-id\r\n", 512},
	}
	for profile, spec := range fixtures {
		entry := pop3FieldsTestEntry(profile)
		for _, entry := range []string{entry, entry + "Carrier"} {
			_, err := parser.ParseBinary(bytes.NewReader([]byte(spec.wire)), pop3FieldsTestRule, entry)
			require.ErrorContains(t, err, "explicit")
			_, err = parser.GenerateBinary(map[string]any{}, pop3FieldsTestRule, entry)
			require.Error(t, err)
			for _, bits := range []uint64{0, 1, 7, spec.limit*8 + 1, (spec.limit + 1) * 8} {
				r := &tlsSHTestHeldReader{bits: bits}
				_, err := parser.ParseBinary(r, pop3FieldsTestRule, entry)
				require.Error(t, err)
				require.Zero(t, r.reads)
			}
		}
		for offset := uint64(0); offset < 8; offset++ {
			for _, good := range []bool{true, false} {
				w := []byte(spec.wire)
				if !good {
					w[0] = 0xff
				}
				var packed bytes.Buffer
				writer := base.NewBitWriter(&packed)
				if offset > 0 {
					require.NoError(t, writer.WriteBits([]byte{0x55}, offset))
				}
				require.NoError(t, writer.WriteBits(w, uint64(len(w))*8))
				require.NoError(t, writer.WriteBits([]byte{0xd3}, 8))
				if offset > 0 {
					require.NoError(t, writer.WriteBits([]byte{0}, 8-offset))
				}
				root := latTestInline(t, fmt.Sprintf("endian: little\nPackage:\n  Wrapped:\n    operator: |\n      if %d > 0 { this.ProcessSubNode(\"Prefix\") }\n      this.GetSubNode(\"Message\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Message\")\n      this.ProcessSubNode(\"Sentinel\")\n      if %d > 0 { this.ProcessSubNode(\"Padding\") }\n    Prefix: uint8,%dbit\n    Message: \"import:application-layer/pop3_fields.yaml;node:%sCarrier\"\n    Sentinel: uint8\n    Padding: uint8,%dbit\n", offset, len(w), offset, offset, entry, 8-offset))
				root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
				root.Ctx.SetItem("marker", 123)
				r := base.NewBitReader(bytes.NewReader(packed.Bytes()))
				require.NoError(t, r.Backup())
				require.NoError(t, root.ParseSubNode(r, "Wrapped"))
				n := protocolCorpusFindNode(root, "Message")
				tlsCertificateTestTree(t, n, w, offset)
				if !good {
					protocolCorpusRequireValue(t, n, "Unparsed POP3 Wire", w)
					for _, name := range []string{"Command", "Status", "Encoded Value", "Header Fields", "Response Items"} {
						require.Nil(t, protocolCorpusFindNode(n, name))
					}
				} else {
					require.Nil(t, protocolCorpusFindNode(n, "Unparsed POP3 Wire"))
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
		both := []byte(spec.wire + spec.wire)
		smtpFieldsTestWhole(t, both, 0, len(spec.wire), "application-layer/pop3_fields.yaml", entry)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(both), pop3FieldsTestRule, entry)
		require.Error(t, err)
	}
	for _, p := range []string{"list", "uidl", "capa"} {
		w := []byte("+OK\r\n.\r\n")
		n := smtpFieldsTestWhole(t, w, 0, len(w), "application-layer/pop3_fields.yaml", pop3FieldsTestEntry(p))
		items := protocolCorpusFindNode(n, "Response Items")
		require.Empty(t, items.Children)
		require.True(t, stream_parser.NodeHasResult(items))
	}
	for worker := 0; worker < 4; worker++ {
		worker := worker
		t.Run(fmt.Sprintf("isolated-%d", worker), func(t *testing.T) {
			t.Parallel()
			w := []byte("+OK\r\nContent-Type: text/plain\r\n\r\n..text\r\n.\r\n")
			cfg := map[string]any{"marker": worker, "Layout Context": "stat"}
			n := protocolCorpusRequireBoundedRuleParseWithConfig(t, w, pop3FieldsTestRule, "POP3MessageFields", cfg)
			info := n.Cfg.GetItem("additionInfo").(map[string]any)
			require.Equal(t, "message", info["Layout Context"])
			p := info["MIME Fields"].(map[string]any)["Parts"].([]map[string]any)[0]
			p["Decoded Body"].([]byte)[0] = 'X'
			p["Media Parameters"].(map[string]string)["charset"] = "changed"
			p["Body Encoded Relative Byte Ranges"].([][2]int)[0][0] = 999
			p["Headers"].([]map[string]any)[0]["Unfolded Value"] = "changed"
			require.Equal(t, byte('.'), w[bytes.Index(w, []byte("..text"))], "decoded values do not alias wire")
			n = protocolCorpusRequireBoundedRuleParse(t, w, pop3FieldsTestRule, "POP3MessageFields")
			p = n.Cfg.GetItem("additionInfo").(map[string]any)["MIME Fields"].(map[string]any)["Parts"].([]map[string]any)[0]
			require.Equal(t, []byte(".text\r\n"), p["Decoded Body"])
			require.Empty(t, p["Media Parameters"])
			require.Equal(t, bytes.Index(w, []byte("..text"))+1, p["Body Encoded Relative Byte Ranges"].([][2]int)[0][0])
			require.Equal(t, " text/plain", p["Headers"].([]map[string]any)[0]["Unfolded Value"])
			require.Equal(t, map[string]any{"marker": worker, "Layout Context": "stat"}, cfg)
			status := []byte("+OK 3 19191\r\n")
			n = protocolCorpusRequireBoundedRuleParse(t, status, pop3FieldsTestRule, "POP3StatusFields")
			protocolCorpusRequireValue(t, n, "Status Text", "3 19191")
			require.Nil(t, protocolCorpusFindNode(n, "Message Count"))
			n = protocolCorpusRequireBoundedRuleParse(t, status, pop3FieldsTestRule, "POP3StatFields")
			protocolCorpusRequireValue(t, n, "Message Count", "3")
		})
	}
}

func pop3FieldsTestEntry(profile string) string {
	return "POP3" + map[string]string{"command": "Command", "status": "Status", "stat": "Stat", "list": "List", "uidl": "UIDL", "capa": "Capabilities", "message": "Message", "challenge": "Challenge", "client-continuation": "ClientContinuation", "list-one": "ListItem", "uidl-one": "UIDLItem"}[profile] + "Fields"
}

func TestProtocolCorpusPOP3FieldsAllOriginalRecords(t *testing.T) {
	path := "testdata/protocol-corpus/captures/ndpi/ndpi-pop3.pcap"
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "1f8afabbdc3194c2dc071af6dbab6409136c05afcac2d920d9f6c60db15c3c7b", tlsCertificateTestSHA(raw))
	records := protocolCorpusAuditPackets(t, path)
	require.Len(t, records, 144)
	groups := map[int]int{20: 20, 22: 20, 24: 20, 113: 113, 114: 113, 116: 113, 117: 113, 119: 113, 121: 121, 122: 121, 124: 121, 125: 121, 127: 121, 128: 121, 130: 121, 132: 132, 133: 132, 135: 132, 136: 132, 138: 132}
	type span struct{ frame, offset, start, end int }
	spans := map[int][]span{}
	messages := map[int][]byte{}
	counts, commands := map[string]int{}, map[string]int{}
	next := map[string]uint32{}
	appBytes, tailBytes, tails := 0, 0, 0
	for i, record := range records {
		frame := i + 1
		p := gopacket.NewPacket(record, layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, p.ErrorLayer())
		ip := p.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
		tcp := p.Layer(layers.LayerTypeTCP).(*layers.TCP)
		require.Equal(t, uint8(5), ip.IHL)
		offset := 14 + int(ip.IHL)*4 + int(tcp.DataOffset)*4
		w := tcp.Payload
		require.Equal(t, len(w)+offset-14, int(ip.Length))
		require.Equal(t, w, record[offset:offset+len(w)])
		tail := record[offset+len(w):]
		if len(tail) > 0 {
			tails++
			tailBytes += len(tail)
		}
		if frame == 107 {
			require.Equal(t, []byte{0xda, 0x8a, 0x9c, 0x19, 0x28}, tail)
			require.Len(t, w, 13)
		}
		key := fmt.Sprint(p.NetworkLayer().NetworkFlow(), tcp.TransportFlow())
		if tcp.SYN {
			next[key] = tcp.Seq + 1
		}
		if len(w) == 0 {
			counts["transport only"]++
			continue
		}
		require.Equal(t, next[key], tcp.Seq, "frame %d gap/overlap", frame)
		next[key] += uint32(len(w))
		appBytes += len(w)
		if tcp.SrcPort == 110 {
			counts["server records"]++
		} else {
			require.Equal(t, layers.TCPPort(110), tcp.DstPort)
			counts["client records"]++
		}
		if id, ok := groups[frame]; ok {
			require.Equal(t, layers.TCPPort(110), tcp.SrcPort)
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(w), pop3FieldsTestRule, "POP3MessageFields")
			require.Error(t, err, "the status prefix or a body segment is not a complete response")
			n := smtpFieldsTestWhole(t, record, offset, len(w), "application-layer/pop3_fields.yaml", "POP3MessageFieldsCarrier")
			protocolCorpusRequireValue(t, n, "Unparsed POP3 Wire", w)
			spans[id] = append(spans[id], span{frame, offset, len(messages[id]), len(messages[id]) + len(w)})
			messages[id] = append(messages[id], w...)
			counts["message segments"]++
			continue
		}
		profile := "status"
		if tcp.DstPort == 110 {
			profile = "command"
		}
		switch frame {
		case 56, 85, 104:
			profile = "client-continuation"
		case 55, 84, 103:
			profile = "challenge"
		case 14, 109:
			profile = "list"
		case 17, 111:
			profile = "uidl"
		case 39, 53, 69, 82, 101:
			profile = "capa"
		case 107:
			profile = "stat"
		}
		t.Run(fmt.Sprintf("frame-%d", frame), func(t *testing.T) {
			n := smtpFieldsTestWhole(t, record, offset, len(w), "application-layer/pop3_fields.yaml", pop3FieldsTestEntry(profile))
			info := n.Cfg.GetItem("additionInfo").(map[string]any)
			require.Equal(t, profile, info["Layout Context"])
			for _, key := range []string{"Session State Validated", "Mailbox State Validated", "TCP Reassembly Performed", "Mechanism Payload Decoded", "Structured Generation Supported"} {
				require.Equal(t, false, info[key], key)
			}
			protocolCorpusRequireValue(t, n, "CRLF", []byte("\r\n"))
			lines := strings.Split(string(w[:len(w)-2]), "\r\n")
			switch profile {
			case "command":
				fields := strings.SplitN(lines[0], " ", 2)
				command := strings.ToUpper(fields[0])
				protocolCorpusRequireValue(t, n, "Command", fields[0])
				require.Equal(t, command, info["Command Name"])
				require.Equal(t, command == "AUTH" && len(fields) == 1, info["Legacy AUTH Probe"])
				switch command {
				case "USER":
					protocolCorpusRequireValue(t, n, "User Name", []byte(fields[1]))
				case "PASS":
					protocolCorpusRequireValue(t, n, "Password", []byte(fields[1]))
				case "RETR":
					protocolCorpusRequireValue(t, n, "Message Number", fields[1])
				case "AUTH":
					if len(fields) > 1 {
						protocolCorpusRequireValue(t, n, "Mechanism", fields[1])
					}
				case "LIST", "UIDL", "CAPA", "STAT", "QUIT":
					require.Len(t, fields, 1)
				default:
					t.Fatalf("unaccounted command %s", command)
				}
				commands[command]++
			case "challenge", "client-continuation":
				value := w[:len(w)-2]
				if profile == "challenge" {
					value = value[2:]
					protocolCorpusRequireValue(t, n, "Continuation Marker", []byte("+"))
					require.Equal(t, 0, info["Decoded Response Octets"])
				} else {
					require.True(t, bytes.HasSuffix(value, []byte("==")))
					size := 46
					if frame == 56 {
						size = 43
					}
					require.Equal(t, size, info["Decoded Response Octets"])
				}
				protocolCorpusRequireValue(t, n, "Encoded Value", value)
				require.NotContains(t, info, "Decoded Response")
			case "status":
				fields := strings.SplitN(lines[0], " ", 2)
				protocolCorpusRequireValue(t, n, "Status", fields[0])
				text := ""
				if len(fields) == 2 {
					text = fields[1]
				}
				protocolCorpusRequireValue(t, n, "Status Text", text)
				require.Nil(t, protocolCorpusFindNode(n, "Message Count"), "response type is never guessed from text")
			case "stat":
				protocolCorpusRequireValue(t, n, "Message Count", "3")
				protocolCorpusRequireValue(t, n, "Message Octets", "19191")
				require.Equal(t, map[string]any{"Message Count": uint64(3), "Message Octets": uint64(19191)}, info["Listing"])
			case "list", "uidl", "capa":
				rows := info["Items"].([]map[string]any)
				items := protocolCorpusFindNode(n, "Response Items")
				require.Len(t, rows, len(lines)-2)
				require.Len(t, items.Children, len(rows))
				for j, row := range rows {
					text := lines[j+1]
					fields := strings.Split(text, " ")
					span := row["Relative Byte Range"].([2]int)
					require.Equal(t, text+"\r\n", string(w[span[0]:span[1]]))
					if profile == "capa" {
						require.Equal(t, fields[0], row["Name"])
						require.Equal(t, strings.Join(fields[1:], " "), strings.Join(row["Parameters"].([]string), " "))
						protocolCorpusRequireValue(t, items.Children[j], "Capability Name", fields[0])
					} else {
						v, err := strconv.ParseUint(fields[0], 10, 64)
						require.NoError(t, err)
						require.Equal(t, v, row["Message Number"])
						protocolCorpusRequireValue(t, items.Children[j], "Message Number", fields[0])
						if profile == "list" {
							v, err := strconv.ParseUint(fields[1], 10, 64)
							require.NoError(t, err)
							require.Equal(t, v, row["Message Octets"])
							protocolCorpusRequireValue(t, items.Children[j], "Message Octets", fields[1])
						} else {
							require.Equal(t, fields[1], row["Unique ID"])
							protocolCorpusRequireValue(t, items.Children[j], "Unique ID", fields[1])
						}
					}
				}
				if profile == "capa" {
					require.Len(t, rows, 6)
				}
				protocolCorpusRequireValue(t, n, "Response Terminator", []byte(".\r\n"))
			}
			counts[profile]++
		})
	}
	require.Equal(t, map[string]int{"transport only": 62, "client records": 30, "server records": 52, "message segments": 20, "command": 27, "status": 19, "challenge": 3, "client-continuation": 3, "list": 2, "uidl": 2, "capa": 5, "stat": 1}, counts)
	require.Equal(t, map[string]int{"USER": 1, "PASS": 1, "LIST": 2, "UIDL": 2, "RETR": 4, "QUIT": 5, "CAPA": 5, "AUTH": 6, "STAT": 1}, commands)
	require.Len(t, next, 12)
	require.Equal(t, 22700, appBytes)
	require.Equal(t, 176, tailBytes)
	require.Equal(t, 12, tails)
	require.Len(t, messages, 4)
	want := map[int]struct {
		octets, messageOctets, headers, parts int
		sha                                   string
	}{
		20:  {1479, 1476, 17, 1, "33f7d7eaa3bca6e2e6ea2d5687c7b6355ccfa379693860fd4fc0dcde0c94fe01"},
		113: {5569, 5565, 15, 3, "1180dc2f4328aab4de3dd214d663bfd09b8a07000ec5c755ec7e543e6498bb7c"},
		121: {8416, 8412, 32, 3, "555fc0fb52f590aea366a78f14601ef82dd753a46f72e834c27ba88911c3c847"},
		132: {5217, 5214, 36, 1, "b951d7ffa8a27fb2216e2d87bceb99ef3ef47340a1b4ba5418a93afac94109c7"},
	}
	for id, w := range messages {
		t.Run(fmt.Sprintf("message-%d", id), func(t *testing.T) {
			oracle := want[id]
			require.Equal(t, []byte("+OK\r\n"), w[:5])
			require.Len(t, w, oracle.octets+5)
			require.Equal(t, oracle.sha, tlsCertificateTestSHA(w[5:]))
			n := protocolCorpusRequireBoundedRuleParse(t, w, pop3FieldsTestRule, "POP3MessageFields")
			tlsCertificateTestTree(t, n, w, 0)
			info := n.Cfg.GetItem("additionInfo").(map[string]any)
			require.Equal(t, true, info["MIME Decoded"])
			require.Equal(t, false, info["TCP Reassembly Performed"])
			layout := info["Message Fields"].(map[string]any)
			require.Equal(t, oracle.messageOctets, layout["Message Octets"])
			require.Equal(t, oracle.headers, layout["Header Count"])
			// Reconstruct a range solely from original physical frame bytes.
			restore := func(start, end int) []byte {
				var got []byte
				for _, s := range spans[id] {
					a, b := max(start, s.start), min(end, s.end)
					if a < b {
						got = append(got, records[s.frame-1][s.offset+a-s.start:s.offset+b-s.start]...)
					}
				}
				require.True(t, bytes.Equal(w[start:end], got), "original range [%d,%d)", start, end)
				return got
			}
			var physical func(*base.Node)
			physical = func(n *base.Node) {
				if len(n.Children) > 0 {
					for _, c := range n.Children {
						physical(c)
					}
					return
				}
				p := stream_parser.GetNodeResultPos(n)
				restore(int(p[0]/8), int(p[1]/8))
			}
			physical(n)
			var metadata func(any)
			metadata = func(v any) {
				switch x := v.(type) {
				case map[string]any:
					for _, v := range x {
						metadata(v)
					}
				case []map[string]any:
					for _, v := range x {
						metadata(v)
					}
				case [][2]int:
					for _, s := range x {
						require.GreaterOrEqual(t, s[0], 5)
						require.LessOrEqual(t, s[1], len(w)-3)
						restore(s[0], s[1])
					}
				case [][][2]int:
					for _, v := range x {
						metadata(v)
					}
				}
			}
			mimeInfo := info["MIME Fields"].(map[string]any)
			metadata(mimeInfo)
			parts := mimeInfo["Parts"].([]map[string]any)
			require.Len(t, parts, oracle.parts)
			// Independent standard mail/multipart readers select the same bodies.
			var message []byte
			for _, line := range bytes.SplitAfter(w[5:len(w)-3], []byte("\r\n")) {
				if bytes.HasPrefix(line, []byte(".")) {
					line = line[1:]
				}
				message = append(message, line...)
			}
			m, err := mail.ReadMessage(bytes.NewReader(message))
			require.NoError(t, err)
			media, params, err := mime.ParseMediaType(m.Header.Get("Content-Type"))
			require.NoError(t, err)
			var bodies [][]byte
			decode := func(reader io.Reader, encoding string) {
				if strings.EqualFold(encoding, "quoted-printable") {
					reader = quotedprintable.NewReader(reader)
				}
				b, err := io.ReadAll(reader)
				require.NoError(t, err)
				bodies = append(bodies, b)
			}
			if strings.HasPrefix(media, "multipart/") {
				r := multipart.NewReader(m.Body, params["boundary"])
				for {
					p, err := r.NextRawPart()
					if err == io.EOF {
						break
					}
					require.NoError(t, err)
					decode(p, p.Header.Get("Content-Transfer-Encoding"))
				}
			} else {
				decode(m.Body, m.Header.Get("Content-Transfer-Encoding"))
			}
			j := 0
			for _, p := range parts {
				if p["Body Transfer Decoded"] == true {
					require.Equal(t, bodies[j], p["Decoded Body"])
					require.Equal(t, tlsCertificateTestSHA(bodies[j]), p["Decoded SHA256"])
					require.Equal(t, false, p["Content Rendered"])
					j++
				}
				for _, h := range p["Headers"].([]map[string]any) {
					var unfolded []byte
					for _, s := range h["Value Encoded Relative Byte Ranges"].([][2]int) {
						unfolded = append(unfolded, restore(s[0], s[1])...)
					}
					require.Equal(t, h["Unfolded Value"], string(unfolded))
				}
			}
			require.Len(t, bodies, j)
		})
	}
}
