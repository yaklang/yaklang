package bin_parser

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func TestIMAPSASLRuleCatalogAndWireCoordinates(t *testing.T) {
	for entry, wire := range map[string]string{"IMAPSASLResponse": "ZHVtbXk=\r\n", "IMAPSASLChallenge": "+ ZHVtbXk=\r\n", "IMAPSASLInitialValue": "="} {
		_, err := parser.ParseBinary(bytes.NewReader([]byte(wire)), "application-layer.imap_sasl", entry)
		require.ErrorContains(t, err, "explicit")
		_, err = parser.GenerateBinary(map[string]any{}, "application-layer.imap_sasl", entry)
		require.Error(t, err)
		for offset := uint64(0); offset < 8; offset++ {
			var packed bytes.Buffer
			writer := base.NewBitWriter(&packed)
			if offset > 0 {
				require.NoError(t, writer.WriteBits([]byte{0x55}, offset))
			}
			require.NoError(t, writer.WriteBits([]byte(wire), uint64(len(wire))*8))
			require.NoError(t, writer.WriteBits([]byte{0xd3}, 8))
			if offset > 0 {
				require.NoError(t, writer.WriteBits([]byte{0}, 8-offset))
			}
			root := latTestInline(t, fmt.Sprintf("endian: little\nPackage:\n  Wrapped:\n    operator: |\n      if %d > 0 { this.ProcessSubNode(\"Prefix\") }\n      this.GetSubNode(\"Message\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Message\")\n      this.ProcessSubNode(\"Sentinel\")\n      if %d > 0 { this.ProcessSubNode(\"Padding\") }\n    Prefix: uint8,%dbit\n    Message: \"import:application-layer/imap_sasl.yaml;node:%s\"\n    Sentinel: uint8\n    Padding: uint8,%dbit\n", offset, len(wire), offset, offset, entry, 8-offset))
			root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
			require.NoError(t, root.ParseSubNode(base.NewBitReader(bytes.NewReader(packed.Bytes())), "Wrapped"))
			node := protocolCorpusFindNode(root, "Message")
			tlsCertificateTestTree(t, node, []byte(wire), offset)
			protocolCorpusRequireValue(t, root, "Sentinel", uint64(0xd3))
			require.Equal(t, packed.Bytes(), NodeToBytes(base.GetNodeByPath(root, "@Wrapped")))
		}
	}
}

func TestIMAPSASLCanonicalBoundedScope(t *testing.T) {
	// Execute actual wire/bit-boundary tests before recognizing these exact
	// partial rows. This is no blanket exemption for incomplete P1 protocols.
	TestIMAPSASLRuleCatalogAndWireCoordinates(t)
	for _, name := range []string{"IMAP SASL Response", "IMAP SASL Challenge", "IMAP SASL Initial Value"} {
		catalog, roadmap := false, false
		for _, entry := range ProtocolCatalog {
			if entry.Name == name {
				catalog = true
				require.Equal(t, statusPartial, entry.Status)
				require.Equal(t, "application-layer/imap_sasl.yaml", entry.RuleFile)
				require.NotEmpty(t, entry.Notes)
			}
		}
		for _, entry := range ProtocolRoadmap {
			if entry.Name == name {
				roadmap = true
				require.Equal(t, stPartial, entry.Status)
				require.Equal(t, priP1, entry.Priority)
				require.NotEmpty(t, entry.Notes)
			}
		}
		require.True(t, catalog)
		require.True(t, roadmap)
		score, ok := ResolveP1Scorecard(name)
		require.True(t, ok)
		require.Equal(t, "L4", score.SampleClass)
		require.Zero(t, score.Schema+score.Traffic+score.Tests+score.Branches+score.Stack)
	}
	info, err := stream_parser.DecodeIMAPSASLValue([]byte("ZHVtbXk=\r\n"), "response")
	require.NoError(t, err)
	for _, key := range []string{"Authentication Verified", "Peer Identity Verified", "Security Layer Verified", "Decoded Values Are Wire Spans"} {
		require.Equal(t, false, info[key])
	}
	_, err = stream_parser.DecodeIMAPSASLValue([]byte("%%%invalid\r\n"), "response")
	require.Error(t, err)
}

func TestRTSPRuleFlushAndExactMethod(t *testing.T) {
	root, err := parser.ParseBinary(bytes.NewReader([]byte("FLUSH rtsp://example.test/a RTSP/1.0\r\nCSeq: 7\r\nRTP-Info: seq=1;rtptime=2\r\n\r\n")), "rtsp", "RTSP")
	require.NoError(t, err)
	protocolCorpusRequireValue(t, root, "Method", "FLUSH")
	for _, method := range []string{"OPTIONSBOGUS", "PLAYBOGUS", "FLUSHBOGUS"} {
		_, err := parser.ParseBinary(bytes.NewReader([]byte(method+" rtsp://example.test/a RTSP/1.0\r\nCSeq: 7\r\n\r\n")), "rtsp", "RTSP")
		require.ErrorContains(t, err, "unknown method")
	}
	for _, rule := range []string{"rtsp", "rtsp_session"} {
		for _, method := range []string{"FLUSH", "OPTIONSBOGUS", "FLUSHBOGUS"} {
			wire := []byte(method + " rtsp://example.test/a RTSP/1.0\r\nCSeq: 7\r\n\r\n")
			root := latTestInline(t, fmt.Sprintf("Package:\n  Message: \"import:%s.yaml;node:RTSP\"\n", rule))
			root.Cfg.SetItem(base.CfgLength, uint64(len(wire))*8)
			err := root.ParseSubNode(base.NewBitReader(bytes.NewReader(wire)), "Message")
			if method == "FLUSH" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "unknown method")
			}
		}
	}
}
