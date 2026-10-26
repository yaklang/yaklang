package ssadb

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	_ "github.com/yaklang/gorm/dialects/sqlite"
)

// A full compile revisits instructions whose rows already exist. The old
// update path issued one db.Save per row (one transaction each); the batched
// path must keep the row count far below the instruction count.
func TestSaveIrCodeUpdatesWithoutIdentityIndexAreBatched(t *testing.T) {
	const rows = 2000
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "updates.db"))
	require.NoError(t, err)
	t.Cleanup(func() {
		clearIrCodeRowIDs()
		irCodeIdentitySuspended.Store(false)
		_ = db.Close()
	})
	EnsureDBOpCallbacks(db)
	require.NoError(t, db.AutoMigrate(&IrCode{}).Error)
	patchIrCodeIndex(db)
	_, err = SuspendIrCodeIdentityIndex(db)
	require.NoError(t, err)

	items := make([]*IrCode, 0, rows)
	for i := 1; i <= rows; i++ {
		items = append(items, &IrCode{
			ProgramName: "batch-updates",
			CodeID:      int64(i),
			OpcodeName:  "ConstInst",
			String:      "v0",
		})
	}
	require.NoError(t, saveIrCodesWithoutIdentityIndex(db, items))
	rowIDs := make(map[int64]uint, rows)
	for _, item := range items {
		rowIDs[item.CodeID] = item.ID
	}

	for _, item := range items {
		item.String = "v1"
	}
	ResetDBOpStats()
	require.NoError(t, saveIrCodesWithoutIdentityIndex(db, items))
	stats := SnapshotDBOpStats()

	writeOps := stats.Ops[DBOpUpdate].Count + stats.Ops[DBOpCreate].Count
	require.Less(t, writeOps, int64(rows/irCodeBatchChunk+8),
		"batched updates must not issue one statement per row")
	require.Greater(t, writeOps, int64(0))

	// The rowid stays the identity of the row, and the new payload is visible.
	for _, item := range items {
		require.Equal(t, rowIDs[item.CodeID], item.ID)
		got := nativeGetIrCodeItemById(db, "batch-updates", item.CodeID)
		require.NotNil(t, got)
		require.Equal(t, "v1", got.String)
		require.Equal(t, rowIDs[item.CodeID], got.ID)
	}
	var total int64
	require.NoError(t, db.Model(&IrCode{}).Where("program_name = ?", "batch-updates").Count(&total).Error)
	require.Equal(t, int64(rows), total, "an update must not insert a second row")
}

// Repeating the same code_id inside one chunk must not double-affect the row:
// the later value wins, matching the sequential-save order.
func TestSaveIrCodeUpdatesDedupeRepeatedRowID(t *testing.T) {
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "dupe.db"))
	require.NoError(t, err)
	t.Cleanup(func() {
		clearIrCodeRowIDs()
		irCodeIdentitySuspended.Store(false)
		_ = db.Close()
	})
	require.NoError(t, db.AutoMigrate(&IrCode{}).Error)
	patchIrCodeIndex(db)
	_, err = SuspendIrCodeIdentityIndex(db)
	require.NoError(t, err)

	first := &IrCode{ProgramName: "dupe", CodeID: 7, OpcodeName: "ConstInst", String: "v0"}
	require.NoError(t, saveIrCodesWithoutIdentityIndex(db, []*IrCode{first}))

	second := &IrCode{ProgramName: "dupe", CodeID: 7, OpcodeName: "ConstInst", String: "v1"}
	third := &IrCode{ProgramName: "dupe", CodeID: 7, OpcodeName: "ConstInst", String: "v2"}
	require.NoError(t, saveIrCodesWithoutIdentityIndex(db, []*IrCode{second, third}))

	got := nativeGetIrCodeItemById(db, "dupe", 7)
	require.NotNil(t, got)
	require.Equal(t, "v2", got.String)
	require.Equal(t, first.ID, got.ID)
	var total int64
	require.NoError(t, db.Model(&IrCode{}).Where("program_name = ?", "dupe").Count(&total).Error)
	require.Equal(t, int64(1), total)
}

// The batched path must fall back cleanly when a row is missing from the
// rowid table: that row is an insert, not an update.
func TestSaveIrCodeUpdatesInsertMissingRow(t *testing.T) {
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "mixed.db"))
	require.NoError(t, err)
	t.Cleanup(func() {
		clearIrCodeRowIDs()
		irCodeIdentitySuspended.Store(false)
		_ = db.Close()
	})
	require.NoError(t, db.AutoMigrate(&IrCode{}).Error)
	patchIrCodeIndex(db)
	_, err = SuspendIrCodeIdentityIndex(db)
	require.NoError(t, err)

	known := &IrCode{ProgramName: "mixed", CodeID: 1, OpcodeName: "ConstInst", String: "a"}
	require.NoError(t, saveIrCodesWithoutIdentityIndex(db, []*IrCode{known}))
	fresh := &IrCode{ProgramName: "mixed", CodeID: 2, OpcodeName: "ConstInst", String: "b"}
	require.NoError(t, saveIrCodesWithoutIdentityIndex(db, []*IrCode{fresh}))

	require.Equal(t, fmt.Sprintf("a"), nativeGetIrCodeItemById(db, "mixed", 1).String)
	require.Equal(t, fmt.Sprintf("b"), nativeGetIrCodeItemById(db, "mixed", 2).String)
}
