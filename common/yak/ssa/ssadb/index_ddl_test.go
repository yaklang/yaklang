package ssadb

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

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

func mysqlAdapter(functional bool) indexDialect {
	return mysqlIndexDialect{functionalIndex: functional}
}

// TestCreateIndexSQLDoesNotRelyOnIfNotExists pins the portability rule that
// made MySQL and DM unable to create any IR index: both reject
// "CREATE INDEX IF NOT EXISTS". Existence is answered by the catalog instead.
func TestCreateIndexSQLDoesNotRelyOnIfNotExists(t *testing.T) {
	for _, adapter := range []indexDialect{ansiIndexDialect{}, mysqlAdapter(true)} {
		for _, spec := range irUniqueIndexes {
			ddl, ok := adapter.CreateIndexSQL(spec)
			require.True(t, ok)
			require.NotContains(t, ddl, "IF NOT EXISTS", spec.name)
			require.True(t, strings.HasPrefix(ddl, "CREATE UNIQUE INDEX "), ddl)
		}
	}
}

// TestCreateIndexSQLQuotesPerDialect keeps identifier quoting with the adapter.
// MySQL reads double quotes as string literals unless ANSI_QUOTES is set.
func TestCreateIndexSQLQuotesPerDialect(t *testing.T) {
	codes := specByName(t, "ux_ir_codes_program_code")

	ddl, ok := ansiIndexDialect{}.CreateIndexSQL(codes)
	require.True(t, ok)
	require.Equal(t,
		`CREATE UNIQUE INDEX "ux_ir_codes_program_code" ON "ir_codes" ("program_name", "code_id")`,
		ddl)

	ddl, ok = mysqlAdapter(true).CreateIndexSQL(codes)
	require.True(t, ok)
	require.Equal(t,
		"CREATE UNIQUE INDEX `ux_ir_codes_program_code` ON `ir_codes` (`program_name`, `code_id`)",
		ddl)
}

// TestCreateIndexSQLWrapsMySQLExpressionKeys covers the one syntax difference
// left in the key itself: MySQL functional key parts need their own
// parentheses.
func TestCreateIndexSQLWrapsMySQLExpressionKeys(t *testing.T) {
	offsets := specByName(t, "ux_ir_offsets_program_value_file_range")

	ddl, ok := ansiIndexDialect{}.CreateIndexSQL(offsets)
	require.True(t, ok)
	require.Contains(t, ddl, "COALESCE(variable_name, '')")
	require.NotContains(t, ddl, "(COALESCE(variable_name, ''))")

	ddl, ok = mysqlAdapter(true).CreateIndexSQL(offsets)
	require.True(t, ok, "MySQL 8.0.13+ indexes the expression as a functional key part")
	require.Contains(t, ddl, "(COALESCE(variable_name, ''))")
}

// TestCreateIndexSQLSkipsUnsupportedExpressionKey proves an older MySQL server
// is skipped explicitly instead of receiving a statement it rejects on every
// open.
func TestCreateIndexSQLSkipsUnsupportedExpressionKey(t *testing.T) {
	offsets := specByName(t, "ux_ir_offsets_program_value_file_range")
	_, ok := mysqlAdapter(false).CreateIndexSQL(offsets)
	require.False(t, ok, "MySQL before 8.0.13 cannot index an expression")

	// A key made only of columns is still creatable on that server.
	codes := specByName(t, "ux_ir_codes_program_code")
	_, ok = mysqlAdapter(false).CreateIndexSQL(codes)
	require.True(t, ok)
}

func TestCreateIndexSQLPartialPredicate(t *testing.T) {
	spec := indexSpec{
		table:   TableIrCodes,
		name:    "idx_partial",
		key:     []indexKeyPart{indexCol("program_name")},
		partial: "const_id <> 0",
	}
	ddl, ok := ansiIndexDialect{}.CreateIndexSQL(spec)
	require.True(t, ok)
	require.True(t, strings.HasSuffix(ddl, "WHERE const_id <> 0"), ddl)
}

// TestDeleteDuplicatesSQLIsPortable pins the repair statement: an aggregate and
// an anti-join instead of a window function, so MySQL 5.7 and DM run it too,
// and a derived table so MySQL accepts reading the table it deletes from.
func TestDeleteDuplicatesSQLIsPortable(t *testing.T) {
	for _, adapter := range []indexDialect{ansiIndexDialect{}, mysqlAdapter(true)} {
		for _, spec := range irUniqueIndexes {
			sql := adapter.DeleteDuplicatesSQL(spec)
			require.Contains(t, sql, "GROUP BY")
			require.Contains(t, sql, "MIN(")
			require.NotContains(t, sql, "ROW_NUMBER", spec.name)
			require.NotContains(t, sql, "PARTITION BY", spec.name)
		}
	}

	mysqlSQL := mysqlAdapter(true).DeleteDuplicatesSQL(specByName(t, "ux_ir_offsets_program_value_file_range"))
	require.NotContains(t, mysqlSQL, `"`, "MySQL reads double quotes as string literals")
	require.Contains(t, mysqlSQL, "`")
	// The derived table is what keeps MySQL from rejecting the subquery on the
	// table being deleted (error 1093).
	require.Contains(t, mysqlSQL, "SELECT `keep_id` FROM (")
	// GROUP BY takes the expression as written; only the index key needs the
	// functional form.
	require.Contains(t, mysqlSQL, "GROUP BY `program_name`, `value_id`")
	require.Contains(t, mysqlSQL, "COALESCE(variable_name, '')")
	require.NotContains(t, mysqlSQL, "(COALESCE(variable_name, ''))")
}

func TestDialectCapabilities(t *testing.T) {
	require.True(t, ansiIndexDialect{}.SupportsPartialIndex())
	require.False(t, mysqlAdapter(true).SupportsPartialIndex())
	require.False(t, dmIndexDialect{}.SupportsPartialIndex())

	require.True(t, ansiIndexDialect{}.SupportsIndexPruning())
	require.False(t, mysqlAdapter(true).SupportsIndexPruning())
	require.False(t, dmIndexDialect{}.SupportsIndexPruning())
}

func TestIndexDialectForSQLiteUsesAnsiAdapter(t *testing.T) {
	db := openRepairTestDB(t)
	_, ok := indexDialectFor(db).(ansiIndexDialect)
	require.True(t, ok)
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
