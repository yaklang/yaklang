package ssadb

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
)

// A read must survive a handle that already carries model scopes. GORM's Raw
// path on such a handle rewrites FROM into "FROM <model>, <query>", which is a
// syntax error; that turned every type read into nil (and the ARTEX scan lost
// 97 reflected-XSS risks) before the read used database/sql directly.
func TestIrTypeReadPathSurvivesScopedHandle(t *testing.T) {
	db := setupIrTypeTestDB(t)
	require.NoError(t, SaveIrTypeBatch(db, []*IrType{{ProgramName: "scoped", TypeId: 7, Kind: 1, String: "number"}}))

	scoped := db.Model(&IrType{}).Where("program_name = ?", "scoped")
	loaded := GetIrTypeItemById(scoped, "scoped", 7)
	require.NotNil(t, loaded, "a scoped handle must still resolve the type")
	require.Equal(t, uint64(7), loaded.TypeId)
	require.Equal(t, "number", loaded.String)

	var count int64
	require.NoError(t, scoped.Count(&count).Error)
	require.Equal(t, int64(1), count, "the caller's scope must not be consumed by the read")
}

// A program keeps one row per type id. Two bodies for one id in one batch
// follow the established last-write-wins rule, which also keeps PostgreSQL
// from rejecting a duplicate conflict target.
func TestIrTypeDuplicateIDInOneBatchUsesLastBody(t *testing.T) {
	db := setupIrTypeTestDB(t)
	require.NoError(t, SaveIrTypeBatch(db, []*IrType{
		{ProgramName: "p", TypeId: 11, String: "first"},
		{ProgramName: "p", TypeId: 11, String: "last"},
	}))
	require.Equal(t, 1, countIrTypes(t, db, "p"))
	require.Equal(t, "last", loadIrType(t, db, "p", 11).String)
}

// Transaction-scoped readers and writers use the same schema and visibility.
func TestIrTypeTableWorksInsideTransaction(t *testing.T) {
	db := setupIrTypeTestDB(t)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := SaveIrTypeBatch(tx, []*IrType{{ProgramName: "p", TypeId: 21, String: "inside"}}); err != nil {
			return err
		}
		got := GetIrTypeItemById(tx, "p", 21)
		if got == nil || got.String != "inside" {
			return fmt.Errorf("transaction cannot read its type: %#v", got)
		}
		return nil
	}))
	require.Equal(t, "inside", loadIrType(t, db, "p", 21).String)
}

// Deleting a program must take its type rows with it, or a recompile would
// read the previous run's bodies.
func TestDeleteProgramClearsTypeRows(t *testing.T) {
	db := newProductionIRSchema(t)
	require.NoError(t, SaveIrTypeBatch(db, []*IrType{
		{ProgramName: "p", TypeId: 1, String: "number"},
		{ProgramName: "p", TypeId: 2, String: "string"},
	}))
	require.NoError(t, deleteProgramCodeOnly(db, "p"))
	require.Zero(t, countIrTypes(t, db, "p"))
}
