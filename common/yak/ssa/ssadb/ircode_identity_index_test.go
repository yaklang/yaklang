package ssadb

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	_ "github.com/yaklang/gorm/dialects/sqlite"
)

func setupIdentityDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "identity.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&IrCode{}).Error)
	patchIrCodeIndex(db)
	return db
}

// The unique code_id index is a schema invariant: it is never dropped for a
// bulk load, so a second row for the same instruction cannot appear.
func TestIrCodeIdentityIndexIsAlwaysPresent(t *testing.T) {
	db := setupIdentityDB(t)
	require.True(t, hasNamedIndex(db, TableIrCodes, "ux_ir_codes_program_code"))

	prog := t.Name()
	require.NoError(t, SaveIrCodeBatch(db, []*IrCode{{
		ProgramName: prog,
		CodeID:      7,
		OpcodeName:  "ConstInst",
		String:      "kept",
	}}))
	first := nativeGetIrCodeItemById(db, prog, 7)
	require.NotNil(t, first)
	require.Equal(t, "kept", first.String)

	require.NoError(t, SaveIrCodeBatch(db, []*IrCode{{
		ProgramName: prog,
		CodeID:      7,
		OpcodeName:  "ConstInst",
		String:      "changed",
	}}))
	updated := nativeGetIrCodeItemById(db, prog, 7)
	require.NotNil(t, updated)
	require.Equal(t, "changed", updated.String)
	require.Equal(t, first.ID, updated.ID, "an upsert keeps the row id")

	var total int64
	require.NoError(t, db.Model(&IrCode{}).Where("program_name = ?", prog).Count(&total).Error)
	require.Equal(t, int64(1), total)

	require.Error(t, InsertIrCodes(db, []*IrCode{{
		ProgramName: prog,
		CodeID:      7,
		OpcodeName:  "ConstInst",
	}}, 1), "a plain insert must be rejected by the unique index")
}
