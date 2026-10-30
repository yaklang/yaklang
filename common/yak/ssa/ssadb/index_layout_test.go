package ssadb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewIRSchemaUsesCanonicalLookupIndexes(t *testing.T) {
	db := setupA2LoaderDB(t)
	require.NoError(t, db.AutoMigrate(&IrTypeID{}, &IrTypePayload{}, &IrOffset{}).Error)
	patchIrCodeIndex(db)
	require.NoError(t, ensureIrTypeDedupeIndexes(db))
	for _, tc := range []struct{ table, index string }{
		{TableIrCodes, "ux_ir_codes_program_code"},
		{TableIrTypeIDs, "ux_ir_type_ids_program_type"},
		{TableIrTypePayloads, "ux_ir_type_payloads_program_hash"},
	} {
		t.Run(tc.table, func(t *testing.T) {
			require.True(t, hasNamedIndex(db, tc.table, tc.index))
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
