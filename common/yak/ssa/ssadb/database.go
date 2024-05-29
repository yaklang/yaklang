package ssadb

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
)

var SSAProjectTables = []any{
	// instruction
	&IrCode{},
	// instruction index by name or class-name
	&IrVariable{},
	// scope
	&IrScopeNode{},
	// source code, and type
	&IrSource{}, &IrType{},
}

func init() {
	schema.RegisterDatabaseSchema(schema.KEY_SCHEMA_SSA_DATABASE, SSAProjectTables...)
}

func GetDB() *gorm.DB {
	return consts.GetGormDefaultSSADataBase()
}

func DeleteProgram(db *gorm.DB, program string) {
	db.Model(&IrCode{}).Where("program_name = ?", program).Unscoped().Delete(&IrCode{})
	db.Model(&IrVariable{}).Where("program_name = ?", program).Unscoped().Delete(&IrVariable{})
	db.Model(&IrScopeNode{}).Where("program_name = ?", program).Unscoped().Delete(&IrScopeNode{})
}
