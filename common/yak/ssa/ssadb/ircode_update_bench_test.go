package ssadb

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	_ "github.com/yaklang/gorm/dialects/sqlite"
)

// setupSuspendedIdentityDB builds a fresh project DB with the compile-time
// schema, then drops the ir_codes identity index the way a full compile does.
func setupSuspendedIdentityDB(b *testing.B, rows int, program string) (*gorm.DB, []*IrCode) {
	b.Helper()
	db, err := gorm.Open("sqlite3", filepath.Join(b.TempDir(), "identity.db"))
	require.NoError(b, err)
	b.Cleanup(func() {
		clearIrCodeRowIDs()
		irCodeIdentitySuspended.Store(false)
		_ = db.Close()
	})
	require.NoError(b, db.AutoMigrate(&IrCode{}).Error)
	patchIrCodeIndex(db)
	_, err = SuspendIrCodeIdentityIndex(db)
	require.NoError(b, err)

	items := make([]*IrCode, 0, rows)
	for i := 1; i <= rows; i++ {
		items = append(items, &IrCode{
			ProgramName: program,
			CodeID:      int64(i),
			OpcodeName:  "ConstInst",
			Name:        fmt.Sprintf("value_%d", i),
			String:      "v0",
		})
	}
	require.NoError(b, saveIrCodesWithoutIdentityIndex(db, items))
	return db, items
}

// A compile revisits instructions: with the identity index suspended those
// saves are updates keyed by the remembered rowid. Measure that path on its
// own so the per-row cost is visible without a full scan.
func BenchmarkSaveIrCodeUpdatesWithoutIdentityIndex(b *testing.B) {
	const rows = 20000
	db, items := setupSuspendedIdentityDB(b, rows, "bench-updates")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, item := range items {
			item.String = fmt.Sprintf("v%d", i+1)
		}
		if err := saveIrCodesWithoutIdentityIndex(db, items); err != nil {
			b.Fatal(err)
		}
	}
}
