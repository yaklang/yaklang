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

const (
	FilterLibRuleTrue  string = "lib"
	FilterLibRuleFalse string = "noLib"

	FilterBuiltinRuleTrue  string = "buildIn"
	FilterBuiltinRuleFalse string = "unBuildIn"
)

type SyntaxFlowRuleFilterOption func(*ypb.SyntaxFlowRuleFilter)

func WithSyntaxFlowRuleLib(b bool) SyntaxFlowRuleFilterOption {
	return func(sfrf *ypb.SyntaxFlowRuleFilter) {
		if b {
			sfrf.FilterLibRuleKind = FilterLibRuleTrue
		} else {
			sfrf.FilterLibRuleKind = FilterLibRuleFalse
		}
	}
}

func WithSyntaxFlowRuleBuiltin(b bool) SyntaxFlowRuleFilterOption {
	return func(sfrf *ypb.SyntaxFlowRuleFilter) {
		if b {
			sfrf.FilterRuleKind = FilterBuiltinRuleTrue
		} else {
			sfrf.FilterRuleKind = FilterBuiltinRuleFalse
		}
	}
}

func WithSyntaxFlowRuleName(name ...string) SyntaxFlowRuleFilterOption {
	return func(sfrf *ypb.SyntaxFlowRuleFilter) {
		sfrf.RuleNames = append(sfrf.RuleNames, name...)
	}
}

func FilterSyntaxFlowRule(db *gorm.DB, params *ypb.SyntaxFlowRuleFilter, opt ...SyntaxFlowRuleFilterOption) *gorm.DB {
	if params == nil {
		if len(opt) == 0 {
			// if no param and no option, return db
			return db
		} else {
			// if no param but has option, create a new param
			params = &ypb.SyntaxFlowRuleFilter{}
		}
	}
	// apply options to it
	for _, o := range opt {
		o(params)
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
	//if !params.GetIncludeLibraryRule() {
	//	db = db.Where("allow_included = ?", false)
	//}

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
	switch params.GetFilterRuleKind() {
	case FilterBuiltinRuleTrue:
		db = bizhelper.QueryByBool(db, "is_build_in_rule", true)
	case FilterBuiltinRuleFalse:
		db = bizhelper.QueryByBool(db, "is_build_in_rule", false)
	}

	switch params.GetFilterLibRuleKind() {
	case FilterLibRuleTrue:
		db = bizhelper.QueryByBool(db, "allow_included", true)
	case FilterLibRuleFalse:
		db = bizhelper.QueryByBool(db, "allow_included", false)
	}
	return db
}

func QuerySyntaxFlowRule(db *gorm.DB, params *ypb.QuerySyntaxFlowRuleRequest) (*bizhelper.Paginator, []*schema.SyntaxFlowRule, error) {
	if params == nil {
		params = &ypb.QuerySyntaxFlowRuleRequest{}
	}
	db = db.Model(&schema.SyntaxFlowRule{})
	p := params.Pagination
	if p == nil {
		p = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "updated_at",
			Order:   "desc",
		}
	}
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

func DeleteSyntaxFlowRule(db *gorm.DB, params *ypb.DeleteSyntaxFlowRuleRequest) (int64, error) {
	if params == nil || params.Filter == nil {
		return 0, utils.Errorf("delete syntaxFlow rule failed: syntax flow filter is nil")
	}
	db = db.Model(&schema.SyntaxFlowRule{})
	query := db
	query = FilterSyntaxFlowRule(query, params.Filter)
	// 如果filter包含groupName,FilterSyntaxFlowRule会使用联表查询，导致无法直接db.delete()
	// 所以需要先查出来再删除
	var ids []uint64
	query.Pluck("syntax_flow_rules.id", &ids)
	if len(ids) == 0 {
		return 0, nil
	}
	db = bizhelper.ExactQueryUInt64ArrayOr(db, "id", ids)
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
	if rule.Content == "" {
		return nil, utils.Errorf("update syntaxFlow rule failed: rule content is empty")
	}
	dbRule, err2 := ParseSyntaxFlowInput(rule)
	if err2 != nil {
		return nil, utils.Errorf("update syntaxFlow rule failed: %s", err2)
	}
	updateRule, err := sfdb.QueryRuleByName(consts.GetGormProfileDatabase(), rule.GetRuleName())
	if err != nil {
		return nil, utils.Errorf("update syntaxFlow rule failed: %s", err)
	}
	updateRule.Language = rule.GetLanguage()
	updateRule.Content = rule.GetContent()
	updateRule.Tag = strings.Join(rule.GetTags(), ",")
	updateRule.Description = rule.GetDescription()
	updateRule.AlertDesc = dbRule.AlertDesc
	updateRule.TitleZh = dbRule.TitleZh
	updateRule.OpCodes = dbRule.OpCodes
	updateRule.Hash = dbRule.CalcHash()

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

func ParseSyntaxFlowInput(ruleInput *ypb.SyntaxFlowRuleInput) (*schema.SyntaxFlowRule, error) {
	language, err := sfdb.CheckSyntaxFlowLanguage(ruleInput.Language)
	if err != nil {
		return nil, err
	}
	rule, _ := sfdb.CheckSyntaxFlowRuleContent(ruleInput.Content)
	rule.Language = string(language)
	rule.RuleName = ruleInput.RuleName
	rule.Tag = strings.Join(ruleInput.Tags, "|")
	rule.Title = ruleInput.RuleName
	//rule.Groups = sfdb.GetOrCreateGroups(consts.GetGormProfileDatabase(), ruleInput.GroupNames)
	rule.Description = ruleInput.Description
	for s, message := range ruleInput.AlertMsg {
		rule.AlertDesc[s] = schema.ToSyntaxFlowAlertDesc(message)
	}
	return rule, nil
}

func QueryBuildInRule(db *gorm.DB) []*schema.SyntaxFlowRule {
	db = db.Model(&schema.SyntaxFlowRule{})
	db = bizhelper.QueryByBool(db, "is_build_in_rule", true)
	var rules []*schema.SyntaxFlowRule
	db.Find(&rules)
	return rules
}

func AllSyntaxFlowRule(db *gorm.DB, req *ypb.SyntaxFlowRuleFilter) ([]*schema.SyntaxFlowRule, error) {
	db = db.Model(&schema.SyntaxFlowRule{})
	db = FilterSyntaxFlowRule(db, req)
	var ret []*schema.SyntaxFlowRule
	db = db.Preload("Groups")
	if err := db.Find(&ret).Error; err != nil {
		return nil, utils.Errorf("query failed: %s", err)
	}
	return ret, nil
}
