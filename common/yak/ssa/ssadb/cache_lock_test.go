package ssadb

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	_ "github.com/yaklang/gorm/dialects/sqlite"
)

func TestGetIrCodeByIdDoesNotReserveSQLiteWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ssa.db")
	db, err := gorm.Open("sqlite3", path+"?cache=private&_busy_timeout=100&_txlock=immediate")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	db.DB().SetMaxOpenConns(2)
	require.NoError(t, db.Exec("PRAGMA journal_mode=WAL").Error)
	require.NoError(t, db.AutoMigrate(&IrCode{}).Error)

	const program = "read_does_not_reserve_writer"
	require.NoError(t, db.Create(&IrCode{ProgramName: program, CodeID: 1, OpcodeName: "test"}).Error)

	// The SSA connection's BEGIN IMMEDIATE reserves the sole writer slot.
	// A cache lookup must still read the committed instruction from WAL.
	writer := db.Begin()
	require.NoError(t, writer.Error)
	defer writer.Rollback()

	got := GetIrCodeById(db, program, 1)
	require.NotNil(t, got)
	require.Equal(t, "test", got.OpcodeName)
}
