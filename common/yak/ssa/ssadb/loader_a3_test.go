package ssadb

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
)

// setupA3ConstDB creates a fresh file-backed DB via consts.CreateSSAProjectDatabaseRaw
// (SQLiteExtend driver, which registers the `regexp` SQL function used by the
// ConstType REGEXP path in production).
func setupA3ConstDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := GetDB()
	dir := t.TempDir()
	db, err := consts.CreateSSAProjectDatabaseRaw(filepath.Join(dir, "a3.db"))
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&IrCode{}).Error)
	sqlDB := db.DB()
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	SetDB(db)
	t.Cleanup(func() {
		SetDB(oldDB)
		_ = db.Close()
	})
	return db
}

// TestSearchVariableConstType_A3_UsesNativeIDQuery proves that the ConstType
// branch of searchVariableWithFileFilter routes through a native-SQL code_id
// query (incrementing NativeConstTypeIDQueries) instead of GORM
// Model+Pluck+YieldIrCode.
//
// RED: before A3, the ConstType branch uses YieldIrCode (GORM Pluck), so the
// counter does not move.
func TestSearchVariableConstType_A3_UsesNativeIDQuery(t *testing.T) {
	db := setupA3ConstDB(t)
	prog := "a3-const-prog"
	require.NoError(t, db.Create(&IrCode{CodeID: 1, ProgramName: prog, Opcode: 5, ConstType: "normal", String: "abc"}).Error)
	require.NoError(t, db.Create(&IrCode{CodeID: 2, ProgramName: prog, Opcode: 5, ConstType: "normal", String: "xyz"}).Error)
	require.NoError(t, db.Create(&IrCode{CodeID: 3, ProgramName: prog, Opcode: 3, ConstType: "normal", String: "abc"}).Error)
	require.NoError(t, db.Create(&IrCode{CodeID: 4, ProgramName: prog, Opcode: 5, ConstType: "placeholder", String: "abc"}).Error)
	deleteCache(prog)

	before := NativeConstTypeIDQueries()
	ch := SearchVariableWithExcludeFiles(db, context.Background(), prog, NewNameCache(prog, false), ExactCompare, ConstType, "abc", nil)
	got := make(map[int64]bool)
	for ir := range ch {
		got[ir.CodeID] = true
	}
	require.Equal(t, map[int64]bool{1: true}, got, "only code 1 matches opcode=5 const_type=normal string=abc")
	after := NativeConstTypeIDQueries()
	require.Greater(t, after, before,
		"ConstType search must use the native-SQL ID query (A3), not GORM Pluck")
}

// TestSearchVariableConstType_A3_Equivalence covers the non-Exact (regexp)
// compare path: both GORM REGEXP and the native query must return the same set.
func TestSearchVariableConstType_A3_Equivalence(t *testing.T) {
	db := setupA3ConstDB(t)
	prog := "a3-const-regexp-prog"
	require.NoError(t, db.Create(&IrCode{CodeID: 1, ProgramName: prog, Opcode: 5, ConstType: "normal", String: "user_input"}).Error)
	require.NoError(t, db.Create(&IrCode{CodeID: 2, ProgramName: prog, Opcode: 5, ConstType: "normal", String: "userid"}).Error)
	require.NoError(t, db.Create(&IrCode{CodeID: 3, ProgramName: prog, Opcode: 5, ConstType: "normal", String: "other"}).Error)
	require.NoError(t, db.Create(&IrCode{CodeID: 4, ProgramName: prog, Opcode: 5, ConstType: "placeholder", String: "user_input"}).Error)
	deleteCache(prog)

	ch := SearchVariableWithExcludeFiles(db, context.Background(), prog, NewNameCache(prog, false), RegexpCompare, ConstType, `user.*`, nil)
	got := make(map[int64]bool)
	for ir := range ch {
		got[ir.CodeID] = true
	}
	require.Equal(t, map[int64]bool{1: true, 2: true}, got,
		"regexp 'user.*' on opcode=5 const_type=normal must match codes 1 and 2")
}
