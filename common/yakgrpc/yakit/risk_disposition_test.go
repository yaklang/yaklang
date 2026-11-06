package yakit

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
)

func openRiskDispositionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "risk-disposition.db"))
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.Risk{}).Error)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestUpdateRiskDispositionsPreservesRecords(t *testing.T) {
	db := openRiskDispositionTestDB(t)
	canonical := &schema.Risk{Hash: "canonical", Title: "Canonical", RuntimeId: "runtime-a"}
	duplicate := &schema.Risk{Hash: "duplicate", Title: "Duplicate", RuntimeId: "runtime-a"}
	irrelevant := &schema.Risk{Hash: "irrelevant", Title: "Irrelevant", RuntimeId: "runtime-a"}
	require.NoError(t, db.Create(canonical).Error)
	require.NoError(t, db.Create(duplicate).Error)
	require.NoError(t, db.Create(irrelevant).Error)

	result, err := UpdateRiskDispositions(
		db,
		[]int64{int64(duplicate.ID), int64(irrelevant.ID)},
		schema.RiskDispositionFalsePositive,
		0,
		"observed behavior is expected",
	)
	require.NoError(t, err)
	require.Equal(t, 2, result.UpdatedCount)
	require.Empty(t, result.MissingIDs)

	var storedDuplicate schema.Risk
	require.NoError(t, db.First(&storedDuplicate, duplicate.ID).Error)
	require.Equal(t, schema.RiskDispositionFalsePositive, storedDuplicate.Disposition)
	require.Equal(t, "observed behavior is expected", storedDuplicate.TagReason)
	require.Equal(t, "Duplicate", storedDuplicate.Title)

	var count int64
	require.NoError(t, db.Model(&schema.Risk{}).Count(&count).Error)
	require.EqualValues(t, 3, count, "disposition changes must never delete records")

	result, err = UpdateRiskDispositions(
		db,
		[]int64{int64(duplicate.ID), int64(irrelevant.ID)},
		schema.RiskDispositionFalsePositive,
		0,
		"observed behavior is expected",
	)
	require.NoError(t, err)
	require.Zero(t, result.UpdatedCount)
	require.Equal(t, 2, result.UnchangedCount)

	result, err = UpdateRiskDispositions(db, []int64{int64(duplicate.ID)}, schema.RiskDispositionActive, 0, "")
	require.NoError(t, err)
	require.Equal(t, 1, result.UpdatedCount)
	var restored schema.Risk
	require.NoError(t, db.First(&restored, duplicate.ID).Error)
	require.Equal(t, schema.RiskDispositionActive, restored.Disposition)
	require.Zero(t, restored.DuplicateOfRiskID)
	require.Empty(t, restored.TagReason)
}

func TestUpdateRiskDispositionsValidatesDuplicateTarget(t *testing.T) {
	db := openRiskDispositionTestDB(t)
	canonical := &schema.Risk{Hash: "canonical", Title: "Canonical"}
	duplicate := &schema.Risk{Hash: "duplicate", Title: "Duplicate"}
	require.NoError(t, db.Create(canonical).Error)
	require.NoError(t, db.Create(duplicate).Error)

	_, err := UpdateRiskDispositions(
		db,
		[]int64{int64(duplicate.ID)},
		schema.RiskDispositionDuplicate,
		int64(duplicate.ID),
		"same logical issue",
	)
	require.ErrorContains(t, err, "cannot be one of the risks")

	_, err = UpdateRiskDispositions(
		db,
		[]int64{int64(duplicate.ID)},
		schema.RiskDispositionDuplicate,
		int64(canonical.ID),
		"same logical issue",
	)
	require.NoError(t, err)
	var stored schema.Risk
	require.NoError(t, db.First(&stored, duplicate.ID).Error)
	require.Equal(t, schema.RiskDispositionDuplicate, stored.Disposition)
	require.Equal(t, canonical.ID, stored.DuplicateOfRiskID)

	third := &schema.Risk{Hash: "third", Title: "Third"}
	require.NoError(t, db.Create(third).Error)
	_, err = UpdateRiskDispositions(
		db,
		[]int64{int64(third.ID)},
		schema.RiskDispositionDuplicate,
		int64(duplicate.ID),
		"duplicate chain",
	)
	require.ErrorContains(t, err, "must be active")
}

func TestUpdateRiskDispositionsReportsMissingIDs(t *testing.T) {
	db := openRiskDispositionTestDB(t)
	risk := &schema.Risk{Hash: "present", Title: "Present"}
	require.NoError(t, db.Create(risk).Error)

	result, err := UpdateRiskDispositions(
		db,
		[]int64{int64(risk.ID), 999999},
		schema.RiskDispositionIgnored,
		0,
		"out of scope for this review",
	)
	require.NoError(t, err)
	require.Equal(t, 1, result.UpdatedCount)
	require.Equal(t, []int64{999999}, result.MissingIDs)
}

func TestRiskRediscoveryPreservesDisposition(t *testing.T) {
	db := openRiskDispositionTestDB(t)
	risk := &schema.Risk{
		Hash:        "rediscovered",
		Title:       "Initial",
		RiskType:    "xss",
		Url:         "https://example.test/login",
		Disposition: schema.RiskDispositionIgnored,
		TagReason:   "known false positive",
	}
	require.NoError(t, db.Create(risk).Error)

	updated := &schema.Risk{
		Hash:        "rediscovered",
		Title:       "Updated evidence",
		RiskType:    "xss",
		Url:         "https://example.test/login",
		Disposition: schema.RiskDispositionActive,
		TagReason:   "",
	}
	require.NoError(t, CreateOrUpdateRisk(db, updated.Hash, updated))

	var stored schema.Risk
	require.NoError(t, db.First(&stored, risk.ID).Error)
	require.Equal(t, "Updated evidence", stored.Title)
	require.Equal(t, schema.RiskDispositionIgnored, stored.Disposition)
	require.Equal(t, "known false positive", stored.TagReason)

	fresh := &schema.Risk{
		Hash:      "fresh",
		Title:     "Fresh",
		RiskType:  "info",
		TagReason: "imported disposition note",
	}
	require.NoError(t, CreateOrUpdateRisk(db, fresh.Hash, fresh))
	var storedFresh schema.Risk
	require.NoError(t, db.Where("hash = ?", fresh.Hash).First(&storedFresh).Error)
	require.Equal(t, "imported disposition note", storedFresh.TagReason)
}

func TestRiskDispositionSchemaValidation(t *testing.T) {
	db := openRiskDispositionTestDB(t)
	require.Error(t, db.Create(&schema.Risk{Hash: "invalid", Disposition: "deleted"}).Error)
	require.Error(t, db.Create(&schema.Risk{Hash: "duplicate-without-target", Disposition: schema.RiskDispositionDuplicate}).Error)
}
