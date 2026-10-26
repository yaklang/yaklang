package ssadb

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
)

func TestConstLookupIndexServesExactAndRegexp(t *testing.T) {
	db := setupA3ConstDB(t)
	require.True(t, hasNamedIndex(db, TableIrCodes, irCodeConstLookupIndex))

	prog := "const-lookup"
	require.NoError(t, db.Create(&IrCode{CodeID: 1, ProgramName: prog, Opcode: irCodeConstOpcode, ConstType: irCodeConstTypeNormal, String: "abc"}).Error)
	require.NoError(t, db.Create(&IrCode{CodeID: 2, ProgramName: prog, Opcode: irCodeConstOpcode, ConstType: irCodeConstTypeNormal, String: "user_input"}).Error)
	require.NoError(t, db.Create(&IrCode{CodeID: 3, ProgramName: prog, Opcode: irCodeConstOpcode, ConstType: "placeholder", String: "abc"}).Error)
	require.NoError(t, db.Create(&IrCode{CodeID: 4, ProgramName: prog, Opcode: 3, ConstType: irCodeConstTypeNormal, String: "abc"}).Error)
	gone := &IrCode{CodeID: 5, ProgramName: prog, Opcode: irCodeConstOpcode, ConstType: irCodeConstTypeNormal, String: "abc"}
	require.NoError(t, db.Create(gone).Error)
	require.NoError(t, db.Delete(gone).Error)

	exact, err := nativeGetIrCodeIDsByConstType(db, prog, ExactCompare, "abc")
	require.NoError(t, err)
	require.Equal(t, []int64{1}, exact)

	matched, err := nativeGetIrCodeIDsByConstType(db, prog, RegexpCompare, `user.*`)
	require.NoError(t, err)
	require.Equal(t, []int64{2}, matched)

	none, err := nativeGetIrCodeIDsByConstType(db, prog, ExactCompare, "missing")
	require.NoError(t, err)
	require.Empty(t, none)

	require.NoError(t, db.Model(&IrCode{}).Where("program_name = ? AND code_id = ?", prog, 1).Update("string", "renamed").Error)
	after, err := nativeGetIrCodeIDsByConstType(db, prog, ExactCompare, "renamed")
	require.NoError(t, err)
	require.Equal(t, []int64{1}, after)
	stale, err := nativeGetIrCodeIDsByConstType(db, prog, ExactCompare, "abc")
	require.NoError(t, err)
	require.Empty(t, stale)

	exactPlan := sqliteQueryPlan(t, db, constLookupSQL("=", irCodeConstLookupIndex), prog, "renamed")
	require.Containsf(t, exactPlan, irCodeConstLookupIndex, "plan=%s", exactPlan)
	require.Contains(t, exactPlan, "string=")
	require.NotContains(t, exactPlan, "SCAN")

	regexpPlan := sqliteQueryPlan(t, db, constLookupSQL("REGEXP", irCodeConstLookupIndex), prog, `user.*`)
	require.Contains(t, regexpPlan, irCodeConstLookupIndex)
	require.NotContains(t, regexpPlan, "SCAN")

	// The production SQL carries the literal predicate and the INDEXED BY hint:
	// that is what lets SQLite prove the partial index applies. A bound opcode
	// cannot prove it, so production must never send this shape.
	production := constLookupSQL("=", irCodeConstLookupIndex)
	require.Contains(t, production, fmt.Sprintf("opcode = %d", irCodeConstOpcode))
	require.Contains(t, production, "'"+irCodeConstTypeNormal+"'")
	require.Contains(t, production, `INDEXED BY "`+irCodeConstLookupIndex+`"`)
	boundPlan := sqliteQueryPlan(t, db,
		`SELECT code_id FROM `+TableIrCodes+` WHERE program_name = ? AND opcode = ? AND const_type = ? AND "string" = ? AND deleted_at IS NULL`,
		prog, irCodeConstOpcode, irCodeConstTypeNormal, "renamed")
	require.NotContains(t, boundPlan, "SCAN "+TableIrCodes)
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
		return ""
	}
}
