package bin_parser

import (
	"encoding/binary"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

func TestProtocolCorpusObservedMessageBoundaries(t *testing.T) {
	const corpusDir = "testdata/protocol-corpus"
	var manifest protocolCorpusManifest
	readProtocolCorpusJSON(t, filepath.Join(corpusDir, "manifest.json"), &manifest)
	captures := make(map[string]protocolCorpusCapture)
	for _, capture := range manifest.Captures {
		captures[capture.ID] = capture
	}
	catalog := make(map[string]ProtocolInfo)
	for _, info := range ProtocolCatalog {
		catalog[info.Name] = info
	}
	tests := []struct {
		id         string
		end        func([]byte) int
		mutate     func([]byte)
		diagnostic string
	}{
		{"scapy-doip", nil, func(b []byte) { b[1] ^= 1 }, "doip: inverse version mismatch"},
		{"mrhenrike-ethercat", func(b []byte) int { return 2 + int(binary.LittleEndian.Uint16(b[:2])&0x7ff) }, func(b []byte) { b[1] = b[1]&0x0f | 0x20 }, "ethercat: invalid frame header"},
		{"mgadelha-sv", func(b []byte) int { return int(binary.BigEndian.Uint16(b[2:4])) }, func(b []byte) { b[8] = 0x61 }, "sv: expected savPdu"},
		{"iti-goose", func(b []byte) int { return int(binary.BigEndian.Uint16(b[2:4])) }, func(b []byte) { b[8] = 0x60 }, "goose: expected goosePdu"},
		{"iti-profinet-dcp", func(b []byte) int { return 12 + int(binary.BigEndian.Uint16(b[10:12])) }, func(b []byte) { b[10], b[11] = 0xff, 0xff }, "dcp: data length exceeds input"},
		{"iti-tpkt", nil, func(b []byte) { b[3] ^= 1 }, "tpkt: packet length mismatch"},
		// COTP declares a header length, not the upper-layer data length. A
		// shortened body is only detectably truncated through its TPKT envelope.
		{"iti-cotp", func(b []byte) int { return int(b[0]) + 1 }, func(b []byte) { b[0] = 1 }, "cotp: invalid header length"},
	}
	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			capture, ok := captures[tc.id]
			require.True(t, ok)
			require.NotNil(t, capture.RoadmapName)
			info, ok := catalog[*capture.RoadmapName]
			require.True(t, ok)
			input := protocolCorpusParseInput(t, corpusDir, capture, info, protocolCorpusParseContracts[info.Name])
			rule := strings.TrimSuffix(strings.ReplaceAll(info.RuleFile, "/", "."), ".yaml")
			protocolCorpusRequireBoundedRuleParse(t, input, rule, info.EntryNode)
			end := len(input)
			if tc.end != nil {
				end = tc.end(input)
			}
			require.Greater(t, end, 2)
			require.LessOrEqual(t, end, len(input))
			seenCuts := map[int]bool{}
			for _, cut := range []int{0, 1, 2, 3, end / 2, end - 1} {
				if cut >= end || seenCuts[cut] {
					continue
				}
				seenCuts[cut] = true
				t.Run(fmt.Sprintf("truncated-at-%d", cut), func(t *testing.T) {
					_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(input[:cut]), rule, info.EntryNode)
					require.Error(t, err, "incomplete message accepted")
					diagnostic := protocolCorpusFailureDiagnostic(err)
					require.NotContains(t, diagnostic, "unknown type", "failed because of an uninitialized schema, not truncation")
					require.NotContains(t, diagnostic, "runtime error:", "unchecked runtime failure")
				})
			}
			invalid := append([]byte(nil), input...)
			tc.mutate(invalid)
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(invalid), rule, info.EntryNode)
			require.Error(t, err)
			require.Contains(t, protocolCorpusFailureDiagnostic(err), tc.diagnostic)
		})
	}
}
