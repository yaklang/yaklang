package bin_parser

import (
	"bytes"
	"crypto/sha256"
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
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

const smtpReplyOriginal = "220-gator4223.hostgator.com ESMTP Exim 4.93 #2 Fri, 05 Mar 2021 04:01:45 -0600\r\n220-We do not authorize the use of this system to transport unsolicited,\r\n220 and/or bulk e-mail.\r\n"

func smtpReplyTestParse(t *testing.T, wire []byte, entry string) *base.Node {
	t.Helper()
	n := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.smtp_reply", entry)
	require.Equal(t, wire, NodeToBytes(n))
	h225TestTree(t, n, wire, 0)
	return n
}

func smtpReplyTestWhole(t *testing.T, wire []byte, entry string) *base.Node {
	t.Helper()
	source := fmt.Sprintf("unit: byte\nPackage:\n  Capture:\n    operator: |\n      this.ProcessSubNode(\"Envelope\")\n      this.GetSubNode(\"Reply\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Reply\")\n    Envelope: raw,54\n    Reply: \"import:application-layer/smtp_reply.yaml;node:%s\"\n", len(wire)-54, entry)
	var doc yaml.MapSlice
	require.NoError(t, yaml.Unmarshal([]byte(source), &doc))
	root, err := base.NewNodeTree(doc)
	require.NoError(t, err)
	root.Cfg.SetItem(base.CfgLength, uint64(len(wire))*8)
	reader := base.NewBitReader(bytes.NewReader(wire))
	require.NoError(t, root.ParseSubNode(reader, "Capture"))
	n := base.GetNodeByPath(root, "@Capture")
	h225TestTree(t, n, wire, 0)
	require.Equal(t, wire, NodeToBytes(n))
	require.ErrorContains(t, reader.Recovery(), "no backup")
	return protocolCorpusFindNode(n, "Reply")
}

func TestProtocolCorpusSMTPReplyAllOriginalRecords(t *testing.T) {
	path := "testdata/protocol-corpus/captures/ndpi/ndpi-smtps.pcapng"
	file, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "8d68f3726c5ea2b8527cf52a28b8e132b2c332633562f24280f82de05832f169", fmt.Sprintf("%x", sha256.Sum256(file)))
	records := protocolCorpusAuditPackets(t, path)
	require.Len(t, records, 4)
	var classified []string
	for i, record := range records {
		packet := gopacket.NewPacket(record, layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, packet.ErrorLayer())
		tcp := packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
		ip := packet.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
		require.Equal(t, len(record)-14, int(ip.Length))
		if i < 2 {
			require.True(t, tcp.SYN)
			require.Equal(t, i == 1, tcp.ACK)
			require.Empty(t, tcp.Payload)
			classified = append(classified, "transport-only")
			continue
		}
		require.Equal(t, uint8(5), ip.IHL)
		require.Equal(t, uint8(5), tcp.DataOffset)
		require.Equal(t, record[54:], tcp.Payload)
		if i == 2 {
			require.Len(t, tcp.Payload, 517)
			require.Equal(t, []byte{22, 3, 1, 2, 0, 1}, tcp.Payload[:6])
			n, err := parser.ParseBinary(newProtocolCorpusBoundedReader(tcp.Payload), "application-layer.tls")
			require.NoError(t, err)
			protocolCorpusRequireValue(t, n, "ContentType", uint64(22))
			require.Equal(t, tcp.Payload, NodeToBytes(n))
			_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(tcp.Payload), "application-layer.smtp_reply", "SMTPReply")
			require.Error(t, err)
			fallback := smtpReplyTestWhole(t, record, "SMTPReplyCarrier")
			protocolCorpusRequireValue(t, fallback, "Unparsed SMTP Reply", tcp.Payload)
			require.Nil(t, protocolCorpusFindNode(fallback, "Reply Lines"))
			classified = append(classified, "TLS ClientHello, not SMTP reply")
			continue
		}
		require.Equal(t, layers.TCPPort(465), tcp.SrcPort)
		require.Equal(t, layers.TCPPort(37682), tcp.DstPort)
		initial := gopacket.NewPacket(records[0], layers.LayerTypeEthernet, gopacket.Default).Layer(layers.LayerTypeTCP).(*layers.TCP)
		require.Equal(t, initial.Seq+1, tcp.Ack) // Relative ACK 1, not absolute sequence 1.
		require.Equal(t, []byte(smtpReplyOriginal), tcp.Payload)
		require.Len(t, tcp.Payload, 179)
		n := smtpReplyTestWhole(t, record, "SMTPReply")
		list := protocolCorpusFindNode(n, "Reply Lines")
		require.True(t, list.Cfg.GetBool(stream_parser.CfgIsList))
		require.Len(t, list.Children, 3)
		for j, line := range strings.Split(strings.TrimSuffix(smtpReplyOriginal, "\r\n"), "\r\n") {
			child := list.Children[j]
			protocolCorpusRequireValue(t, child, "Code", "220")
			protocolCorpusRequireValue(t, child, "Separator", uint64(line[3]))
			protocolCorpusRequireValue(t, child, "Message", line[4:])
			protocolCorpusRequireValue(t, child, "CRLF", []byte("\r\n"))
		}
		info := n.Cfg.GetItem("additionInfo").(map[string]any)
		require.Equal(t, 220, info["Reply Code"])
		require.Equal(t, 3, info["Line Count"])
		for _, key := range []string{"Connection State Validated", "TLS Handshake Validated", "Greeting Grammar Validated", "Endpoint Identity Proven", "Message Delivery Proven", "Sender Conformance Validated"} {
			require.Equal(t, false, info[key], key)
		}
		// The legacy TLS rule is a stop-on-error list, so a nil error from it
		// cannot prove full TLS acceptance. Check the actual record bytes instead:
		// type 0x32 is not a TLS record type and declared size 0x2d67 exceeds 174.
		require.Equal(t, []byte{0x32, 0x32, 0x30, 0x2d, 0x67}, tcp.Payload[:5])
		require.NotContains(t, []byte{20, 21, 22, 23, 24}, tcp.Payload[0])
		require.Greater(t, int(binary.BigEndian.Uint16(tcp.Payload[3:5])), len(tcp.Payload)-5)
		classified = append(classified, "plaintext SMTP multiline reply, not TLS")
	}
	require.Equal(t, []string{"transport-only", "transport-only", "TLS ClientHello, not SMTP reply", "plaintext SMTP multiline reply, not TLS"}, classified)
}

func TestProtocolCorpusSMTPReplyBoundariesAndCompatibility(t *testing.T) {
	for _, wire := range []string{smtpReplyOriginal, "250\r\n", "250-\r\n250 \r\n", "550 \tmessage\r\n"} {
		smtpReplyTestParse(t, []byte(wire), "SMTPReply")
		for cut := 0; cut < len(wire); cut++ {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader([]byte(wire[:cut])), "application-layer.smtp_reply", "SMTPReply")
			require.Error(t, err, "prefix %d", cut)
		}
	}
	for _, wire := range []string{"220-first\r\n221 last\r\n", "220 done\r\n220 extra\r\n", "220-first\r\n", "000 invalid\r\n", "220\tbad\r\n", "220 bad\x7f\r\n", "220 bad\n", "220 " + strings.Repeat("x", 507) + "\r\n"} {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader([]byte(wire)), "application-layer.smtp_reply", "SMTPReply")
		require.Error(t, err)
		n := smtpReplyTestParse(t, []byte(wire), "SMTPReplyCarrier")
		protocolCorpusRequireValue(t, n, "Unparsed SMTP Reply", []byte(wire))
		require.Nil(t, protocolCorpusFindNode(n, "Reply Lines"))
	}
	for _, entry := range []string{"SMTPReply", "SMTPReplyCarrier"} {
		_, err := parser.ParseBinary(bytes.NewReader([]byte(smtpReplyOriginal)), "application-layer.smtp_reply", entry)
		require.ErrorContains(t, err, "explicit")
		_, err = parser.GenerateBinary(map[string]any{}, "application-layer.smtp_reply", entry)
		require.Error(t, err)
	}
	// Keep the pre-existing self-delimited single-line API and its field names.
	reader := bytes.NewReader([]byte(smtpReplyOriginal))
	n, err := parser.ParseBinary(reader, "application-layer.smtp", "SMTP")
	require.NoError(t, err)
	protocolCorpusRequireValue(t, n, "Code", "220")
	protocolCorpusRequireValue(t, n, "Separator", uint64('-'))
	first := bytes.Index([]byte(smtpReplyOriginal), []byte("\r\n")) + 2
	require.Equal(t, []byte(smtpReplyOriginal[:first]), NodeToBytes(n))
	require.Equal(t, len(smtpReplyOriginal)-first, reader.Len())
}

func TestProtocolCorpusSMTPReplyOffsetsRollbackAndIsolation(t *testing.T) {
	for offset := uint64(0); offset < 8; offset++ {
		for _, good := range []bool{true, false} {
			wire := []byte("250-first\r\n250 last\r\n")
			if !good {
				wire[11] = '5'
			}
			var packed bytes.Buffer
			writer := base.NewBitWriter(&packed)
			if offset > 0 {
				require.NoError(t, writer.WriteBits([]byte{0x55}, offset))
			}
			require.NoError(t, writer.WriteBits(wire, uint64(len(wire))*8))
			require.NoError(t, writer.WriteBits([]byte{0xd3}, 8))
			if offset > 0 {
				require.NoError(t, writer.WriteBits([]byte{0}, 8-offset))
			}
			source := fmt.Sprintf("endian: little\nPackage:\n  Wrapped:\n    operator: |\n      if %d > 0 { this.ProcessSubNode(\"Prefix\") }\n      this.GetSubNode(\"Record\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Record\")\n      this.ProcessSubNode(\"Sentinel\")\n      if %d > 0 { this.ProcessSubNode(\"Padding\") }\n    Prefix: uint8,%dbit\n    Record: \"import:application-layer/smtp_reply.yaml;node:SMTPReplyCarrier\"\n    Sentinel: uint8\n    Padding: uint8,%dbit\n", offset, len(wire), offset, offset, 8-offset)
			var doc yaml.MapSlice
			require.NoError(t, yaml.Unmarshal([]byte(source), &doc))
			root, err := base.NewNodeTree(doc)
			require.NoError(t, err)
			root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
			root.Ctx.SetItem("body_length", 123)
			reader := base.NewBitReader(bytes.NewReader(packed.Bytes()))
			require.NoError(t, root.ParseSubNode(reader, "Wrapped"))
			n := base.GetNodeByPath(root, "@Wrapped")
			record := protocolCorpusFindNode(n, "Record")
			h225TestTree(t, record, wire, offset)
			if good {
				protocolCorpusRequireValue(t, record, "Code", "250")
			} else {
				protocolCorpusRequireValue(t, record, "Unparsed SMTP Reply", wire)
				require.Nil(t, protocolCorpusFindNode(record, "Reply Lines"))
			}
			protocolCorpusRequireValue(t, n, "Sentinel", uint64(0xd3))
			require.Equal(t, packed.Bytes(), NodeToBytes(n))
			require.Equal(t, 123, root.Ctx.GetItem("body_length"))
			require.ErrorContains(t, reader.Recovery(), "no backup")
			_, err = reader.ReadBits(8)
			require.ErrorIs(t, err, io.EOF)
		}
	}
	for worker := 0; worker < 4; worker++ {
		worker := worker
		t.Run(fmt.Sprintf("isolated-%d", worker), func(t *testing.T) {
			t.Parallel()
			code := fmt.Sprintf("25%d", worker)
			wire := []byte(code + "-first\r\n" + code + " last\r\n")
			cfg := map[string]any{"marker": worker}
			n := protocolCorpusRequireBoundedRuleParseWithConfig(t, wire, "application-layer.smtp_reply", "SMTPReply", cfg)
			protocolCorpusRequireValue(t, n, "Code", code)
			require.Equal(t, map[string]any{"marker": worker}, cfg)
		})
	}
}
