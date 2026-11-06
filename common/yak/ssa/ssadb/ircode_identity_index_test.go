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

func TestNativeInsertIrCodesRoundTrip(t *testing.T) {
	db := setupIdentityDB(t)
	prog := t.Name()
	first := &IrCode{
		ProgramName:           prog,
		CodeID:                1,
		OpcodeName:            "ConstInst",
		Name:                  "a",
		String:                "hello",
		FormalArgs:            Int64Slice{1, 2},
		Variable:              StringSlice{"x", "y"},
		ObjectMembers:         Int64Map{{1, 2}},
		ObjectMemberPairs:     Int64Map{{3, 4}},
		ObjectOwnerPairs:      Int64Map{{5, 6}},
		MaskedCodes:           Int64Slice{7},
		ProgramCompileHash:    "hash",
		TypeID:                9,
		ExtraInformation:      "extra",
		ConstType:             "normal",
		SourceCodeStartOffset: 3,
		SourceCodeEndOffset:   8,
	}
	second := &IrCode{ProgramName: prog, CodeID: 2, Name: "b", String: "a\x00b"}
	require.NoError(t, InsertIrCodes(db, []*IrCode{nil, first, second}, 2))

	got := GetIrCodeItemById(db, prog, 1)
	require.NotNil(t, got)
	require.Equal(t, "hello", got.String)
	require.Equal(t, Int64Slice{1, 2}, got.FormalArgs)
	require.Equal(t, StringSlice{"x", "y"}, got.Variable)
	require.Equal(t, Int64Map{{1, 2}}, got.ObjectMembers)
	require.Equal(t, Int64Map{{3, 4}}, got.ObjectMemberPairs)
	require.Equal(t, Int64Map{{5, 6}}, got.ObjectOwnerPairs)
	require.Equal(t, Int64Slice{7}, got.MaskedCodes)
	require.Equal(t, "hash", got.ProgramCompileHash)
	require.Equal(t, int64(9), got.TypeID)
	require.Equal(t, "extra", got.ExtraInformation)
	require.Equal(t, "normal", got.ConstType)
	require.Equal(t, int64(3), got.SourceCodeStartOffset)
	require.Equal(t, int64(8), got.SourceCodeEndOffset)
	require.False(t, got.CreatedAt.IsZero())

	sanitized := GetIrCodeItemById(db, prog, 2)
	require.NotNil(t, sanitized)
	require.Equal(t, "ab", sanitized.String, "native insert must keep BeforeSave sanitization")

	var total int64
	require.NoError(t, db.Model(&IrCode{}).Where("program_name = ?", prog).Count(&total).Error)
	require.Equal(t, int64(2), total)
}
