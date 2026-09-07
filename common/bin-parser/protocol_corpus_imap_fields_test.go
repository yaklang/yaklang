package bin_parser

import (
	"bytes"
	"fmt"
	"io"
	"net/mail"
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

const imapFieldsTestRule = "application-layer.imap_fields"

func TestProtocolCorpusIMAPFieldsBoundariesAndIsolation(t *testing.T) {
	fixtures := map[string]string{
		"IMAPCommandFields":       "A01 LIST \"\" \"*\"\r\n",
		"IMAPResponseFields":      "* 1 FETCH (FLAGS () BODY[HEADER] {18}\r\nSubject: value\r\n\r\n)\r\n",
		"IMAPResponseBlockFields": "* LIST () NIL INBOX\r\nA01 OK done\r\n",
	}
	for entry, wire := range fixtures {
		for _, entry := range []string{entry, entry + "Carrier"} {
			_, err := parser.ParseBinary(bytes.NewReader([]byte(wire)), imapFieldsTestRule, entry)
			require.ErrorContains(t, err, "explicit")
			_, err = parser.GenerateBinary(map[string]any{}, imapFieldsTestRule, entry)
			require.Error(t, err)
			for _, bits := range []uint64{0, 1, 7, (1<<20)*8 + 1, (1<<20 + 1) * 8} {
				r := &tlsSHTestHeldReader{bits: bits}
				_, err := parser.ParseBinary(r, imapFieldsTestRule, entry)
				require.Error(t, err)
				require.Zero(t, r.reads)
			}
		}
		for offset := uint64(0); offset < 8; offset++ {
			for _, good := range []bool{true, false} {
				w := []byte(wire)
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
				root := latTestInline(t, fmt.Sprintf("endian: little\nPackage:\n  Wrapped:\n    operator: |\n      if %d > 0 { this.ProcessSubNode(\"Prefix\") }\n      this.GetSubNode(\"Message\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Message\")\n      this.ProcessSubNode(\"Sentinel\")\n      if %d > 0 { this.ProcessSubNode(\"Padding\") }\n    Prefix: uint8,%dbit\n    Message: \"import:application-layer/imap_fields.yaml;node:%sCarrier\"\n    Sentinel: uint8\n    Padding: uint8,%dbit\n", offset, len(w), offset, offset, entry, 8-offset))
				root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
				root.Ctx.SetItem("marker", 123)
				r := base.NewBitReader(bytes.NewReader(packed.Bytes()))
				require.NoError(t, r.Backup())
				require.NoError(t, root.ParseSubNode(r, "Wrapped"))
				n := protocolCorpusFindNode(root, "Message")
				tlsCertificateTestTree(t, n, w, offset)
				if good {
					require.Nil(t, protocolCorpusFindNode(n, "Unparsed IMAP Wire"))
				} else {
					protocolCorpusRequireValue(t, n, "Unparsed IMAP Wire", w)
					for _, name := range []string{"Tag", "Response Marker", "Literal Data", "Responses"} {
						require.Nil(t, protocolCorpusFindNode(n, name))
					}
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
		w := []byte(wire + wire)
		smtpFieldsTestWhole(t, w, 0, len(wire), "application-layer/imap_fields.yaml", entry)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(w), imapFieldsTestRule, entry)
		if entry == "IMAPResponseBlockFields" {
			require.NoError(t, err, "a bounded block deliberately permits multiple complete responses")
		} else {
			require.Error(t, err)
		}
	}
	for worker := 0; worker < 4; worker++ {
		worker := worker
		t.Run(fmt.Sprintf("isolated-%d", worker), func(t *testing.T) {
			t.Parallel()
			w := []byte(fixtures["IMAPResponseFields"])
			cfg := map[string]any{"marker": worker, "Layout Context": "command"}
			n := protocolCorpusRequireBoundedRuleParseWithConfig(t, w, imapFieldsTestRule, "IMAPResponseFields", cfg)
			info := n.Cfg.GetItem("additionInfo").(map[string]any)
			require.Equal(t, "response", info["Layout Context"])
			attrs := info["Attributes"].([]map[string]any)
			attrs[1]["Value Relative Byte Ranges"].([][2]int)[0][0] = 999
			attrs[1]["Headers"].([]map[string]any)[0]["Unfolded Value"] = "changed"
			n = protocolCorpusRequireBoundedRuleParse(t, w, imapFieldsTestRule, "IMAPResponseFields")
			attrs = n.Cfg.GetItem("additionInfo").(map[string]any)["Attributes"].([]map[string]any)
			require.Equal(t, bytes.Index(w, []byte("Subject:")), attrs[1]["Value Relative Byte Ranges"].([][2]int)[0][0])
			require.Equal(t, " value", attrs[1]["Headers"].([]map[string]any)[0]["Unfolded Value"])
			require.Equal(t, map[string]any{"marker": worker, "Layout Context": "command"}, cfg)
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(w), imapFieldsTestRule, "IMAPCommandFields")
			require.Error(t, err, "direction cannot be learned from another cached invocation")
		})
	}
}

func TestProtocolCorpusIMAPFieldsAllOriginalRecords(t *testing.T) {
	path := "testdata/protocol-corpus/captures/ndpi/ndpi-imap.pcap"
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "2eb094e37baa8a18b16090239468e691d792f8f8acaa25d7b2407701b51db2be", tlsCertificateTestSHA(raw))
	records := protocolCorpusAuditPackets(t, path)
	require.Len(t, records, 33)
	next := map[string]uint32{}
	var fetch []byte
	appBytes, controls, clientRecords, serverRecords, responses := 0, 0, 0, 0, 0
	commands := map[string]int{}
	responseNames := map[int][]string{4: {"OK"}, 7: {"CAPABILITY"}, 9: {"OK"}, 12: {"OK"}, 15: {"LIST"}, 17: {"LIST", "OK"}, 20: {"LSUB"}, 22: {"LSUB", "OK"}, 25: {"EXISTS"}, 27: {"RECENT", "OK", "OK", "FLAGS", "OK", "OK"}}
	for i, record := range records {
		frame := i + 1
		p := gopacket.NewPacket(record, layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, p.ErrorLayer())
		ip := p.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
		tcp := p.Layer(layers.LayerTypeTCP).(*layers.TCP)
		offset := 14 + int(ip.IHL)*4 + int(tcp.DataOffset)*4
		require.Equal(t, int(ip.Length)+14, len(record), "no original link padding")
		require.Equal(t, tcp.Payload, record[offset:])
		key := fmt.Sprint(p.NetworkLayer().NetworkFlow(), tcp.TransportFlow())
		if tcp.SYN {
			next[key] = tcp.Seq + 1
		}
		w := tcp.Payload
		if len(w) == 0 {
			controls++
			continue
		}
		require.Equal(t, 66, offset)
		require.Equal(t, next[key], tcp.Seq, "original frame %d gap/overlap", frame)
		next[key] += uint32(len(w))
		appBytes += len(w)
		if tcp.SrcPort == 143 {
			serverRecords++
		} else {
			require.Equal(t, layers.TCPPort(143), tcp.DstPort)
			clientRecords++
		}
		if frame == 30 || frame == 32 {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(w), imapFieldsTestRule, "IMAPResponseBlockFields")
			require.Error(t, err, "a header or literal fragment is not a complete response")
			n := smtpFieldsTestWhole(t, record, 66, len(w), "application-layer/imap_fields.yaml", "IMAPResponseBlockFieldsCarrier")
			protocolCorpusRequireValue(t, n, "Unparsed IMAP Wire", w)
			fetch = append(fetch, w...)
			continue
		}
		t.Run(fmt.Sprintf("frame-%d", frame), func(t *testing.T) {
			entry := "IMAPResponseBlockFields"
			if tcp.DstPort == 143 {
				entry = "IMAPCommandFields"
			}
			n := smtpFieldsTestWhole(t, record, 66, len(w), "application-layer/imap_fields.yaml", entry)
			info := n.Cfg.GetItem("additionInfo").(map[string]any)
			for _, key := range []string{"Session State Validated", "Mailbox State Validated", "TCP Reassembly Performed", "Structured Generation Supported", "Mailbox Names Decoded", "MIME Decoded"} {
				require.Equal(t, false, info[key], key)
			}
			require.Equal(t, 0, info["Literal Count"])
			if tcp.DstPort == 143 {
				fields := strings.Fields(string(w))
				protocolCorpusRequireValue(t, n, "Tag", fields[0])
				protocolCorpusRequireValue(t, n, "Command", fields[1])
				commands[fields[1]]++
				switch frame {
				case 6:
					require.Equal(t, "CAPABILITY", info["Command Name"])
				case 11:
					for i, name := range []string{"User Name", "Password"} {
						v := protocolCorpusFindNode(n, name)
						protocolCorpusRequireValue(t, v, "String Data", []byte(fields[i+2]))
					}
					require.NotContains(t, info, "Password")
					require.NotContains(t, info, "User Name")
				case 14, 19:
					require.Empty(t, info["Reference Name"])
					require.Equal(t, []byte("*"), info["Mailbox Pattern"])
				case 24:
					require.Equal(t, []byte("INBOX"), info["Mailbox"])
				case 29:
					require.Equal(t, "FETCH", info["UID Command"])
					seq := info["Sequence Set"].([]map[string]any)
					require.Len(t, seq, 1)
					require.Equal(t, uint64(1), seq[0]["First"])
					require.Equal(t, "*", seq[0]["Last"])
					attrs := info["Requested Attributes"].([]map[string]any)
					require.Len(t, attrs, 4)
					for i, name := range []string{"FLAGS", "RFC822.SIZE", "INTERNALDATE", "BODY.PEEK"} {
						require.Equal(t, name, attrs[i]["Name"])
					}
					require.Equal(t, "HEADER", attrs[3]["Section"])
				default:
					t.Fatal("unaccounted original command")
				}
			} else {
				messages := info["Responses"].([]map[string]any)
				require.Len(t, messages, len(responseNames[frame]))
				responses += len(messages)
				for i, m := range messages {
					require.Equal(t, responseNames[frame][i], m["Response Name"])
					span := m["Relative Byte Range"].([2]int)
					unit := w[span[0]:span[1]]
					one := protocolCorpusRequireBoundedRuleParse(t, unit, imapFieldsTestRule, "IMAPResponseFields")
					tlsCertificateTestTree(t, one, unit, 0)
					if m["Response Name"] == "LIST" || m["Response Name"] == "LSUB" {
						require.Empty(t, m["Mailbox Flags"])
						require.Equal(t, []byte("/"), m["Hierarchy Delimiter"])
						box := "INBOX"
						if frame == 17 || frame == 22 {
							box = "Sent Messages"
						}
						require.Equal(t, []byte(box), m["Mailbox"])
					}
				}
				if frame == 7 {
					require.Equal(t, []string{"IMAP4rev1", "IDLE", "AUTH=LOGIN", "AUTH=PLAIN", "AUTH=CRAM-MD5"}, messages[0]["Capabilities"])
				}
				if frame == 27 {
					require.Equal(t, uint64(1), messages[0]["Observed Number"])
					require.Equal(t, "UIDVALIDITY", messages[1]["Response Code"])
					require.Equal(t, uint64(1), messages[1]["Response Code Value"])
					require.Equal(t, "UNSEEN", messages[2]["Response Code"])
					require.Equal(t, uint64(1), messages[2]["Response Code Value"])
					for i, name := range []string{"\\Seen", "\\Flagged", "\\Deleted", "\\Answered", "\\Draft"} {
						require.Equal(t, name, messages[3]["Flags"].([]map[string]any)[i]["Name"])
						require.Equal(t, name, messages[4]["Permanent Flags"].([]map[string]any)[i]["Name"])
					}
					require.Equal(t, true, messages[4]["Empty Response Text Observed"])
					require.Equal(t, false, messages[4]["Response Text Layout Validated"])
					require.Equal(t, "READ-WRITE", messages[5]["Response Code"])
				}
			}
		})
	}
	require.Len(t, next, 2)
	require.Equal(t, 1580, appBytes)
	require.Equal(t, 15, controls)
	require.Equal(t, 6, clientRecords)
	require.Equal(t, 12, serverRecords)
	require.Equal(t, map[string]int{"CAPABILITY": 1, "LOGIN": 1, "LIST": 1, "LSUB": 1, "SELECT": 1, "UID": 1}, commands)
	require.Equal(t, 17, responses)
	require.Len(t, fetch, 808)
	require.Equal(t, "c974e7b31cde61bc611bd13c3190f89e584c3db0c0c58f5657caa0562de08a6a", tlsCertificateTestSHA(fetch))
	n := protocolCorpusRequireBoundedRuleParse(t, fetch, imapFieldsTestRule, "IMAPResponseBlockFields")
	tlsCertificateTestTree(t, n, fetch, 0)
	info := n.Cfg.GetItem("additionInfo").(map[string]any)
	require.Equal(t, 1, info["Literal Count"])
	require.Equal(t, 2, info["Response Count"])
	messages := info["Responses"].([]map[string]any)
	require.Equal(t, [2]int{0, 781}, messages[0]["Relative Byte Range"])
	require.Equal(t, [2]int{781, 808}, messages[1]["Relative Byte Range"])
	require.Equal(t, "ac45e7bb7562ea403edfff9478ea9508a75db3c0770d352bac916f190e289fe9", tlsCertificateTestSHA(fetch[:781]))
	attrs := messages[0]["Attributes"].([]map[string]any)
	require.Len(t, attrs, 5)
	require.Equal(t, uint64(10), attrs[0]["Value"])
	require.Equal(t, int64(1213095248), attrs[1]["Date Unix Seconds"])
	require.Equal(t, uint64(698), attrs[2]["Value"])
	require.Equal(t, "\\Recent", attrs[3]["Flags"].([]map[string]any)[0]["Name"])
	require.Equal(t, "HEADER", attrs[4]["Section"])
	require.Equal(t, "literal", attrs[4]["String Kind"])
	require.Equal(t, [][2]int{{112, 778}}, attrs[4]["Value Relative Byte Ranges"])
	literal := fetch[112:778]
	require.Equal(t, "5ed4c540f8af52db4475ef6514737e442bdc64f410b9123d354631543e581c09", tlsCertificateTestSHA(literal))
	protocolCorpusRequireValue(t, n, "Literal Octets", "666")
	protocolCorpusRequireValue(t, n, "Literal Data", literal)
	mailMessage, err := mail.ReadMessage(bytes.NewReader(literal))
	require.NoError(t, err)
	body, err := io.ReadAll(mailMessage.Body)
	require.NoError(t, err)
	require.Empty(t, body, "only the original header section is present")
	headers := attrs[4]["Headers"].([]map[string]any)
	require.Len(t, headers, 18)
	require.Equal(t, false, attrs[4]["MIME Decoded"])
	restore := func(start, end int) []byte {
		var value []byte
		if start < 112 {
			stop := min(end, 112)
			value = append(value, records[29][66+start:66+stop]...)
		}
		if end > 112 {
			from := max(start, 112)
			value = append(value, records[31][66+from-112:66+end-112]...)
		}
		require.True(t, bytes.Equal(fetch[start:end], value), "original range [%d,%d)", start, end)
		return value
	}
	var fields func(*base.Node)
	fields = func(n *base.Node) {
		if len(n.Children) > 0 {
			for _, c := range n.Children {
				fields(c)
			}
			return
		}
		p := stream_parser.GetNodeResultPos(n)
		restore(int(p[0]/8), int(p[1]/8))
	}
	fields(n)
	for _, h := range headers {
		var unfolded []byte
		for _, span := range h["Value Encoded Relative Byte Ranges"].([][2]int) {
			unfolded = append(unfolded, restore(span[0], span[1])...)
		}
		require.Equal(t, string(unfolded), h["Unfolded Value"])
		require.Equal(t, strings.Join(strings.Fields(mailMessage.Header.Get(h["Name"].(string))), " "), strings.Join(strings.Fields(string(unfolded)), " "))
	}
	require.Len(t, headers[0]["Value Encoded Relative Byte Ranges"], 3)
}
