package ssadb

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
)

// TestSSADialectConformance runs the SSA store's dialect-sensitive statements
// against every backend CI provides. SQLite runs locally; PostgreSQL and MySQL
// run when their DSN environment variable is set, which the ssa-sql-dialect
// workflow does with service containers.
func TestSSADialectConformance(t *testing.T) {
	tests := []struct {
		backend string
		env     string
		dialect sqlDialect
	}{
		{backend: "sqlite", dialect: sqlDialectSQLite},
		{backend: "postgres", env: "YAK_SSA_CONFORMANCE_PG_DSN", dialect: sqlDialectPostgres},
		{backend: "mysql", env: "YAK_SSA_CONFORMANCE_MYSQL_DSN", dialect: sqlDialectMySQL},
	}
	for _, tt := range tests {
		t.Run(tt.backend, func(t *testing.T) {
			dsn := conformanceDSN(t, tt.backend, tt.env)
			db := openConformanceDB(t, dsn)
			require.Equal(t, tt.dialect, dialectOf(db))
			runSSAConformance(t, db, tt.backend)
		})
	}
}

func conformanceDSN(t *testing.T, backend, env string) string {
	t.Helper()
	if backend == "sqlite" {
		return "sqlite://" + filepath.ToSlash(filepath.Join(t.TempDir(), "conformance.db"))
	}
	dsn := ""
	if env != "" {
		dsn = os.Getenv(env)
	}
	if dsn == "" {
		t.Skipf("set %s to run the %s conformance test", env, backend)
	}
	return dsn
}

func openConformanceDB(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	db, err := consts.CreateSSAProjectDatabaseRaw(dsn)
	require.NoError(t, err)
	previous := GetDB()
	SetDB(db)
	t.Cleanup(func() {
		SetDB(previous)
		require.NoError(t, db.Close())
	})
	return db
}

func runSSAConformance(t *testing.T, db *gorm.DB, backend string) {
	t.Helper()
	program := "dialect-conformance-" + uuid.NewString()
	t.Cleanup(func() {
		if err := DeleteProgramChecked(db, program); err != nil {
			t.Logf("cleanup program %s: %v", program, err)
		}
	})

	// The schema patches run on open; call them again to prove index
	// maintenance is idempotent on every dialect.
	patchIrCodeIndex(db)
	patchIrTypeIndexes(db)
	require.NoError(t, db.Error)
	for _, spec := range []IndexSpec{
		{Name: "idx_ir_codes_program_opcode", Table: TableIrCodes},
		{Name: "idx_ir_indices_program_value", Table: TableIrIndices},
		{Name: "idx_ir_name_pool_program_name_name", Table: TableIrNamePool},
		{Name: "ux_ir_codes_program_code", Table: TableIrCodes},
		{Name: "ux_ir_offsets_program_value_file_range", Table: TableIrOffsets},
		{Name: irConstPoolProgramIDIndex, Table: TableIrConstPool},
		{Name: irConstPoolSizeIndex, Table: TableIrConstPool},
		{Name: irCodeConstIDIndex, Table: TableIrCodes},
		{Name: irTypeUniqueIndex, Table: TableIrTypes},
	} {
		require.True(t, hasNamedIndex(db, spec.Table, spec.Name),
			"%s: index %s on %s was not created", backend, spec.Name, spec.Table)
	}

	require.NoError(t, db.Create(&IrProgram{
		ProgramName: program,
		ProgramKind: Application,
	}).Error)

	// Const pool: intern, flush, and resolve through the quoted read path.
	const text = "conformance-const"
	constID := InternIrConst(program, IrConstKindString, text)
	require.NotZero(t, constID)
	require.NoError(t, GetIrConstPool(program).Flush(db))
	value := GetIrConstValue(db, program, constID)
	require.NotNil(t, value)
	require.Equal(t, text, value.Text)

	// IR upsert: the second write must replace the row, not duplicate it.
	code := &IrCode{
		ProgramName: program,
		CodeID:      1,
		Opcode:      irCodeConstOpcode,
		ConstType:   irCodeConstTypeNormal,
		ConstID:     constID,
		Name:        "original",
		String:      text,
	}
	require.NoError(t, SaveIrCodeBatch(db, []*IrCode{code}))
	code.Name = "updated"
	require.NoError(t, SaveIrCodeBatch(db, []*IrCode{code}))

	var stored IrCode
	require.NoError(t, db.Where("program_name = ? AND code_id = ?", program, code.CodeID).First(&stored).Error)
	require.Equal(t, "updated", stored.Name)
	nativeStored := nativeGetIrCodeItemById(db, program, code.CodeID)
	require.NotNil(t, nativeStored)
	require.Equal(t, "updated", nativeStored.Name)
	var codeCount int64
	require.NoError(t, db.Model(&IrCode{}).Where("program_name = ?", program).Count(&codeCount).Error)
	require.Equal(t, int64(1), codeCount)

	// Const search: exact and regexp both go through the pool query.
	ids, err := nativeGetIrCodeIDsByConstType(db, program, ExactCompare, text)
	require.NoError(t, err)
	require.Equal(t, []int64{code.CodeID}, ids)
	ids, err = nativeGetIrCodeIDsByConstType(db, program, RegexpCompare, "conformance-.*")
	require.NoError(t, err)
	require.Equal(t, []int64{code.CodeID}, ids)

	// Offsets: the COALESCE expression index must accept a NULL/empty name.
	require.NoError(t, SaveIrOffsetBatch(db, []*IrOffset{{
		ProgramName: program,
		ValueID:     1,
		FileHash:    "conformance-hash",
		StartOffset: 10,
		EndOffset:   20,
	}}))
	var offsetCount int64
	require.NoError(t, db.Model(&IrOffset{}).Where("program_name = ?", program).Count(&offsetCount).Error)
	require.Equal(t, int64(1), offsetCount)

	// Types: upsert on (program_name, type_id).
	require.NoError(t, SaveIrTypeBatch(db, []*IrType{{ProgramName: program, TypeId: 1, String: "first"}}))
	require.NoError(t, SaveIrTypeBatch(db, []*IrType{{ProgramName: program, TypeId: 1, String: "second"}}))
	var storedType IrType
	require.NoError(t, db.Where("program_name = ? AND type_id = ?", program, 1).First(&storedType).Error)
	require.Equal(t, "second", storedType.String)
	nativeType := GetIrTypeItemById(db, program, 1)
	require.NotNil(t, nativeType)
	require.Equal(t, "second", nativeType.String)

	require.NoError(t, SaveIrIndexBatch(db, []*IrIndex{{ProgramName: program, ValueID: 1}}))

	// Name pool: reserveNamePoolID issues the quoted MAX(name_id) query.
	cache := NewNameCache(program, true)
	nameID := cache.GetID("conformance_name")
	require.NotZero(t, nameID)
	require.NoError(t, cache.Flush())
	var nameCount int64
	require.NoError(t, db.Model(&IrNamePool{}).Where("program_name = ?", program).Count(&nameCount).Error)
	require.Equal(t, int64(1), nameCount)

	// Program lookup moved into ssadb so PostgreSQL can use "~" instead of
	// the unsupported REGEXP operator.
	found, err := LoadProgramsByRegexp(db, "^"+regexp.QuoteMeta(program)+"$")
	require.NoError(t, err)
	require.Len(t, found, 1)
	require.Equal(t, program, found[0].ProgramName)

	require.NoError(t, DeleteProgramChecked(db, program))
	var remaining int64
	require.NoError(t, db.Model(&IrCode{}).Where("program_name = ?", program).Count(&remaining).Error)
	require.Zero(t, remaining)
}
