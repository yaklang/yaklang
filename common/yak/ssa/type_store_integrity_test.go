package ssa

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	_ "github.com/yaklang/gorm/dialects/sqlite"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

// TestTypeStoreFlushFileDBKeepsIndexIntegrity exercises the real
// typeStore.flush + type2IrCode path (including the pooled JSON buffer) against
// a file-backed SQLite DB and verifies idx_ir_types_program_type stays intact.
func TestTypeStoreFlushFileDBKeepsIndexIntegrity(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "typestore-integrity.db")

	db, err := gorm.Open("sqlite3", dbPath)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&ssadb.IrType{}).Error)

	store := &typeStore{
		mode:        ProgramCacheDBWrite,
		db:          db,
		programName: "prog",
		saveSize:    4096,
		resident:    utils.NewSafeMapWithKey[int64, Type](),
	}
	longName := strings.Repeat("org.apache.hadoop.example.TypeName", 20)
	for i := 1; i <= 5000; i++ {
		typ := NewObjectType()
		typ.SetId(int64(i))
		typ.Name = fmt.Sprintf("Type%d", i)
		typ.AddFullTypeName(longName)
		store.remember(typ)
	}

	for round := 0; round < 5; round++ {
		require.NoError(t, store.flush(), "round %d", round)
		sqlDB := db.DB()
		rows, err := sqlDB.Query("PRAGMA integrity_check")
		require.NoError(t, err)
		var checks []string
		for rows.Next() {
			var row string
			require.NoError(t, rows.Scan(&row))
			checks = append(checks, row)
		}
		require.NoError(t, rows.Close())
		require.Equal(t, []string{"ok"}, checks, "SQLite integrity_check must pass after round %d", round)
	}
}
