package ssadb

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	_ "github.com/yaklang/gorm/dialects/postgres"
)

// TestPostgresUniqueIndexMaintenance is the real-database counterpart of the
// SQLite tests: the reported failure only happens on PostgreSQL, where the old
// probe asked sqlite_master.
//
// It is opt-in because it creates and drops tables. Point it at a scratch
// database, never at a schema that holds real IR:
//
//	SSA_DB_TEST_POSTGRES_DSN='postgres://user:pass@host:5432/ssa_verify?sslmode=disable' \
//	  go test ./common/yak/ssa/ssadb -run TestPostgresUniqueIndexMaintenance -v
func TestPostgresUniqueIndexMaintenance(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("SSA_DB_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("set SSA_DB_TEST_POSTGRES_DSN to a scratch PostgreSQL database to run this test")
	}

	db, err := gorm.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.Equal(t, "postgres", db.Dialect().GetName())

	resetPostgresIRTables(t, db)
	const rows = 200000
	insertPostgresIrCodes(t, db, rows)
	insertPostgresIrOffsets(t, db, rows)

	// 1. Both unique indexes exist: the maintenance path must issue no
	//    statement at all. This is the exact field state that kept running the
	//    whole-table dedup on every scan process start.
	require.NoError(t, db.Exec(
		`CREATE UNIQUE INDEX "ux_ir_codes_program_code" ON "ir_codes" ("program_name", "code_id")`,
	).Error)
	require.NoError(t, db.Exec(
		`CREATE UNIQUE INDEX "ux_ir_offsets_program_value_file_range" ON "ir_offsets"
			("program_name", "value_id", "file_hash", "start_offset", "end_offset", COALESCE("variable_name", ''))`,
	).Error)

	recorder := installRecorder(db)
	start := time.Now()
	ensureAllUniqueIndexes(db)
	elapsed := time.Since(start)
	require.Empty(t, recorder.statements(), "an existing unique index must not trigger any statement")
	t.Logf("index present: %d rows per table, %s, 0 statements issued", rows, elapsed.Round(time.Millisecond))

	// 2. Index missing but the table is clean: build the index, no dedup. This
	//    is the state an interrupted legacy run leaves behind, and the old
	//    order deduplicated the whole table again on every start.
	require.NoError(t, db.Exec(`DROP INDEX "ux_ir_codes_program_code"`).Error)
	require.NoError(t, db.Exec(`DROP INDEX "ux_ir_offsets_program_value_file_range"`).Error)

	recorder = installRecorder(db)
	start = time.Now()
	ensureAllUniqueIndexes(db)
	elapsed = time.Since(start)
	statements := recorder.statements()
	require.NotEmpty(t, statements, "the CREATE statements must be visible")
	for _, statement := range statements {
		require.NotContains(t, statement, "DELETE FROM ir_codes")
		require.NotContains(t, statement, "DELETE FROM ir_offsets")
	}
	require.True(t, hasNamedIndex(db, TableIrCodes, "ux_ir_codes_program_code"))
	require.True(t, hasNamedIndex(db, TableIrOffsets, irOffsetsUniqueIndex))
	t.Logf("index missing, clean table: %d rows per table, %s, no dedup", rows, elapsed.Round(time.Millisecond))

	// 3. Duplicate rows really do block the CREATE: the repair runs once, then
	//    the index is created.
	require.NoError(t, db.Exec(`DROP INDEX "ux_ir_codes_program_code"`).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO ir_codes (program_name, code_id) VALUES ('duplicate-prog', 1), ('duplicate-prog', 1)`,
	).Error)

	recorder = installRecorder(db)
	ensureAllUniqueIndexes(db)
	statements = recorder.statements()
	deduped := false
	for _, statement := range statements {
		if strings.Contains(statement, "DELETE FROM ir_codes") {
			deduped = true
		}
	}
	require.True(t, deduped, "duplicate rows must trigger exactly the repair that was skipped on the fast path")
	require.True(t, hasNamedIndex(db, TableIrCodes, "ux_ir_codes_program_code"))

	var duplicateRows int64
	require.NoError(t, db.Raw(
		`SELECT COUNT(*) FROM ir_codes WHERE program_name = 'duplicate-prog' AND code_id = 1`,
	).Row().Scan(&duplicateRows))
	require.Equal(t, int64(1), duplicateRows)
	t.Logf("duplicate rows: repaired and indexed")
}

func installRecorder(db *gorm.DB) *statementRecorder {
	recorder := &statementRecorder{}
	db.SetLogger(recorder)
	db.LogMode(true)
	return recorder
}

func resetPostgresIRTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Exec(`DROP TABLE IF EXISTS "ir_codes"`).Error)
	require.NoError(t, db.Exec(`DROP TABLE IF EXISTS "ir_offsets"`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE "ir_codes" (
		id BIGSERIAL PRIMARY KEY,
		program_name TEXT NOT NULL,
		code_id BIGINT NOT NULL
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE "ir_offsets" (
		id BIGSERIAL PRIMARY KEY,
		program_name TEXT NOT NULL,
		value_id BIGINT NOT NULL,
		file_hash TEXT NOT NULL,
		start_offset BIGINT NOT NULL,
		end_offset BIGINT NOT NULL,
		variable_name TEXT
	)`).Error)
}

func insertPostgresIrCodes(t *testing.T, db *gorm.DB, rows int) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO "ir_codes" ("program_name", "code_id")
		 SELECT 'prog-' || (g % 7), g FROM generate_series(1, ?) AS g`, rows,
	).Error)
}

func insertPostgresIrOffsets(t *testing.T, db *gorm.DB, rows int) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO "ir_offsets" ("program_name", "value_id", "file_hash", "start_offset", "end_offset", "variable_name")
		 SELECT 'prog', g, 'hash-' || (g % 3), g * 10, g * 10 + 5, ''
		 FROM generate_series(1, ?) AS g`, rows,
	).Error)
}
