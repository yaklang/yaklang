package yakit

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func newProjectKVTestDB(tb testing.TB) *gorm.DB {
	tb.Helper()
	db, err := gorm.Open("sqlite3", filepath.Join(tb.TempDir(), "project.db"))
	require.NoError(tb, err)
	require.NoError(tb, db.AutoMigrate(&schema.ProjectGeneralStorage{}).Error)
	db.DB().SetMaxOpenConns(1)
	tb.Cleanup(func() { _ = db.Close() })
	return db
}

func TestSetProjectKeyWithGroupSQLiteUpsertPreservesUntouchedFields(t *testing.T) {
	db := newProjectKVTestDB(t)
	key := "project-kv-upsert"
	keyStr := strconv.Quote(key)
	expiresAt := time.Now().Add(time.Hour).Truncate(time.Second)
	deletedAt := time.Now().Add(-time.Hour).Truncate(time.Second)
	original := &schema.ProjectGeneralStorage{
		Key:        keyStr,
		Value:      strconv.Quote("old-value"),
		Group:      "old-group",
		ExpiredAt:  expiresAt,
		ProcessEnv: true,
		Verbose:    "preserve-me",
	}
	original.DeletedAt = &deletedAt
	require.NoError(t, db.Create(original).Error)

	require.NoError(t, SetProjectKeyWithGroup(db, key, []byte("new-value"), BARE_RESPONSE_GROUP))

	var got schema.ProjectGeneralStorage
	require.NoError(t, db.Unscoped().Where("key = ?", keyStr).First(&got).Error)
	require.Equal(t, original.ID, got.ID)
	require.Equal(t, original.CreatedAt.UnixNano(), got.CreatedAt.UnixNano())
	require.Equal(t, strconv.Quote("new-value"), got.Value)
	require.Equal(t, BARE_RESPONSE_GROUP, got.Group)
	require.Equal(t, expiresAt.UnixNano(), got.ExpiredAt.UnixNano())
	require.NotNil(t, got.DeletedAt)
	require.Equal(t, deletedAt.UnixNano(), got.DeletedAt.UnixNano())
	require.True(t, got.ProcessEnv)
	require.Equal(t, "preserve-me", got.Verbose)

	var count int
	require.NoError(t, db.Unscoped().Model(&schema.ProjectGeneralStorage{}).Where("key = ?", keyStr).Count(&count).Error)
	require.Equal(t, 1, count)
}

func TestSetProjectKeyWithGroupSQLiteUpsertCreatesReadableValue(t *testing.T) {
	db := newProjectKVTestDB(t)
	require.NoError(t, SetProjectKeyWithGroup(db, "fresh-key", []byte("fresh-value"), BARE_RESPONSE_GROUP))
	require.Equal(t, "fresh-value", GetProjectKey(db, "fresh-key"))

	var got schema.ProjectGeneralStorage
	require.NoError(t, db.Where("key = ?", strconv.Quote("fresh-key")).First(&got).Error)
	require.False(t, got.CreatedAt.IsZero())
	require.Equal(t, got.CreatedAt.UnixNano(), got.UpdatedAt.UnixNano())
	require.Nil(t, got.DeletedAt)
	require.True(t, got.ExpiredAt.IsZero())
	require.False(t, got.ProcessEnv)
	require.Empty(t, got.Verbose)
}

func TestSetProjectKeyWithGroupSQLiteUpsertUsesOuterTransaction(t *testing.T) {
	db := newProjectKVTestDB(t)
	tx := db.Begin()
	require.NoError(t, tx.Error)
	require.NoError(t, SetProjectKeyWithGroup(tx, "rolled-back-key", "value", BARE_REQUEST_GROUP))
	require.NoError(t, tx.Rollback().Error)

	var count int
	require.NoError(t, db.Unscoped().Model(&schema.ProjectGeneralStorage{}).
		Where("key = ?", strconv.Quote("rolled-back-key")).Count(&count).Error)
	require.Zero(t, count)
}

func legacySetProjectKeyWithGroupForBenchmark(db *gorm.DB, key, value, group string) error {
	keyStr := strconv.Quote(key)
	valueStr := ""
	if value != "" {
		valueStr = strconv.Quote(value)
	}
	if result := db.Model(&schema.ProjectGeneralStorage{}).Where(`key = ?`, keyStr).Assign(map[string]interface{}{
		"key": keyStr, "value": valueStr, "group": group,
	}).FirstOrCreate(&schema.ProjectGeneralStorage{}); result.Error != nil {
		return utils.Errorf("create project storage kv failed: %s", result.Error)
	}
	return nil
}

func gormUpsertProjectKeyWithGroupForBenchmark(db *gorm.DB, key, value, group string) error {
	keyStr := strconv.Quote(key)
	valueStr := ""
	if value != "" {
		valueStr = strconv.Quote(value)
	}
	storage := &schema.ProjectGeneralStorage{Key: keyStr, Value: valueStr, Group: group}
	if result := db.Model(&schema.ProjectGeneralStorage{}).
		OnConflictDoUpdate(`"key"`, `"value"`, `"group"`, `"updated_at"`).
		Create(storage); result.Error != nil {
		return utils.Errorf("upsert project storage kv failed: %s", result.Error)
	}
	return nil
}

func BenchmarkSetProjectKeyWithGroupUniqueKeys(b *testing.B) {
	payload := strings.Repeat("wire-response", 27)
	benchmark := func(b *testing.B, setter func(*gorm.DB, string, string, string) error) {
		db := newProjectKVTestDB(b)
		tx := db.Begin()
		require.NoError(b, tx.Error)
		b.Cleanup(func() { tx.Rollback() })
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := setter(tx, "bare-response-"+strconv.Itoa(i), payload, BARE_RESPONSE_GROUP); err != nil {
				b.Fatal(err)
			}
		}
	}

	b.Run("LegacyFirstOrCreate", func(b *testing.B) {
		benchmark(b, legacySetProjectKeyWithGroupForBenchmark)
	})
	b.Run("SQLiteGORMUpsert", func(b *testing.B) {
		benchmark(b, gormUpsertProjectKeyWithGroupForBenchmark)
	})
	b.Run("SQLiteDirectUpsert", func(b *testing.B) {
		benchmark(b, func(db *gorm.DB, key, value, group string) error {
			return SetProjectKeyWithGroup(db, key, value, group)
		})
	})
}
