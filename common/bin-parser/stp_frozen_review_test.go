package bin_parser

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// This immutable input was labelled generated-positive at the archived head.
// It actually has version2/type0 and no rapid Version1Length byte. Preserve
// the archive and old answer; the active matrix must report its refusal,
// rather than weakening the current grammar or silently deleting the input.
func reviewedSTPInvalidPositive(c protocolCorpusCapture) bool {
	return c.ID == "pr5023-gen-rstp" && c.EvidenceKind == "generated-positive"
}

func requireReviewedSTPOriginalPositiveRefusal(t *testing.T, input []byte, err error) {
	t.Helper()
	require.Len(t, input, 35)
	require.Equal(t, "d1d47f057a5d8ee69479821a3b301e71cf1a4770ca5ee718fc6b7b75a103c4b8", fmt.Sprintf("%x", sha256.Sum256(input)))
	require.Equal(t, []byte{0, 0, 2, 0}, input[:4])
	require.ErrorContains(t, err, "STP version/type combination")
	_, strictErr := ParseStructured(input, "stp", "STPFields")
	require.ErrorContains(t, strictErr, "STP version/type combination")
	control := append(append([]byte(nil), input...), 0)
	control[3] = 2
	v, controlErr := ParseStructured(control, "stp", "STPFields")
	require.NoError(t, controlErr)
	m := v["fields"].(map[string]any)["Message"].(map[string]any)
	require.Equal(t, "RSTP", m["Protocol Name"])
	t.Log("original archived RSTP positive retained unchanged; exact version2/type0 wire now refused, independent complete type2 control accepted")
}
