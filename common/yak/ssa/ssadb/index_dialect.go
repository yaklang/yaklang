package ssadb

import (
	"strings"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/utils/damenggorm"
)

// indexDialect is the adapter between the shared index-maintenance template and
// one SQL backend. Every backend-specific decision lives in an implementation:
// identifier quoting, how an expression key is written, and partial index
// support. The template in index_maintenance.go never inspects a dialect name.
type indexDialect interface {
	// CreateIndexSQL renders CREATE [UNIQUE] INDEX for spec. ok is false when
	// this backend cannot express the key.
	CreateIndexSQL(spec indexSpec) (string, bool)
	// DeleteDuplicatesSQL renders the statement that keeps MIN(id) per key.
	DeleteDuplicatesSQL(spec indexSpec) string
	// SupportsPartialIndex reports whether spec.partial can be expressed.
	SupportsPartialIndex() bool
	// SupportsIndexPruning reports whether the legacy redundant IR indexes can
	// be dropped on this backend.
	SupportsIndexPruning() bool
}

// indexSetInstaller is implemented by a backend that owns the whole index set
// instead of accepting one spec at a time. DM needs it: it reuses an existing
// index on the same columns rather than adding a second one, and it refuses to
// delete duplicate rows.
type indexSetInstaller interface {
	InstallIndexSet(db *gorm.DB)
}

// indexDialectFor is the single dispatch point of index maintenance; every
// function below it works through the returned adapter.
func indexDialectFor(db *gorm.DB) indexDialect {
	if db == nil || db.Dialect() == nil {
		return ansiIndexDialect{}
	}
	if damenggorm.IsDM(db) {
		return dmIndexDialect{}
	}
	if strings.EqualFold(db.Dialect().GetName(), "mysql") {
		return mysqlIndexDialect{functionalIndex: mysqlFunctionalIndexSupported(db)}
	}
	return ansiIndexDialect{}
}

// ansiIndexDialect is the shape SQLite and PostgreSQL share: double-quoted
// identifiers, expression key parts as written, partial indexes supported. The
// remaining difference between the two is their catalog, which
// Dialect().HasIndex already owns.
type ansiIndexDialect struct{}

func (ansiIndexDialect) CreateIndexSQL(spec indexSpec) (string, bool) {
	return createIndexSQL(doubleQuote, asWrittenKeyPart, spec)
}

func (ansiIndexDialect) DeleteDuplicatesSQL(spec indexSpec) string {
	return deleteDuplicatesSQL(doubleQuote, spec)
}

func (ansiIndexDialect) SupportsPartialIndex() bool { return true }

func (ansiIndexDialect) SupportsIndexPruning() bool { return true }

// mysqlIndexDialect uses backtick identifiers. A functional key part needs its
// own parentheses and only exists from MySQL 8.0.13; a server without it makes
// the adapter refuse expression keys instead of emitting a rejected statement on
// every open.
type mysqlIndexDialect struct {
	functionalIndex bool
}

func (d mysqlIndexDialect) CreateIndexSQL(spec indexSpec) (string, bool) {
	return createIndexSQL(backtick, func(expression string) (string, bool) {
		return "(" + expression + ")", d.functionalIndex
	}, spec)
}

func (mysqlIndexDialect) DeleteDuplicatesSQL(spec indexSpec) string {
	// GROUP BY takes the expression as written; only the index key needs the
	// functional form.
	return deleteDuplicatesSQL(backtick, spec)
}

func (mysqlIndexDialect) SupportsPartialIndex() bool { return false }

// SupportsIndexPruning stays false until a MySQL-specific drop path exists;
// the legacy indexes are harmless there, only unused.
func (mysqlIndexDialect) SupportsIndexPruning() bool { return false }

// dmIndexDialect hands the whole set to installDMIRIndexes, which reads
// ALL_INDEXES and refuses duplicate rows. The rendering methods still produce
// DM-compatible SQL, so an accidental per-spec call does not emit garbage.
type dmIndexDialect struct {
	ansiIndexDialect
}

func (dmIndexDialect) SupportsPartialIndex() bool { return false }

func (dmIndexDialect) SupportsIndexPruning() bool { return false }

func (dmIndexDialect) InstallIndexSet(db *gorm.DB) {
	if err := installDMIRIndexes(db); err != nil {
		db.AddError(err)
	}
}

// createIndexSQL renders the statement from the backend's quoting and
// expression rules; it contains no dialect name.
func createIndexSQL(quote func(string) string, keyPart func(string) (string, bool), spec indexSpec) (string, bool) {
	parts := make([]string, 0, len(spec.key))
	for _, part := range spec.key {
		if part.expression == "" {
			parts = append(parts, quote(part.column))
			continue
		}
		rendered, ok := keyPart(part.expression)
		if !ok {
			return "", false
		}
		parts = append(parts, rendered)
	}
	kind := "INDEX"
	if spec.unique {
		kind = "UNIQUE INDEX"
	}
	ddl := "CREATE " + kind + " " + quote(spec.name) + " ON " + quote(spec.table) + " (" + strings.Join(parts, ", ") + ")"
	if spec.partial != "" {
		ddl += " WHERE " + spec.partial
	}
	return ddl, true
}

// deleteDuplicatesSQL keeps the oldest row (MIN id) per unique key. An
// aggregate plus an anti-join replaces the window function so MySQL 5.7 and DM
// run the same statement, and the derived table keeps MySQL from reading the
// table it deletes from.
func deleteDuplicatesSQL(quote func(string) string, spec indexSpec) string {
	table := quote(spec.table)
	id := quote("id")
	keepID := quote("keep_id")
	return `DELETE FROM ` + table + ` WHERE ` + id + ` NOT IN (
		SELECT ` + keepID + ` FROM (
			SELECT MIN(` + id + `) AS ` + keepID + ` FROM ` + table + `
			GROUP BY ` + dedupKeySQL(quote, spec.key) + `
		) ` + quote("keep") + `
	)`
}

// dedupKeySQL renders the key for GROUP BY, where every backend accepts an
// expression as written.
func dedupKeySQL(quote func(string) string, key []indexKeyPart) string {
	parts := make([]string, 0, len(key))
	for _, part := range key {
		if part.expression == "" {
			parts = append(parts, quote(part.column))
			continue
		}
		parts = append(parts, part.expression)
	}
	return strings.Join(parts, ", ")
}

func doubleQuote(name string) string { return `"` + name + `"` }

func backtick(name string) string { return "`" + name + "`" }

func asWrittenKeyPart(expression string) (string, bool) { return expression, true }
