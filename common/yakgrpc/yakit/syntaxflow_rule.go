package yakit

import (
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func QuerySyntaxFlowRule(db *gorm.DB, params *ypb.QuerySyntaxFlowRuleRequest) (*bizhelper.Paginator, []*schema.SyntaxFlowRule, error) {
	if params == nil {
		params = &ypb.QuerySyntaxFlowRuleRequest{}
	}
	db = db.Model(&schema.SyntaxFlowRule{})
	p := params.Pagination
	db = bizhelper.OrderByPaging(db, p)
	db = FilterSyntaxFlowRule(db, params.GetFilter())
	var ret []*schema.SyntaxFlowRule
	db = db.Preload("Groups")
	paging, db := bizhelper.Paging(db, int(p.Page), int(p.Limit), &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}
	return paging, ret, nil
}

func QuerySyntaxFlowRuleNames(db *gorm.DB, filter *ypb.SyntaxFlowRuleFilter) ([]string, error) {
	if filter == nil {
		return nil, utils.Error("query syntax flow rule names failed: filter is nil")
	}
	db = db.Model(&schema.SyntaxFlowRule{})
	db = FilterSyntaxFlowRule(db, filter)
	var names []string
	db.Pluck("rule_name", &names)
	return names, db.Error
}

func FilterSyntaxFlowRule(db *gorm.DB, params *ypb.SyntaxFlowRuleFilter) *gorm.DB {
	if params == nil {
		return db
	}

	db = db.Model(&schema.SyntaxFlowRule{})

	if len(params.GetGroupNames()) > 0 {
		db = db.Joins("JOIN syntax_flow_rule_and_group ON syntax_flow_rule_and_group.syntax_flow_rule_id = syntax_flow_rules.id").
			Joins("JOIN syntax_flow_groups ON syntax_flow_groups.id = syntax_flow_rule_and_group.syntax_flow_group_id").
			Where("syntax_flow_groups.group_name IN (?)", params.GetGroupNames()).
			Group("syntax_flow_rules.id")
	}

	db = bizhelper.ExactOrQueryStringArrayOr(db, "severity", params.GetSeverity())
	db = bizhelper.ExactOrQueryStringArrayOr(db, "rule_name", params.GetRuleNames())
	db = bizhelper.ExactOrQueryStringArrayOr(db, "language", params.GetLanguage())
	db = bizhelper.ExactOrQueryStringArrayOr(db, "purpose", params.GetPurpose())
	db = bizhelper.ExactOrQueryStringArrayOr(db, "tag", params.GetTag())
	if !params.GetIncludeLibraryRule() {
		db = db.Where("allow_included = ?", false)
	}

	if params.GetKeyword() != "" {
		db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{
			"rule_name", "title", "title_zh", "description", "content", "tag",
		}, []string{params.GetKeyword()}, false)
	}
	if params.GetAfterId() > 0 {
		db = db.Where("id > ?", params.GetAfterId())
	}
	if params.GetBeforeId() > 0 {
		db = db.Where("id < ?", params.GetBeforeId())
	}
	if params.GetIsNotBuiltin() {
		db = db.Where("is_build_in_rule = ?", false)
	}
	return db
}

func DeleteSyntaxFlowRule(db *gorm.DB, params *ypb.DeleteSyntaxFlowRuleRequest) (int64, error) {
	return deleteSyntaxFlowRuleEx(db, params)
}

func DeleteSyntaxFlowNonBuildInRule(db *gorm.DB, params *ypb.DeleteSyntaxFlowRuleRequest) (int64, error) {
	return deleteSyntaxFlowRuleEx(db, params, false)
}

func deleteSyntaxFlowRuleEx(db *gorm.DB, params *ypb.DeleteSyntaxFlowRuleRequest, isBuildIn ...bool) (int64, error) {
	db = db.Model(&schema.SyntaxFlowRule{})
	if params == nil || params.Filter == nil {
		return 0, utils.Errorf("delete syntaxFlow rule failed: synatx flow filter is nil")
	}
	db = FilterSyntaxFlowRule(db, params.Filter)
	if len(isBuildIn) > 0 {
		db = bizhelper.QueryByBool(db, "is_build_in_rule", isBuildIn[0])
	}
	db = db.Unscoped().Delete(&schema.SyntaxFlowRule{})
	return db.RowsAffected, db.Error
}

func QuerySyntaxFlowRuleCount(db *gorm.DB, filter *ypb.SyntaxFlowRuleFilter) (int64, error) {
	db = db.Model(&schema.SyntaxFlowRule{})
	db = FilterSyntaxFlowRule(db, filter)
	var count int64
	db.Count(&count)
	return count, db.Error
}

func UpdateSyntaxFlowRule(db *gorm.DB, rule *ypb.SyntaxFlowRuleInput) (*schema.SyntaxFlowRule, error) {
	if rule == nil {
		return nil, utils.Errorf("update syntaxFlow rule failed: rule is nil")
	}
	if rule.RuleName == "" {
		return nil, utils.Errorf("update syntaxFlow rule failed: rule name is empty")
	}

	updateRule, err := sfdb.QueryRuleByName(consts.GetGormProfileDatabase(), rule.GetRuleName())
	if err != nil {
		return nil, utils.Errorf("update syntaxFlow rule failed: %s", err)
	}

	updateRule.Language = rule.GetLanguage()
	updateRule.Content = rule.GetContent()
	updateRule.Tag = strings.Join(rule.GetTags(), ",")
	updateRule.Description = rule.GetDescription()
	groups := sfdb.GetOrCreateGroups(consts.GetGormProfileDatabase(), rule.GetGroupNames())
	if err := db.Model(&schema.SyntaxFlowRule{}).Update(&updateRule).Error; err != nil {
		return nil, utils.Errorf("update syntaxFlow rule failed: %s", err)
	}
	if err := db.Model(&updateRule).Association("Groups").Replace(groups).Error; err != nil {
		return nil, utils.Errorf("update syntaxFlow rule failed: %s", err)
	}
	return updateRule, nil
}

func QuerySameGroupByRule(db *gorm.DB, req *ypb.SyntaxFlowRuleFilter) ([]*schema.SyntaxFlowGroup, error) {
	db = FilterSyntaxFlowRule(db, req)
	var rules []*schema.SyntaxFlowRule
	err := db.Model(&schema.SyntaxFlowRule{}).Preload("Groups").Find(&rules).Error
	if err != nil {
		return nil, err
	}
	var groups [][]*schema.SyntaxFlowGroup
	for _, rule := range rules {
		groups = append(groups, rule.Groups)
	}
	if len(rules) == 1 {
		return rules[0].Groups, nil
	}
	return sfdb.GetIntersectionGroup(consts.GetGormProfileDatabase(), groups), nil
}
