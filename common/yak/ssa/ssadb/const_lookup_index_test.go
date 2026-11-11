package ssadb

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
)

// The const table replaced a partial index over ir_codes(program_name, string,
// code_id) whose payload was 96% long, unique text. Exact lookups seek a 64-bit
// hash; pattern lookups read the narrow table in row order.
func TestConstLookupUsesNarrowConstTable(t *testing.T) {
	db := setupA3ConstDB(t)
	require.True(t, db.HasTable(TableIrConsts))
	require.True(t, hasNamedIndex(db, TableIrConsts, irConstLookupIndex))
	require.True(t, hasNamedIndex(db, TableIrConsts, irConstProgramCodeIndex))
	require.False(t, hasNamedIndex(db, TableIrCodes, irCodeConstLookupIndex),
		"the text index over ir_codes must not come back")

	prog := "const-lookup"
	require.NoError(t, InsertIrCodes(db, []*IrCode{
		{CodeID: 1, ProgramName: prog, Opcode: irCodeConstOpcode, ConstType: irCodeConstTypeNormal, String: "abc"},
		{CodeID: 2, ProgramName: prog, Opcode: irCodeConstOpcode, ConstType: irCodeConstTypeNormal, String: "user_input"},
		{CodeID: 3, ProgramName: prog, Opcode: irCodeConstOpcode, ConstType: "placeholder", String: "abc"},
		{CodeID: 4, ProgramName: prog, Opcode: 3, ConstType: irCodeConstTypeNormal, String: "abc"},
	}, 10))

	var stored int64
	require.NoError(t, db.Model(&IrConst{}).Where("program_name = ?", prog).Count(&stored).Error)
	require.Equal(t, int64(2), stored, "only normal const instructions are searchable")

	exact, err := nativeGetIrCodeIDsByConstType(db, prog, ExactCompare, "abc")
	require.NoError(t, err)
	require.Equal(t, []int64{1}, exact)

	matched, err := nativeGetIrCodeIDsByConstType(db, prog, RegexpCompare, `user.*`)
	require.NoError(t, err)
	require.Equal(t, []int64{2}, matched)

	none, err := nativeGetIrCodeIDsByConstType(db, prog, ExactCompare, "missing")
	require.NoError(t, err)
	require.Empty(t, none)

	// The upsert path must replace the const row, not append a second one.
	require.NoError(t, SaveIrCodeBatch(db, []*IrCode{
		{CodeID: 1, ProgramName: prog, Opcode: irCodeConstOpcode, ConstType: irCodeConstTypeNormal, String: "renamed"},
	}))
	require.NoError(t, db.Model(&IrConst{}).Where("program_name = ?", prog).Count(&stored).Error)
	require.Equal(t, int64(2), stored)

	after, err := nativeGetIrCodeIDsByConstType(db, prog, ExactCompare, "renamed")
	require.NoError(t, err)
	require.Equal(t, []int64{1}, after)
	stale, err := nativeGetIrCodeIDsByConstType(db, prog, ExactCompare, "abc")
	require.NoError(t, err)
	require.Empty(t, stale)

	exactPlan := sqliteQueryPlan(t, db,
		`SELECT code_id FROM "`+TableIrConsts+`" WHERE program_name = ? AND const_hash = ? AND "string" = ?`,
		prog, IrConstHash("renamed"), "renamed")
	require.Containsf(t, exactPlan, irConstLookupIndex, "plan=%s", exactPlan)
	require.NotContains(t, exactPlan, "SCAN "+TableIrConsts)

	regexpPlan := sqliteQueryPlan(t, db,
		`SELECT code_id FROM "`+TableIrConsts+`" NOT INDEXED WHERE program_name = ? AND "string" REGEXP ?`,
		prog, `user.*`)
	require.Containsf(t, regexpPlan, "SCAN "+TableIrConsts, "plan=%s", regexpPlan)
	require.NotContains(t, regexpPlan, irConstLookupIndex,
		"a pattern has no hash constraint; the hash index would add a row lookup per const")
}

// A long const keeps its text only in ir_consts_v1: ir_codes.string stops at
// IrCodeInlineConstStringLimit so the wide column no longer duplicates it.
func TestLongConstTextLivesOnlyInConstTable(t *testing.T) {
	db := setupA3ConstDB(t)
	prog := "long-const"
	long := strings.Repeat("x", IrCodeInlineConstStringLimit+1)
	require.NoError(t, InsertIrCodes(db, []*IrCode{
		{CodeID: 1, ProgramName: prog, Opcode: irCodeConstOpcode, ConstType: irCodeConstTypeNormal, ConstText: long},
	}, 10))

	var ir IrCode
	require.NoError(t, db.Where("program_name = ? AND code_id = ?", prog, 1).First(&ir).Error)
	require.Empty(t, ir.String)

	ids, err := nativeGetIrCodeIDsByConstType(db, prog, ExactCompare, long)
	require.NoError(t, err)
	require.Equal(t, []int64{1}, ids)

	matched, err := nativeGetIrCodeIDsByConstType(db, prog, RegexpCompare, `^x{512,}$`)
	require.NoError(t, err)
	require.Equal(t, []int64{1}, matched)
}

// Databases written before the const table only have ir_codes.string. The
// lookup must keep answering from it instead of returning an empty set.
func TestConstLookupFallsBackToIrCodesString(t *testing.T) {
	db := setupA3ConstDB(t)
	prog := "legacy-const"
	require.NoError(t, db.Create(&IrCode{
		CodeID: 1, ProgramName: prog, Opcode: irCodeConstOpcode,
		ConstType: irCodeConstTypeNormal, String: "legacy_value",
	}).Error)

	var constRows int64
	require.NoError(t, db.Model(&IrConst{}).Where("program_name = ?", prog).Count(&constRows).Error)
	require.Zero(t, constRows)

	ids, err := nativeGetIrCodeIDsByConstType(db, prog, ExactCompare, "legacy_value")
	require.NoError(t, err)
	require.Equal(t, []int64{1}, ids)
}

// The legacy covering index is not recreated on a new database; it is not
// listed as redundant either, so a populated legacy database keeps it until its
// program is rewritten.
func TestNewDatabaseOmitsLegacyConstTextIndex(t *testing.T) {
	db := newProductionIRSchema(t)
	require.False(t, hasNamedIndex(db, TableIrCodes, irCodeConstLookupIndex))
	require.True(t, hasNamedIndex(db, TableIrConsts, irConstLookupIndex))
}

func sqliteQueryPlan(t *testing.T, db *gorm.DB, query string, args ...interface{}) string {
	t.Helper()
	rows, err := db.Raw("EXPLAIN QUERY PLAN "+query, args...).Rows()
	require.NoError(t, err)
	defer rows.Close()
	cols, err := rows.Columns()
	require.NoError(t, err)
	var b strings.Builder
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		require.NoError(t, rows.Scan(ptrs...))
		for _, v := range vals {
			b.WriteString(stringFromAny(v))
			b.WriteByte(' ')
		}
		b.WriteByte('\n')
	}
	require.NoError(t, rows.Err())
	plan := b.String()
	t.Log(plan)
	return plan
}

func stringFromAny(v interface{}) string {
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	default:
		return fmt.Sprintf("%v", s)
	}
}
