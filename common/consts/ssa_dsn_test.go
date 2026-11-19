package consts

import (
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/damenggorm"
)

func TestParseDatabaseURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		dialect string
		dsn     string
	}{
		{"bare sqlite path", "default-yakssa.db", SQLiteExtend, "default-yakssa.db"},
		{"absolute sqlite path", "/tmp/ssa.db", SQLiteExtend, "/tmp/ssa.db"},
		{"sqlite url", "sqlite:///tmp/ssa.db", SQLiteExtend, "/tmp/ssa.db"},
		{"sqlite3 url", "sqlite3:///tmp/ssa.db", SQLiteExtend, "/tmp/ssa.db"},
		{"file url keeps uri and query", "file:///tmp/ssa.db?mode=ro", SQLiteExtend, "file:///tmp/ssa.db?mode=ro"},
		{"file uri without slashes", "file:/tmp/ssa.db?mode=ro", SQLiteExtend, "file:/tmp/ssa.db?mode=ro"},
		{"uppercase file scheme", "FILE:///tmp/ssa.db?mode=ro", SQLiteExtend, "file:///tmp/ssa.db?mode=ro"},
		{
			"mysql url keeps its query",
			"mysql://root:pass@tcp(db:3306)/ir?charset=utf8&tls=skip-verify",
			MySQL,
			"root:pass@tcp(db:3306)/ir?charset=utf8&tls=skip-verify",
		},
		{
			"postgres url preserved",
			"postgres://app:secret@db.example:5432/ir?sslmode=disable",
			Postgres,
			"postgres://app:secret@db.example:5432/ir?sslmode=disable",
		},
		{"postgresql url preserved", "postgresql://app:secret@db.example/ir", Postgres, "postgresql://app:secret@db.example/ir"},
		{"dm url preserved", "dm://app:secret@dm.example:5236/ir", damenggorm.Dialect, "dm://app:secret@dm.example:5236/ir"},
		{"uppercase scheme", "POSTGRES://app:secret@db.example/ir", Postgres, "POSTGRES://app:secret@db.example/ir"},
		{"surrounding whitespace", "\n  postgres://app:secret@db.example/ir  \n", Postgres, "postgres://app:secret@db.example/ir"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dialect, dsn, err := parseDatabaseURL(tt.raw)
			require.NoError(t, err)
			require.Equal(t, tt.dialect, dialect)
			require.Equal(t, tt.dsn, dsn)
		})
	}
}

// TestParseDatabaseURLRejectsUnknownScheme pins the failure mode that made a
// mistyped DSN look healthy: the value used to fall back to SQLite with the
// whole string as a file path.
func TestParseDatabaseURLRejectsUnknownScheme(t *testing.T) {
	for _, raw := range []string{
		"postgrs://app:secret@db.example/ir",
		"mongodb://db.example/ir",
		"https://db.example/ir",
	} {
		_, _, err := parseDatabaseURL(raw)
		require.Error(t, err, raw)
		require.Contains(t, err.Error(), "unsupported SSA database URL scheme")
	}
}

func TestSetSSADatabaseInfoKeepsPreviousConfigOnError(t *testing.T) {
	oldDialect, oldRaw := SSA_PROJECT_DB_DIALECT, SSA_PROJECT_DB_RAW
	t.Cleanup(func() {
		SSA_PROJECT_DB_DIALECT, SSA_PROJECT_DB_RAW = oldDialect, oldRaw
	})

	require.NoError(t, SetSSADatabaseInfo("sqlite:///tmp/keep.db"))

	err := SetSSADatabaseInfo("postgrs://app:secret@db.example/ir")
	require.Error(t, err)
	require.Equal(t, SQLiteExtend, SSA_PROJECT_DB_DIALECT, "a rejected value must not rebind the process")
	require.Equal(t, "/tmp/keep.db", SSA_PROJECT_DB_RAW)
	require.NotContains(t, err.Error(), "secret", "the DSN can hold a password and must not be interpolated")
}

func TestWithDSNParams(t *testing.T) {
	t.Run("adds required parameters", func(t *testing.T) {
		got := withDSNParams("root:pass@tcp(db:3306)/ir", url.Values{
			"charset":   {"utf8mb4"},
			"parseTime": {"True"},
		})
		require.Equal(t, "root:pass@tcp(db:3306)/ir?charset=utf8mb4&parseTime=True", got)
	})

	t.Run("keeps the caller's query and adds the missing keys", func(t *testing.T) {
		got := withDSNParams("root:pass@tcp(db:3306)/ir?charset=utf8&tls=skip-verify", url.Values{
			"charset":   {"utf8mb4"},
			"parseTime": {"True"},
		})
		require.Equal(t, "root:pass@tcp(db:3306)/ir?charset=utf8&parseTime=True&tls=skip-verify", got)
		require.Equal(t, 1, strings.Count(got, "?"), "a second '?' breaks the whole parameter list")
	})

	t.Run("does not rewrite an already complete dsn", func(t *testing.T) {
		const dsn = "root:pass@tcp(db:3306)/ir?charset=utf8&parseTime=True&loc=Local"
		got := withDSNParams(dsn, url.Values{
			"charset":   {"utf8mb4"},
			"parseTime": {"True"},
			"loc":       {"Local"},
		})
		require.Equal(t, dsn, got)
	})

	t.Run("mysql password containing ? is not split", func(t *testing.T) {
		got := withMySQLDSNParams("root:pa?ss@tcp(db:3306)/ir", url.Values{
			"charset":   {"utf8mb4"},
			"parseTime": {"True"},
		})
		require.Equal(t, "root:pa?ss@tcp(db:3306)/ir?charset=utf8mb4&parseTime=True", got)
		require.Equal(t, 2, strings.Count(got, "?"), "only one '?' starts the query; the other belongs to the password")
	})

	t.Run("mysql password containing ? keeps its query", func(t *testing.T) {
		got := withMySQLDSNParams("root:pa?ss@tcp(db:3306)/ir?charset=utf8", url.Values{
			"parseTime": {"True"},
		})
		require.Equal(t, "root:pa?ss@tcp(db:3306)/ir?charset=utf8&parseTime=True", got)
	})
}

// sqliteFileURITestDSN builds a file: URI for path on every platform.
func sqliteFileURITestDSN(path string) string {
	slashPath := filepath.ToSlash(path)
	if !strings.HasPrefix(slashPath, "/") {
		// Windows drive paths need the third slash: file:///C:/...
		slashPath = "/" + slashPath
	}
	return "file://" + slashPath
}

// TestSQLiteFileURIModeReadOnlyIsPreserved pins the connection-level behavior
// of file: URIs: the driver keeps the URI (and SQLite applies mode=ro) only
// while the file: prefix survives parsing. Dropping it made an explicit
// read-only DSN open the database read-write.
func TestSQLiteFileURIModeReadOnlyIsPreserved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "readonly.db")

	seed, err := createAndConfigDatabase(path, SQLiteExtend)
	require.NoError(t, err)
	require.NoError(t, seed.Exec("CREATE TABLE t (id INTEGER)").Error)
	seedSQL := seed.DB()
	require.NotNil(t, seedSQL)
	require.NoError(t, seedSQL.Close())

	_, dsn, err := parseDatabaseURL(sqliteFileURITestDSN(path) + "?mode=ro")
	require.NoError(t, err)

	db, err := createAndConfigDatabase(dsn, SQLiteExtend)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.DB().Close()
	})

	err = db.Exec("INSERT INTO t (id) VALUES (1)").Error
	require.Error(t, err, "a mode=ro URI must not accept writes")
	require.Contains(t, strings.ToLower(err.Error()), "readonly")
}

func TestGetSSADataBaseInfoKeepsFileURI(t *testing.T) {
	oldDialect, oldRaw := SSA_PROJECT_DB_DIALECT, SSA_PROJECT_DB_RAW
	t.Cleanup(func() {
		SSA_PROJECT_DB_DIALECT, SSA_PROJECT_DB_RAW = oldDialect, oldRaw
	})

	require.NoError(t, SetSSADatabaseInfo("file:///tmp/readonly.db?mode=ro"))
	dialect, raw := GetSSADataBaseInfo()
	require.Equal(t, SQLiteExtend, dialect)
	require.Equal(t, "file:///tmp/readonly.db?mode=ro", raw, "a file: URI is not a path and must not be joined with the base dir")
}
