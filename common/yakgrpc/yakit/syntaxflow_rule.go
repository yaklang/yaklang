package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
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

	if len(params.GetGroupNames()) > 0 {
		db = db.Joins("LEFT JOIN syntax_flow_rule_groups P ON syntax_flow_rules.rule_name = P.rule_name")
		db = bizhelper.ExactQueryStringArrayOr(db, "`group_name`", params.GetGroupNames())
	}

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

	return db
}

func CreateSyntaxFlowRule(db *gorm.DB, rule *schema.SyntaxFlowRule) error {
	if rule == nil {
		return utils.Errorf("create syntaxFlow rule failed: rule is nil")
	}
	if rule.RuleName == "" {
		return utils.Errorf("create syntaxFlow rule failed: rule name is empty")
	}

	db = db.Model(&schema.SyntaxFlowRule{})
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

	db = db.Model(&schema.SyntaxFlowRule{})
	if err := db.Where("rule_name = ?", rule.RuleName).Update(rule).Error; err != nil {
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
