package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
)

func DeleteSsaDiffResultById(id int) {
	db := consts.GetGormDefaultSSADataBase()
	db.Model(&schema.SSADiffResult{}).Delete(&schema.SSADiffResult{
		Model: &gorm.Model{
			ID: uint(id),
		},
	})
}
func DeleteSsaDiffByHash(hash string) {
	db := consts.GetGormDefaultSSADataBase()
	db.Model(&schema.SSADiffResult{}).Where("result_hash = ?", hash).Delete(&schema.SSADiffResult{ResultHash: hash})
}
