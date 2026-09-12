package bin_parser

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

// Parsing uses the native list operation, but structured generation must still
// execute the field-oriented YAML operator. Exercise both empty and nonempty
// fragments with a real public GenerateBinary input, not just a mode mock.
func TestProtocolCorpusDICOMGeneratePDVCompatibility(t *testing.T) {
	input := map[string]any{
		"PDU Type": 4, "Reserved": 0, "PDU Length": 14,
		"Presentation Data Values": []any{
			map[string]any{"PDV Length": 2, "Context ID": 253, "Control Reserved": 63, "Last Fragment": 1, "Command Fragment": 1},
			map[string]any{"PDV Length": 4, "Context ID": 253, "Control Reserved": 32, "Last Fragment": 1, "Command Fragment": 0, "Message Fragment": []byte{0xab, 0xcd}},
		},
	}
	want := dicomTestPDU(4, append(dicomTestPDV(253, 0xff, nil), dicomTestPDV(253, 0x82, []byte{0xab, 0xcd})...))
	node, err := parser.GenerateBinary(input, dicomRule, "DICOM")
	require.NoError(t, err)
	require.Equal(t, want, NodeToBytes(node))
	dicomNativeRequireFieldRanges(t, node, 0, [][]byte{nil, {0xab, 0xcd}}, []byte{0xff, 0x82})
}
