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
