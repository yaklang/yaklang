package sfdb

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
)

// CreateDefaultRuleLoader 创建默认的规则加载器（数据库）
func CreateDefaultRuleLoader(db *gorm.DB) RuleLoader {
	if db == nil {
		db = consts.GetGormProfileDatabase()
	}
	return NewDBRuleLoader(db)
}
