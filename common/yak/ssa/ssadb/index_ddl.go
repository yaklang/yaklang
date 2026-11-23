package ssadb

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/yaklang/gorm"
)

// IndexSpec describes one index in dialect-neutral terms. Callers describe
// intent (columns, optional expression, uniqueness, predicate) and EnsureIndex
// renders the SQL the active database accepts.
type IndexSpec struct {
	Name        string
	Table       string
	Columns     []string
	Expressions []string
	Unique      bool
	Where       string
	// Prefixes bounds a column's key length on MySQL. GORM maps an untagged
	// string to VARCHAR(255) on MySQL, and three of them plus the COALESCE
	// expression in the offsets index exceed the 3072-byte InnoDB key limit.
	Prefixes map[string]int
}

// BuildIndexDDL renders the CREATE INDEX statement for db.
func (s IndexSpec) BuildIndexDDL(db *gorm.DB) (string, error) {
	if s.Name == "" {
		return "", fmt.Errorf("index name is required")
	}
	if s.Table == "" {
		return "", fmt.Errorf("index table is required")
	}

	kind := dialectOf(db)
	var parts []string
	for _, column := range s.Columns {
		if column == "" {
			return "", fmt.Errorf("index %s has an empty column", s.Name)
		}
		part := QuoteIdent(db, column)
		if kind == sqlDialectMySQL {
			if prefix := s.Prefixes[column]; prefix > 0 {
				part += fmt.Sprintf("(%d)", prefix)
			}
		}
		parts = append(parts, part)
	}
	for _, expression := range s.Expressions {
		if expression == "" {
			return "", fmt.Errorf("index %s has an empty expression", s.Name)
		}
		if !supportsExpressionIndex(kind) {
			return "", fmt.Errorf("dialect %s does not support expression index %s", kind, s.Name)
		}
		if kind == sqlDialectMySQL {
			// MySQL functional key parts need one pair of parentheses around
			// the expression; BuildIndexDDL's outer pair is the key list.
			// The rendered form is: (column, (COALESCE(column, ''))).
			parts = append(parts, "("+expression+")")
			continue
		}
		parts = append(parts, expression)
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("index %s has no columns", s.Name)
	}

	var b strings.Builder
	b.WriteString("CREATE ")
	if s.Unique {
		b.WriteString("UNIQUE ")
	}
	b.WriteString("INDEX ")
	if supportsIfNotExists(kind) {
		b.WriteString("IF NOT EXISTS ")
	}
	b.WriteString(QuoteIdent(db, s.Name))
	b.WriteString(" ON ")
	b.WriteString(QuoteIdent(db, s.Table))
	b.WriteString(" (")
	b.WriteString(strings.Join(parts, ", "))
	b.WriteString(")")

	if s.Where != "" && supportsPartialIndex(kind) {
		b.WriteString(" WHERE ")
		b.WriteString(s.Where)
	}
	return b.String(), nil
}

// EnsureIndex creates spec when an index of that name does not exist. It
// intentionally leaves duplicate-key failures to the caller: SSA index
// maintenance logs the failure instead of deleting rows.
func EnsureIndex(db *gorm.DB, spec IndexSpec) error {
	if db == nil {
		return fmt.Errorf("nil database")
	}
	if !db.HasTable(spec.Table) {
		// A partially migrated or legacy database may not carry every IR
		// table. There is nothing to index yet, and HasIndex would log a
		// missing-table error for a condition the caller already tolerates.
		return nil
	}
	if hasNamedIndex(db, spec.Table, spec.Name) {
		return nil
	}
	ddl, err := spec.BuildIndexDDL(db)
	if err != nil {
		return err
	}
	if err := nativeExec(db, ddl); err != nil {
		return fmt.Errorf("create index %s on %s: %w", spec.Name, spec.Table, err)
	}
	return nil
}

// installDMIRIndexes installs the IR index set on DM. DM rejects a second index
// on the same ordered columns, so equivalent indexes are reused, and an
// equivalent non-unique index is replaced only when installing the IR
// uniqueness invariant. Existing duplicate data fails before index removal.
func installDMIRIndexes(db *gorm.DB) error {
	for _, spec := range []struct {
		name, table, columns string
		unique               bool
	}{
		{"ux_ir_codes_program_code", TableIrCodes, "program_name,code_id", true},
		{"ux_ir_offsets_program_value_file_range", TableIrOffsets, "program_name,value_id,file_hash,start_offset,end_offset,COALESCE(variable_name,'')", true},
		{"idx_ir_codes_program_opcode", TableIrCodes, "program_name,opcode", false},
		{irTypeUniqueIndex, TableIrTypes, "program_name,type_id", true},
		{"idx_ir_indices_program_value", TableIrIndices, "program_name,value_id", false},
		{"idx_ir_name_pool_program_name_name", TableIrNamePool, "program_name,name", false},
	} {
		if db.Dialect().HasIndex(spec.table, spec.name) {
			continue
		}
		rows, err := nativeQueryContextDB(db, nil, `SELECT I.INDEX_NAME,I.UNIQUENESS,C.COLUMN_NAME FROM ALL_INDEXES I JOIN ALL_IND_COLUMNS C ON C.INDEX_OWNER=I.OWNER AND C.INDEX_NAME=I.INDEX_NAME WHERE I.OWNER=SYS_CONTEXT('USERENV','CURRENT_SCHEMA') AND I.TABLE_NAME=? ORDER BY I.INDEX_NAME,C.COLUMN_POSITION`, strings.ToUpper(spec.table))
		if err != nil {
			return err
		}
		type definition struct {
			unique  bool
			columns []string
		}
		indexes := map[string]*definition{}
		for rows.Next() {
			var name, unique string
			var column sql.NullString
			if err = rows.Scan(&name, &unique, &column); err != nil {
				rows.Close()
				return err
			}
			idx := indexes[name]
			if idx == nil {
				idx = &definition{unique: unique == "UNIQUE"}
				indexes[name] = idx
			}
			idx.columns = append(idx.columns, strings.ToLower(column.String))
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		matched := []string{}
		reused := false
		for name, index := range indexes {
			if strings.Join(index.columns, ",") != spec.columns {
				continue
			}
			if !spec.unique || index.unique {
				reused = true
				break
			}
			matched = append(matched, name)
		}
		if reused {
			continue
		}
		if spec.unique {
			var duplicates int64
			row := nativeQueryRowContext(db, nil, "SELECT COUNT(*) FROM (SELECT "+spec.columns+" FROM "+QuoteIdent(db, spec.table)+" GROUP BY "+spec.columns+" HAVING COUNT(*)>1)")
			if row == nil {
				return sql.ErrConnDone
			}
			if err = row.Scan(&duplicates); err != nil {
				return err
			}
			if duplicates > 0 {
				return fmt.Errorf("DM IR index %s requires explicit duplicate repair", spec.name)
			}
		}
		for _, name := range matched {
			if err = nativeExec(db, "DROP INDEX "+QuoteIdent(db, name)); err != nil {
				return err
			}
		}
		indexName := spec.name
		if len(matched) > 0 {
			indexName = matched[0]
		}
		kind := ""
		if spec.unique {
			kind = "UNIQUE "
		}
		if err = nativeExec(db, "CREATE "+kind+"INDEX "+QuoteIdent(db, indexName)+" ON "+QuoteIdent(db, spec.table)+" ("+spec.columns+")"); err != nil {
			return err
		}
	}
	return nil
}
