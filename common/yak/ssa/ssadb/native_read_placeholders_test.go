package ssadb

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	_ "github.com/yaklang/gorm/dialects/sqlite"
)

func TestBindSQLPlaceholdersPostgres(t *testing.T) {
	t.Parallel()

	const q = "SELECT 1 WHERE a = ? AND b = ?"
	require.Equal(t, "SELECT 1 WHERE a = $1 AND b = $2", bindSQLPlaceholders("postgres", q))
	require.Equal(t, "SELECT 1 WHERE a = $1 AND b = $2", bindSQLPlaceholders("PostgreSQL", q))
	require.Equal(t, "SELECT 1 WHERE a = $1 AND b = $2", bindSQLPlaceholders("postgresql", q))
	require.Equal(t, "SELECT 1 WHERE a = $1 AND b = $2", bindSQLPlaceholders("cloudsqlpostgres", q))
	require.Equal(t, q, bindSQLPlaceholders("sqlite3", q))
	require.Equal(t, q, bindSQLPlaceholders("sqlite", q))
	require.Equal(t, q, bindSQLPlaceholders("mysql", q))
	require.Equal(t, q, bindSQLPlaceholders("", q))
	require.Equal(t, "SELECT 1", bindSQLPlaceholders("postgres", "SELECT 1"))
}

func TestBindSQLPlaceholdersINList(t *testing.T) {
	t.Parallel()
	got := bindSQLPlaceholders("postgres",
		`SELECT code_id FROM ir_codes WHERE program_name = ? AND code_id IN (?,?,?) AND deleted_at IS NULL`)
	require.Equal(t,
		`SELECT code_id FROM ir_codes WHERE program_name = $1 AND code_id IN ($2,$3,$4) AND deleted_at IS NULL`,
		got)
}

func TestBindSQLPlaceholdersQuotedStringColumn(t *testing.T) {
	t.Parallel()
	got := bindSQLPlaceholders("postgres",
		`SELECT code_id FROM ir_codes WHERE program_name = ? AND "string" = ?`)
	require.Equal(t,
		`SELECT code_id FROM ir_codes WHERE program_name = $1 AND "string" = $2`,
		got)
}

func TestBindSQLPlaceholdersDB_SQLiteLeavesQMark(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	q := `SELECT 1 WHERE a = ? AND b = ?`
	require.Equal(t, q, bindSQLPlaceholdersDB(db, q))
	require.Equal(t, "sqlite3", db.Dialect().GetName())
}

type stubSQLCommon struct{}

func (stubSQLCommon) Exec(string, ...interface{}) (sql.Result, error) {
	return nil, fmt.Errorf("stub")
}
func (stubSQLCommon) Prepare(string) (*sql.Stmt, error) { return nil, fmt.Errorf("stub") }
func (stubSQLCommon) Query(string, ...interface{}) (*sql.Rows, error) {
	return nil, fmt.Errorf("stub")
}
func (stubSQLCommon) QueryRow(string, ...interface{}) *sql.Row { return nil }

func TestBindSQLPlaceholdersDB_PostgresDialectStub(t *testing.T) {
	db, err := gorm.Open("postgres", stubSQLCommon{})
	require.NoError(t, err)
	require.Equal(t, "postgres", db.Dialect().GetName())
	got := bindSQLPlaceholdersDB(db, "SELECT 1 WHERE a = ? AND b = ?")
	require.Equal(t, "SELECT 1 WHERE a = $1 AND b = $2", got)

	// nativeGetIrCodeItemById / nativeGetIrTypeItemById / nativeGetIrCodesByIds
	// / nativeGetIrCodeIDsByConstType all call bindSQLPlaceholdersDB before
	// Query/QueryRow, so a postgres-named dialect never sends raw "?" to
	// database/sql.
	codeQ := bindSQLPlaceholdersDB(db, `SELECT `+nativeIrCodeColumns+` FROM `+TableIrCodes+
		` WHERE code_id = ? AND program_name = ? AND deleted_at IS NULL LIMIT 1`)
	require.Contains(t, codeQ, "$1")
	require.Contains(t, codeQ, "$2")
	require.NotContains(t, codeQ, "?")
}

func TestNativeConstTypeQuery_PostgresPlaceholdersOnSQLite(t *testing.T) {
	// SQLite accepts $n dollar placeholders. Opening a sqlite *sql.DB with a
	// postgres GORM dialect proves nativeGetIrCodeIDsByConstType rewrites "?"
	// and the bound SQL still executes.
	sqlDB, err := sql.Open("sqlite3", "file:pgbind?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	_, err = sqlDB.Exec(`CREATE TABLE ir_codes (
		code_id INTEGER,
		program_name TEXT,
		opcode INTEGER,
		const_type TEXT,
		"string" TEXT,
		deleted_at DATETIME
	)`)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`INSERT INTO ir_codes (code_id, program_name, opcode, const_type, "string", deleted_at)
		VALUES (11, 'p', 5, 'normal', 'hello', NULL)`)
	require.NoError(t, err)

	db, err := gorm.Open("postgres", sqlDB)
	require.NoError(t, err)
	require.Equal(t, "postgres", db.Dialect().GetName())

	ids, err := nativeGetIrCodeIDsByConstType(db, "p", ExactCompare, "hello")
	require.NoError(t, err)
	require.Equal(t, []int64{11}, ids)
}

func TestNativeQueryTimeoutHelper(t *testing.T) {
	t.Parallel()
	require.Greater(t, nativeQueryTimeout(), time.Duration(0))
	require.Equal(t, 30*time.Second, nativeQueryTimeout())
}

func TestNativeQueryContextCancelledFailsFast(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	started := time.Now()
	_, err = nativeQueryContextDB(db, ctx, "SELECT 1")
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	require.Less(t, time.Since(started), 2*time.Second)

	row := nativeQueryRowContext(db, ctx, "SELECT 1")
	require.NotNil(t, row)
	var n int
	err = row.Scan(&n)
	require.Error(t, err)
}

func TestSQLFileLogWritesNativeAndGorm(t *testing.T) {
	dir := t.TempDir()
	cleanup, err := StartSQLFileLog(dir)
	require.NoError(t, err)
	t.Cleanup(cleanup)

	logNativeSQL("SELECT code_id FROM ir_codes WHERE id = $1", 150*time.Millisecond, nil)
	logNativeSQL("SELECT 1", time.Millisecond, nil) // below threshold, omitted

	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	attachSQLLogger(db)
	require.NoError(t, db.Exec("CREATE TABLE t (id INTEGER)").Error)
	require.NoError(t, db.Exec("INSERT INTO t (id) VALUES (?)", 1).Error)

	raw, err := os.ReadFile(filepath.Join(dir, "db.log"))
	require.NoError(t, err)
	body := string(raw)
	require.Contains(t, body, "SELECT code_id FROM ir_codes")
	require.Contains(t, body, "INSERT")
	require.NotContains(t, body, "SELECT 1 status=ok")
}
