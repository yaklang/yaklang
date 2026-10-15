package ssaapi

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func batchRisk(hash, feature, mode, title string) *schema.SSARisk {
	risk := &schema.SSARisk{
		ResultUUID:      hash,
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
	risk.PrepareForSave()
	return risk
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
		// Unscoped: a soft-deleted row still occupies the unique index on
		// hash, and this test writes the same synthetic rows on every run.
		ssadb.GetDB().Unscoped().Where("runtime_id = ?", "batch-runtime").Delete(&schema.SSARisk{})
	})

	saver := NewRiskDBSaver()
	t.Cleanup(func() { _ = saver.Close() })
	first := batchRisk("batch-hash-1", "batch-feature-1", string(schema.SFR_MODE_SOURCE), "first")
	second := batchRisk("batch-hash-2", "batch-feature-2", string(schema.SFR_MODE_SSA), "second")
	require.NoError(t, saver.ApplyRiskUpdate(schema.RiskUpdateItem{Mode: schema.RiskUpdateCreate, Risk: first}))
	require.NoError(t, saver.ApplyRiskUpdate(schema.RiskUpdateItem{Mode: schema.RiskUpdateCreate, Risk: second}))
	require.Zero(t, countBatchedRisks(t, "batch-runtime"), "nothing is written before the batch is flushed")

	require.NoError(t, saver.Flush())
	require.Equal(t, int64(2), countBatchedRisks(t, "batch-runtime"))
	var original schema.SSARisk
	require.NoError(t, ssadb.GetDB().Where("hash = ?", first.Hash).First(&original).Error)

	// A later mode covers the first finding: the same row is rewritten.
	covered := batchRisk("batch-hash-1-ssa", "batch-feature-1", string(schema.SFR_MODE_SSA), "covered")
	require.NoError(t, saver.ApplyRiskUpdate(schema.RiskUpdateItem{
		Mode:    schema.RiskUpdateUpdate,
		Risk:    covered,
		OldHash: first.Hash,
	}))
	require.NoError(t, saver.Flush())
	require.Equal(t, int64(2), countBatchedRisks(t, "batch-runtime"), "a cover rewrites instead of inserting")

	var stored schema.SSARisk
	require.NoError(t, ssadb.GetDB().Where("id = ?", original.ID).First(&stored).Error)
	require.Equal(t, string(schema.SFR_MODE_SSA), stored.ScanMode)
	require.Equal(t, covered.Hash, stored.Hash)
}

// TestDBSaver_CoverBeforeFlushInsertsOnce covers the common case: the earlier
// mode is still waiting in the batch when the later mode reports the same
// finding, so the superseded row never reaches the database at all.
func TestDBSaver_CoverBeforeFlushInsertsOnce(t *testing.T) {
	t.Cleanup(func() {
		// Unscoped: a soft-deleted row still occupies the unique index on
		// hash, and this test writes the same synthetic rows on every run.
		ssadb.GetDB().Unscoped().Where("runtime_id = ?", "batch-runtime").Delete(&schema.SSARisk{})
	})

	saver := NewRiskDBSaver()
	t.Cleanup(func() { _ = saver.Close() })

	source := batchRisk("pending-source-hash", "pending-feature", string(schema.SFR_MODE_SOURCE), "source")
	require.NoError(t, saver.ApplyRiskUpdate(schema.RiskUpdateItem{Mode: schema.RiskUpdateCreate, Risk: source}))

	ssa := batchRisk("pending-ssa-hash", "pending-feature", string(schema.SFR_MODE_SSA), "ssa")
	// The source row is still only queued. The runtime still says update,
	// because the feature hash is already in this task.
	require.NoError(t, saver.ApplyRiskUpdate(schema.RiskUpdateItem{
		Mode:    schema.RiskUpdateUpdate,
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

func TestSaveRisk_CreateThenSaveDoesNotUpdateIDZero(t *testing.T) {
	const runtimeID = "save-risk-create-then-save"
	t.Cleanup(func() {
		ssadb.GetDB().Unscoped().Where("runtime_id = ?", runtimeID).Delete(&schema.SSARisk{})
	})
	risk := batchRisk("create-then-save", "create-then-save-feature", string(schema.SFR_MODE_SSA), "created")
	risk.RuntimeId = runtimeID
	risk.PrepareForSave()
	require.Zero(t, risk.ID)

	require.NoError(t, SaveRisk(ssadb.GetDB(), risk))
	require.NotZero(t, risk.ID)
	id := risk.ID
	risk.Description = "saved again"
	require.NoError(t, SaveRisk(ssadb.GetDB(), risk))

	require.Equal(t, id, risk.ID, "saving a created row must update it, not insert id 0")
	var n int64
	require.NoError(t, ssadb.GetDB().Model(&schema.SSARisk{}).Where("runtime_id = ?", runtimeID).Count(&n).Error)
	require.Equal(t, int64(1), n)
	require.NoError(t, ssadb.GetDB().Model(&schema.SSARisk{}).Where("id = ?", 0).Count(&n).Error)
	require.Zero(t, n)
	var stored schema.SSARisk
	require.NoError(t, ssadb.GetDB().Where("id = ?", id).First(&stored).Error)
	require.Equal(t, "saved again", stored.Description)
}

func TestSaveRiskUpdates_UpdateRewritesTheRiskID(t *testing.T) {
	const runtimeID = "update-by-risk-id"
	t.Cleanup(func() {
		ssadb.GetDB().Unscoped().Where("runtime_id = ?", runtimeID).Delete(&schema.SSARisk{})
	})
	saved := batchRisk("update-id-old", "update-id-feature", string(schema.SFR_MODE_SOURCE), "old")
	saved.RuntimeId = runtimeID
	saved.PrepareForSave()
	require.NoError(t, SaveRisk(ssadb.GetDB(), saved))
	require.NotZero(t, saved.ID)

	next := batchRisk("update-id-new", "update-id-feature", string(schema.SFR_MODE_SSA), "new")
	next.RuntimeId = runtimeID
	next.PrepareForSave()
	require.Zero(t, next.ID)
	require.NoError(t, SaveRiskUpdates(ssadb.GetDB(), schema.RiskUpdateItem{
		Mode:    schema.RiskUpdateUpdate,
		Risk:    next,
		OldHash: saved.Hash,
	}))

	var n int64
	require.NoError(t, ssadb.GetDB().Model(&schema.SSARisk{}).Where("runtime_id = ?", runtimeID).Count(&n).Error)
	require.Equal(t, int64(1), n)
	var stored schema.SSARisk
	require.NoError(t, ssadb.GetDB().Where("id = ?", saved.ID).First(&stored).Error)
	require.Equal(t, next.Hash, stored.Hash)
	require.Equal(t, string(schema.SFR_MODE_SSA), stored.ScanMode)
}

func TestSaveRiskUpdates_KeepsDisposalAndOtherTasks(t *testing.T) {
	const runtimeID = "cover-keeps-disposal"
	const otherID = "cover-other-task"
	const feature = "cover-disposal-feature"
	t.Cleanup(func() {
		ssadb.GetDB().Unscoped().Where("runtime_id IN (?)", []string{runtimeID, otherID}).Delete(&schema.SSARisk{})
	})
	saved := batchRisk("disposal-old", feature, string(schema.SFR_MODE_SOURCE), "old")
	saved.RuntimeId = runtimeID
	saved.Description = "old description"
	saved.PrepareForSave()
	require.NoError(t, SaveRisk(ssadb.GetDB(), saved))
	require.NoError(t, ssadb.GetDB().Model(&schema.SSARisk{}).Where("id = ?", saved.ID).UpdateColumns(map[string]interface{}{
		"latest_disposal_status": "is_issue",
		"is_read":                true,
		"ignore":                 true,
		"tags":                   "kept",
	}).Error)
	var before schema.SSARisk
	require.NoError(t, ssadb.GetDB().Where("id = ?", saved.ID).First(&before).Error)

	other := batchRisk("disposal-other", feature, string(schema.SFR_MODE_SOURCE), "other")
	other.RuntimeId = otherID
	other.PrepareForSave()
	require.NoError(t, SaveRisk(ssadb.GetDB(), other))

	next := batchRisk("disposal-new", feature, string(schema.SFR_MODE_SSA), "new")
	next.RuntimeId = runtimeID
	next.IsRead = false
	next.Ignore = false
	next.Tags = ""
	next.LatestDisposalStatus = ""
	next.PrepareForSave()
	require.NoError(t, SaveRiskUpdates(ssadb.GetDB(), schema.RiskUpdateItem{
		Mode: schema.RiskUpdateUpdate, Risk: next, OldHash: saved.Hash,
	}))

	var stored schema.SSARisk
	require.NoError(t, ssadb.GetDB().Where("runtime_id = ? AND risk_feature_hash = ?", runtimeID, feature).First(&stored).Error)
	require.Equal(t, before.ID, stored.ID)
	require.Equal(t, before.CreatedAt.Unix(), stored.CreatedAt.Unix())
	require.Equal(t, "is_issue", stored.LatestDisposalStatus)
	require.True(t, stored.IsRead)
	require.True(t, stored.Ignore)
	require.Equal(t, "kept", stored.Tags)
	require.Equal(t, next.Hash, stored.Hash)
	require.Equal(t, "new", stored.Title)
	require.Empty(t, stored.Description, "a cover writes zero values of the new finding")

	var untouched schema.SSARisk
	require.NoError(t, ssadb.GetDB().Where("runtime_id = ?", otherID).First(&untouched).Error)
	require.Equal(t, "other", untouched.Title)
	require.Equal(t, other.Hash, untouched.Hash)
}

func TestSaveRiskUpdates_DeleteAndEmptyFeature(t *testing.T) {
	const runtimeID = "cover-delete-task"
	const otherID = "cover-delete-other"
	t.Cleanup(func() {
		ssadb.GetDB().Unscoped().Where("runtime_id IN (?)", []string{runtimeID, otherID}).Delete(&schema.SSARisk{})
	})
	kept := batchRisk("delete-other", "delete-feature", string(schema.SFR_MODE_SSA), "kept")
	kept.RuntimeId = otherID
	kept.PrepareForSave()
	gone := batchRisk("delete-this", "delete-feature", string(schema.SFR_MODE_SSA), "gone")
	gone.RuntimeId = runtimeID
	gone.PrepareForSave()
	emptyA := batchRisk("empty-a", "", string(schema.SFR_MODE_SSA), "a")
	emptyA.RuntimeId = runtimeID
	emptyA.RiskFeatureHash = ""
	emptyA.PrepareForSave()
	emptyB := batchRisk("empty-b", "", string(schema.SFR_MODE_SSA), "b")
	emptyB.RuntimeId = runtimeID
	emptyB.RiskFeatureHash = ""
	emptyB.PrepareForSave()
	require.NoError(t, SaveRiskUpdates(ssadb.GetDB(),
		schema.RiskUpdateItem{Mode: schema.RiskUpdateCreate, Risk: kept},
		schema.RiskUpdateItem{Mode: schema.RiskUpdateCreate, Risk: gone},
		schema.RiskUpdateItem{Mode: schema.RiskUpdateCreate, Risk: emptyA},
		schema.RiskUpdateItem{Mode: schema.RiskUpdateCreate, Risk: emptyB},
	))
	require.NoError(t, SaveRiskUpdates(ssadb.GetDB(), schema.RiskUpdateItem{Mode: schema.RiskUpdateDelete, Risk: gone}))

	var n int64
	require.NoError(t, ssadb.GetDB().Model(&schema.SSARisk{}).Where("runtime_id = ? AND risk_feature_hash = ?", runtimeID, "delete-feature").Count(&n).Error)
	require.Zero(t, n)
	require.NoError(t, ssadb.GetDB().Model(&schema.SSARisk{}).Where("runtime_id = ?", otherID).Count(&n).Error)
	require.Equal(t, int64(1), n)
	require.NoError(t, ssadb.GetDB().Model(&schema.SSARisk{}).Where("runtime_id = ? AND risk_feature_hash = ?", runtimeID, "").Count(&n).Error)
	require.Equal(t, int64(2), n, "an empty feature hash is always inserted")

	missing := batchRisk("no-runtime", "delete-feature", string(schema.SFR_MODE_SSA), "x")
	missing.RuntimeId = ""
	missing.PrepareForSave()
	require.Error(t, SaveRiskUpdates(ssadb.GetDB(), schema.RiskUpdateItem{Mode: schema.RiskUpdateUpdate, Risk: missing}))
}

func TestRiskDBSaver_SubmitDoesNotWaitForAudit(t *testing.T) {
	const runtimeID = "audit-does-not-block"
	t.Cleanup(func() {
		ssadb.GetDB().Unscoped().Where("runtime_id = ?", runtimeID).Delete(&schema.SSARisk{})
	})
	saver := NewRiskDBSaver()
	t.Cleanup(func() { _ = saver.Close() })
	risk := batchRisk("audit-block", "audit-feature", string(schema.SFR_MODE_SSA), "audit")
	risk.RuntimeId = runtimeID
	risk.PrepareForSave()
	started := make(chan struct{})
	release := make(chan struct{})
	require.NoError(t, saver.ApplyRiskUpdate(schema.RiskUpdateItem{
		Mode: schema.RiskUpdateCreate,
		Risk: risk,
		Audit: []func(*gorm.DB) error{func(*gorm.DB) error {
			close(started)
			<-release
			return nil
		}},
	}))
	select {
	case <-started:
		t.Fatal("queueing a risk ran its audit")
	case <-time.After(30 * time.Millisecond):
	}
	flushDone := make(chan error, 1)
	go func() { flushDone <- saver.Flush() }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("flush did not start the audit save")
	}
	select {
	case <-flushDone:
		t.Fatal("flush returned before the audit save finished")
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-flushDone)
}

func TestSaveRiskUpdates_AuditErrorRollsBackTheRisk(t *testing.T) {
	const runtimeID = "audit-rolls-back-risk"
	t.Cleanup(func() {
		ssadb.GetDB().Unscoped().Where("runtime_id = ?", runtimeID).Delete(&schema.SSARisk{})
	})
	risk := batchRisk("audit-rollback", "audit-rollback-feature", string(schema.SFR_MODE_SSA), "rollback")
	risk.RuntimeId = runtimeID
	risk.Description = ""
	risk.PrepareForSave()
	err := SaveRiskUpdates(ssadb.GetDB(), schema.RiskUpdateItem{
		Mode: schema.RiskUpdateCreate,
		Risk: risk,
		Audit: []func(*gorm.DB) error{func(*gorm.DB) error {
			return utils.Error("audit failed")
		}},
	})
	require.Error(t, err)
	var n int64
	require.NoError(t, ssadb.GetDB().Model(&schema.SSARisk{}).Where("runtime_id = ?", runtimeID).Count(&n).Error)
	require.Zero(t, n, "a failed audit writer rolls the risk insert back with it")
}
