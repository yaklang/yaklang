package ssadb

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	_ "github.com/yaklang/gorm/dialects/sqlite"
)

func setupUpsertDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "updates.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	EnsureDBOpCallbacks(db)
	require.NoError(t, db.AutoMigrate(&IrCode{}).Error)
	patchIrCodeIndex(db)
	return db
}

// A full compile revisits instructions whose rows already exist. One upsert
// per chunk keeps the statement count far below the row count, and the row
// keeps its id.
func TestSaveIrCodeBatchUpsertsInPlace(t *testing.T) {
	const rows = 2000
	db := setupUpsertDB(t)

	items := make([]*IrCode, 0, rows)
	for i := 1; i <= rows; i++ {
		items = append(items, &IrCode{
			ProgramName: "batch-updates",
			CodeID:      int64(i),
			OpcodeName:  "ConstInst",
			String:      "v0",
		})
	}
	require.NoError(t, SaveIrCodeBatch(db, items))
	rowIDs := make(map[int64]uint, rows)
	for _, item := range items {
		stored := nativeGetIrCodeItemById(db, "batch-updates", item.CodeID)
		require.NotNil(t, stored)
		rowIDs[item.CodeID] = stored.ID
		item.String = "v1"
	}

	ResetIrCodeWriteBreakdown("batch-updates")
	require.NoError(t, SaveIrCodeBatch(db, items))
	_, updateRows, statements := IrCodeWriteBreakdown("batch-updates")
	require.LessOrEqual(t, statements, int64(rows/irCodeBatchChunk+8),
		"an upsert must not issue one statement per row")
	require.Greater(t, updateRows, int64(0))

	// A second program's writes must not land in this program's breakdown: a
	// backend can run several compile projects at once.
	other := &IrCode{ProgramName: "batch-updates-other", CodeID: 1, String: "x"}
	require.NoError(t, SaveIrCodeBatch(db, []*IrCode{other}))
	insertRows, otherRows, otherStatements := IrCodeWriteBreakdown("batch-updates-other")
	require.Zero(t, insertRows)
	require.Equal(t, int64(1), otherRows)
	require.Equal(t, int64(1), otherStatements)
	_, unchangedRows, unchangedStatements := IrCodeWriteBreakdown("batch-updates")
	require.Equal(t, updateRows, unchangedRows, "another program must not add rows to this breakdown")
	require.Equal(t, statements, unchangedStatements, "another program must not add statements to this breakdown")

	for _, item := range items {
		got := nativeGetIrCodeItemById(db, "batch-updates", item.CodeID)
		require.NotNil(t, got)
		require.Equal(t, "v1", got.String)
		require.Equal(t, rowIDs[item.CodeID], got.ID)
	}
	var total int64
	require.NoError(t, db.Model(&IrCode{}).Where("program_name = ?", "batch-updates").Count(&total).Error)
	require.Equal(t, int64(rows), total, "an upsert must not insert a second row")
}

// Repeating the same code_id inside one chunk must not name the conflict key
// twice; the later value wins.
func TestSaveIrCodeBatchDedupesRepeatedConflictKey(t *testing.T) {
	db := setupUpsertDB(t)

	first := &IrCode{ProgramName: "dupe", CodeID: 7, OpcodeName: "ConstInst", String: "v0"}
	require.NoError(t, SaveIrCodeBatch(db, []*IrCode{first}))
	firstRow := nativeGetIrCodeItemById(db, "dupe", 7)
	require.NotNil(t, firstRow)
	second := &IrCode{ProgramName: "dupe", CodeID: 7, OpcodeName: "ConstInst", String: "v1"}
	third := &IrCode{ProgramName: "dupe", CodeID: 7, OpcodeName: "ConstInst", String: "v2"}
	require.NoError(t, SaveIrCodeBatch(db, []*IrCode{second, third}))

	got := nativeGetIrCodeItemById(db, "dupe", 7)
	require.NotNil(t, got)
	require.Equal(t, "v2", got.String)
	require.Equal(t, firstRow.ID, got.ID, "an upsert keeps the row id")
	var total int64
	require.NoError(t, db.Model(&IrCode{}).Where("program_name = ?", "dupe").Count(&total).Error)
	require.Equal(t, int64(1), total)
}
