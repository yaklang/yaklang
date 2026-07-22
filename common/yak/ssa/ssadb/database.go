package ssadb

import (
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
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
}

func GetDB() *gorm.DB {
	return consts.GetGormSSAProjectDataBase()
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
