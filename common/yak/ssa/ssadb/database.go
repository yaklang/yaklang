package ssadb

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/damenggorm"
	"github.com/yaklang/yaklang/common/yak/ssa/reportstore"
)

var SSAProjectTables = []any{
	// instruction
	&IrCode{},
	&IrConst{},
	&IrIndex{},
	&IrNamePool{},
	// source code
	&IrSource{},
	// type
	&IrType{},
	// program
	&IrProgram{},
	&IrOffset{},

	// audit
	&AuditResult{},
	&AuditNode{},
	&AuditEdge{},
	&schema.SSARisk{},
	&schema.SSARiskDisposals{},

	&schema.SyntaxFlowScanTask{},

	// report
	&schema.ReportRecord{},
	&reportstore.SSAReportRecord{},
	&reportstore.SSAReportRecordFile{},

	//diff result
	&schema.SSADiffResult{},

	&schema.ProjectGeneralStorage{},
}

func init() {
	schema.RegisterDatabaseSchema(schema.KEY_SCHEMA_SSA_DATABASE, SSAProjectTables...)
	schema.RegisterDatabasePatch(schema.KEY_SCHEMA_SSA_DATABASE, patchIrSourceQuotedCode)
	schema.RegisterDatabasePatch(schema.KEY_SCHEMA_SSA_DATABASE, patchIrCodeIndex)
	schema.RegisterDatabasePatch(schema.KEY_SCHEMA_SSA_DATABASE, patchIrTypeIndexes)
	// gorm.Open copies gorm.DefaultCallback exactly once and never re-reads
	// it, so the timing callbacks have to be on the default before the SSA
	// database opens. Importing ssadb is what triggers the open.
	EnsureDBOpCallbacks(nil)
}

// patchIrSourceQuotedCode patches the QuotedCode column type based on database dialect
// MySQL: use LONGTEXT for large text storage (up to 4GB)
// PostgreSQL: use TEXT (unlimited length)
// SQLite: use TEXT (supports up to 2GB, no modification needed)
func patchIrSourceQuotedCode(db *gorm.DB) {
	if !db.HasTable(TableIrSources) {
		return
	}

	dialect := db.Dialect().GetName()
	switch dialect {
	case "mysql":
		// For MySQL, change TEXT to LONGTEXT to support larger source files
		// TEXT in MySQL is limited to ~64KB, but LONGTEXT can store up to 4GB
		err := db.Exec("ALTER TABLE " + TableIrSources + " MODIFY COLUMN quoted_code LONGTEXT").Error
		if err != nil {
			log.Warnf("failed to modify %s.quoted_code to LONGTEXT for MySQL: %v", TableIrSources, err)
		} else {
			log.Infof("MySQL: %s.quoted_code column type changed to LONGTEXT", TableIrSources)
		}
	case "postgres", "postgresql":
		// PostgreSQL TEXT type already supports unlimited length, no modification needed
		log.Debugf("PostgreSQL: %s.quoted_code uses TEXT type (unlimited length)", TableIrSources)
	case "sqlite3", "sqlite":
		// SQLite TEXT type supports up to 2GB (SQLITE_MAX_LENGTH), no modification needed
		log.Debugf("SQLite: %s.quoted_code uses TEXT type (up to 2GB)", TableIrSources)
	default:
		// For other databases, use default TEXT type
		log.Debugf("Database dialect %s: using default TEXT type for %s.quoted_code", dialect, TableIrSources)
	}
}

// doSSAPatch 添加数据库索引以优化查询性能
func patchIrCodeIndex(db *gorm.DB) {
	if damenggorm.IsDM(db) {
		if err := installDMIRIndexes(db); err != nil {
			db.AddError(err)
		}
		return
	}
	if !db.HasTable(TableIrCodes) {
		return
	}

	// 为 ir_codes 表添加复合索引 (program_name, code_id)
	// 这是最常见的查询模式: WHERE program_name = ? AND code_id IN (...)
	indexQueries := []struct {
		name  string
		query string
	}{
		{
			"idx_ir_codes_program_opcode",
			// composite index for program+opcode lookups
			`CREATE INDEX IF NOT EXISTS "idx_ir_codes_program_opcode" ON "` + TableIrCodes + `" ("program_name", "opcode");`,
		},
		// 为 ir_indices 表添加复合索引以优化常见查询
		{
			"idx_ir_indices_program_value",
			`CREATE INDEX IF NOT EXISTS "idx_ir_indices_program_value" ON "` + TableIrIndices + `" ("program_name", "value_id");`,
		},
		{
			"idx_ir_name_pool_program_name_name",
			`CREATE INDEX IF NOT EXISTS "idx_ir_name_pool_program_name_name" ON "` + TableIrNamePool + `" ("program_name", "name");`,
		},
	}

	for _, idx := range indexQueries {
		if err := db.Exec(idx.query).Error; err != nil {
			log.Warnf("failed to add index %s: %v", idx.name, err)
		}
	}

	// The IR unique keys are a hard database invariant: the async persist
	// pipeline upserts on them, and the write path relies on the database
	// rejecting a second row for the same key.
	for _, spec := range irUniqueIndexes {
		ensureUniqueIndex(db, spec)
	}
	ensureIrConstIndexes(db)

	// Supported SQLite and PostgreSQL databases shed these redundant indexes
	// without rewriting rows. Other dialects keep their legacy indexes until a
	// dialect-specific drop path is available.
	pruneRedundantIRIndexes(db)

}

// irCodeConstLookupIndex is the legacy covering lookup for ConstType searches.
// New databases write const text to ir_consts_v1 and never create this index;
// it survives only on databases written before the const table, where the
// legacy fallback query still uses it. Creating it again is intentionally not
// done: the partial predicate it needs is satisfied by short consts only, and
// a rebuilt index would be dead weight during every compile. The WHERE clause
// in constLookupSQL matches these literals; a bound opcode or const_type
// parameter makes SQLite ignore this index.
const irCodeConstLookupIndex = "idx_ir_codes_const_lookup"

// uniqueIndex is one unique key the SSA schema requires. Both IR indexes
// differ only in the table, the name, the DDL and the columns that must be
// unique, so a single maintenance template serves them.
type uniqueIndex struct {
	table        string
	name         string
	definition   string // idempotent CREATE UNIQUE INDEX
	duplicateKey string // partition key used only when a repair is needed
}

var irUniqueIndexes = []uniqueIndex{
	{
		table:        TableIrCodes,
		name:         "ux_ir_codes_program_code",
		definition:   `CREATE UNIQUE INDEX IF NOT EXISTS "ux_ir_codes_program_code" ON "` + TableIrCodes + `" ("program_name", "code_id");`,
		duplicateKey: `program_name, code_id`,
	},
	{
		table:      TableIrOffsets,
		name:       "ux_ir_offsets_program_value_file_range",
		definition: `CREATE UNIQUE INDEX IF NOT EXISTS "ux_ir_offsets_program_value_file_range" ON "` + TableIrOffsets + `" ("program_name", "value_id", "file_hash", "start_offset", "end_offset", COALESCE("variable_name", ''));`,
		// A NULL variable name and '' name the same location, so the index
		// treats them as one key. The duplicate key stays unquoted: MySQL reads
		// double quotes as string literals (ANSI_QUOTES is off by default), and
		// a constant partition would collapse every row into one group.
		duplicateKey: `program_name, value_id, file_hash, start_offset, end_offset, COALESCE(variable_name, '')`,
	},
}

// hasNamedIndex is the polymorphic seam of index maintenance: each backend
// answers from its own catalog through Dialect().HasIndex. SQLite reads
// sqlite_master, PostgreSQL reads pg_indexes filtered by CURRENT_SCHEMA(),
// MySQL reads SHOW INDEXES and DM reads ALL_INDEXES. No catalog name appears
// in this file, so an index that exists is always reported as existing.
// db.New() clears a program_name row scope while keeping the connection.
func hasNamedIndex(db *gorm.DB, table, index string) bool {
	if db == nil || db.Dialect() == nil {
		return false
	}
	return db.New().Dialect().HasIndex(table, index)
}

// removeDuplicateRows keeps the oldest row (MIN id) per unique key and returns
// how many rows it removed. Window functions are standard SQL, so every
// supported backend runs the same statement and only the partition key
// changes. The oldest row is what the single-read paths already return, so a
// repaired database behaves exactly as before.
func removeDuplicateRows(db *gorm.DB, spec uniqueIndex) (int64, error) {
	res := db.Exec(`DELETE FROM ` + spec.table + ` WHERE id IN (
		SELECT id FROM (
			SELECT id, ROW_NUMBER() OVER (PARTITION BY ` + spec.duplicateKey + ` ORDER BY id) AS rn
			FROM ` + spec.table + `
		) WHERE rn > 1
	)`)
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
// DM never reaches this path: patchIrCodeIndex installs its indexes through
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

// ensureUniqueIndex is the single maintenance template for every unique index:
//
//  1. hasNamedIndex asks the backend whether the index is already there. This
//     is the only dialect-specific step and it is delegated to the dialect.
//  2. The idempotent CREATE runs only on a database that really lacks it.
//  3. Only when the backend rejects that CREATE because duplicate rows exist
//     does the shared repair run, once, before the same CREATE is retried.
//
// The repair is a whole-table window delete, so it must never sit on the
// common path: an interrupted legacy run never reaches its CREATE, and
// treating a missing index as "repair needed" made every later process dedup
// the entire table again.
func ensureUniqueIndex(db *gorm.DB, spec uniqueIndex) {
	if db == nil || !db.HasTable(spec.table) {
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
func createUniqueIndexRepairingDuplicates(db *gorm.DB, spec uniqueIndex) error {
	err := db.Exec(spec.definition).Error
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
	return db.Exec(spec.definition).Error
}

func GetDB() *gorm.DB {
	db := consts.GetGormSSAProjectDataBase()
	return db
}

func SetDB(db *gorm.DB) {
	consts.SetGormSSAProjectDatabase(db)
}

func DeleteProgram(db *gorm.DB, program string) {
	if err := DeleteProgramChecked(db, program); err != nil {
		log.Errorf("delete program %q: %v", program, err)
	}
}

// DeleteProgramChecked retries each table independently and reports all
// failures after attempting every child table. The program identity is removed
// last, only after its children are gone, so node-driven cleanup can retry a
// partial deletion instead of mistaking it for an already-absent program.
func DeleteProgramChecked(db *gorm.DB, program string) error {
	if err := validateProgramDeletion(db, program); err != nil {
		return err
	}
	err := errors.Join(
		deleteProgramCodeOnly(db, program),
		deleteProgramAuditResult(db, program),
		deleteProgramRiskAndScanTask(db, program),
	)
	if err != nil {
		return err
	}
	return retryProgramDelete(TableIrPrograms, func() error {
		return db.Model(&IrProgram{}).Where("program_name = ?", program).Unscoped().Delete(&IrProgram{}).Error
	})
}

// DeleteProgramIrCode removes IR and dependent audit rows for recompilation.
// It returns the collected failures only after all tables have been attempted.
func DeleteProgramIrCode(db *gorm.DB, program string) error {
	if err := validateProgramDeletion(db, program); err != nil {
		return err
	}
	return errors.Join(
		deleteProgramCodeOnly(db, program),
		deleteProgramAuditResult(db, program), // audit results depend on IR code
	)
}

func deleteProgramCodeOnly(db *gorm.DB, program string) error {
	deleteCache(program)
	DropIrConstPool(program)
	DropIrConstValueCache(program)
	return errors.Join(
		deleteProgramRows(db, TableIrCodes, "program_name = ?", program),
		deleteOptionalProgramRows(db, TableIrConsts, "program_name = ?", program),
		deleteOptionalProgramRows(db, TableIrConstPool, "program_name = ?", program),
		deleteProgramRows(db, TableIrIndices, "program_name = ?", program),
		deleteProgramRows(db, TableIrNamePool, "program_name = ?", program),
		deleteProgramRows(db, TableIrSources, "program_name = ?", program),
		deleteProgramRows(db, TableIrSources, "folder_path = ? AND file_name = ?", "/", program),
		deleteProgramRows(db, TableIrTypes, "program_name = ?", program),
		deleteProgramRows(db, TableIrOffsets, "program_name = ?", program),
	)
}

func deleteProgramAuditResult(db *gorm.DB, program string) error {
	return errors.Join(
		deleteProgramRows(db, TableAuditResults, "program_name = ?", program),
		deleteProgramRows(db, TableAuditNodes, "program_name = ?", program),
		deleteProgramRows(db, TableAuditEdges, "program_name = ?", program),
	)
}

func deleteProgramRiskAndScanTask(db *gorm.DB, program string) error {
	return errors.Join(
		deleteProgramRows(db, schema.TableSSARisks, "program_name = ?", program),
		deleteProgramRows(db, schema.TableSyntaxFlowScanTask, "programs = ?", program),
	)
}

func validateProgramDeletion(db *gorm.DB, program string) error {
	if db == nil {
		return errors.New("SSA IR database is not configured")
	}
	if strings.TrimSpace(program) == "" {
		return errors.New("SSA IR program name is required")
	}
	return nil
}

// deleteOptionalProgramRows drops rows from a table that a database written by
// an older build may not have: the legacy const table is absent on new
// databases, and the const pool is absent on every database written before
// it. A missing table is not a failed delete, and it is a stable condition:
// retrying it only burns the backoff budget below.
func deleteOptionalProgramRows(db *gorm.DB, table, condition string, args ...any) error {
	err := deleteProgramRows(db, table, condition, args...)
	if err != nil && isMissingTableError(err) {
		return nil
	}
	return err
}

// isMissingTableError reports the stable "this schema has no such table" case.
// It is not transient, so a retry loop must not spend its backoff on it.
func isMissingTableError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no such table")
}

func deleteProgramRows(db *gorm.DB, table, condition string, args ...any) error {
	return retryProgramDelete(table, func() error {
		return db.Exec("DELETE FROM "+table+" WHERE "+condition, args...).Error
	})
}

const programDeleteAttempts = 3

func retryProgramDelete(name string, deleteFn func() error) error {
	var err error
	for attempt := 1; attempt <= programDeleteAttempts; attempt++ {
		if err = deleteFn(); err == nil {
			return nil
		}
		// A missing table will not appear on the next attempt. Returning at
		// once keeps the caller's "optional table" handling cheap: a schema
		// without the legacy const table used to spend 3 attempts and 150ms of
		// backoff on every program delete.
		if isMissingTableError(err) {
			return fmt.Errorf("%s: %w", name, err)
		}
		if attempt < programDeleteAttempts {
			time.Sleep(time.Duration(attempt) * 50 * time.Millisecond)
		}
	}
	return fmt.Errorf("%s: delete failed after %d attempts: %w", name, programDeleteAttempts, err)
}

// redundantIRIndexes are single-column B-trees that a composite index already
// answers, or that no IR query filters on. They cost a B-tree build on every
// compile and, on a Hue-sized database, about 860MB of the file. The model
// tags no longer create them; this list prunes databases created by older
// binaries. Removing a redundant index preserves every row and also improves
// writes when a populated database is recompiled.
var redundantIRIndexes = []struct{ table, index string }{
	{TableIrCodes, "idx_ir_codes_program_name"},           // ux_ir_codes_program_code is (program_name, code_id)
	{TableIrCodes, "idx_ir_codes_code_id"},                // every lookup also binds program_name
	{TableIrCodes, "idx_ircode_program_opcode"},           // idx_ir_codes_program_opcode is (program_name, opcode)
	{TableIrCodes, "idx_ir_codes_version"},                // version lives on ir_programs
	{TableIrCodes, "idx_ir_codes_program_compile_hash"},   // never a predicate
	{TableIrCodes, "idx_ir_codes_const_type"},             // const lookups use idx_ir_codes_const_lookup
	{TableIrOffsets, "idx_ir_offsets_program_name"},       // ux_ir_offsets_program_value_file_range prefix
	{TableIrOffsets, "idx_ir_offsets_value_id"},           // the same UNIQUE index covers (program_name, value_id)
	{TableIrOffsets, "idx_ir_offsets_start_offset"},       // never a predicate
	{TableIrIndices, "idx_ir_indices_v1_program_name"},    // idx_ir_indices_program_value is (program_name, value_id)
	{TableIrIndices, "idx_ir_indices_v1_value_id"},        // the composite covers it
	{TableIrIndices, "idx_ir_indices_v1_owner_value_id"},  // never a predicate
	{TableIrIndices, "idx_ir_indices_v1_version_id"},      // read, never filtered
	{TableIrNamePool, "idx_ir_name_pool_v1_program_name"}, // idx_ir_name_pool_program_name_name prefix
	{TableIrNamePool, "idx_ir_name_pool_v1_name"},         // name lookups bind program_name too
	// deleted_at is NULL on every IR row (the IR tables are never soft
	// deleted), and every IR query binds program_name first, so these
	// B-trees have no selectivity and are never chosen. They come from
	// gorm.Model's `sql:"index"` tag, which IRModel drops.
	{TableIrCodes, "idx_ir_codes_deleted_at"},
	{TableIrOffsets, "idx_ir_offsets_deleted_at"},
	{TableIrIndices, "idx_ir_indices_v1_deleted_at"},
	{TableIrTypes, "idx_ir_types_v2_deleted_at"},
	// GetScope is the only scope_name reader and it is dead code
	// (GetScopeFromIrScopeName returns "not implemented").
	{TableIrIndices, "idx_ir_indices_v1_scope_name"},
	// applyMatchCondition builds an OR over variable_id/class_id/field_id,
	// but yieldFromIrIndex always adds program_name, so the planner uses
	// idx_ir_indices_program_value for every production query.
	{TableIrIndices, "idx_ir_indices_v1_variable_id"},
	{TableIrIndices, "idx_ir_indices_v1_class_id"},
	{TableIrIndices, "idx_ir_indices_v1_field_id"},
	// offsetVariableQuery adds a unary "+" to variable_name on purpose: the
	// index makes SQLite pick a scan of every instruction named "this"
	// (about 40ms per lookup instead of 20us through the value prefix).
	{TableIrOffsets, "idx_ir_offsets_variable_name"},
	// GetValueBeforeEndOffset is the only end_offset reader; with the
	// file_hash index present SQLite picks file_hash (4ms against 77ms).
	{TableIrOffsets, "idx_ir_offsets_end_offset"},
}

func pruneRedundantIRIndexes(db *gorm.DB) {
	if db == nil || db.Dialect() == nil {
		return
	}
	dialect := db.Dialect().GetName()
	if dialect != "sqlite3" && dialect != "sqlite" && dialect != "postgres" && dialect != "postgresql" {
		return
	}
	dropped := 0
	for _, redundant := range redundantIRIndexes {
		if !hasNamedIndex(db, redundant.table, redundant.index) {
			continue
		}
		if err := db.Exec(`DROP INDEX IF EXISTS ` + quoteSQLIdent(redundant.index)).Error; err != nil {
			log.Warnf("[ir-index-prune] drop %s: %v", redundant.index, err)
			continue
		}
		dropped++
	}
	if dropped > 0 {
		log.Infof("[ir-index-prune] removed %d redundant IR indexes", dropped)
	}
}

// patchIrTypeIndexes installs the UNIQUE key the type writes conflict on.
func patchIrTypeIndexes(db *gorm.DB) {
	if db == nil || db.Dialect() == nil {
		return
	}
	if damenggorm.IsDM(db) {
		// installDMIRIndexes creates this key for the DM dialect.
		return
	}
	if err := ensureIrTypeIndexes(db); err != nil {
		log.Warnf("[ir-type] index patch: %v", err)
	}
}
