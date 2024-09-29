package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"sync"
)

var syntaxFlowOpLock = new(sync.Mutex)

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
	if params.RuleName != "" {
		db = db.Where("rule_name = ?", params.RuleName)
	}
	if len(params.Language) > 0 {
		db = bizhelper.ExactQueryStringArrayOr(db, "language", params.Language)
	}
	if len(params.Purpose) > 0 {
		db = bizhelper.ExactQueryStringArrayOr(db, "purpose", params.Purpose)
	}
	if len(params.Severity) > 0 {
		db = bizhelper.ExactQueryStringArrayOr(db, "severity", params.Severity)
	}

	tags := utils.StringArrayFilterEmpty(params.GetTag())
	if len(tags) > 0 {
		db = bizhelper.FuzzQueryStringArrayOrLike(db, "tag", tags)
	}
	if params.Verified {
		db = bizhelper.QueryByBool(db, "is_general_module", true)
	}
	if params.IsBuildInRule {
		db = bizhelper.QueryByBool(db, "is_build_in", true)
	}
	if params.AllowIncluded {
		db = bizhelper.QueryByBool(db, "allow_included", true)
	}
	if params.GetKeyword() != "" {
		db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{
			"rule_name", "title", "title_zh", "description", "content", "tag",
		}, strings.Split(params.GetKeyword(), ","), false)
	}
	return db
}

func DeleteSyntaxFlowRuleByFilter(db *gorm.DB, filter *ypb.SyntaxFlowRuleFilter) (int64, error) {
	db = FilterSyntaxFlowRule(db, filter)
	if db = db.Unscoped().Delete(&schema.SyntaxFlowRule{}); db.Error != nil {
		return 0, utils.Errorf("delete syntax flow rule failed: %s", db.Error)
	}
	return db.RowsAffected, nil
}
