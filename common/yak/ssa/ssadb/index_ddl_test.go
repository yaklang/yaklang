package ssadb

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func doubleQuoted(name string) string { return `"` + name + `"` }

func backticked(name string) string { return "`" + name + "`" }

// sqliteLikeCaps is the SQLite/PostgreSQL shape: double-quoted identifiers and
// expression key parts taken as written.
func sqliteLikeCaps() indexCapabilities {
	return indexCapabilities{
		quote:             doubleQuoted,
		expressionKeyPart: func(expression string) (string, bool) { return expression, true },
	}
}

// mysqlCaps is the MySQL shape: backticked identifiers and functional key
// parts wrapped in their own parentheses.
func mysqlCaps(functional bool) indexCapabilities {
	return indexCapabilities{
		quote: backticked,
		expressionKeyPart: func(expression string) (string, bool) {
			return "(" + expression + ")", functional
		},
	}
}

func specByName(t *testing.T, name string) indexSpec {
	t.Helper()
	for _, spec := range irUniqueIndexes {
		if spec.name == name {
			return spec
		}
	}
	t.Fatalf("index spec %s is not declared", name)
	return indexSpec{}
}

// TestBuildIndexDDLDoesNotRelyOnIfNotExists pins the portability rule that
// made MySQL and DM unable to create any IR index: both reject
// "CREATE INDEX IF NOT EXISTS". Existence is answered by the catalog instead.
func TestBuildIndexDDLDoesNotRelyOnIfNotExists(t *testing.T) {
	for _, spec := range irUniqueIndexes {
		ddl, ok := buildIndexDDL(sqliteLikeCaps(), spec)
		require.True(t, ok)
		require.NotContains(t, ddl, "IF NOT EXISTS", spec.name)
		require.True(t, strings.HasPrefix(ddl, "CREATE UNIQUE INDEX "), ddl)
	}
}

// TestBuildIndexDDLQuotesPerDialect keeps identifier quoting with the dialect.
// MySQL reads double quotes as string literals unless ANSI_QUOTES is set.
func TestBuildIndexDDLQuotesPerDialect(t *testing.T) {
	codes := specByName(t, "ux_ir_codes_program_code")

	ddl, ok := buildIndexDDL(sqliteLikeCaps(), codes)
	require.True(t, ok)
	require.Equal(t,
		`CREATE UNIQUE INDEX "ux_ir_codes_program_code" ON "ir_codes" ("program_name", "code_id")`,
		ddl)

	ddl, ok = buildIndexDDL(mysqlCaps(true), codes)
	require.True(t, ok)
	require.Equal(t,
		"CREATE UNIQUE INDEX `ux_ir_codes_program_code` ON `ir_codes` (`program_name`, `code_id`)",
		ddl)
}

// TestBuildIndexDDLWrapsMySQLExpressionKeys covers the one syntax difference
// left in the key itself: MySQL functional key parts need their own
// parentheses.
func TestBuildIndexDDLWrapsMySQLExpressionKeys(t *testing.T) {
	offsets := specByName(t, "ux_ir_offsets_program_value_file_range")

	ddl, ok := buildIndexDDL(sqliteLikeCaps(), offsets)
	require.True(t, ok)
	require.Contains(t, ddl, "COALESCE(variable_name, '')")
	require.NotContains(t, ddl, "(COALESCE(variable_name, ''))")

	ddl, ok = buildIndexDDL(mysqlCaps(true), offsets)
	require.True(t, ok, "MySQL 8.0.13+ indexes the expression as a functional key part")
	require.Contains(t, ddl, "(COALESCE(variable_name, ''))")
}

// TestBuildIndexDDLSkipsUnsupportedExpressionKey proves an older MySQL server
// is skipped explicitly instead of receiving a statement it rejects on every
// open.
func TestBuildIndexDDLSkipsUnsupportedExpressionKey(t *testing.T) {
	offsets := specByName(t, "ux_ir_offsets_program_value_file_range")
	_, ok := buildIndexDDL(mysqlCaps(false), offsets)
	require.False(t, ok, "MySQL before 8.0.13 cannot index an expression")

	// A key made only of columns is still creatable on that server.
	codes := specByName(t, "ux_ir_codes_program_code")
	_, ok = buildIndexDDL(mysqlCaps(false), codes)
	require.True(t, ok)
}

func TestBuildIndexDDLPartialPredicate(t *testing.T) {
	spec := indexSpec{
		table:   TableIrCodes,
		name:    "idx_partial",
		key:     []indexKeyPart{indexCol("program_name")},
		partial: "const_id <> 0",
	}
	ddl, ok := buildIndexDDL(sqliteLikeCaps(), spec)
	require.True(t, ok)
	require.True(t, strings.HasSuffix(ddl, "WHERE const_id <> 0"), ddl)
}

// TestDuplicateRepairSQLIsPortable pins the repair statement: an aggregate and
// an anti-join instead of a window function, so MySQL 5.7 and DM run it too,
// and a derived table so MySQL accepts reading the table it deletes from.
func TestDuplicateRepairSQLIsPortable(t *testing.T) {
	for _, spec := range irUniqueIndexes {
		sql := duplicateRepairSQL(sqliteLikeCaps(), spec)
		require.Contains(t, sql, "GROUP BY")
		require.Contains(t, sql, "MIN(")
		require.NotContains(t, sql, "ROW_NUMBER", spec.name)
		require.NotContains(t, sql, "PARTITION BY", spec.name)

		mysqlSQL := duplicateRepairSQL(mysqlCaps(true), spec)
		require.NotContains(t, mysqlSQL, `"`, "MySQL reads double quotes as string literals")
		require.Contains(t, mysqlSQL, "`")
		// The derived table is what keeps MySQL from rejecting the subquery on
		// the table being deleted (error 1093).
		require.Contains(t, mysqlSQL, "SELECT "+backticked("keep_id")+" FROM (")
	}
}

func TestSupportsPartialIndex(t *testing.T) {
	require.True(t, supportsPartialIndex("sqlite3"))
	require.True(t, supportsPartialIndex("sqlite"))
	require.True(t, supportsPartialIndex("postgres"))
	require.True(t, supportsPartialIndex("postgresql"))
	require.False(t, supportsPartialIndex("mysql"))
	require.False(t, supportsPartialIndex("dm"))
}

func TestMySQLVersionSupportsFunctionalIndex(t *testing.T) {
	require.True(t, mysqlVersionSupportsFunctionalIndex("8.0.13"))
	require.True(t, mysqlVersionSupportsFunctionalIndex("8.0.36-log"))
	require.True(t, mysqlVersionSupportsFunctionalIndex("8.4.0"))
	require.False(t, mysqlVersionSupportsFunctionalIndex("8.0.12"))
	require.False(t, mysqlVersionSupportsFunctionalIndex("5.7.44-log"))
	require.False(t, mysqlVersionSupportsFunctionalIndex("10.11.6-MariaDB"))
	require.False(t, mysqlVersionSupportsFunctionalIndex(""))
}
