package ssadb

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
)

// The const pool replaced a partial index over ir_codes(program_name, string,
// code_id) whose payload was 96% long, unique text. ir_codes carries a 64-bit
// const id, exact lookups seek it, and pattern lookups read the narrow pool in
// row order.
func TestConstLookupUsesNarrowConstTable(t *testing.T) {
	db := setupA3ConstDB(t)
	require.True(t, db.HasTable(TableIrConstPool))
	require.True(t, hasNamedIndex(db, TableIrConstPool, irConstPoolProgramIDIndex))
	require.True(t, hasNamedIndex(db, TableIrCodes, irCodeConstIDIndex))
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

	// A re-saved row repoints its const id at the new text; the old pool row is
	// inert because no ir_codes row references it any more.
	require.NoError(t, SaveIrCodeBatch(db, []*IrCode{
		{CodeID: 1, ProgramName: prog, Opcode: irCodeConstOpcode, ConstType: irCodeConstTypeNormal, String: "renamed"},
	}))
	after, err := nativeGetIrCodeIDsByConstType(db, prog, ExactCompare, "renamed")
	require.NoError(t, err)
	require.Equal(t, []int64{1}, after)
	stale, err := nativeGetIrCodeIDsByConstType(db, prog, ExactCompare, "abc")
	require.NoError(t, err)
	require.Empty(t, stale)

	exactPlan := sqliteQueryPlan(t, db,
		`SELECT c.code_id FROM "`+TableIrConstPool+`" p JOIN "`+TableIrCodes+`" c`+
			` ON c.program_name = p.program_name AND c.const_id = p.const_id`+
			` WHERE p.program_name = ? AND c.const_id <> 0 AND c.deleted_at IS NULL AND p.const_id = ?`,
		prog, IrConstHash("renamed"))
	require.Containsf(t, exactPlan, irConstPoolProgramIDIndex, "plan=%s", exactPlan)
	require.Containsf(t, exactPlan, irCodeConstIDIndex, "plan=%s", exactPlan)

	// A pattern cannot seek, so the pool must be scanned narrow and the wide
	// table probed once per hit rather than the other way round.
	regexpPlan := sqliteQueryPlan(t, db,
		`SELECT c.code_id FROM "`+TableIrCodes+`" c`+
			` WHERE c.program_name = ? AND c.const_id <> 0 AND c.deleted_at IS NULL`+
			` AND c.const_id IN (SELECT const_id FROM "`+TableIrConstPool+`" WHERE program_name = ? AND "string" REGEXP ?)`,
		prog, prog, `user.*`)
	require.Containsf(t, regexpPlan, TableIrConstPool+" USING", "the narrow pool is the pattern side; plan=%s", regexpPlan)
	require.NotContainsf(t, regexpPlan, "SCAN "+TableIrCodes, "plan=%s", regexpPlan)
}

// A long const keeps its text only in the pool: ir_codes.string stops at
// IrCodeInlineConstStringLimit and the row only stores the const id.
func TestLongConstTextLivesOnlyInConstTable(t *testing.T) {
	db := setupA3ConstDB(t)
	prog := "long-const"
	long := strings.Repeat("x", IrCodeInlineConstStringLimit+1)
	require.NoError(t, InsertIrCodes(db, []*IrCode{
		{CodeID: 1, ProgramName: prog, Opcode: irCodeConstOpcode, ConstType: irCodeConstTypeNormal,
			ConstID: InternIrConst(prog, IrConstKindString, long)},
	}, 10))

	var ir IrCode
	require.NoError(t, db.Where("program_name = ? AND code_id = ?", prog, 1).First(&ir).Error)
	require.Empty(t, ir.String)
	require.NotZero(t, ir.ConstID)

	var stored int64
	require.NoError(t, db.Model(&IrConst{}).Where("program_name = ? AND const_id = ?", prog, ir.ConstID).
		Count(&stored).Error)
	require.Equal(t, int64(1), stored)

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

// The legacy covering index is not recreated on a new database; the pool
// answers const searches instead.
func TestNewDatabaseOmitsLegacyConstTextIndex(t *testing.T) {
	db := newProductionIRSchema(t)
	require.False(t, hasNamedIndex(db, TableIrCodes, irCodeConstLookupIndex))
	require.True(t, hasNamedIndex(db, TableIrConstPool, irConstPoolProgramIDIndex))
	require.True(t, hasNamedIndex(db, TableIrCodes, irCodeConstIDIndex))
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
