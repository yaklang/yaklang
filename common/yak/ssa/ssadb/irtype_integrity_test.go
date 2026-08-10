package ssadb

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	_ "github.com/yaklang/gorm/dialects/sqlite"
)

// TestSaveIrTypeBatchLargeBatchesKeepsIndexIntegrity reproduces the
// "row missing from index idx_ir_types_program_type" corruption seen on an
// EngineCMS run. It repeatedly upserts 2000-row batches with long JSON text
// into a file-backed SQLite DB and runs PRAGMA integrity_check.
func TestSaveIrTypeBatchLargeBatchesKeepsIndexIntegrity(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "irtype-integrity.db")
	db, err := gorm.Open("sqlite3", dbPath)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&IrType{}).Error)

	longJSON := `{"fullTypeName":["` + strings.Repeat("org.apache.hadoop.example.TypeName", 20) + `"],"name":"x"}`
	for round := 0; round < 10; round++ {
		items := make([]*IrType, 0, 2000)
		for i := 0; i < 2000; i++ {
			items = append(items, &IrType{
				ProgramName:      "prog",
				TypeId:           uint64(round*2000 + i + 1),
				Kind:             i % 7,
				String:           fmt.Sprintf("T%d", i),
				ExtraInformation: longJSON,
			})
		}
		require.NoError(t, SaveIrTypeBatch(db, items), "round %d", round)
	}

	sqlDB := db.DB()
	rows, err := sqlDB.Query("PRAGMA integrity_check")
	require.NoError(t, err)
	defer rows.Close()
	var result []string
	for rows.Next() {
		var row string
		require.NoError(t, rows.Scan(&row))
		result = append(result, row)
	}
	require.NoError(t, rows.Err())
	t.Logf("integrity_check rows: %#v", result)
	require.Equal(t, []string{"ok"}, result, "SQLite integrity_check must pass")

	var count int64
	require.NoError(t, db.Model(&IrType{}).Where("program_name = ?", "prog").Count(&count).Error)
	require.Equal(t, int64(20000), count)
}
