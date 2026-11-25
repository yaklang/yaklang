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
		err := nativeExec(db, "ALTER TABLE "+QuoteIdent(db, TableIrSources)+" MODIFY COLUMN "+QuoteIdent(db, "quoted_code")+" LONGTEXT")
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
	indexSpecs := []IndexSpec{
		// composite index for program+opcode lookups
		{Name: "idx_ir_codes_program_opcode", Table: TableIrCodes, Columns: []string{"program_name", "opcode"}},
		// 为 ir_indices 表添加复合索引以优化常见查询
		{Name: "idx_ir_indices_program_value", Table: TableIrIndices, Columns: []string{"program_name", "value_id"}},
		{Name: "idx_ir_name_pool_program_name_name", Table: TableIrNamePool, Columns: []string{"program_name", "name"}},
	}

	for _, spec := range indexSpecs {
		if err := EnsureIndex(db, spec); err != nil {
			log.Warnf("failed to add index %s: %v", spec.Name, err)
		}
	}

	// Add UNIQUE constraint on (program_name, code_id) to prevent
	// duplicate instruction INSERTs from race conditions in the async
	// persist pipeline. This is a hard database invariant.
	ensureUniqueIrCodesProgramCodeIndex(db)
	ensureUniqueIrOffsetsIndex(db)
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

// hasNamedIndex asks the dialect whether an index exists. db.New() clears a
// program_name row scope while keeping the current connection.
func hasNamedIndex(db *gorm.DB, table, index string) bool {
	if db == nil || db.Dialect() == nil {
		return false
	}
	return db.New().Dialect().HasIndex(table, index)
}

// namedIndexDefinition returns the stored CREATE INDEX text for dialects whose
// catalog is readable. SQLite keeps it in sqlite_master; PostgreSQL exposes it
// through pg_indexes. MySQL and DM have no portable equivalent, so callers
// fall back to HasIndex and leave an existing index untouched.
func namedIndexDefinition(db *gorm.DB, table, index string) (string, bool) {
	if db == nil || db.Dialect() == nil {
		return "", false
	}
	var query string
	switch dialectOf(db) {
	case sqlDialectSQLite:
		query = `SELECT sql FROM sqlite_master WHERE type = 'index' AND tbl_name = ? AND name = ?`
	case sqlDialectPostgres:
		query = `SELECT indexdef FROM pg_indexes WHERE schemaname = current_schema() AND tablename::text = ? AND indexname::text = ?`
	default:
		return "", false
	}
	var ddl string
	row := nativeQueryRowContext(db, nil, query, table, index)
	if row == nil {
		return "", false
	}
	err := row.Scan(&ddl)
	return ddl, err == nil && strings.TrimSpace(ddl) != ""
}

// tableColumnType is the catalog read used by migrations and conformance
// checks. Callers must not query sqlite_master, pg_indexes, information_schema
// or ALL_TAB_COLUMNS directly; each dialect exposes a different catalog shape.
func tableColumnType(db *gorm.DB, table, column string) (string, bool) {
	if db == nil || db.Dialect() == nil {
		return "", false
	}
	if dialectOf(db) == sqlDialectSQLite {
		rows, err := nativeQueryContextDB(db, nil, `PRAGMA table_info(`+QuoteIdent(db, table)+`)`)
		if err != nil {
			return "", false
		}
		defer rows.Close()
		for rows.Next() {
			var cid, notNull, primaryKey int
			var name, columnType string
			var defaultValue interface{}
			if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
				return "", false
			}
			if strings.EqualFold(name, column) {
				return strings.ToLower(strings.TrimSpace(columnType)), true
			}
		}
		return "", false
	}
	var query string
	args := []interface{}{table, column}
	switch dialectOf(db) {
	case sqlDialectMySQL:
		query = `SELECT DATA_TYPE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?`
	case sqlDialectPostgres:
		query = `SELECT data_type FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`
	case sqlDialectDM:
		query = `SELECT DATA_TYPE FROM ALL_TAB_COLUMNS WHERE OWNER = SYS_CONTEXT('USERENV','CURRENT_SCHEMA') AND TABLE_NAME = ? AND COLUMN_NAME = ?`
		args = []interface{}{strings.ToUpper(table), strings.ToUpper(column)}
	default:
		return "", false
	}
	var columnType string
	row := nativeQueryRowContext(db, nil, query, args...)
	if row == nil {
		return "", false
	}
	if err := row.Scan(&columnType); err != nil {
		return "", false
	}
	return strings.ToLower(strings.TrimSpace(columnType)), true
}

// ensureUniqueIrCodesProgramCodeIndex creates a UNIQUE INDEX on
// (program_name, code_id) for the ir_codes table.
//
// Duplicate rows from a legacy database are deliberately left in place: if
// they exist, CREATE UNIQUE INDEX fails and the error is logged. Deleting rows
// automatically would hide the write bug that produced them and discard data
// an operator may still need to inspect.
func ensureUniqueIrCodesProgramCodeIndex(db *gorm.DB) {
	if !db.HasTable(TableIrCodes) {
		return
	}

	indexName := "ux_ir_codes_program_code"
	existed := hasNamedIndex(db, TableIrCodes, indexName)
	spec := IndexSpec{
		Name:    indexName,
		Table:   TableIrCodes,
		Columns: []string{"program_name", "code_id"},
		Unique:  true,
	}
	if err := EnsureIndex(db, spec); err != nil {
		log.Errorf("[unique-constraint] failed to create UNIQUE INDEX %s: %v", indexName, err)
	} else if !existed {
		log.Infof("[unique-constraint] created UNIQUE INDEX %s on ir_codes (program_name, code_id)", indexName)
	}
}

// ensureUniqueIrOffsetsIndex creates a UNIQUE INDEX for ir_offsets that treats
// NULL variable names as empty strings. This prevents duplicate offset INSERTs.
//
// If an older non-COALESCE index with the same name exists, it is dropped and
// recreated with COALESCE. Duplicate rows from a legacy database are left in
// place; index creation then fails and the error is logged instead of deleting
// rows.
func ensureUniqueIrOffsetsIndex(db *gorm.DB) {
	if !db.HasTable(TableIrOffsets) {
		return
	}

	indexName := "ux_ir_offsets_program_value_file_range"

	// SQLite and PostgreSQL expose the stored index definition, so a legacy
	// non-COALESCE index can be recognized and upgraded. Other dialects ask
	// HasIndex once and, if it is absent, create the COALESCE form directly.
	if ddl, ok := namedIndexDefinition(db, TableIrOffsets, indexName); ok {
		if strings.Contains(strings.ToUpper(ddl), "COALESCE") {
			return // already the COALESCE version
		}
		log.Infof("[unique-constraint] dropping old non-COALESCE index %s to upgrade", indexName)
		if err := nativeExec(db, "DROP INDEX IF EXISTS "+QuoteIdent(db, indexName)); err != nil {
			log.Errorf("[unique-constraint] failed to drop old index %s: %v", indexName, err)
			return
		}
	} else if hasNamedIndex(db, TableIrOffsets, indexName) {
		return
	}

	existed := hasNamedIndex(db, TableIrOffsets, indexName)
	spec := IndexSpec{
		Name:        indexName,
		Table:       TableIrOffsets,
		Columns:     []string{"program_name", "value_id", "file_hash", "start_offset", "end_offset"},
		Expressions: []string{"COALESCE(" + QuoteIdent(db, "variable_name") + ", '')"},
		Unique:      true,
		// MySQL counts program_name and file_hash as VARCHAR(255); with the
		// COALESCE expression the full key would exceed the 3072-byte limit.
		Prefixes: map[string]int{"program_name": 191, "file_hash": 64},
	}
	if err := EnsureIndex(db, spec); err != nil {
		log.Errorf("[unique-constraint] failed to create UNIQUE INDEX %s: %v", indexName, err)
	} else if !existed {
		log.Infof("[unique-constraint] created UNIQUE INDEX %s on ir_offsets (program_name, value_id, file_hash, start_offset, end_offset, COALESCE(variable_name, ''))", indexName)
	}
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
	// Skip the query when the table is absent. Relying on the missing-table
	// error alone still makes the ORM log a red error for a condition that is
	// expected on new databases.
	if !db.HasTable(table) {
		return nil
	}
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
		return nativeExec(db, "DELETE FROM "+QuoteIdent(db, table)+" WHERE "+condition, args...)
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
	// The SQLite offset query adds a unary "+" to variable_name on purpose: the
	// index makes SQLite pick a scan of every instruction named "this"
	// (about 40ms per lookup instead of 20us through the value prefix).
	// PostgreSQL rejects that syntax, so other dialects keep the plain column.
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
		if err := nativeExec(db, `DROP INDEX IF EXISTS `+QuoteIdent(db, redundant.index)); err != nil {
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
