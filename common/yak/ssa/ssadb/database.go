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
}

func init() {
	schema.RegisterDatabaseSchema(schema.KEY_SCHEMA_SSA_DATABASE, SSAProjectTables...)
}

func GetDB() *gorm.DB {
	return consts.GetGormDefaultSSADataBase()
}

func deleteProgramDBOnly(db *gorm.DB, program string) {
	// delete the program
	db.Model(&IrCode{}).Where("program_name = ?", program).Unscoped().Delete(&IrCode{})
	db.Model(&IrIndex{}).Where("program_name = ?", program).Unscoped().Delete(&IrIndex{})
	db.Model(&IrSource{}).Where("program_name = ?", program).Unscoped().Delete(&IrSource{})
	db.Model(&IrSource{}).Where("folder_path = ? AND file_name = ?", "/", program).Unscoped().Delete(&IrSource{})
	db.Model(&IrProgram{}).Where("program_name = ?", program).Unscoped().Delete(&IrProgram{})
}
