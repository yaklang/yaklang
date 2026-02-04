package bizhelper

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type ftsDoc struct {
	ID    int64  `gorm:"primary_key;column:id"`
	Title string `gorm:"column:title"`
	Body  string `gorm:"column:body"`
}

func (ftsDoc) TableName() string { return "test_fts_docs" }

func TestSQLiteFTS5SetupAndBM25Match(t *testing.T) {
	db, err := createTempTestDatabase()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, db.AutoMigrate(&ftsDoc{}).Error)

	// Seed base table.
	d1 := &ftsDoc{Title: "hello world", Body: "yaklang is great"}
	d2 := &ftsDoc{Title: "other", Body: "nothing to see here"}
	require.NoError(t, db.Create(d1).Error)
	require.NoError(t, db.Create(d2).Error)

	cfg := &SQLiteFTS5Config{
		BaseModel: &ftsDoc{},
		FTSTable:  "test_fts_docs_fts",
		Columns:   []string{"title", "body"},
	}

	// FTS5 might not be available in some build environments; skip in that case.
	if err := SQLiteFTS5Setup(db, cfg); err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			t.Skipf("fts5 not available: %v", err)
		}
		require.NoError(t, err)
	}

	// Search should return the original struct(s) from base table.
	{
		got, err := SQLiteFTS5BM25Match[ftsDoc](db, cfg, "yaklang", 10, 0)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, d1.ID, got[0].ID)
		require.Equal(t, d1.Title, got[0].Title)
	}

	// Update should be reflected via triggers.
	require.NoError(t, db.Model(&ftsDoc{}).Where("id = ?", d1.ID).Update("body", "sqlite fts5 works").Error)
	{
		got, err := SQLiteFTS5BM25Match[ftsDoc](db, cfg, "yaklang", 10, 0)
		require.NoError(t, err)
		require.Len(t, got, 0)
	}
	{
		got, err := SQLiteFTS5BM25Match[ftsDoc](db, cfg, "fts5", 10, 0)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, d1.ID, got[0].ID)
	}

	// Delete should be reflected via triggers.
	require.NoError(t, db.Delete(&ftsDoc{}, d1.ID).Error)
	{
		got, err := SQLiteFTS5BM25Match[ftsDoc](db, cfg, "fts5", 10, 0)
		require.NoError(t, err)
		require.Len(t, got, 0)
	}
}

func TestSQLiteFTS5Config_BaseTableFromModel(t *testing.T) {
	db, err := createTempTestDatabase()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, db.AutoMigrate(&ftsDoc{}).Error)

	cfg := &SQLiteFTS5Config{
		BaseModel: &ftsDoc{},
		FTSTable:  "test_fts_docs_fts2",
		Columns:   []string{"title"},
	}

	err = SQLiteFTS5CreateVirtualTable(db, cfg)
	if err != nil && strings.Contains(err.Error(), "no such module: fts5") {
		t.Skipf("fts5 not available: %v", err)
	}
	require.NoError(t, err)

	// Sanity: raw query should not error.
	var row struct {
		Count int `gorm:"column:count"`
	}
	require.NoError(t, db.Raw(`SELECT count(*) AS count FROM "`+cfg.FTSTable+`"`).Scan(&row).Error)
}

func TestSQLiteFTS5BM25Match_PreservesCallerFilters(t *testing.T) {
	db, err := createTempTestDatabase()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, db.AutoMigrate(&ftsDoc{}).Error)

	d1 := &ftsDoc{Title: "hello world", Body: "yaklang is great"}
	d2 := &ftsDoc{Title: "other", Body: "nothing to see here"}
	require.NoError(t, db.Create(d1).Error)
	require.NoError(t, db.Create(d2).Error)

	cfg := &SQLiteFTS5Config{
		BaseModel: &ftsDoc{},
		FTSTable:  "test_fts_docs_filter_fts",
		Columns:   []string{"title", "body"},
	}
	if err := SQLiteFTS5Setup(db, cfg); err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			t.Skipf("fts5 not available: %v", err)
		}
		require.NoError(t, err)
	}

	// Caller filter should be preserved (previous Raw() implementation ignored it).
	{
		got, err := SQLiteFTS5BM25Match[ftsDoc](db.Where("id = ?", d2.ID), cfg, "yaklang", 10, 0)
		require.NoError(t, err)
		require.Len(t, got, 0)
	}
	{
		got, err := SQLiteFTS5BM25Match[ftsDoc](db.Where("id = ?", d1.ID), cfg, "yaklang", 10, 0)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, d1.ID, got[0].ID)
	}
}

func TestSQLiteFTS5BM25MatchYield_PreservesCallerFilters(t *testing.T) {
	db, err := createTempTestDatabase()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, db.AutoMigrate(&ftsDoc{}).Error)

	d1 := &ftsDoc{Title: "hello world", Body: "yaklang is great"}
	d2 := &ftsDoc{Title: "other", Body: "nothing to see here"}
	require.NoError(t, db.Create(d1).Error)
	require.NoError(t, db.Create(d2).Error)

	cfg := &SQLiteFTS5Config{
		BaseModel: &ftsDoc{},
		FTSTable:  "test_fts_docs_yield_filter_fts",
		Columns:   []string{"title", "body"},
	}
	if err := SQLiteFTS5Setup(db, cfg); err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			t.Skipf("fts5 not available: %v", err)
		}
		require.NoError(t, err)
	}

	ctx := context.Background()

	{
		var got []ftsDoc
		for item := range SQLiteFTS5BM25MatchYield[ftsDoc](ctx, db.Where("id = ?", d2.ID), cfg, "yaklang", WithYieldModel_PageSize(10)) {
			got = append(got, item)
		}
		require.Len(t, got, 0)
	}
	{
		var got []ftsDoc
		for item := range SQLiteFTS5BM25MatchYield[ftsDoc](ctx, db.Where("id = ?", d1.ID), cfg, "yaklang", WithYieldModel_PageSize(10)) {
			got = append(got, item)
		}
		require.Len(t, got, 1)
		require.Equal(t, d1.ID, got[0].ID)
	}
}

func TestSQLiteFTS5Drop_IdempotentAndCleansArtifacts(t *testing.T) {
	db, err := createTempTestDatabase()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, db.AutoMigrate(&ftsDoc{}).Error)
	require.NoError(t, db.Create(&ftsDoc{Title: "hello", Body: "yaklang"}).Error)

	cfg := &SQLiteFTS5Config{
		BaseModel: &ftsDoc{},
		FTSTable:  "test_fts_docs_drop_fts",
		Columns:   []string{"title", "body"},
	}

	if err := SQLiteFTS5Setup(db, cfg); err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			t.Skipf("fts5 not available: %v", err)
		}
		require.NoError(t, err)
	}

	// Sanity: fts table exists.
	{
		var row struct {
			Count int `gorm:"column:count"`
		}
		require.NoError(t, db.Raw(`SELECT count(*) AS count FROM sqlite_master WHERE type='table' AND name=?;`, cfg.FTSTable).Scan(&row).Error)
		require.Equal(t, 1, row.Count)
	}

	// Drop base table first; triggers will be removed by SQLite automatically.
	db.DropTableIfExists(&ftsDoc{})

	// Drop artifacts should still work and remove the orphan FTS table.
	require.NoError(t, SQLiteFTS5Drop(db, cfg))
	require.NoError(t, SQLiteFTS5Drop(db, cfg)) // idempotent

	{
		var row struct {
			Count int `gorm:"column:count"`
		}
		require.NoError(t, db.Raw(`SELECT count(*) AS count FROM sqlite_master WHERE type='table' AND name=?;`, cfg.FTSTable).Scan(&row).Error)
		require.Equal(t, 0, row.Count)
	}
}
