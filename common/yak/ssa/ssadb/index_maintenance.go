package ssadb

import (
	"fmt"
	"strings"

	"github.com/yaklang/gorm"
)

// The index-maintenance template. Everything here is backend-neutral: it
// describes the SSA indexes, asks the catalog whether one exists, and runs the
// repair policy. All syntax differences live behind indexDialect
// (index_dialect.go).

// indexKeyPart is one column or SQL expression of an index key. Expressions
// stay unevaluated because each backend renders an expression key part
// differently: SQLite and PostgreSQL take it as written, MySQL needs it
// wrapped in its own parentheses.
type indexKeyPart struct {
	column     string
	expression string
}

func indexCol(column string) indexKeyPart { return indexKeyPart{column: column} }

func indexExpr(expression string) indexKeyPart { return indexKeyPart{expression: expression} }

// indexSpec is one index the SSA schema installs. The same description drives
// the existence check, the CREATE statement and, for unique keys, the
// duplicate repair.
type indexSpec struct {
	table   string
	name    string
	key     []indexKeyPart
	unique  bool
	partial string // WHERE predicate; empty means a full index
}

var irUniqueIndexes = []indexSpec{
	{
		table:  TableIrCodes,
		name:   "ux_ir_codes_program_code",
		unique: true,
		key:    []indexKeyPart{indexCol("program_name"), indexCol("code_id")},
	},
	{
		table:  TableIrOffsets,
		name:   "ux_ir_offsets_program_value_file_range",
		unique: true,
		key: []indexKeyPart{
			indexCol("program_name"),
			indexCol("value_id"),
			indexCol("file_hash"),
			indexCol("start_offset"),
			indexCol("end_offset"),
			// A NULL variable name and '' name the same location, so they
			// must collide.
			indexExpr("COALESCE(variable_name, '')"),
		},
	},
}

// hasNamedIndex is the catalog half of the adapter: each backend answers from
// its own catalog through Dialect().HasIndex. SQLite reads sqlite_master,
// PostgreSQL reads pg_indexes filtered by CURRENT_SCHEMA(), MySQL reads SHOW
// INDEXES and DM reads ALL_INDEXES, so an index that exists is always reported
// as existing. db.New() clears a program_name row scope while keeping the
// connection.
func hasNamedIndex(db *gorm.DB, table, index string) bool {
	if db == nil || db.Dialect() == nil {
		return false
	}
	return db.New().Dialect().HasIndex(table, index)
}

// ensureIndex installs one index from its spec. An existing index costs a
// single catalog lookup; only a missing one is built. A failure after a
// concurrent process won the race is reported as success, because the state the
// caller asked for is the state the database is in.
func ensureIndex(db *gorm.DB, spec indexSpec) error {
	if db == nil || db.Dialect() == nil || !db.HasTable(spec.table) {
		return nil
	}
	if hasNamedIndex(db, spec.table, spec.name) {
		return nil
	}
	ddl, ok := indexDialectFor(db).CreateIndexSQL(spec)
	if !ok {
		return fmt.Errorf("%s cannot express index %s with its key", db.Dialect().GetName(), spec.name)
	}
	if err := db.Exec(ddl).Error; err != nil {
		if hasNamedIndex(db, spec.table, spec.name) {
			return nil
		}
		return err
	}
	return nil
}

// ensureUniqueIndex is the maintenance template for the IR unique keys:
//
//  1. hasNamedIndex asks the backend whether the index is already there. The
//     dialect answers from its own catalog, so no backend re-runs the repair.
//  2. Only a missing index is built.
//  3. Only when the backend rejects that build because duplicate rows exist
//     does the shared repair run, once, before the build is retried.
//
// The repair is a whole-table delete, so it must never sit on the common
// path: an interrupted legacy run never reaches its CREATE, and treating a
// missing index as "repair needed" made every later process dedup the entire
// table again.
func ensureUniqueIndex(db *gorm.DB, spec indexSpec) {
	if db == nil || db.Dialect() == nil || !db.HasTable(spec.table) {
		return
	}
	if hasNamedIndex(db, spec.table, spec.name) {
		return
	}
	if err := createUniqueIndexRepairingDuplicates(db, spec); err != nil {
		log.Errorf("[unique-constraint] failed to ensure UNIQUE INDEX %s: %v", spec.name, err)
	}
}

// createUniqueIndexRepairingDuplicates builds the index and recovers from the
// one expected failure: rows that are already duplicated.
func createUniqueIndexRepairingDuplicates(db *gorm.DB, spec indexSpec) error {
	err := ensureIndex(db, spec)
	if err == nil {
		return nil
	}
	if !isUniqueViolation(err) {
		return err
	}

	removed, repairErr := removeDuplicateRows(db, spec)
	if repairErr != nil {
		return fmt.Errorf("remove duplicate rows: %w (index create failed with: %v)", repairErr, err)
	}
	if removed > 0 {
		log.Infof("[unique-constraint] removed %d duplicate rows blocking %s", removed, spec.name)
	}
	return ensureIndex(db, spec)
}

// removeDuplicateRows keeps the oldest row (MIN id) per unique key and returns
// how many rows it removed. The adapter renders the statement, so the repair
// works on every backend the adapter supports. The oldest row is what the
// single-read paths already return, so a repaired database behaves exactly as
// before.
func removeDuplicateRows(db *gorm.DB, spec indexSpec) (int64, error) {
	res := db.Exec(indexDialectFor(db).DeleteDuplicatesSQL(spec))
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

// isUniqueViolation reports whether err is the backend refusing to build a
// unique index over duplicate rows. The message is the backend's own error
// vocabulary; only that refusal may start the whole-table repair, so a
// permission error, a read-only credential or a lock timeout is returned as is.
//
// DM never reaches this path: its adapter installs the index set through
// installDMIRIndexes, which does not repair duplicates.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{
		"unique constraint",             // SQLite: UNIQUE constraint failed
		"could not create unique index", // PostgreSQL: SQLSTATE 23505
		"duplicate entry",               // MySQL: Duplicate entry ... for key
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}
