package phase3

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
)

func TestFindingArtifactStore_ConcurrentVerificationsAndMerge(t *testing.T) {
	state := model.NewAuditState()
	state.AddFinding(&model.Finding{ID: "VULN-001", Title: "A", File: "a.php", Line: 1, Severity: "HIGH"})
	state.AddFinding(&model.Finding{ID: "VULN-002", Title: "B", File: "b.php", Line: 2, Severity: "MEDIUM"})
	state.AddFinding(&model.Finding{ID: "VULN-003", Title: "C", File: "c.php", Line: 3, Severity: "LOW"})

	store := newFindingArtifactStore(state)
	dir := t.TempDir()

	var wg sync.WaitGroup
	ids := []string{"VULN-001", "VULN-002", "VULN-003"}
	for _, id := range ids {
		wg.Add(1)
		go func(findingID string) {
			defer wg.Done()
			f := state.GetFindingByID(findingID)
			require.NotNil(t, f)
			state.UpsertVerifiedFinding(&model.VerifiedFinding{
				Finding:    f,
				Status:     model.VerifyConfirmed,
				Confidence: 8,
				Reason:     "test",
			})
		}(id)
	}
	wg.Wait()

	require.NoError(t, store.MergeAll(dir))

	mergedPath := filepath.Join(dir, "verified_vulns.json")
	mergedData, err := os.ReadFile(mergedPath)
	require.NoError(t, err)
	var merged []*model.VerifiedFinding
	require.NoError(t, json.Unmarshal(mergedData, &merged))
	require.Len(t, merged, 3)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "phase3 should only persist verified_vulns.json")
}
