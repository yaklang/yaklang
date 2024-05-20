package ssadb

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
)

var SSAProjectTables = []any{
	&IrCode{}, &IrVariable{},
	&IrScopeNode{}, &IrSource{},
}

func init() {
	consts.RegisterDatabaseSchema(consts.KEY_SCHEMA_SSA_DATABASE, SSAProjectTables...)
}

func GetDB() *gorm.DB {
	return consts.GetGormDefaultSSADataBase()
}
