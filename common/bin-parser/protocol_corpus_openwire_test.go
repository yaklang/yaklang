package bin_parser

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProtocolCorpusOpenWireWireFormatInfoAndMalformedFields(t *testing.T) {
	const corpusDir = "testdata/protocol-corpus"
	const expectedSHA256 = "0846d67344609325ec6cb4afb55a30f17597420a79813798913542a33d388a76"
	var manifest protocolCorpusManifest
	readProtocolCorpusJSON(t, filepath.Join(corpusDir, "manifest.json"), &manifest)
	var capture protocolCorpusCapture
	for _, candidate := range manifest.Captures {
		if candidate.ID == "ndpi-openwire" {
			capture = candidate
			break
		}
	}
	require.Equal(t, "ndpi-openwire", capture.ID)
	require.Equal(t, "ActiveMQ OpenWire", capture.Protocol)
	require.Equal(t, expectedSHA256, capture.SHA256)
	require.Equal(t, 43, capture.PacketCount)
	require.Equal(t, "Null", capture.LinkType)

	var sources protocolCorpusSourceSpec
	readProtocolCorpusJSON(t, filepath.Join(corpusDir, "sources.json"), &sources)
	var source protocolCorpusSourceCapture
	for _, candidate := range sources.Captures {
		if candidate.ID == "ndpi-openwire" {
			source = candidate
			break
		}
	}
	require.Equal(t, "ndpi-openwire", source.ID)
	require.Equal(t, expectedSHA256, source.SourceSHA256)

	captureData := readProtocolCorpusFile(t, ".", filepath.Join(corpusDir, capture.CaptureFile))
	assertProtocolCorpusHash(t, capture.CaptureFile, captureData, expectedSHA256)
	packetCount, linkType, frame := inspectProtocolCorpusCapture(t, captureData, capture.RepresentativeFrame)
	require.Equal(t, capture.PacketCount, packetCount)
	require.Equal(t, capture.LinkType, linkType)
	input := protocolCorpusRuleBytes(t, frame, linkType, ProtocolInfo{Name: capture.Protocol, Layer: "L7"})
	require.Equal(t, 222, len(input))

	contract := protocolCorpusParseContracts[capture.Protocol]
	node := protocolCorpusRequireBoundedRuleParse(t, input, "application-layer.extended_protocols", "ActiveMQOpenWire")
	protocolCorpusRequireValue(t, node, "Frame Length", uint64(218))
	protocolCorpusRequireValue(t, node, "Data Type", uint64(1))
	protocolCorpusRequireValue(t, node, "Magic", uint64(0x4163746976654d51))
	protocolCorpusRequireValue(t, node, "Version", uint64(10))
	protocolCorpusRequireValue(t, node, "Properties Present", uint64(1))
	protocolCorpusRequireValue(t, node, "Properties Length", uint64(200))

	mutations := []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{"wrong command type", func(w []byte) []byte { w[4] = 2; return w }},
		{"wrong magic", func(w []byte) []byte { w[5] ^= 1; return w }},
		{"zero version", func(w []byte) []byte { clear(w[13:17]); return w }},
		{"invalid properties marker", func(w []byte) []byte { w[17] = 2; return w }},
		{"properties length mismatch", func(w []byte) []byte { w[21]--; return w }},
		{"frame length mismatch", func(w []byte) []byte { w[3]++; return w }},
		{"truncated properties", func(w []byte) []byte { return w[:len(w)-1] }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			bad := mutation.mutate(append([]byte(nil), input...))
			reader := newProtocolCorpusBoundedReader(bad)
			_, err := protocolCorpusParseRule(reader, contract)
			require.Error(t, err, "%s malformed WireFormatInfo was accepted", mutation.name)
			require.Contains(t, err.Error(), "ActiveMQOpenWire", "%s failed outside the WireFormatInfo entry: %v", mutation.name, err)
		})
	}
	_, err := protocolCorpusParseRule(newProtocolCorpusBoundedReader(input), contract)
	require.NoError(t, err, "the independent nDPI WireFormatInfo frame must remain parseable")
}
