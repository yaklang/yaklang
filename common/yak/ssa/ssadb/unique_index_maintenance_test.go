package ssadb

import (
	"database/sql"
	"sync"
	"testing"

	"github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	_ "github.com/yaklang/gorm/dialects/sqlite"
)

const irOffsetsUniqueIndex = "ux_ir_offsets_program_value_file_range"

// ensureAllUniqueIndexes runs the production template over the declared specs,
// the same way patchIrCodeIndex does.
func ensureAllUniqueIndexes(db *gorm.DB) {
	for _, spec := range irUniqueIndexes {
		ensureUniqueIndex(db, spec)
	}
}

// statementRecorder captures the statements gorm prints in detailed log mode,
// so a test can prove which maintenance statements were issued.
type statementRecorder struct {
	mu    sync.Mutex
	lines []string
}

func (r *statementRecorder) Print(values ...interface{}) {
	line := formatGormSQLLog(values...)
	if line == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, line)
}

func (r *statementRecorder) statements() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.lines...)
}

// TestUniqueIndexMaintenanceSkipsRepairWhenIndexExists proves the maintenance
// path is an index lookup once the unique index exists: re-running it must not
// issue the ROW_NUMBER() dedup, which scans and rewrites the whole table.
func TestUniqueIndexMaintenanceSkipsRepairWhenIndexExists(t *testing.T) {
	db := openRepairTestDB(t)

	// First pass creates both unique indexes on the empty schema.
	ensureAllUniqueIndexes(db)
	require.True(t, hasNamedIndex(db, TableIrCodes, "ux_ir_codes_program_code"))
	require.True(t, hasNamedIndex(db, TableIrOffsets, irOffsetsUniqueIndex))

	recorder := &statementRecorder{}
	db.SetLogger(recorder)
	db.LogMode(true)

	ensureAllUniqueIndexes(db)

	// Both backends answer the existence question from their own catalog, so a
	// database that already has the indexes must not issue any statement at
	// all - in particular not the ROW_NUMBER() delete.
	require.Empty(t, recorder.statements(),
		"re-running maintenance on an existing index must issue no statement")
}

// TestUniqueIndexCreationDoesNotDedupCleanTable pins the ordering fix: a
// database that only lacks the index must build it in one pass. The repair
// runs only when duplicate rows actually block the CREATE, which is the case
// an interrupted legacy run leaves behind.
func TestUniqueIndexCreationDoesNotDedupCleanTable(t *testing.T) {
	db := openRepairTestDB(t)

	recorder := &statementRecorder{}
	db.SetLogger(recorder)
	db.LogMode(true)

	ensureAllUniqueIndexes(db)

	require.True(t, hasNamedIndex(db, TableIrCodes, "ux_ir_codes_program_code"))
	require.True(t, hasNamedIndex(db, TableIrOffsets, irOffsetsUniqueIndex))

	statements := recorder.statements()
	require.NotEmpty(t, statements, "the CREATE statements must be logged")
	for _, statement := range statements {
		require.NotContains(t, statement, `DELETE FROM "ir_codes"`)
		require.NotContains(t, statement, `DELETE FROM "ir_offsets"`)
	}
}

var pgCatalogDriverOnce sync.Once

// openPostgresShapedSQLite opens the postgres dialect over a SQLite connection
// that carries the catalogs the dialect reads, so the PostgreSQL probe path can
// be exercised without a server.
func openPostgresShapedSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	const driverName = "sqlite3_pg_catalog"

	pgCatalogDriverOnce.Do(func() {
		sql.Register(driverName, &sqlite3.SQLiteDriver{
			ConnectHook: func(conn *sqlite3.SQLiteConn) error {
				return conn.RegisterFunc("CURRENT_SCHEMA", func() string { return "public" }, false)
			},
		})
	})

	sqlDB, err := sql.Open(driverName, "file:"+t.Name()+"?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	for _, statement := range []string{
		`CREATE TABLE pg_indexes (schemaname TEXT, tablename TEXT, indexname TEXT, indexdef TEXT)`,
		`ATTACH DATABASE ':memory:' AS INFORMATION_SCHEMA`,
		`CREATE TABLE INFORMATION_SCHEMA.tables (table_schema TEXT, table_name TEXT, table_type TEXT)`,
		`INSERT INTO INFORMATION_SCHEMA.tables VALUES ('public', 'ir_codes', 'BASE TABLE')`,
		`INSERT INTO INFORMATION_SCHEMA.tables VALUES ('public', 'ir_offsets', 'BASE TABLE')`,
		`INSERT INTO pg_indexes VALUES ('public', 'ir_codes', 'ux_ir_codes_program_code', 'CREATE UNIQUE INDEX ux_ir_codes_program_code ON ir_codes (program_name, code_id)')`,
		`INSERT INTO pg_indexes VALUES ('public', 'ir_offsets', 'ux_ir_offsets_program_value_file_range', 'CREATE UNIQUE INDEX ux_ir_offsets_program_value_file_range ON ir_offsets (program_name, value_id, file_hash, start_offset, end_offset, COALESCE(variable_name, ''''))')`,
	} {
		_, err := sqlDB.Exec(statement)
		require.NoError(t, err, statement)
	}

	db, err := gorm.Open("postgres", sqlDB)
	require.NoError(t, err)
	require.Equal(t, "postgres", db.Dialect().GetName())
	return db
}

// TestUniqueIndexProbeUsesPostgresCatalog is the regression guard for the
// reported field failure: with PostgreSQL's catalogs carrying the indexes, the
// maintenance path must issue no statement at all. The old probe asked
// sqlite_master, saw nothing, and ran the whole-table dedup on every open.
func TestUniqueIndexProbeUsesPostgresCatalog(t *testing.T) {
	db := openPostgresShapedSQLite(t)
	require.True(t, db.HasTable(TableIrCodes))
	require.True(t, db.HasTable(TableIrOffsets))

	recorder := &statementRecorder{}
	db.SetLogger(recorder)
	db.LogMode(true)

	ensureAllUniqueIndexes(db)

	require.Empty(t, recorder.statements(), "a present index must not trigger any write")
}
