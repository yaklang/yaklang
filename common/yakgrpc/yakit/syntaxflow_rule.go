package yakit

import (
	"errors"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
)

func QuerySyntaxFlowRule(db *gorm.DB, params *ypb.QuerySyntaxFlowRuleRequest) (*bizhelper.Paginator, []*schema.SyntaxFlowRule, error) {
	if params == nil {
		params = &ypb.QuerySyntaxFlowRuleRequest{}
	}
	db = db.Model(&schema.SyntaxFlowRule{})
	if params.Pagination == nil {
		params.Pagination = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "updated_at",
			Order:   "desc",
		}
	}
	p := params.Pagination
	db = bizhelper.OrderByPaging(db, p)
	db = FilterSyntaxFlowRule(db, params.GetFilter())
	var ret []*schema.SyntaxFlowRule
	paging, db := bizhelper.Paging(db, int(p.Page), int(p.Limit), &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}
	return paging, ret, nil
}

func FilterSyntaxFlowRule(db *gorm.DB, params *ypb.SyntaxFlowRuleFilter) *gorm.DB {
	if params == nil {
		return db
	}
	if params.GetAll() {
		return db
	}
	if params.GetUnsetGroup() {
		db = db.Joins("LEFT JOIN syntax_flow_rule_groups P ON syntax_flow_rules.rule_name = P.rule_name")
		db = db.Where("P.rule_name IS NULL")
	} else if params.GetGroupName() != nil {
		db = db.Joins("LEFT JOIN syntax_flow_rule_groups P ON syntax_flow_rules.rule_name = P.rule_name")
		db = bizhelper.ExactQueryStringArrayOr(db, "`group_name`", params.GetGroupName())
	}
	rule := params.GetRule()
	keyWord := params.GetKeyWord()

	if rule.GetRuleName() != "" {
		db = db.Where("rule_name = ?", rule.GetRuleName())
	}
	if len(rule.GetLanguage()) > 0 {
		db = db.Where("language = ?", rule.GetLanguage())
	}
	if len(rule.GetPurpose()) > 0 {
		db = db.Where("purpose = ?", rule.GetPurpose())
	}
	if len(rule.GetSeverity()) > 0 {
		db = db.Where("severity = ?", rule.GetSeverity())
	}

	if rule.GetTag() != "" {
		db = bizhelper.FuzzQuery(db, "tag", rule.GetTag())
	}
	if rule.GetVerified() {
		db = bizhelper.QueryByBool(db, "verified", true)
	}
	if rule.GetIsBuildInRule() {
		db = bizhelper.QueryByBool(db, "is_build_in", true)
	}
	if rule.GetAllowIncluded() {
		db = bizhelper.QueryByBool(db, "allow_included", true)
	}
	if keyWord != "" {
		db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{
			"rule_name", "title", "title_zh", "description", "content", "tag",
		}, strings.Split(keyWord, ","), false)
	}

	return db
}

func CreateSyntaxFlowRule(db *gorm.DB, rule *schema.SyntaxFlowRule) error {
	if rule == nil {
		return utils.Errorf("create syntaxFlow rule failed: rule is nil")
	}
	if rule.RuleName == "" {
		return utils.Errorf("create syntaxFlow rule failed: rule name is empty")
	}

	var findRule schema.SyntaxFlowRule
	db = db.Model(&schema.SyntaxFlowRule{})
	if err := db.First(&findRule, "rule_name = ?", rule.RuleName).Error; err == nil {
		return utils.Errorf("create syntaxFlow rule failed: rule name %s already exists", rule.RuleName)
	}

	if err := db.Create(rule).Error; err != nil {
		return utils.Errorf("create syntaxFlow rule failed: %s", err)
	}
	return nil
}
func UpdateSyntaxFlowRule(db *gorm.DB, rule *schema.SyntaxFlowRule) error {
	if rule == nil {
		return utils.Errorf("update syntaxFlow rule failed: rule is nil")
	}
	if rule.RuleName == "" {
		return utils.Errorf("update syntaxFlow rule failed: rule name is empty")
	}

	findRule := schema.SyntaxFlowRule{}
	db = db.Model(&schema.SyntaxFlowRule{})
	db = db.First(&findRule, "rule_name = ?", rule.RuleName)
	if err := db.Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.Errorf("update syntaxFlow rule failed: rule name %s not found", rule.RuleName)
		} else {
			return utils.Errorf("update syntaxFlow rule failed: %s", err)
		}
	}
	if err := db.Model(findRule).Update(rule).Error; err != nil {
		return utils.Errorf("update syntaxFlow rule failed: %s", err)
	}
	return nil
}

func DeleteSyntaxFlowRule(db *gorm.DB, params *ypb.DeleteSyntaxFlowRuleRequest) (int64, error) {
	db = db.Model(&schema.SyntaxFlowRule{})
	if params == nil || params.Filter == nil {
		return 0, utils.Errorf("delete syntaxFlow rule failed: synatx flow filter is nil")
	}
	db = FilterSyntaxFlowRule(db, params.Filter)
	db = db.Unscoped().Delete(&schema.SyntaxFlowRule{})
	return db.RowsAffected, db.Error
}
