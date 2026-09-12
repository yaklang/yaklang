package bin_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func parsePR5023NetworkSample(t *testing.T, input []byte, entry string) (*base.Node, error) {
	t.Helper()
	reader := newProtocolCorpusBoundedReader(input)
	node, err := parser.ParseBinary(reader, "network_samples", entry)
	if err == nil {
		require.Zero(t, reader.Len(), "%s left input bytes unread", entry)
	}
	return node, err
}

func TestProtocolCorpusPR5023NetworkFieldsAndBoundaries(t *testing.T) {
	const corpus = "testdata/protocol-corpus/captures"
	for _, spec := range []struct {
		path   string
		entry  string
		offset int
		bad    string
	}{
		{"generated-pr5023/pr5023-gen-ipcomp.pcap", "IPComp", 34, "invalid DEFLATE"},
		{"generated-validated/gen-ipcomp-valid.pcap", "IPComp", 34, ""},
		{"generated-pr5023/pr5023-gen-nvgre.pcap", "NVGRE", 34, "mandatory key flag"},
		{"generated-validated/gen-nvgre-valid.pcap", "NVGRE", 34, ""},
		{"generated-pr5023/pr5023-gen-mpls-pw.pcap", "MPLSEthernetPW", 14, ""},
	} {
		t.Run(spec.path, func(t *testing.T) {
			frames := protocolCorpusAuditPackets(t, filepath.Join(corpus, spec.path))
			require.Len(t, frames, 1)
			for index, frame := range frames {
				t.Run(fmt.Sprintf("frame-%d", index+1), func(t *testing.T) {
					require.Greater(t, len(frame), spec.offset)
					wire := frame[spec.offset:]
					node, err := parsePR5023NetworkSample(t, wire, spec.entry)
					if spec.bad != "" {
						require.ErrorContains(t, err, spec.bad)
						return
					}
					require.NoError(t, err)
					switch spec.entry {
					case "IPComp":
						protocolCorpusRequireValue(t, node, "Next Header", uint64(17))
						protocolCorpusRequireValue(t, node, "Compression Parameter Index", uint64(2))
						info := node.Cfg.GetItem("additionInfo").(map[string]any)
						require.Equal(t, true, info["Payload Decoded"])
						plain := info["Payload"].([]byte)
						require.Len(t, plain, 520)
						require.Equal(t, uint16(12000), binary.BigEndian.Uint16(plain))
						require.Equal(t, uint16(len(plain)), binary.BigEndian.Uint16(plain[4:]))
						require.Equal(t, bytes.Repeat([]byte("protocol sample "), 32), plain[8:])
					case "NVGRE":
						protocolCorpusRequireValue(t, node, "Virtual Subnet ID", uint64(0x123456))
						protocolCorpusRequireValue(t, node, "Flow ID", uint64(0x78))
						protocolCorpusRequireValue(t, node, "Destination Port", uint64(12001))
					case "MPLSEthernetPW":
						labels := protocolCorpusFindNode(node, "Label Stack")
						require.Len(t, labels.Children, 1)
						protocolCorpusRequireValue(t, labels.Children[0].Children[0], "Label", uint64(16))
						protocolCorpusRequireValue(t, node, "Sequence Number", uint64(0))
						protocolCorpusRequireValue(t, node, "Type", uint64(0x0800))
					}
					for cut := 0; cut < len(wire); cut++ {
						_, err := parsePR5023NetworkSample(t, wire[:cut], spec.entry)
						require.Errorf(t, err, "%s accepted truncated input at %d/%d", spec.entry, cut, len(wire))
					}
				})
			}
		})
	}
}

func TestPR5023NetworkProtocolVariants(t *testing.T) {
	const corpus = "testdata/protocol-corpus/captures"
	ipcomp := []byte{6, 0x80, 1, 0, 1, 2, 3}
	node, err := parsePR5023NetworkSample(t, ipcomp, "IPComp")
	require.NoError(t, err)
	info := node.Cfg.GetItem("additionInfo").(map[string]any)
	require.Equal(t, false, info["Payload Decoded"], "negotiated CPI must not guess an algorithm")
	require.Equal(t, ipcomp[4:], info["Payload"])
	protocolCorpusRequireValue(t, node, "Flags", uint64(0x80))

	nvgre := protocolCorpusAuditPackets(t, filepath.Join(corpus, "generated-validated/gen-nvgre-valid.pcap"))[0][34:]
	ignoredFlags := bytes.Clone(nvgre)
	binary.BigEndian.PutUint16(ignoredFlags, 0x23f8)
	node, err = parsePR5023NetworkSample(t, ignoredFlags, "NVGRE")
	require.NoError(t, err, "GRE reserved bits 6-12 must be ignored on receipt")
	protocolCorpusRequireValue(t, node, "Flags And Version", uint64(0x23f8))
	for _, flags := range []uint16{0, 0xa000, 0x3000, 0x2001, 0x6000, 0x2800, 0x2400} {
		invalid := bytes.Clone(nvgre)
		binary.BigEndian.PutUint16(invalid, flags)
		_, err := parsePR5023NetworkSample(t, invalid, "NVGRE")
		require.ErrorContains(t, err, "mandatory key flag")
	}
	invalidType := bytes.Clone(nvgre)
	binary.BigEndian.PutUint16(invalidType[2:], 0x0800)
	_, err = parsePR5023NetworkSample(t, invalidType, "NVGRE")
	require.ErrorContains(t, err, "transparent Ethernet")
	tagged := append(bytes.Clone(nvgre[:20]), append([]byte{0x81, 0, 0, 1}, nvgre[20:]...)...)
	_, err = parsePR5023NetworkSample(t, tagged, "NVGRE")
	require.ErrorContains(t, err, "inner IEEE 802.1Q tag")

	pw := protocolCorpusAuditPackets(t, filepath.Join(corpus, "generated-pr5023/pr5023-gen-mpls-pw.pcap"))[0][14:]
	stacked := append([]byte{0, 2, 0, 64}, pw...)
	node, err = parsePR5023NetworkSample(t, stacked, "MPLSEthernetPW")
	require.NoError(t, err)
	require.Len(t, protocolCorpusFindNode(node, "Label Stack").Children, 2)
	reserved := bytes.Clone(pw)
	reserved[4], reserved[5], reserved[6], reserved[7] = 0x0f, 0xff, 0xab, 0xcd
	node, err = parsePR5023NetworkSample(t, reserved, "MPLSEthernetPW")
	require.NoError(t, err, "Ethernet PW reserved bits must be retained and ignored")
	protocolCorpusRequireValue(t, node, "Reserved", uint64(0x0fff))
	protocolCorpusRequireValue(t, node, "Sequence Number", uint64(0xabcd))
	reserved[4] = 0x10
	_, err = parsePR5023NetworkSample(t, reserved, "MPLSEthernetPW")
	require.ErrorContains(t, err, "control word prefix")
	_, err = parsePR5023NetworkSample(t, bytes.Repeat([]byte{0, 2, 0, 64}, 32), "MPLSEthernetPW")
	require.ErrorContains(t, err, "label stack exceeds limit")
}
