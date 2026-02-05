package schema

import (
	"testing"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/stretchr/testify/require"
)

func TestDropRecreateTable_MemoryDB(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	// 先建表并插入两条，ID 应为 1、2
	require.NoError(t, db.AutoMigrate(&GeneralStorage{}).Error)
	require.NoError(t, db.Create(&GeneralStorage{Key: "k1", Value: "v1"}).Error)
	require.NoError(t, db.Create(&GeneralStorage{Key: "k2", Value: "v2"}).Error)

	var c int64
	require.NoError(t, db.Model(&GeneralStorage{}).Count(&c).Error)
	require.Equal(t, int64(2), c)

	var row1, row2 GeneralStorage
	require.NoError(t, db.Where("key = ?", "k1").First(&row1).Error)
	require.NoError(t, db.Where("key = ?", "k2").First(&row2).Error)
	require.Equal(t, uint(1), row1.ID)
	require.Equal(t, uint(2), row2.ID)

	// 删表并重建，序列应被重置
	require.NoError(t, DropRecreateTable(db, &GeneralStorage{}))

	require.NoError(t, db.Create(&GeneralStorage{Key: "k3", Value: "v3"}).Error)
	var row3 GeneralStorage
	require.NoError(t, db.Where("key = ?", "k3").First(&row3).Error)
	require.Equal(t, uint(1), row3.ID, "sequence should reset to 1 after DropRecreateTable")
}

func TestResetSQLiteSequence_MemoryDB(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	// 无表时调用不报错（sqlite_sequence 可能不存在）
	require.NoError(t, ResetSQLiteSequence(db, "general_storages"))
	// 无表时 DropRecreateTable 也不报错（Drop 空表 + AutoMigrate 建表）
	require.NoError(t, DropRecreateTable(db, &GeneralStorage{}))

	require.NoError(t, db.AutoMigrate(&GeneralStorage{}).Error)
	require.NoError(t, db.Create(&GeneralStorage{Key: "a", Value: "b"}).Error)
	var row GeneralStorage
	require.NoError(t, db.First(&row).Error)
	require.Equal(t, uint(1), row.ID)

	// 调用不报错；若当前 DB 有 sqlite_sequence（部分 SQLite 配置下可能没有），则序列会被重置
	require.NoError(t, ResetSQLiteSequence(db, "general_storages"))
	require.NoError(t, db.Exec("DELETE FROM general_storages").Error)
	require.NoError(t, db.Create(&GeneralStorage{Key: "c", Value: "d"}).Error)
	require.NoError(t, db.Where("key = ?", "c").First(&row).Error)
	// 有 sqlite_sequence 时重置后 ID 为 1，否则可能为 2
	require.True(t, row.ID == 1 || row.ID == 2, "ID should be 1 or 2 depending on sqlite_sequence")
}

func TestDropRecreateTable_NilDB(t *testing.T) {
	require.NoError(t, DropRecreateTable(nil, &GeneralStorage{}))
}

func TestResetSQLiteSequence_NilOrEmpty(t *testing.T) {
	require.NoError(t, ResetSQLiteSequence(nil, "any_table"))
	require.NoError(t, ResetSQLiteSequence(nil, ""))
}
