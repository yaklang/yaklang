package ssadb

import (
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
	if err := db.Exec(ddl).Error; err != nil {
		return fmt.Errorf("create index %s on %s: %w", spec.Name, spec.Table, err)
	}
	return nil
}
