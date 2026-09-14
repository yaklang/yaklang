package ssadb

import (
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/reportstore"
	"strings"
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
			"idx_ir_codes_program_code",
			`CREATE INDEX IF NOT EXISTS "idx_ir_codes_program_code" ON "` + TableIrCodes + `" ("program_name", "code_id");`,
		},
		{
			"idx_ir_codes_program_opcode",
			// composite index for program+opcode lookups
			`CREATE INDEX IF NOT EXISTS "idx_ir_codes_program_opcode" ON "` + TableIrCodes + `" ("program_name", "opcode");`,
		},
		// 为 ir_types 表添加复合索引
		{
			"idx_ir_types_program_type",
			`CREATE INDEX IF NOT EXISTS "idx_ir_types_program_type" ON "` + TableIrTypes + `" ("program_name", "type_id");`,
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

// ensureUniqueIrOffsetsIndex creates a UNIQUE INDEX with COALESCE on
// (program_name, value_id, file_hash, start_offset, end_offset, COALESCE(variable_name, ''))
// for the ir_offsets table. This prevents duplicate offset INSERTs.
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
	utils.GormTransaction(db, func(tx *gorm.DB) error {
		tx.Model(&IrProgram{}).Where("program_name = ?", program).Unscoped().Delete(&IrProgram{})
		deleteProgramCodeOnly(tx, program)
		deleteProgramAuditResult(tx, program)
		deleteProgramRiskAndScanTask(tx, program)
		return nil
	})
}

func DeleteProgramIrCode(db *gorm.DB, program string) {
	utils.GormTransaction(db, func(tx *gorm.DB) error {
		deleteProgramCodeOnly(tx, program)
		deleteProgramAuditResult(tx, program) // because audit result depends on ir code
		return nil
	})
}

func deleteProgramCodeOnly(db *gorm.DB, program string) {
	deleteCache(program)
	// Batch all DELETEs into a single Exec call to reduce round-trips.
	// Each DELETE is still a separate statement but they're sent in one
	// batch to SQLite, cutting 7 round-trips to 1.
	db.Exec(`DELETE FROM `+TableIrCodes+` WHERE program_name = ?;
DELETE FROM `+TableIrIndices+` WHERE program_name = ?;
DELETE FROM `+TableIrNamePool+` WHERE program_name = ?;
DELETE FROM `+TableIrSources+` WHERE program_name = ?;
DELETE FROM `+TableIrSources+` WHERE folder_path = ? AND file_name = ?;
DELETE FROM `+TableIrTypes+` WHERE program_name = ?;
DELETE FROM `+TableIrOffsets+` WHERE program_name = ?;`,
		program, program, program, program, "/", program, program, program)
}

func deleteProgramAuditResult(db *gorm.DB, program string) {
	db.Exec(`DELETE FROM `+TableAuditResults+` WHERE program_name = ?;
DELETE FROM `+TableAuditNodes+` WHERE program_name = ?;
DELETE FROM `+TableAuditEdges+` WHERE program_name = ?;`,
		program, program, program)
}

func deleteProgramRiskAndScanTask(db *gorm.DB, program string) {
	db.Exec(`DELETE FROM `+schema.TableSSARisks+` WHERE program_name = ?;
DELETE FROM `+schema.TableSyntaxFlowScanTask+` WHERE programs = ?;`,
		program, program)
}
