package ssadb

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Secondary indexes are what make a multi-gigabyte SQLite IR write random.
// Compile must be able to drop them and put the same definitions back before
// any rule query, without touching the UNIQUE keys inserts rely on.
func TestSuspendSecondaryIndexesRestoresDefinitions(t *testing.T) {
	db := setupA2LoaderDB(t)
	patchIrCodeIndex(db)
	require.True(t, hasNamedIndex(db, TableIrCodes, "ux_ir_codes_program_code"))
	require.True(t, hasNamedIndex(db, TableIrCodes, "idx_ir_codes_program_opcode"))
	require.True(t, hasNamedIndex(db, TableIrCodes, irCodeConstLookupIndex))

	ddls, err := SuspendSecondaryIndexes(db)
	require.NoError(t, err)
	require.NotEmpty(t, ddls)
	require.False(t, hasNamedIndex(db, TableIrCodes, "idx_ir_codes_program_opcode"))
	require.False(t, hasNamedIndex(db, TableIrCodes, irCodeConstLookupIndex))
	require.True(t, hasNamedIndex(db, TableIrCodes, "ux_ir_codes_program_code"), "unique key stays for the insert path")

	prog := t.Name()
	require.NoError(t, SaveIrCodeBatch(db, []*IrCode{{
		ProgramName: prog,
		CodeID:      1,
		Opcode:      irCodeConstOpcode,
		ConstType:   irCodeConstTypeNormal,
		String:      "abc",
	}}))

	require.NoError(t, RestoreSecondaryIndexes(db, ddls))
	require.True(t, hasNamedIndex(db, TableIrCodes, "idx_ir_codes_program_opcode"))
	require.True(t, hasNamedIndex(db, TableIrCodes, irCodeConstLookupIndex))
	require.True(t, hasNamedIndex(db, TableIrCodes, "ux_ir_codes_program_code"))

	ids, err := nativeGetIrCodeIDsByConstType(db, prog, ExactCompare, "abc")
	require.NoError(t, err)
	require.Equal(t, []int64{1}, ids)

	for _, ddl := range ddls {
		upper := strings.ToUpper(ddl)
		require.NotContains(t, upper, "UNIQUE", "suspend must not replay a unique index")
		require.Contains(t, upper, "CREATE INDEX")
	}
}

func TestSuspendSecondaryIndexesSkipsNonSQLite(t *testing.T) {
	ddls, err := SuspendSecondaryIndexes(nil)
	require.NoError(t, err)
	require.Nil(t, ddls)
	require.NoError(t, RestoreSecondaryIndexes(nil, []string{"CREATE INDEX i ON t(a)"}))
	require.NoError(t, RestoreSecondaryIndexes(nil, nil))
	_ = context.Background()
}
