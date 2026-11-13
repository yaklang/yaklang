package ssadb

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/ssautil"
)

// Secondary indexes are what make a multi-gigabyte SQLite IR write random.
// Compile must be able to drop them and put the same definitions back before
// any rule query, without touching the UNIQUE keys inserts rely on.
func TestSuspendSecondaryIndexesRestoresDefinitions(t *testing.T) {
	db := setupA2LoaderDB(t)
	patchIrCodeIndex(db)
	require.True(t, hasNamedIndex(db, TableIrCodes, "ux_ir_codes_program_code"))
	require.True(t, hasNamedIndex(db, TableIrCodes, "idx_ir_codes_program_opcode"))
	// Const text is not indexed on ir_codes any more; the narrow const table
	// carries its own indexes.
	require.False(t, hasNamedIndex(db, TableIrCodes, irCodeConstLookupIndex))
	require.True(t, hasNamedIndex(db, TableIrConstPool, irConstPoolProgramIDIndex))
	require.True(t, hasNamedIndex(db, TableIrCodes, irCodeConstIDIndex))

	ddls, err := SuspendSecondaryIndexes(db)
	require.NoError(t, err)
	require.NotEmpty(t, ddls)
	require.False(t, hasNamedIndex(db, TableIrCodes, "idx_ir_codes_program_opcode"))
	require.False(t, hasNamedIndex(db, TableIrCodes, irCodeConstIDIndex),
		"the const join index is rebuilt with the other secondary indexes")
	require.True(t, hasNamedIndex(db, TableIrConstPool, irConstPoolProgramIDIndex),
		"the pool upsert conflict key stays for the insert path")
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
	require.False(t, hasNamedIndex(db, TableIrCodes, irCodeConstLookupIndex))
	require.True(t, hasNamedIndex(db, TableIrCodes, irCodeConstIDIndex))
	require.True(t, hasNamedIndex(db, TableIrConstPool, irConstPoolProgramIDIndex))
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

func TestMemoryHeadroomForIndexSortReadsHost(t *testing.T) {
	available, ok := ssautil.AvailableMemoryBytes()
	if !ok {
		require.False(t, memoryHeadroomForIndexSort(), "an unknown host must fall back to the file sorter")
		return
	}
	require.Positive(t, available)
	require.Equal(t, available >= indexSortMemoryFloorBytes, memoryHeadroomForIndexSort())
}

func TestSuspendSecondaryIndexesSkipsNonSQLite(t *testing.T) {
	ddls, err := SuspendSecondaryIndexes(nil)
	require.NoError(t, err)
	require.Nil(t, ddls)
	require.NoError(t, RestoreSecondaryIndexes(nil, []string{"CREATE INDEX i ON t(a)"}))
	require.NoError(t, RestoreSecondaryIndexes(nil, nil))
	_ = context.Background()
}

// A scan can read one project while another project is between the suspend and
// restore stages of a compile. The const lookup must not depend on an index
// that is temporarily absent: SQLite rejects the statement instead of falling
// back, and the search would silently return no rows.
func TestConstSearchSurvivesSecondaryIndexSuspension(t *testing.T) {
	db := setupA3ConstDB(t)
	prog := t.Name()
	require.NoError(t, SaveIrCodeBatch(db, []*IrCode{{
		ProgramName: prog,
		CodeID:      7,
		Opcode:      irCodeConstOpcode,
		ConstType:   irCodeConstTypeNormal,
		String:      "needle",
	}}))
	search := func() ([]int64, error) {
		var queryErr error
		ctx := WithQueryErrorHandler(context.Background(), func(err error) { queryErr = err })
		var ids []int64
		for row := range SearchVariable(db, ctx, prog, NewNameCache(prog, false), ExactCompare, ConstType, "needle") {
			ids = append(ids, row.CodeID)
		}
		return ids, queryErr
	}

	before, err := search()
	require.NoError(t, err)
	require.Equal(t, []int64{7}, before)

	ddls, err := SuspendSecondaryIndexes(db)
	require.NoError(t, err)
	require.NotEmpty(t, ddls)
	require.False(t, hasNamedIndex(db, TableIrCodes, irCodeConstLookupIndex))

	during, duringErr := search()
	require.NoError(t, duringErr, "a readable project must stay readable while another compile suspends indexes")
	require.Equal(t, before, during)

	require.NoError(t, RestoreSecondaryIndexes(db, ddls))
	after, err := search()
	require.NoError(t, err)
	require.Equal(t, before, after)
}
