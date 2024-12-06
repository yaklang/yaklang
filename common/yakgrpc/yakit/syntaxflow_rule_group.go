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
	db = FilterSyntaxFlowGroups(db, params.GetFilter())
	db = db.Select("group_name, count(*) as count").
		Group("group_name").Order("count desc")
	db = db.Scan(&result)
	err = db.Error
	return
}

func FilterSyntaxFlowGroups(db *gorm.DB, filter *ypb.SyntaxFlowRuleGroupFilter) *gorm.DB {
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

func CreateSyntaxFlowRuleGroup(db *gorm.DB, group string) error {
	db = db.Model(&schema.SyntaxFlowGroup{})
	if group == "" {
		return utils.Errorf("add syntax flow rule group failed:group name is empty")
	}
	i := &schema.SyntaxFlowGroup{
		GroupName: group,
	}
	if db = db.Create(i); db.Error != nil {
		return utils.Errorf("create SyntaxFlowGroup failed: %s", db.Error)
	}
	return nil
}

func AddSFRuleAndGroupRelation(db *gorm.DB, rules []string, group string) (int64, error) {
	if len(rules) == 0 {
		return 0, utils.Errorf("add syntax flow rule group failed:rule name is empty")
	}
	if group == "" {
		return 0, utils.Errorf("add syntax flow rule group failed:group name is empty")
	}

	db = db.Model(&schema.SyntaxFlowRuleGroup{})
	var count int64
	var errs error
	for _, rule := range rules {
		i := &schema.SyntaxFlowRuleGroup{
			RuleName:  rule,
			GroupName: group,
		}
		if db := db.Create(i); db.Error != nil {
			errs = utils.JoinErrors(errs, db.Error)
			continue
		} else {
			count++
		}
	}
	return count, errs
}

func RemoveSFRuleAndGroupRelation(db *gorm.DB, rules []string, group string) (int64, error) {
	if len(rules) == 0 {
		return 0, utils.Errorf("add syntax flow rule group failed:rule name is empty")
	}
	if group == "" {
		return 0, utils.Errorf("add syntax flow rule group failed:group name is empty")
	}

	db = db.Model(&schema.SyntaxFlowRuleGroup{})
	var count int64
	var errs error
	for _, rule := range rules {
		i := &schema.SyntaxFlowRuleGroup{
			RuleName:  rule,
			GroupName: group,
		}
		if db := db.Where("rule_name = ? AND group_name = ?", rule, group).Unscoped().Delete(i); db.Error != nil {
			errs = utils.JoinErrors(errs, db.Error)
			continue
		} else {
			count++
		}
	}
	return count, errs
}

func RemoveSFRuleAndGroupRelationByGroupNames(db *gorm.DB, groupNames []string) (int64, error) {
	if len(groupNames) == 0 {
		return 0, utils.Errorf("add syntax flow rule group failed:group name is empty")
	}

	db = db.Model(&schema.SyntaxFlowRuleGroup{})
	if db = db.Where("group_name IN (?)", groupNames).Unscoped().Delete(&schema.SyntaxFlowRuleGroup{}); db.Error != nil {
		return 0, db.Error
	} else {
		return db.RowsAffected, nil
	}
}

func DeleteSyntaxFlowRuleGroup(db *gorm.DB, params *ypb.DeleteSyntaxFlowRuleGroupRequest) (int64, error) {
	if params == nil {
		return 0, utils.Error("delete syntax flow rule group failed: delete syntaxflow rule request is nil")
	}
	if params.GetFilter() == nil {
		return 0, utils.Error("delete syntax flow rule group failed: delete filter is nil")
	}
	ruleDb := FilterSyntaxFlowGroups(db, params.GetFilter())

	var (
		groupNames []string
		errs       error
	)

	ruleDb.Model(&schema.SyntaxFlowGroup{}).Pluck("DISTINCT group_name", &groupNames)
	count, err := RemoveSFRuleAndGroupRelationByGroupNames(db, groupNames)
	if err != nil {
		errs = utils.JoinErrors(errs, err)
	}
	if db = db.Model(&schema.SyntaxFlowGroup{}).Where("group_name IN (?)", groupNames).Unscoped().Delete(&schema.SyntaxFlowGroup{}); db.Error != nil {
		errs = utils.JoinErrors(errs, db.Error)
	}
	return count, db.Error
}

func QuerySyntaxFlowGroupCount(db *gorm.DB, groupNames []string) int64 {
	db = db.Model(&schema.SyntaxFlowGroup{})
	var count int64
	db.Where("group_name IN (?)", groupNames).Count(&count)
	return count
}
