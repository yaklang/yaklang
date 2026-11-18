package consts

import (
	"net/url"
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
		{"file url", "file:///tmp/ssa.db", SQLiteExtend, "/tmp/ssa.db"},
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
}
