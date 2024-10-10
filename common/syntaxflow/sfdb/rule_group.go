package sfdb

import (
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func CreateOrUpdateSyntaxFlowGroup(hash string,i *schema.SyntaxFlowRuleGroup) error {
	db := consts.GetGormProfileDatabase()
	db = db.Model(&schema.SyntaxFlowRuleGroup{})
	if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(&schema.SyntaxFlowRuleGroup{}); db.Error != nil {
		return utils.Errorf("create/update SyntaxFlowGroup failed: %s", db.Error)
	}
	return nil
}

