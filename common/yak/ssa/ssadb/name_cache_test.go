package ssadb

import (
	"testing"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/stretchr/testify/require"
)

func setupNameCacheDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()

	oldDB := GetDB()
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&IrNamePool{}).Error)
	SetDB(db)

	cleanup := func() {
		SetDB(oldDB)
		_ = db.Close()
	}
	return db, cleanup
}

func TestIrNamePoolMultiProgramQuery(t *testing.T) {
	db, cleanup := setupNameCacheDB(t)
	defer cleanup()

	require.NoError(t, db.Create(&IrNamePool{ProgramName: "progA", Name: "nameA"}).Error)
	require.NoError(t, db.Create(&IrNamePool{ProgramName: "progB", Name: "nameB"}).Error)

	cacheA := NewNameCache("progA")
	cacheB := NewNameCache("progB")

	require.Len(t, cacheA.GetIDsByPattern("nameA", ExactCompare), 1)
	require.Len(t, cacheA.GetIDsByPattern("nameB", ExactCompare), 0)
	require.Len(t, cacheB.GetIDsByPattern("nameB", ExactCompare), 1)
	require.Len(t, cacheB.GetIDsByPattern("nameA", ExactCompare), 0)
}

func TestNameCacheProgramIsolation(t *testing.T) {
	db, cleanup := setupNameCacheDB(t)
	defer cleanup()

	require.NoError(t, db.Create(&IrNamePool{ProgramName: "progA", Name: "nameA"}).Error)
	require.NoError(t, db.Create(&IrNamePool{ProgramName: "progA", Name: "common"}).Error)
	require.NoError(t, db.Create(&IrNamePool{ProgramName: "progB", Name: "nameB"}).Error)
	require.NoError(t, db.Create(&IrNamePool{ProgramName: "progB", Name: "common"}).Error)

	cacheA := NewNameCache("progA")
	cacheB := NewNameCache("progB")

	require.Len(t, cacheA.GetIDsByPattern("nameA", ExactCompare), 1)
	require.Len(t, cacheA.GetIDsByPattern("nameB", ExactCompare), 0)
	require.Len(t, cacheB.GetIDsByPattern("nameB", ExactCompare), 1)
	require.Len(t, cacheB.GetIDsByPattern("nameA", ExactCompare), 0)

	require.Len(t, cacheA.GetIDsByPattern("com*", GlobCompare), 1)
	require.Len(t, cacheB.GetIDsByPattern("com.*", RegexpCompare), 1)

	idA := cacheA.GetID("common")
	idB := cacheB.GetID("common")
	require.NotEqual(t, idA, int64(0))
	require.NotEqual(t, idB, int64(0))
	require.NotEqual(t, idA, idB)
	require.Equal(t, "common", cacheA.GetName(idA))
	require.Equal(t, "common", cacheB.GetName(idB))
}

func TestNameCachePreloadWhenDBBecomesAvailable(t *testing.T) {
	oldDB := GetDB()
	SetDB(nil)
	cache := NewNameCache("progA")

	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer func() {
		SetDB(oldDB)
		_ = db.Close()
	}()
	require.NoError(t, db.AutoMigrate(&IrNamePool{}).Error)
	SetDB(db)

	require.NoError(t, db.Create(&IrNamePool{ProgramName: "progA", Name: "late"}).Error)
	require.Len(t, cache.GetIDsByPattern("late", ExactCompare), 1)
}
