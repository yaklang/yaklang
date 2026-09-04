package phase3

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
)

func TestSortFindingsByConfidenceDesc(t *testing.T) {
	in := []*model.Finding{
		{ID: "VULN-001", Confidence: 4},
		{ID: "VULN-002", Confidence: 9},
		{ID: "VULN-003", Confidence: 7},
		{ID: "VULN-004", Confidence: 9},
		{ID: "VULN-005", Confidence: 2},
	}
	got := sortFindingsByConfidenceDesc(in)
	require.Equal(t, []string{"VULN-002", "VULN-004", "VULN-003", "VULN-001", "VULN-005"},
		[]string{got[0].ID, got[1].ID, got[2].ID, got[3].ID, got[4].ID})
	// Original slice unchanged.
	require.Equal(t, "VULN-001", in[0].ID)
}
