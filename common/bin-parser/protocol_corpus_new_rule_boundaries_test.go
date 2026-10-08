package bin_parser

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProtocolCorpusNewRuleBoundaries(t *testing.T) {
	const corpusDir = "testdata/protocol-corpus"

	var manifest protocolCorpusManifest
	readProtocolCorpusJSON(t, corpusDir+"/manifest.json", &manifest)
	captures := make(map[string]protocolCorpusCapture, len(manifest.Captures))
	for _, capture := range manifest.Captures {
		captures[capture.ID] = capture
	}

	tests := []struct {
		captureID string
		name      string
		mutate    func([]byte)
		wantError string
	}{
		{
			captureID: "ndpi-elasticsearch",
			name:      "Elasticsearch",
			mutate:    func(input []byte) { input[0] = 0 },
			wantError: "elasticsearch: magic must be ES",
		},
		{
			captureID: "ndpi-ocsp",
			name:      "OCSP",
			mutate:    func(input []byte) { input[0] = 0x31 },
			wantError: "ocsp: request must be a DER sequence",
		},
		{
			captureID: "ndpi-profinet-io",
			name:      "Profinet IO",
			mutate:    func(input []byte) { input[0] = 3 },
			wantError: "profinet-io: DCE/RPC version must be 4",
		},
		{
			captureID: "ndpi-upnp",
			name:      "UPnP",
			mutate:    func(input []byte) { input[0] = 'X' },
			wantError: "upnp: expected SSDP NOTIFY",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			capture, ok := captures[test.captureID]
			require.True(t, ok, "missing capture %s", test.captureID)
			contract, ok := protocolCorpusParseContracts[test.name]
			require.True(t, ok, "missing parse contract for %s", test.name)
			info := ProtocolInfo{Name: test.name, Layer: contract.Layer, RuleFile: contract.RuleFile}
			input := protocolCorpusParseInput(t, corpusDir, capture, info, contract)
			require.NotEmpty(t, input)

			controlReader := newProtocolCorpusBoundedReader(input)
			controlNode, err := protocolCorpusParseRule(controlReader, contract)
			require.NoError(t, err)
			require.NotNil(t, controlNode)
			require.Zero(t, controlReader.Len(), "positive control left bytes unread")

			invalid := append([]byte(nil), input...)
			test.mutate(invalid)
			invalidReader := newProtocolCorpusBoundedReader(invalid)
			_, err = protocolCorpusParseRule(invalidReader, contract)
			require.ErrorContains(t, err, test.wantError)
		})
	}
}
