package ssadb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewIRSchemaUsesCanonicalLookupIndexes(t *testing.T) {
	db := setupA2LoaderDB(t)
	require.NoError(t, db.AutoMigrate(&IrType{}, &IrOffset{}).Error)
	patchIrCodeIndex(db)
	for _, tc := range []struct{ table, unique, redundant string }{
		{TableIrCodes, "ux_ir_codes_program_code", "idx_ir_codes_program_code"},
		{TableIrTypes, "ux_ir_types_program_type", "idx_ir_types_program_type"},
	} {
		t.Run(tc.table, func(t *testing.T) {
			require.True(t, hasNamedIndex(db, tc.table, tc.unique))
			require.False(t, hasNamedIndex(db, tc.table, tc.redundant), "the same lookup columns are already indexed by the canonical UNIQUE key")
		})
	}
	code := &IrCode{ProgramName: t.Name(), CodeID: 7, Name: "before"}
	require.NoError(t, SaveIrCodeBatch(db, []*IrCode{code}))
	var before IrCode
	require.NoError(t, db.Where("program_name=? AND code_id=?", t.Name(), 7).First(&before).Error)
	code.Name = "after"
	require.NoError(t, SaveIrCodeBatch(db, []*IrCode{code}))
	var after IrCode
	require.NoError(t, db.Where("program_name=? AND code_id=?", t.Name(), 7).First(&after).Error)
	require.Equal(t, before.ID, after.ID, "canonical upsert retains the persisted row ID")
	require.Equal(t, "after", after.Name)
}

func TestExistingIRSchemaRetainsLegacyLookupIndexes(t *testing.T) {
	db := setupA2LoaderDB(t)
	require.NoError(t, db.AutoMigrate(&IrType{}, &IrOffset{}).Error)
	require.NoError(t, db.Exec(`CREATE INDEX idx_ir_codes_program_code ON ir_codes(program_name,code_id)`).Error)
	require.NoError(t, db.Exec(`CREATE INDEX idx_ir_types_program_type ON ir_types(program_name,type_id)`).Error)
	patchIrCodeIndex(db)
	require.True(t, hasNamedIndex(db, TableIrCodes, "idx_ir_codes_program_code"), "opening an old database must not drop its indexes")
	require.True(t, hasNamedIndex(db, TableIrTypes, "idx_ir_types_program_type"))
	require.True(t, hasNamedIndex(db, TableIrCodes, "ux_ir_codes_program_code"))
	require.True(t, hasNamedIndex(db, TableIrTypes, "ux_ir_types_program_type"))
}
