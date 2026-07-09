package phase2

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
)

func TestCategoryArtifactStore_ConcurrentObservationsAndMerge(t *testing.T) {
	state := model.NewAuditState()
	store := newCategoryArtifactStore(state)
	dir := t.TempDir()

	var wg sync.WaitGroup
	categories := []string{"sql_injection", "cmd_injection", "xss_injection"}
	for _, cat := range categories {
		wg.Add(1)
		go func(category string) {
			defer wg.Done()
			for i := 0; i < 5; i++ {
				state.AddFinding(&model.Finding{
					Category: category,
					Title:    "f",
					File:     "a.go",
					Line:     i + 1,
					Severity: "LOW",
				})
			}
			obs := &model.ScanObservation{
				CategoryID:      category,
				CategoryName:    category,
				StopReason:      "test",
				CoverageSummary: "ok",
			}
			require.NoError(t, store.PersistCategoryObservation(dir, category, obs))
		}(cat)
	}
	wg.Wait()

	require.NoError(t, store.MergeAll(dir))

	for _, cat := range categories {
		data, err := os.ReadFile(filepath.Join(dir, "scan_obs_"+cat+".json"))
		require.NoError(t, err)
		var obs model.ScanObservation
		require.NoError(t, json.Unmarshal(data, &obs))
		require.Equal(t, cat, obs.CategoryID)
	}

	mergedPath := filepath.Join(dir, "scan_findings.json")
	mergedData, err := os.ReadFile(mergedPath)
	require.NoError(t, err)
	var merged []*model.Finding
	require.NoError(t, json.Unmarshal(mergedData, &merged))
	require.Len(t, merged, 15)
}

func TestAuditState_AddScanObservation_DedupesCategory(t *testing.T) {
	state := model.NewAuditState()
	state.AddScanObservation(&model.ScanObservation{CategoryID: "sql_injection", CategoryName: "SQL"})
	state.AddScanObservation(&model.ScanObservation{CategoryID: "sql_injection", CategoryName: "SQL"})
	state.AddScanObservation(&model.ScanObservation{CategoryID: "cmd_injection", CategoryName: "CMD"})
	require.Len(t, state.GetScanObservations(), 2)
}
