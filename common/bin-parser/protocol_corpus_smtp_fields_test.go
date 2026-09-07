package bin_parser

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/mail"
	"net/textproto"
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

const smtpFieldsTestRule = "application-layer.smtp_fields"

func smtpFieldsTestWhole(t *testing.T, frame []byte, offset, size int, rule, entry string) *base.Node {
	t.Helper()
	tail := len(frame) - offset - size
	root := latTestInline(t, fmt.Sprintf("endian: little\nPackage:\n  Capture:\n    operator: |\n      if %d > 0 { this.ProcessSubNode(\"Envelope\") }\n      this.GetSubNode(\"Message\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Message\")\n      if %d > 0 { this.ProcessSubNode(\"Tail\") }\n    Envelope: raw,%d\n    Message: \"import:%s;node:%s\"\n    Tail: raw,%d\n", offset, size, tail, offset, rule, entry, tail))
	root.Cfg.SetItem(base.CfgLength, uint64(len(frame))*8)
	r := base.NewBitReader(bytes.NewReader(frame))
	require.NoError(t, r.Backup())
	require.NoError(t, root.ParseSubNode(r, "Capture"))
	require.Equal(t, frame, NodeToBytes(base.GetNodeByPath(root, "@Capture")))
	n := protocolCorpusFindNode(root, "Message")
	// This tree contract includes explicit zero-width Parameters lists;
	// the older LAT helper assumes all empty composites are unprocessed.
	tlsCertificateTestTree(t, n, frame[offset:offset+size], uint64(offset)*8)
	var graph func(*base.Node)
	graph = func(parent *base.Node) {
		for index, child := range parent.Children {
			require.Same(t, parent, child.Cfg.GetItem(base.CfgParent))
			require.Same(t, parent.Ctx, child.Ctx)
			if parent.Cfg.GetBool(stream_parser.CfgIsList) {
				require.Equal(t, index, child.Cfg.GetItem(stream_parser.CfgElementIndex))
			}
			graph(child)
		}
	}
	graph(n)
	require.NoError(t, r.Recovery())
	got, err := r.ReadBits(uint64(len(frame)) * 8)
	require.NoError(t, err)
	require.Equal(t, frame, got)
	require.ErrorContains(t, r.PopBackup(), "no backup")
	return n
}

func TestProtocolCorpusSMTPFieldsAllOriginalRecords(t *testing.T) {
	path := "testdata/protocol-corpus/captures/ndpi/ndpi-smtp.pcap"
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "7e0941d2c082f93355adc95d6facabfd555534dbd57382c9866df336216a8f74", tlsCertificateTestSHA(raw))
	records := protocolCorpusAuditPackets(t, path)
	require.Len(t, records, 95)
	counts := map[string]int{}
	var data []byte
	type sourceSpan struct{ frame, start, end int }
	var spans []sourceSpan
	next := map[string]uint32{}
	applicationBytes := 0
	for i, record := range records {
		p := gopacket.NewPacket(record, layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, p.ErrorLayer())
		ip := p.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
		tcp := p.Layer(layers.LayerTypeTCP).(*layers.TCP)
		require.Equal(t, uint8(5), ip.IHL)
		offset := 14 + int(ip.IHL)*4 + int(tcp.DataOffset)*4
		w := tcp.Payload
		require.Equal(t, len(w)+offset-14, int(ip.Length))
		require.True(t, bytes.Equal(w, record[offset:offset+len(w)]))
		// Retain Ethernet minimum-frame padding separately from TCP bytes.
		padding := make([]byte, len(record)-offset-len(w))
		if i == 1 {
			padding = []byte{0x05, 0xb4}
		} // Original nonzero tail; never normalize it.
		require.Equal(t, padding, record[offset+len(w):])
		key := fmt.Sprint(p.NetworkLayer().NetworkFlow(), tcp.TransportFlow())
		if tcp.SYN {
			next[key] = tcp.Seq + 1
		}
		if len(w) == 0 {
			counts["control"]++
			continue
		}
		require.Equal(t, 54, offset, "original application record offset")
		require.Equal(t, next[key], tcp.Seq, "frame %d sequence gap/overlap", i+1)
		next[key] += uint32(len(w))
		applicationBytes += len(w)
		if i+1 >= 76 && i+1 <= 88 {
			require.Equal(t, layers.TCPPort(25), tcp.DstPort)
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(w), smtpFieldsTestRule, "SMTPDataFields")
			require.Error(t, err, "a single original segment is not the complete DATA block")
			fallback := smtpFieldsTestWhole(t, record, 54, len(w), "application-layer/smtp_fields.yaml", "SMTPDataFieldsCarrier")
			protocolCorpusRequireValue(t, fallback, "Unparsed SMTP Wire", w)
			spans = append(spans, sourceSpan{i + 1, len(data), len(data) + len(w)})
			data = append(data, w...)
			counts["data segment"]++
			continue
		}
		t.Run(fmt.Sprintf("frame-%d", i+1), func(t *testing.T) {
			if tcp.SrcPort == 25 {
				n := smtpFieldsTestWhole(t, record, 54, len(w), "application-layer/smtp_reply.yaml", "SMTPReply")
				lines := protocolCorpusFindNode(n, "Reply Lines")
				require.Len(t, lines.Children, 1)
				protocolCorpusRequireValue(t, n, "Code", string(w[:3]))
				protocolCorpusRequireValue(t, n, "Separator", uint64(w[3]))
				protocolCorpusRequireValue(t, lines.Children[0], "Message", string(w[4:len(w)-2]))
				protocolCorpusRequireValue(t, n, "CRLF", []byte("\r\n"))
				counts["reply"]++
			} else {
				n := smtpFieldsTestWhole(t, record, 54, len(w), "application-layer/smtp_fields.yaml", "SMTPCommandFields")
				line := string(w[:len(w)-2])
				command := strings.Fields(line)[0]
				protocolCorpusRequireValue(t, n, "Command", command)
				protocolCorpusRequireValue(t, n, "CRLF", []byte("\r\n"))
				switch command {
				case "EHLO", "HELO":
					protocolCorpusRequireValue(t, n, "Client Domain or Literal", line[5:])
				case "MAIL", "RCPT":
					open, close := strings.IndexByte(line, '<'), strings.IndexByte(line, '>')
					address, err := mail.ParseAddress(line[open : close+1])
					require.NoError(t, err)
					parts := strings.Split(address.Address, "@")
					require.Len(t, parts, 2)
					protocolCorpusRequireValue(t, n, "Local Part", parts[0])
					protocolCorpusRequireValue(t, n, "Mailbox Domain or Literal", parts[1])
					protocolCorpusRequireValue(t, n, "Path Keyword", line[5:open-1])
					protocolCorpusRequireValue(t, n, "Path Open", []byte("<"))
					protocolCorpusRequireValue(t, n, "Path Close", []byte(">"))
					protocolCorpusRequireValue(t, n, "Mailbox Separator", []byte("@"))
					protocolCorpusRequireValue(t, n, "Path Colon", []byte(":"))
				case "DATA", "QUIT":
				default:
					t.Fatalf("unaccounted command %q", command)
				}
				info := n.Cfg.GetItem("additionInfo").(map[string]any)
				require.Equal(t, command, info["Command Name"])
				require.Equal(t, 0, info["Parameter Count"])
				counts[command]++
				counts["command"]++
			}
		})
	}
	require.Equal(t, map[string]int{"control": 12, "data segment": 11, "reply": 37, "command": 35, "EHLO": 1, "HELO": 1, "MAIL": 1, "RCPT": 30, "DATA": 1, "QUIT": 1}, counts)
	require.Len(t, next, 2)
	require.Equal(t, 17955, applicationBytes)
	require.Len(t, data, 15267)
	require.Equal(t, "e7b1814c258e5281fbbb862998081600ac2487c875c5f44d0c0fb23fd3880006", tlsCertificateTestSHA(data))
	n := protocolCorpusRequireBoundedRuleParse(t, data, smtpFieldsTestRule, "SMTPDataFields")
	tlsCertificateTestTree(t, n, data, 0)
	latTestTree(t, n, 0, uint64(len(data))*8)
	info := n.Cfg.GetItem("additionInfo").(map[string]any)
	for k, v := range map[string]any{"Header Count": 5, "Fold Count": 35, "Body Line Count": 87, "Body Octets": 5345, "Message Octets": 15264, "Transparency Dot Count": 0, "Dot Transparency Canonical": true, "Message Conformance Validated": false, "Structured Header Grammar Validated": false, "MIME Decoded": false, "TCP Reassembly Performed": false, "Delivery Outcome Validated": false} {
		require.Equal(t, v, info[k], k)
	}
	message, err := mail.ReadMessage(bytes.NewReader(data[:len(data)-3]))
	require.NoError(t, err)
	body, err := io.ReadAll(message.Body)
	require.NoError(t, err)
	require.Len(t, body, 5345)
	require.Equal(t, "a91c56affcf35a0d6c934062ea7386e59e999923f41e29de96823a509bb31289", tlsCertificateTestSHA(body))
	require.Empty(t, message.Header.Get("From"), "original is retained without adding absent headers")
	headers := protocolCorpusFindNode(n, "Header Fields")
	require.Len(t, headers.Children, 5)
	wantNames := []string{"Received", "Date", "To", "Subject", "Message-Id"}
	wantStarts := []int{0, 118, 157, 9844, 9879, 9917}
	wantLines := []int{2, 1, 35, 1, 1}
	hInfo := info["Headers"].([]map[string]any)
	for i, h := range headers.Children {
		name := wantNames[i]
		protocolCorpusRequireValue(t, h, "Field Name", name)
		protocolCorpusRequireValue(t, h, "Colon", []byte(":"))
		valueLines := protocolCorpusFindNode(h, "Value Lines")
		require.Len(t, valueLines.Children, wantLines[i])
		original := data[wantStarts[i] : wantStarts[i+1]-2]
		physical := bytes.Split(original, []byte("\r\n"))
		require.Len(t, physical, wantLines[i])
		physical[0] = physical[0][bytes.IndexByte(physical[0], ':')+1:]
		unfolded := string(bytes.Join(physical, nil))
		require.Equal(t, unfolded, hInfo[i]["Unfolded Value"])
		require.Equal(t, strings.Join(strings.Fields(message.Header.Get(name)), " "), strings.Join(strings.Fields(unfolded), " "))
		require.Equal(t, [2]int{wantStarts[i], wantStarts[i+1]}, hInfo[i]["Relative Byte Range"])
		for j, l := range valueLines.Children {
			protocolCorpusRequireValue(t, l, "Value Fragment", string(physical[j]))
			protocolCorpusRequireValue(t, l, "CRLF", []byte("\r\n"))
		}
	}
	protocolCorpusRequireValue(t, n, "Header Body Separator", []byte("\r\n"))
	protocolCorpusRequireValue(t, n, "DATA Terminator", []byte(".\r\n"))
	bodyLines := protocolCorpusFindNode(n, "Body Lines")
	originalLines := bytes.Split(body[:len(body)-2], []byte("\r\n"))
	require.Len(t, originalLines, 87)
	for i, l := range bodyLines.Children {
		protocolCorpusRequireValue(t, l, "Text", string(originalLines[i]))
		protocolCorpusRequireValue(t, l, "CRLF", []byte("\r\n"))
	}
	dotReader := textproto.NewReader(bufio.NewReader(bytes.NewReader(data)))
	decoded, err := io.ReadAll(dotReader.DotReader())
	require.NoError(t, err)
	require.Equal(t, bytes.ReplaceAll(data[:len(data)-3], []byte("\r\n"), []byte("\n")), decoded)
	// Every field fragment, including the 35-line To field spanning segments,
	// maps back to original physical frame bytes. This is an oracle-only
	// reconstruction, never a production TCP-reassembly assertion.
	var provenance func(*base.Node)
	provenance = func(node *base.Node) {
		if len(node.Children) > 0 {
			for _, c := range node.Children {
				provenance(c)
			}
			return
		}
		pos := stream_parser.GetNodeResultPos(node)
		start, end := int(pos[0]/8), int(pos[1]/8)
		var restored []byte
		for _, s := range spans {
			a, b := max(start, s.start), min(end, s.end)
			if a < b {
				restored = append(restored, records[s.frame-1][54+a-s.start:54+b-s.start]...)
			}
		}
		require.True(t, bytes.Equal(data[start:end], restored), "field %s physical evidence", node.Name)
	}
	provenance(n)
}

func TestProtocolCorpusSMTPFieldsBoundariesAndIsolation(t *testing.T) {
	for _, spec := range []struct {
		entry string
		wire  []byte
		limit uint64
	}{
		{"SMTPCommandFields", []byte("MAIL FROM:<sample@example.test> SIZE=123\r\n"), 512},
		{"SMTPDataFields", []byte("Subject: example\r\n\tfolded\r\n\r\n..text\r\n.\r\n"), 1 << 20},
	} {
		for _, entry := range []string{spec.entry, spec.entry + "Carrier"} {
			_, err := parser.ParseBinary(bytes.NewReader(spec.wire), smtpFieldsTestRule, entry)
			require.ErrorContains(t, err, "explicit")
			_, err = parser.GenerateBinary(map[string]any{}, smtpFieldsTestRule, entry)
			require.Error(t, err)
			for _, bits := range []uint64{0, 1, 7, spec.limit*8 + 1, (spec.limit + 1) * 8} {
				r := &tlsSHTestHeldReader{bits: bits}
				_, err := parser.ParseBinary(r, smtpFieldsTestRule, entry)
				require.Error(t, err)
				require.Zero(t, r.reads)
			}
		}
		for offset := uint64(0); offset < 8; offset++ {
			for _, good := range []bool{true, false} {
				w := bytes.Clone(spec.wire)
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
				root := latTestInline(t, fmt.Sprintf("endian: little\nPackage:\n  Wrapped:\n    operator: |\n      if %d > 0 { this.ProcessSubNode(\"Prefix\") }\n      this.GetSubNode(\"Message\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Message\")\n      this.ProcessSubNode(\"Sentinel\")\n      if %d > 0 { this.ProcessSubNode(\"Padding\") }\n    Prefix: uint8,%dbit\n    Message: \"import:application-layer/smtp_fields.yaml;node:%sCarrier\"\n    Sentinel: uint8\n    Padding: uint8,%dbit\n", offset, len(w), offset, offset, spec.entry, 8-offset))
				root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
				root.Ctx.SetItem("marker", 123)
				r := base.NewBitReader(bytes.NewReader(packed.Bytes()))
				require.NoError(t, r.Backup())
				require.NoError(t, root.ParseSubNode(r, "Wrapped"))
				n := protocolCorpusFindNode(root, "Message")
				tlsCertificateTestTree(t, n, w, offset)
				if good {
					if spec.entry == "SMTPCommandFields" {
						protocolCorpusRequireValue(t, n, "Local Part", "sample")
					} else {
						protocolCorpusRequireValue(t, n, "Text", ".text")
					}
				} else {
					protocolCorpusRequireValue(t, n, "Unparsed SMTP Wire", w)
					require.Nil(t, protocolCorpusFindNode(n, "Command"))
					require.Nil(t, protocolCorpusFindNode(n, "Header Fields"))
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
		both := append(bytes.Clone(spec.wire), spec.wire...)
		smtpFieldsTestWhole(t, both, 0, len(spec.wire), "application-layer/smtp_fields.yaml", spec.entry)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(both), smtpFieldsTestRule, spec.entry)
		require.Error(t, err)
	}
	for worker := 0; worker < 4; worker++ {
		worker := worker
		t.Run(fmt.Sprintf("isolated-%d", worker), func(t *testing.T) {
			t.Parallel()
			w := []byte("Subject: example\r\n\tfolded\r\n.\r\n")
			cfg := map[string]any{"marker": worker}
			n := protocolCorpusRequireBoundedRuleParseWithConfig(t, w, smtpFieldsTestRule, "SMTPDataFields", cfg)
			h := n.Cfg.GetItem("additionInfo").(map[string]any)["Headers"].([]map[string]any)
			h[0]["Unfolded Value"] = "changed"
			h[0]["Value Relative Byte Ranges"].([][2]int)[0][0] = 999
			next := protocolCorpusRequireBoundedRuleParse(t, w, smtpFieldsTestRule, "SMTPDataFields")
			h = next.Cfg.GetItem("additionInfo").(map[string]any)["Headers"].([]map[string]any)
			require.Equal(t, " example\tfolded", h[0]["Unfolded Value"])
			require.Equal(t, 8, h[0]["Value Relative Byte Ranges"].([][2]int)[0][0])
			require.Equal(t, map[string]any{"marker": worker}, cfg)
		})
	}
}

func TestProtocolCorpusSMTPFieldsListsEmptyAndTransparency(t *testing.T) {
	for _, spec := range []struct {
		wire  string
		count int
	}{
		{"MAIL FROM:<>\r\n", 0},
		{"MAIL FROM:<sample@example.test> SIZE=123 BODY=7BIT X-1\r\n", 3},
	} {
		w := []byte(spec.wire)
		n := protocolCorpusRequireBoundedRuleParse(t, w, smtpFieldsTestRule, "SMTPCommandFields")
		tlsCertificateTestTree(t, n, w, 0)
		list := protocolCorpusFindNode(n, "Parameters")
		require.Len(t, list.Children, spec.count)
		require.True(t, list.Cfg.GetBool(stream_parser.CfgIsList))
		if spec.count == 0 {
			protocolCorpusRequireValue(t, n, "Local Part", "")
		} else {
			for i, key := range []string{"SIZE", "BODY", "X-1"} {
				protocolCorpusRequireValue(t, list.Children[i], "Parameter Keyword", key)
				require.Equal(t, i, list.Children[i].Cfg.GetItem(stream_parser.CfgElementIndex))
			}
			protocolCorpusRequireValue(t, list.Children[0], "Parameter Value", "123")
			protocolCorpusRequireValue(t, list.Children[1], "Parameter Value", "7BIT")
			require.Nil(t, protocolCorpusFindNode(list.Children[2], "Parameter Value"))
		}
	}
	for _, wire := range []string{".\r\n", "X:\r\n\r\n.\r\n", "..X: a\r\n.\tfold\r\n\r\n..one\r\n.single\r\n...two\r\n.\r\n"} {
		w := []byte(wire)
		n := protocolCorpusRequireBoundedRuleParse(t, w, smtpFieldsTestRule, "SMTPDataFields")
		tlsCertificateTestTree(t, n, w, 0)
		info := n.Cfg.GetItem("additionInfo").(map[string]any)
		r := textproto.NewReader(bufio.NewReader(bytes.NewReader(w)))
		message, err := io.ReadAll(r.DotReader())
		require.NoError(t, err)
		decodedOctets := len(message) + bytes.Count(message, []byte("\n"))
		require.Equal(t, decodedOctets, info["Message Octets"])
		if strings.HasPrefix(wire, "..X:") {
			require.Equal(t, false, info["Dot Transparency Canonical"])
			require.Equal(t, 5, info["Transparency Dot Count"])
			h := info["Headers"].([]map[string]any)[0]
			require.Equal(t, ".X", h["Name"])
			require.Equal(t, " a\tfold", h["Unfolded Value"])
			list := protocolCorpusFindNode(n, "Body Lines")
			require.Len(t, list.Children, 3)
			for i, text := range []string{".one", "single", "..two"} {
				protocolCorpusRequireValue(t, list.Children[i], "Text", text)
			}
		}
	}
}
