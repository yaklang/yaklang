package ssadb

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
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

	&schema.SyntaxFlowScanTask{},

	// report
	&schema.ReportRecord{},
}

func init() {
	schema.RegisterDatabaseSchema(schema.KEY_SCHEMA_SSA_DATABASE, SSAProjectTables...)
}
func GetDB() *gorm.DB {
	return consts.GetGormDefaultSSADataBase().Debug()
}

func DeleteProgram(db *gorm.DB, program string) {
	db.Model(&IrProgram{}).Where("program_name = ?", program).Unscoped().Delete(&IrProgram{})
	deleteProgramCodeOnly(db, program)
	deleteProgramAuditResult(db, program)
	deleteProgramRiskAndScanTask(db, program)
}

func DeleteProgramIrCode(db *gorm.DB, program string) {
	deleteProgramCodeOnly(db, program)
	deleteProgramAuditResult(db, program) // because audit result depends on ir code
}

func deleteProgramCodeOnly(db *gorm.DB, program string) {
	// delete the program
	// code
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
