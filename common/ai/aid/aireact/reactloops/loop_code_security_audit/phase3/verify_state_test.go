package phase3

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
)

func sampleFindings() []*model.Finding {
	return []*model.Finding{
		{ID: "VULN-001", Title: "SQL A", File: "a.php", Line: 1, Severity: "HIGH"},
		{ID: "VULN-002", Title: "SQL B", File: "b.php", Line: 2, Severity: "MEDIUM"},
		{ID: "VULN-003", Title: "XSS C", File: "c.php", Line: 3, Severity: "LOW"},
	}
}

func TestVerifyState_SequentialConclude(t *testing.T) {
	verify := newVerifyState(sampleFindings())
	require.Equal(t, "VULN-001", verify.CurrentFindingID())

	ok, _ := verify.CanConclude("VULN-002")
	require.False(t, ok)

	ok, msg := verify.CanConclude("VULN-001")
	require.True(t, ok, msg)
	verify.MarkConcluded("VULN-001")
	require.Equal(t, "VULN-002", verify.CurrentFindingID())

	ok, _ = verify.CanConclude("VULN-001")
	require.False(t, ok)

	ok, _ = verify.CanConclude("VULN-003")
	require.False(t, ok)
}

func TestVerifyState_AllDoneAndDuplicate(t *testing.T) {
	verify := newVerifyState(sampleFindings())
	verify.MarkConcluded("VULN-001")
	verify.MarkConcluded("VULN-002")
	verify.MarkConcluded("VULN-003")
	require.True(t, verify.AllDone())
	require.Equal(t, "", verify.CurrentFindingID())

	ok, _ := verify.CanConclude("VULN-002")
	require.False(t, ok)
}

func TestVerifyState_SyncFromVerified(t *testing.T) {
	verify := newVerifyState(sampleFindings())
	verify.SyncFromVerified([]*model.VerifiedFinding{
		{Finding: &model.Finding{ID: "VULN-001"}},
	})
	require.Equal(t, "VULN-002", verify.CurrentFindingID())
	require.Equal(t, 1, verify.ConcludedCount())
}
