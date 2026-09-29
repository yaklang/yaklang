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
	t.Cleanup(func() { _ = saver.Close() })
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

// TestDBSaver_CoverBeforeFlushInsertsOnce covers the common case: the earlier
// mode is still waiting in the batch when the later mode reports the same
// finding, so the superseded row never reaches the database at all.
func TestDBSaver_CoverBeforeFlushInsertsOnce(t *testing.T) {
	t.Cleanup(func() {
		ssadb.GetDB().Where("runtime_id = ?", "batch-runtime").Delete(&schema.SSARisk{})
	})

	saver := newDBSaver(schema.SFResultKindScan, "batch-task", false)
	t.Cleanup(func() { _ = saver.Close() })

	source := batchRisk("pending-source-hash", "pending-feature", string(schema.SFR_MODE_SOURCE), "source")
	require.NoError(t, saver.ApplyRiskUpdate(schema.RiskUpdateItem{Risk: source}))

	ssa := batchRisk("pending-ssa-hash", "pending-feature", string(schema.SFR_MODE_SSA), "ssa")
	require.NoError(t, saver.ApplyRiskUpdate(schema.RiskUpdateItem{
		Risk:    ssa,
		OldHash: source.Hash,
	}))
	require.NoError(t, saver.Close())

	require.Equal(t, int64(1), countBatchedRisks(t, "batch-runtime"),
		"the covered finding must not leave a second row behind")
	var stored schema.SSARisk
	require.NoError(t, ssadb.GetDB().Where("runtime_id = ?", "batch-runtime").First(&stored).Error)
	require.Equal(t, ssa.Hash, stored.Hash)
	require.Equal(t, string(schema.SFR_MODE_SSA), stored.ScanMode)
}
