package ssadb

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSuspendIrCodeIdentityIndexOnEmptyTable(t *testing.T) {
	t.Cleanup(func() {
		clearIrCodeRowIDs()
		irCodeIdentitySuspended.Store(false)
	})
	db := setupA2LoaderDB(t)
	patchIrCodeIndex(db)
	require.True(t, hasNamedIndex(db, TableIrCodes, "ux_ir_codes_program_code"))

	ddl, err := SuspendIrCodeIdentityIndex(db)
	require.NoError(t, err)
	require.Contains(t, strings.ToUpper(ddl), "UNIQUE")
	require.False(t, hasNamedIndex(db, TableIrCodes, "ux_ir_codes_program_code"))
	require.True(t, IrCodeIdentityIndexSuspended())

	prog := t.Name()
	require.NoError(t, SaveIrCodeBatch(db, []*IrCode{{
		ProgramName: prog,
		CodeID:      7,
		OpcodeName:  "ConstInst",
		String:      "kept",
	}}))
	got := nativeGetIrCodeItemById(db, prog, 7)
	require.NotNil(t, got, "a compile-time read must use the remembered rowid, not scan")
	require.Equal(t, "kept", got.String)
	require.Nil(t, nativeGetIrCodeItemById(db, prog, 8))
	batch, err := nativeGetIrCodesByIds(db, prog, []int64{8, 7, 7})
	require.NoError(t, err)
	require.Len(t, batch, 1)
	require.Equal(t, int64(7), batch[0].CodeID)

	require.NoError(t, SaveIrCodeBatch(db, []*IrCode{{
		ProgramName: prog,
		CodeID:      7,
		OpcodeName:  "ConstInst",
		String:      "changed",
	}}))
	updated := nativeGetIrCodeItemById(db, prog, 7)
	require.NotNil(t, updated)
	require.Equal(t, "changed", updated.String)
	require.Equal(t, got.ID, updated.ID, "updating an existing code keeps its rowid")

	require.NoError(t, RestoreIrCodeIdentityIndex(db, ddl))
	require.True(t, hasNamedIndex(db, TableIrCodes, "ux_ir_codes_program_code"))
	require.False(t, IrCodeIdentityIndexSuspended())
	restored := nativeGetIrCodeItemById(db, prog, 7)
	require.NotNil(t, restored)
	require.Equal(t, "changed", restored.String)
}

func TestSuspendIrCodeIdentityIndexKeepsExistingRows(t *testing.T) {
	t.Cleanup(func() {
		clearIrCodeRowIDs()
		irCodeIdentitySuspended.Store(false)
	})
	db := setupA2LoaderDB(t)
	patchIrCodeIndex(db)
	require.NoError(t, db.Create(&IrCode{ProgramName: "other", CodeID: 1, String: "stay"}).Error)

	ddl, err := SuspendIrCodeIdentityIndex(db)
	require.NoError(t, err)
	require.Empty(t, ddl)
	require.True(t, hasNamedIndex(db, TableIrCodes, "ux_ir_codes_program_code"))
	require.False(t, IrCodeIdentityIndexSuspended())
}
