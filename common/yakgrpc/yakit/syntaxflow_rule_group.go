package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
)

type SyntaxFlowGroupResult struct {
	RuleName      string
	GroupName     string
	IsBuildInRule bool
	Count         int
}

func QuerySyntaxFlowRuleGroup(db *gorm.DB, params *ypb.QuerySyntaxFlowRuleGroupRequest) (result []*SyntaxFlowGroupResult, err error) {
	if params == nil {
		return nil, utils.Error("query syntax flow rule group failed: query params is nil")
	}
	db = db.Model(&schema.SyntaxFlowRuleGroup{})
	db = FilterSyntaxFlowGroup(db, params.GetFilter())
	db = db.Select("group_name, count(*) as count").Group("group_name").Order("count desc")
	db = db.Scan(&result)
	err = db.Error
	return
}

func FilterSyntaxFlowGroup(db *gorm.DB, filter *ypb.SyntaxFlowRuleGroupFilter) *gorm.DB {
	if filter == nil {
		return db
	}
	if filter.GetAll() {
		return db
	}
	if filter.GetIsBuiltinRule() {
		db = bizhelper.QueryByBool(db, "is_build_in", true)
	}
	if filter.GetKeyWord() != "" {
		db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{
			"rule_name", "group_name",
		}, strings.Split(filter.GetKeyWord(), ","), false)
	}
	return db
}

func AddSyntaxFlowRulesGroup(db *gorm.DB, i *schema.SyntaxFlowRuleGroup) error {
	db = db.Model(&schema.SyntaxFlowRuleGroup{})
	if i.RuleName == "" {
		return utils.Error("add syntax flow rule group failed:rule name is empty")
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
	hash := i.CalcHash()
	if err := db.Where("hash = ?", hash).Update(i).Error; err != nil {
		return utils.Errorf("update SyntaxFlowGroup failed: %s", err)
	}
	return nil
}

func DeleteSyntaxFlowRuleGroup(db *gorm.DB, i *schema.SyntaxFlowRuleGroup) error {
	db = db.Model(&schema.SyntaxFlowRuleGroup{})
	hash := i.CalcHash()
	if err := db.Where("hash = ?", hash).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.Errorf("delete syntax flow rule group failed: the rule group not found")
		} else {
			return utils.Errorf("delete syntax flow rule group failed: %s", err)
		}
	}
	if db := db.Where("hash = ?", hash).Unscoped().Delete(&schema.SyntaxFlowRuleGroup{}); db.Error != nil {
		return utils.Errorf("delete SyntaxFlowGroup failed: %s", db.Error)
	}
	return nil
}
