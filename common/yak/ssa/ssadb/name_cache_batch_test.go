package ssadb

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	_ "github.com/yaklang/gorm/dialects/sqlite"
)

func setupNameCacheFileDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	oldDB := GetDB()
	path := filepath.Join(t.TempDir(), "names.db")
	db, err := gorm.Open("sqlite3", path+"?_busy_timeout=5000&_txlock=immediate")
	require.NoError(t, err)
	db.DB().SetMaxOpenConns(4)
	require.NoError(t, db.AutoMigrate(&IrNamePool{}).Error)
	SetDB(db)
	return db, func() {
		SetDB(oldDB)
		_ = db.Close()
	}
}

func TestNameCacheGetIDDoesNotWriteEachName(t *testing.T) {
	db, cleanup := setupNameCacheFileDB(t)
	defer cleanup()

	require.NoError(t, db.Create(&IrNamePool{ProgramName: "prog", Name: "kept"}).Error)
	cache := NewNameCache("prog", true)
	kept := cache.GetID("kept")
	require.NotZero(t, kept)

	ids := make([]int64, 32)
	for i := range ids {
		ids[i] = cache.GetID(fmt.Sprintf("name-%02d", i))
		require.NotZero(t, ids[i])
		require.NotEqual(t, kept, ids[i])
	}

	var rows int64
	require.NoError(t, db.Model(&IrNamePool{}).Where("program_name = ? AND name <> ?", "prog", "kept").Count(&rows).Error)
	require.Zero(t, rows, "a new name must not open its own SQLite transaction inside GetID")

	require.NoError(t, cache.Flush())
	require.NoError(t, db.Model(&IrNamePool{}).Where("program_name = ? AND name <> ?", "prog", "kept").Count(&rows).Error)
	require.Equal(t, int64(32), rows)
	for i, id := range ids {
		var got IrNamePool
		require.NoError(t, db.Where("name_id = ?", id).First(&got).Error)
		require.Equal(t, "prog", got.ProgramName)
		require.Equal(t, fmt.Sprintf("name-%02d", i), got.Name)
		require.Equal(t, id, cache.GetID(got.Name))
	}

	reloaded := NewNameCache("prog", true)
	require.Equal(t, kept, reloaded.GetID("kept"))
	require.Equal(t, ids[0], reloaded.GetID("name-00"))
	require.NoError(t, reloaded.Flush())
	require.NoError(t, db.Model(&IrNamePool{}).Where("program_name = ?", "prog").Count(&rows).Error)
	require.Equal(t, int64(33), rows)
}

func TestNameCacheConcurrentIDsStayUniqueAcrossPrograms(t *testing.T) {
	db, cleanup := setupNameCacheFileDB(t)
	defer cleanup()

	cacheA := NewNameCache("progA", true)
	cacheB := NewNameCache("progB", true)
	var wg sync.WaitGroup
	idsA := make([]int64, 64)
	idsB := make([]int64, 64)
	for i := 0; i < 64; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			idsA[i] = cacheA.GetID(fmt.Sprintf("a-%02d", i))
		}(i)
		go func(i int) {
			defer wg.Done()
			idsB[i] = cacheB.GetID(fmt.Sprintf("b-%02d", i))
		}(i)
	}
	wg.Wait()

	seen := map[int64]string{}
	for i, id := range idsA {
		require.NotZero(t, id)
		_, ok := seen[id]
		require.False(t, ok)
		seen[id] = fmt.Sprintf("A:%d", i)
	}
	for i, id := range idsB {
		require.NotZero(t, id)
		_, ok := seen[id]
		require.False(t, ok, "program B reused an id owned by %s", seen[id])
		seen[id] = fmt.Sprintf("B:%d", i)
	}
	require.NoError(t, cacheA.Flush())
	require.NoError(t, cacheB.Flush())
	var rows int64
	require.NoError(t, db.Model(&IrNamePool{}).Count(&rows).Error)
	require.Equal(t, int64(128), rows)
	require.Equal(t, "a-03", cacheA.GetName(idsA[3]))
	require.Equal(t, "b-03", cacheB.GetName(idsB[3]))
}

// TestNamePoolIDsStayUniqueAcrossDatabaseRebind covers the failure mode where
// the global SSA handle is rebound while a cache still holds unflushed
// reservations. Re-seeding the allocator from MAX(name_id) alone reissues those
// ids, and the pending flush then hits
// "UNIQUE constraint failed: ir_name_pool_v1.name_id".
func TestNamePoolIDsStayUniqueAcrossDatabaseRebind(t *testing.T) {
	db1, cleanup := setupNameCacheFileDB(t)
	defer cleanup()

	cache1 := NewNameCache("progA", true)
	pending := make([]int64, 0, 8)
	for i := 0; i < 8; i++ {
		id := cache1.GetID(fmt.Sprintf("pending-%02d", i))
		require.NotZero(t, id)
		pending = append(pending, id)
	}

	db2, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "rebind.db"))
	require.NoError(t, err)
	require.NoError(t, db2.AutoMigrate(&IrNamePool{}).Error)
	SetDB(db2)
	cache2 := NewNameCache("progB", true)
	require.NotZero(t, cache2.GetID("other"))
	require.NoError(t, cache2.Flush())
	require.NoError(t, db2.Close())

	SetDB(db1)
	late := cache1.GetID("late")
	require.NotZero(t, late)
	for _, id := range pending {
		require.NotEqual(t, id, late, "a rebind must not reissue an unflushed id")
	}
	require.NoError(t, cache1.Flush())

	var rows, distinct int64
	require.NoError(t, db1.Raw("SELECT COUNT(*), COUNT(DISTINCT name_id) FROM \""+TableIrNamePool+"\"").Row().Scan(&rows, &distinct))
	require.Equal(t, int64(9), rows)
	require.Equal(t, rows, distinct, "name_id must stay unique after the rebind")
}
