package ssadb

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	_ "github.com/yaklang/gorm/dialects/sqlite"
	"github.com/yaklang/yaklang/common/schema"
)

// A fresh IR database must not carry the single-column indexes that the
// composite keys already cover: they cost a B-tree build on every compile and
// hundreds of megabytes on a large project.
func TestNewIRDatabaseOmitsRedundantIndexes(t *testing.T) {
	db := newProductionIRSchema(t)
	for _, redundant := range redundantIRIndexes {
		require.False(t, hasNamedIndex(db, redundant.table, redundant.index),
			"%s must not exist on a new database", redundant.index)
	}
	for _, kept := range []struct{ table, index string }{
		{TableIrCodes, "ux_ir_codes_program_code"},
		{TableIrCodes, "idx_ir_codes_program_opcode"},
		{TableIrCodes, irCodeConstLookupIndex},
		{TableIrTypes, "ux_ir_types_program_type"},
		{TableIrOffsets, "ux_ir_offsets_program_value_file_range"},
		{TableIrIndices, "idx_ir_indices_program_value"},
		{TableIrNamePool, "idx_ir_name_pool_program_name_name"},
	} {
		require.True(t, hasNamedIndex(db, kept.table, kept.index),
			"%s must exist on a new database", kept.index)
	}
}

// Pruning an existing populated database must preserve rows and leave the
// canonical query plan on its retained composite key.
func TestPruneRedundantIndexesPreservesPopulatedRows(t *testing.T) {
	db := newProductionIRSchema(t)
	require.NoError(t, db.Exec(`CREATE INDEX idx_ir_codes_program_name ON ir_codes (program_name)`).Error)
	require.NoError(t, db.Exec(`CREATE INDEX idx_ir_codes_code_id ON ir_codes (code_id)`).Error)
	require.NoError(t, db.Create(&IrCode{ProgramName: "p", CodeID: 7, Name: "kept"}).Error)

	pruneRedundantIRIndexes(db)

	require.False(t, hasNamedIndex(db, TableIrCodes, "idx_ir_codes_program_name"))
	require.False(t, hasNamedIndex(db, TableIrCodes, "idx_ir_codes_code_id"))
	var count int64
	require.NoError(t, db.Model(&IrCode{}).Where("program_name = ? AND code_id = ?", "p", 7).Count(&count).Error)
	require.Equal(t, int64(1), count)
	require.Contains(t, queryPlan(t, db,
		`SELECT * FROM "ir_codes" WHERE program_name = 'p' AND code_id = 7 AND deleted_at IS NULL`),
		"ux_ir_codes_program_code")
}

// Every canonical IR lookup must still be served by an index. A plan that
// scans the table means the pruned index was load-bearing.
func TestIRQueryPlansUseRetainedIndexes(t *testing.T) {
	db := newProductionIRSchema(t)
	seedIRRows(t, db, 3000)
	for _, table := range []string{TableIrCodes, TableIrTypes, TableIrOffsets, TableIrIndices, TableIrNamePool} {
		require.NoError(t, db.Exec("ANALYZE "+quoteSQLIdent(table)).Error)
	}

	cases := []struct {
		name  string
		query string
		// index lists the indexes that answer the query; the unique offsets key
		// also covers (program_name, value_id) before filtering variable_name.
		index []string
	}{
		{
			name:  "ir_codes by program and code id",
			query: `SELECT * FROM "ir_codes" WHERE program_name = 'p0' AND code_id = 7 AND deleted_at IS NULL`,
			index: []string{"ux_ir_codes_program_code"},
		},
		{
			name:  "ir_codes by program and opcode",
			query: `SELECT * FROM "ir_codes" WHERE program_name = 'p0' AND opcode IN (5, 6) AND deleted_at IS NULL`,
			index: []string{"idx_ir_codes_program_opcode"},
		},
		{
			name:  "ir_codes const lookup",
			query: `SELECT code_id FROM "ir_codes" WHERE program_name = 'p0' AND opcode = 5 AND const_type = 'normal' AND "string" = 'v7' AND deleted_at IS NULL`,
			index: []string{irCodeConstLookupIndex},
		},
		{
			name:  "ir_types by program and type id",
			query: `SELECT * FROM "ir_types" WHERE program_name = 'p0' AND type_id = 7 AND deleted_at IS NULL`,
			index: []string{"ux_ir_types_program_type"},
		},
		{
			name:  "ir_offsets by program and value id",
			query: `SELECT * FROM "ir_offsets" WHERE program_name = 'p0' AND value_id IN (1,2,3) AND deleted_at IS NULL`,
			index: []string{"ux_ir_offsets_program_value_file_range"},
		},
		{
			name:  "ir_offsets by variable and value",
			query: `SELECT * FROM "ir_offsets" WHERE variable_name = 'x7' AND value_id = 7 AND program_name = 'p0' AND deleted_at IS NULL`,
			index: []string{"idx_ir_offsets_variable_name", "ux_ir_offsets_program_value_file_range"},
		},
		{
			name:  "ir_indices by program and value id",
			query: `SELECT DISTINCT value_id FROM "ir_indices_v1" WHERE program_name = 'p0' AND value_id IN (1,2,3) AND deleted_at IS NULL`,
			index: []string{"idx_ir_indices_program_value"},
		},
		{
			name:  "ir_name_pool by program and name",
			query: `SELECT name_id FROM "ir_name_pool_v1" WHERE program_name = 'p0' AND name = 'v7'`,
			index: []string{"idx_ir_name_pool_program_name_name"},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			plan := queryPlan(t, db, testCase.query)
			require.NotContains(t, plan, "SCAN ir_codes")
			require.NotContains(t, plan, "SCAN ir_types")
			require.NotContains(t, plan, "SCAN ir_offsets")
			require.NotContains(t, plan, "SCAN ir_indices_v1")
			require.NotContains(t, plan, "SCAN ir_name_pool_v1")
			used := false
			for _, index := range testCase.index {
				if strings.Contains(plan, index) {
					used = true
					break
				}
			}
			require.True(t, used, "plan should use one of %v:\n%s", testCase.index, plan)
		})
	}
}

// newProductionIRSchema builds the same schema and patches the scan creates.
func newProductionIRSchema(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "ir.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, schema.AutoMigrateChecked(db, schema.KEY_SCHEMA_SSA_DATABASE))
	schema.ApplyPatches(db, schema.KEY_SCHEMA_SSA_DATABASE)
	return db
}

func seedIRRows(t *testing.T, db *gorm.DB, rows int) {
	t.Helper()
	codes := make([]*IrCode, 0, rows)
	types := make([]*IrType, 0, rows)
	offsets := make([]*IrOffset, 0, rows)
	indices := make([]*IrIndex, 0, rows)
	names := make([]*IrNamePool, 0, rows)
	for i := 0; i < rows; i++ {
		program := fmt.Sprintf("p%d", i%3)
		codes = append(codes, &IrCode{
			ProgramName: program, CodeID: int64(i + 1), Opcode: 5,
			OpcodeName: "ConstInst", ConstType: irCodeConstTypeNormal,
			String: fmt.Sprintf("v%d", i+1), Version: "v1.0.0",
			ProgramCompileHash: fmt.Sprintf("hash%d", i%17),
		})
		types = append(types, &IrType{
			ProgramName: program, TypeId: uint64(i + 1), Kind: 1, String: fmt.Sprintf("T%d", i%97),
		})
		offsets = append(offsets, &IrOffset{
			ProgramName: program, ValueID: int64(i%50 + 1), FileHash: fmt.Sprintf("f%d", i%11),
			StartOffset: int64(i), EndOffset: int64(i + 1), VariableName: fmt.Sprintf("x%d", i%13),
		})
		indices = append(indices, &IrIndex{
			ProgramName: program, ValueID: int64(i%50 + 1),
		})
		names = append(names, &IrNamePool{ProgramName: program, NameID: int64(i + 1), Name: fmt.Sprintf("v%d", i+1)})
	}
	require.NoError(t, db.CreateInBatches(codes, 500).Error)
	require.NoError(t, db.CreateInBatches(types, 500).Error)
	require.NoError(t, db.CreateInBatches(offsets, 500).Error)
	require.NoError(t, db.CreateInBatches(indices, 500).Error)
	require.NoError(t, db.CreateInBatches(names, 500).Error)
}

func queryPlan(t *testing.T, db *gorm.DB, query string) string {
	t.Helper()
	rows, err := db.DB().Query("EXPLAIN QUERY PLAN " + query)
	require.NoError(t, err)
	defer rows.Close()
	var parts []string
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		require.NoError(t, rows.Scan(&id, &parent, &notUsed, &detail))
		parts = append(parts, detail)
	}
	require.NoError(t, rows.Err())
	return strings.Join(parts, " | ")
}
