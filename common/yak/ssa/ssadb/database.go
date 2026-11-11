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

	// Add UNIQUE constraint on (program_name, code_id) to prevent
	// duplicate instruction INSERTs from race conditions in the async
	// persist pipeline. This is a hard database invariant.
	ensureUniqueIrCodesProgramCodeIndex(db)
	ensureUniqueIrOffsetsIndex(db)
	ensureConstLookupIndex(db)

	// Supported SQLite and PostgreSQL databases shed these redundant indexes
	// without rewriting rows. Other dialects keep their legacy indexes until a
	// dialect-specific drop path is available.
	pruneRedundantIRIndexes(db)

}

// irCodeConstLookupIndex is the covering lookup for ConstType searches.
// Equality seeks (program_name, string). A regexp uses the program_name
// prefix and filters strings in the index, instead of reading every wide
// ir_codes row. The WHERE matches constLookupSQL's literals; a bound
// opcode or const_type parameter makes SQLite ignore this index.
const irCodeConstLookupIndex = "idx_ir_codes_const_lookup"

func ensureConstLookupIndex(db *gorm.DB) {
	if db == nil || db.Dialect() == nil || !db.HasTable(TableIrCodes) {
		return
	}
	if hasNamedIndex(db, TableIrCodes, irCodeConstLookupIndex) {
		return
	}
	switch db.Dialect().GetName() {
	case "sqlite3", "sqlite", "postgres", "postgresql", "cloudsqlpostgres":
	default:
		log.Debugf("skip const lookup index on dialect %s", db.Dialect().GetName())
		return
	}
	query := fmt.Sprintf(
		`CREATE INDEX IF NOT EXISTS "%s" ON "%s" ("program_name", "string", "code_id") WHERE opcode = %d AND const_type = '%s' AND deleted_at IS NULL`,
		irCodeConstLookupIndex, TableIrCodes, irCodeConstOpcode, irCodeConstTypeNormal,
	)
	if err := db.Exec(query).Error; err != nil {
		log.Warnf("failed to add index %s: %v", irCodeConstLookupIndex, err)
	}
}

// hasNamedIndex asks the dialect whether an index exists. db.New() clears a
// program_name row scope while keeping the current connection.
func hasNamedIndex(db *gorm.DB, table, index string) bool {
	if db == nil || db.Dialect() == nil {
		return false
	}
	return db.New().Dialect().HasIndex(table, index)
}

// deleteDuplicateIrCodes removes every row except the oldest (MIN id) per
// (program_name, code_id) using a single window query, and returns the number
// of removed rows. The oldest row is exactly what normal single-read paths
// return, so existing databases keep behaving identically after the cleanup.
func deleteDuplicateIrCodes(db *gorm.DB) (int64, error) {
	res := db.Exec(`DELETE FROM ` + TableIrCodes + ` WHERE id IN (
		SELECT id FROM (
			SELECT id, ROW_NUMBER() OVER (PARTITION BY program_name, code_id ORDER BY id) AS rn
			FROM ` + TableIrCodes + `
		) WHERE rn > 1
	)`)
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

// deleteDuplicateIrOffsets removes every row except the oldest (MIN id) per
// composite key using a single window query, and returns the number of
// removed rows.
func deleteDuplicateIrOffsets(db *gorm.DB) (int64, error) {
	res := db.Exec(`DELETE FROM ` + TableIrOffsets + ` WHERE id IN (
		SELECT id FROM (
			SELECT id, ROW_NUMBER() OVER (
				PARTITION BY program_name, value_id, file_hash, start_offset, end_offset, COALESCE(variable_name, '')
				ORDER BY id
			) AS rn
			FROM ` + TableIrOffsets + `
		) WHERE rn > 1
	)`)
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

// ensureUniqueIrCodesProgramCodeIndex creates a UNIQUE INDEX on
// (program_name, code_id) for the ir_codes table. If duplicate rows already
// exist, the extras are removed (keeping MIN(id)) before the index is
// created, so legacy databases upgrade cleanly.
func ensureUniqueIrCodesProgramCodeIndex(db *gorm.DB) {
	if !db.HasTable(TableIrCodes) {
		return
	}

	indexName := "ux_ir_codes_program_code"

	// Check if the unique index already exists
	var exists int64
	db.Raw(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND tbl_name='ir_codes' AND name=?`, indexName).Row().Scan(&exists)
	if exists > 0 {
		return // already created
	}

	// Legacy async-persist runs can leave duplicate rows. Remove every row
	// except the oldest (MIN id) per (program_name, code_id) in one window
	// query, so the unique index can be created. This runs only once: after
	// the index exists the function returns at the top.
	removed, err := deleteDuplicateIrCodes(db)
	if err != nil {
		log.Warnf("[unique-constraint] failed to remove duplicate ir_codes rows: %v", err)
		return
	}
	if removed > 0 {
		log.Infof("[unique-constraint] removed %d duplicate ir_codes rows (kept MIN(id) per (program_name, code_id))", removed)
	}

	// No duplicates — safe to create the UNIQUE INDEX
	query := `CREATE UNIQUE INDEX IF NOT EXISTS "ux_ir_codes_program_code" ON "` + TableIrCodes + `" ("program_name", "code_id");`
	if err := db.Exec(query).Error; err != nil {
		log.Errorf("[unique-constraint] failed to create UNIQUE INDEX %s: %v", indexName, err)
	} else {
		log.Infof("[unique-constraint] created UNIQUE INDEX %s on ir_codes (program_name, code_id)", indexName)
	}
}

// ensureUniqueIrOffsetsIndex creates a UNIQUE INDEX for ir_offsets that treats
// NULL variable names as empty strings. This prevents duplicate offset INSERTs.
//
// If an older non-COALESCE index with the same name exists, it is dropped and
// recreated with COALESCE. If duplicate rows exist, the extras are removed
// (keeping MIN(id)) before the index is created.
func ensureUniqueIrOffsetsIndex(db *gorm.DB) {
	if !db.HasTable(TableIrOffsets) {
		return
	}

	indexName := "ux_ir_offsets_program_value_file_range"

	// Check if the index exists and whether its SQL uses COALESCE
	type idxInfo struct {
		Name string
		SQL  string
	}
	var existing []idxInfo
	rows, err := db.Raw(
		`SELECT name, sql FROM sqlite_master WHERE type='index' AND tbl_name='ir_offsets' AND name=?`,
		indexName,
	).Rows()
	if err == nil {
		for rows.Next() {
			var info idxInfo
			rows.Scan(&info.Name, &info.SQL)
			existing = append(existing, info)
		}
		rows.Close()
	}

	// If the index exists with COALESCE, it's up to date — nothing to do
	for _, info := range existing {
		if strings.Contains(strings.ToUpper(info.SQL), "COALESCE") {
			return // already the COALESCE version
		}
	}

	// If the index exists WITHOUT COALESCE, drop it so we can recreate
	if len(existing) > 0 {
		log.Infof("[unique-constraint] dropping old non-COALESCE index %s to upgrade", indexName)
		if err := db.Exec("DROP INDEX IF EXISTS " + indexName).Error; err != nil {
			log.Errorf("[unique-constraint] failed to drop old index %s: %v", indexName, err)
			return
		}
	}

	// Legacy async-persist runs can leave duplicate rows. Remove every row
	// except the oldest (MIN id) per composite key in one window query, so
	// the unique index can be created.
	removed, err := deleteDuplicateIrOffsets(db)
	if err != nil {
		log.Warnf("[unique-constraint] failed to remove duplicate ir_offsets rows: %v", err)
		return
	}
	if removed > 0 {
		log.Infof("[unique-constraint] removed %d duplicate ir_offsets rows (kept MIN(id) per composite key)", removed)
	}

	query := `CREATE UNIQUE INDEX IF NOT EXISTS "ux_ir_offsets_program_value_file_range" ON "` + TableIrOffsets + `" ("program_name", "value_id", "file_hash", "start_offset", "end_offset", COALESCE("variable_name", ''));`
	if err := db.Exec(query).Error; err != nil {
		log.Errorf("[unique-constraint] failed to create UNIQUE INDEX %s: %v", indexName, err)
	} else {
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
	return errors.Join(
		deleteProgramRows(db, TableIrCodes, "program_name = ?", program),
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
