package sfdb

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// applyRuleFilter 应用规则筛选条件到数据库查询
// 这个函数避免了对yakit包的依赖，解决循环依赖问题
func applyRuleFilter(db *gorm.DB, filter *ypb.SyntaxFlowRuleFilter) *gorm.DB {
	db = db.Model(&schema.SyntaxFlowRule{})

	if filter == nil {
		return db
	}

	// 规则组筛选
	if len(filter.GetGroupNames()) > 0 {
		db = db.Joins("JOIN syntax_flow_rule_and_group ON syntax_flow_rule_and_group.syntax_flow_rule_id = syntax_flow_rules.id").
			Joins("JOIN syntax_flow_groups ON syntax_flow_groups.id = syntax_flow_rule_and_group.syntax_flow_group_id").
			Where("syntax_flow_groups.group_name IN (?)", filter.GetGroupNames()).
			Group("syntax_flow_rules.id")
	}

	// 严重程度筛选
	db = bizhelper.ExactOrQueryStringArrayOr(db, "severity", filter.GetSeverity())

	// 规则名称筛选
	db = bizhelper.ExactOrQueryStringArrayOr(db, "rule_name", filter.GetRuleNames())

	// 语言筛选
	db = bizhelper.ExactOrQueryStringArrayOr(db, "language", filter.GetLanguage())

	// 用途筛选
	db = bizhelper.ExactOrQueryStringArrayOr(db, "purpose", filter.GetPurpose())

	// 标签筛选
	db = bizhelper.ExactOrQueryStringArrayOr(db, "tag", filter.GetTag())

	// 关键词搜索
	if filter.GetKeyword() != "" {
		db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{
			"rule_name", "title", "title_zh", "description", "content", "tag",
		}, []string{filter.GetKeyword()}, false)
	}

	// ID范围筛选
	if filter.GetAfterId() > 0 {
		db = db.Where("id > ?", filter.GetAfterId())
	}
	if filter.GetBeforeId() > 0 {
		db = db.Where("id < ?", filter.GetBeforeId())
	}

	// 规则类型筛选
	if filter.GetFilterRuleKind() != "" {
		switch filter.GetFilterRuleKind() {
		case "buildIn":
			db = db.Where("is_build_in_rule = ?", true)
		case "unBuildIn":
			db = db.Where("is_build_in_rule = ?", false)
		}
	}

	// Lib规则筛选
	if filter.GetFilterLibRuleKind() != "" {
		switch filter.GetFilterLibRuleKind() {
		case "lib":
			db = db.Where("allow_included = ?", true)
		case "noLib":
			db = db.Where("allow_included = ?", false)
		}
	}

	return db
}
