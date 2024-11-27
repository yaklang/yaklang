package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func QuerySyntaxFlowRuleGroup(db *gorm.DB, params *ypb.QuerySyntaxFlowRuleGroupRequest) (result []*ypb.SyntaxFlowGroup, err error) {
	if params == nil {
		return nil, utils.Error("query syntax flow rule group failed: query params is nil")
	}
	db = db.Model(&schema.SyntaxFlowRuleGroup{})
	db = FilterSyntaxFlowGroup(db, params.GetFilter())
	db = db.Select("group_name, count(*) as count").Where("group_name != '' AND group_name IS NOT NULL").Group("group_name").Order("count desc")
	db = db.Scan(&result)
	err = db.Error
	return
}

func FilterSyntaxFlowGroup(db *gorm.DB, filter *ypb.SyntaxFlowRuleGroupFilter) *gorm.DB {
	if filter == nil {
		return db
	}
	db = bizhelper.ExactOrQueryStringArrayOr(db, "group_name", filter.GetGroupNames())
	if filter.GetKeyWord() != "" {
		db = bizhelper.FuzzQueryStringArrayOrLike(db,
			"group_name", []string{filter.GetKeyWord()})
	}
	return db
}

func AddSyntaxFlowRulesGroup(db *gorm.DB, i *schema.SyntaxFlowRuleGroup) error {
	db = db.Model(&schema.SyntaxFlowRuleGroup{})
	if i.RuleName == "" {
		return utils.Error("add syntax flow rule group failed:rule name is empty")
	}
	if i.GroupName == "" {
		return utils.Errorf("add syntax flow rule group failed:group name is empty")
	}
	if db := db.Create(i); db.Error != nil {
		return utils.Errorf("create SyntaxFlowGroup failed: %s", db.Error)
	}
	return nil
}

func UpdateSyntaxFlowRulesGroup(db *gorm.DB, i *schema.SyntaxFlowRuleGroup) error {
	db = db.Model(&schema.SyntaxFlowRuleGroup{})
	if i.RuleName == "" {
		return utils.Error("add syntax flow rule group failed:rule name is empty")
	}
	if i.GroupName == "" {
		return utils.Errorf("add syntax flow rule group failed:group name is empty")
	}
	hash := i.CalcHash()
	if err := db.Where("hash = ?", hash).Update(i).Error; err != nil {
		return utils.Errorf("update SyntaxFlowGroup failed: %s", err)
	}
	return nil
}

func DeleteSyntaxFlowRuleGroup(db *gorm.DB, params *ypb.DeleteSyntaxFlowRuleGroupRequest) (int64, error) {
	db = db.Model(&schema.SyntaxFlowRuleGroup{})
	if params == nil {
		return 0, utils.Error("delete syntax flow rule group failed: delete syntaxflow rule request is nil")
	}
	if params.GetDeleteAll() {
		if err := db.Delete(&schema.SyntaxFlowRuleGroup{}).Error; err != nil {
			return 0, utils.Errorf("delete all SyntaxFlowGroup failed: %s", err)
		}
		return 0, nil
	}
	if params.GetFilter() == nil {
		return 0, utils.Error("delete syntax flow rule group failed: delete filter is nil")
	}
	db = FilterSyntaxFlowGroup(db, params.GetFilter())
	db = db.Unscoped().Delete(&schema.SyntaxFlowRuleGroup{})
	return db.RowsAffected, db.Error
}

func CreateSyntaxFlowRuleGroup(db *gorm.DB, params *ypb.CreateSyntaxFlowRuleGroupRequest) (int64, error) {
	db = db.Model(&schema.SyntaxFlowRuleGroup{})
	if params == nil {
		return 0, utils.Error("create syntax flow rule group failed: create syntaxflow rule request is nil")
	}
	if params.GetGroupName() == "" {
		return 0, utils.Error("create syntax flow rule group failed: group name is empty")
	}
	db = db.Create(&schema.SyntaxFlowRuleGroup{GroupName: params.GetGroupName()})
	return db.RowsAffected, db.Error
}
