package syntaxflow_scan

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func batchRisk(hash, feature, mode, title string) *schema.SSARisk {
	return &schema.SSARisk{
		Hash:            hash,
		RiskFeatureHash: feature,
		ScanMode:        mode,
		FromRule:        "batch-rule",
		Title:           title,
		TitleVerbose:    title,
		RiskType:        "custom",
		Severity:        schema.SFR_SEVERITY_HIGH,
		ProgramName:     "batch-program",
		RuntimeId:       "batch-runtime",
		CodeSourceUrl:   "file:///main.go",
		CodeRange:       "1:1-1:2",
	}
}

func countBatchedRisks(t *testing.T, runtimeID string) int64 {
	t.Helper()
	var count int64
	require.NoError(t, ssadb.GetDB().Model(&schema.SSARisk{}).
		Where("runtime_id = ?", runtimeID).Count(&count).Error)
	return count
}

// TestDBSaver_FlushBatchesCreatesAndRewrites covers the batched path: findings
// accumulate in the saver and reach the database when the batch is flushed,
// with a covered finding rewriting its row instead of inserting a new one.
func TestDBSaver_FlushBatchesCreatesAndRewrites(t *testing.T) {
	t.Cleanup(func() {
		ssadb.GetDB().Where("runtime_id = ?", "batch-runtime").Delete(&schema.SSARisk{})
	})

	saver := newDBSaver(schema.SFResultKindScan, "batch-task", false)
	first := batchRisk("batch-hash-1", "batch-feature-1", string(schema.SFR_MODE_SOURCE), "first")
	second := batchRisk("batch-hash-2", "batch-feature-2", string(schema.SFR_MODE_SSA), "second")
	require.NoError(t, saver.ApplyRiskUpdate(schema.RiskUpdateItem{Risk: first}))
	require.NoError(t, saver.ApplyRiskUpdate(schema.RiskUpdateItem{Risk: second}))
	require.Zero(t, countBatchedRisks(t, "batch-runtime"), "nothing is written before the batch is flushed")

	require.NoError(t, saver.Flush())
	require.Equal(t, int64(2), countBatchedRisks(t, "batch-runtime"))

	// A later mode covers the first finding: the same row is rewritten.
	covered := batchRisk("batch-hash-1-ssa", "batch-feature-1", string(schema.SFR_MODE_SSA), "covered")
	require.NoError(t, saver.ApplyRiskUpdate(schema.RiskUpdateItem{
		Risk:    covered,
		OldID:   first.ID,
		OldHash: first.Hash,
	}))
	require.NoError(t, saver.Flush())
	require.Equal(t, int64(2), countBatchedRisks(t, "batch-runtime"), "a cover rewrites instead of inserting")

	var stored schema.SSARisk
	require.NoError(t, ssadb.GetDB().Where("id = ?", first.ID).First(&stored).Error)
	require.Equal(t, string(schema.SFR_MODE_SSA), stored.ScanMode)
	require.Equal(t, covered.Hash, stored.Hash)
}
