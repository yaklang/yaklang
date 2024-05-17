package ssadb

import (
	"sync"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
)

var SSAProjectTables = []any{
	&IrCode{}, &IrVariable{},
	&IrScopeNode{}, &IrSource{},
}

var (
	ssaProjectDB *gorm.DB
	Once         = new(sync.Once)
)

func GetDB() *gorm.DB {
	Once.Do(func() {
		ssaProjectDB = consts.GetGormDefaultSSADataBase()
		log.Info("init ssa project db")
		ssaProjectDB.AutoMigrate(SSAProjectTables...)
	})
	return ssaProjectDB
}
