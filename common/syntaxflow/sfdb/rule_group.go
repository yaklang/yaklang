package sfdb

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func CreateOrUpdateSyntaxFlowGroup(hash string, i *schema.SyntaxFlowRuleGroup) error {
	db := consts.GetGormProfileDatabase()
	db = db.Model(&schema.SyntaxFlowRuleGroup{})
	if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(&schema.SyntaxFlowRuleGroup{}); db.Error != nil {
		return utils.Errorf("create/update SyntaxFlowGroup failed: %s", db.Error)
	}
	return nil
}

func InitSFBuildInGroup(db *gorm.DB, group string) error {
	db = db.Model(&schema.SyntaxFlowGroup{})
	if group == "" {
		return utils.Errorf("add syntax flow rule group failed:group name is empty")
	}
	i := &schema.SyntaxFlowGroup{
		GroupName: group,
		IsBuildIn: true,
	}

	var count int64
	db.Where("group_name = ?", group).Count(&count)
	if count > 0 {
		return nil
	}
	if db = db.Create(i); db.Error != nil {
		return utils.Errorf("create SyntaxFlowGroup failed: %s", db.Error)
	}
	return nil
}

func QuerySFDefaultGroup(db *gorm.DB, group string) bool {
	db = db.Model(&schema.SyntaxFlowGroup{})
	if group == "" {
		return false
	}
	i := &schema.SyntaxFlowGroup{}
	if db = db.Where("group_name = ?", group).First(i); db.Error != nil {
		return false
	}
	return true
}
