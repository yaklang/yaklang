package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
)

func DeleteRiskByProgram(DB *gorm.DB, programNames []string) error {
	db := DB.Model(&schema.Risk{})
	db = bizhelper.ExactOrQueryStringArrayOr(db, "program_name", programNames)
	if db := db.Unscoped().Delete(&schema.Risk{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func DeleteRiskBySFResult(DB *gorm.DB, resultIDs []int64) error {
	db := DB.Model(&schema.Risk{})
	db = bizhelper.ExactQueryInt64ArrayOr(db, "result_id", resultIDs)
	if db := db.Unscoped().Delete(&schema.Risk{}); db.Error != nil {
		return db.Error
	}
	return nil
}
