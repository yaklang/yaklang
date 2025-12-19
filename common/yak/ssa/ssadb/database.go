package ssadb

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

var SSAProjectTables = []any{
	// instruction
	&IrCode{},
	&IrIndex{},
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

	//diff result
	&schema.SSADiffResult{},

	&schema.ProjectGeneralStorage{},

	&schema.ReportRecord{},
}

func init() {
	schema.RegisterDatabaseSchema(schema.KEY_SCHEMA_SSA_DATABASE, SSAProjectTables...)
	schema.RegisterDatabasePatch(schema.KEY_SCHEMA_SSA_DATABASE, patchIrSourceQuotedCode)
}

// patchIrSourceQuotedCode patches the QuotedCode column type based on database dialect
// MySQL: use LONGTEXT for large text storage (up to 4GB)
// PostgreSQL: use TEXT (unlimited length)
// SQLite: use TEXT (supports up to 2GB, no modification needed)
func patchIrSourceQuotedCode(db *gorm.DB) {
	if !db.HasTable("ir_sources") {
		return
	}

	dialect := db.Dialect().GetName()
	switch dialect {
	case "mysql":
		// For MySQL, change TEXT to LONGTEXT to support larger source files
		// TEXT in MySQL is limited to ~64KB, but LONGTEXT can store up to 4GB
		err := db.Exec("ALTER TABLE ir_sources MODIFY COLUMN quoted_code LONGTEXT").Error
		if err != nil {
			log.Warnf("failed to modify ir_sources.quoted_code to LONGTEXT for MySQL: %v", err)
		} else {
			log.Infof("MySQL: ir_sources.quoted_code column type changed to LONGTEXT")
		}
	case "postgres", "postgresql":
		// PostgreSQL TEXT type already supports unlimited length, no modification needed
		log.Debugf("PostgreSQL: ir_sources.quoted_code uses TEXT type (unlimited length)")
	case "sqlite3", "sqlite":
		// SQLite TEXT type supports up to 2GB (SQLITE_MAX_LENGTH), no modification needed
		log.Debugf("SQLite: ir_sources.quoted_code uses TEXT type (up to 2GB)")
	default:
		// For other databases, use default TEXT type
		log.Debugf("Database dialect %s: using default TEXT type for ir_sources.quoted_code", dialect)
	}
}
func GetDB() *gorm.DB {
	return consts.GetGormSSAProjectDataBase()
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
	// delete the program
	// code
	deleteCache(program)
	db.Model(&IrCode{}).Where("program_name = ?", program).Unscoped().Delete(&IrCode{})
	db.Model(&IrIndex{}).Where("program_name = ?", program).Unscoped().Delete(&IrIndex{})
	db.Model(&IrSource{}).Where("program_name = ?", program).Unscoped().Delete(&IrSource{})
	db.Model(&IrSource{}).Where("folder_path = ? AND file_name = ?", "/", program).Unscoped().Delete(&IrSource{})
	db.Model(&IrType{}).Where("program_name = ?", program).Unscoped().Delete(&IrType{})
	db.Model(&IrOffset{}).Where("program_name = ?", program).Unscoped().Delete(&IrOffset{})
}

func deleteProgramAuditResult(db *gorm.DB, program string) {
	// analyze result
	db.Model(&AuditResult{}).Where("program_name = ?", program).Unscoped().Delete(&AuditResult{})
	db.Model(&AuditNode{}).Where("program_name = ?", program).Unscoped().Delete(&AuditNode{})
	db.Model(&AuditEdge{}).Where("program_name = ?", program).Unscoped().Delete(&AuditEdge{})
}

func deleteProgramRiskAndScanTask(db *gorm.DB, program string) {
	// risk and scan task
	db.Model(&schema.SSARisk{}).Where("program_name = ?", program).Unscoped().Delete(&schema.SSARisk{})
	db.Model(&schema.SyntaxFlowScanTask{}).Where("programs = ?", program).Unscoped().Delete(&schema.SyntaxFlowScanTask{})
}
