package syntaxflow_utils

import (
	"github.com/jinzhu/gorm"

	"github.com/yaklang/yaklang/common/consts"
)

// GetSSADB returns the GORM handle for the SSA project / engine database
// (SyntaxFlowScanTask, SSARisk, compiled programs, etc.), as configured in consts.
// This is not the Yakit profile/business database.
func GetSSADB() *gorm.DB {
	return consts.GetGormSSAProjectDataBase()
}
