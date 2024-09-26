package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
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

func SaveSyntaxFlowRule(db *gorm.DB, param *ypb.SaveSyntaxFlowRuleRequest) error {
	language, err := sfdb.CheckSyntaxFlowLanguage(param.GetLanguage())
	if err != nil {
		return err
	}
	ruleType, err := sfdb.CheckSyntaxFlowRuleType(param.GetRuleName())
	if err != nil {
		return err
	}
	frame, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(param.GetContent())
	if err != nil {
		return err
	}
	rule := frame.GetRule()
	rule.Type = ruleType
	rule.RuleName = param.GetRuleName()
	rule.Language = string(language)
	rule.Tag = strings.Join(param.GetTags(), "|")
	err = createOrUpdateSyntaxFlow(db, rule.CalcHash(), rule)
	if err != nil {
		return utils.Wrap(err, "ImportRuleWithoutValid create or update syntax flow rule error")
	}
	return nil
}

func createOrUpdateSyntaxFlow(db *gorm.DB, hash string, i *schema.SyntaxFlowRule) error {
	var rules []*schema.SyntaxFlowRule
	if hash == "" {
		hash = i.CalcHash()
	}

	var sameHashRule schema.SyntaxFlowRule
	if db.Where("hash = ?", hash).First(&sameHashRule); sameHashRule.ID > 0 {
		return nil
	}

	if err := db.Where("rule_name = ?", i.RuleName).Find(&rules).Error; err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return db.Create(i).Error
		}
		return err
	}
	if len(rules) == 1 {
		i.Hash = i.CalcHash()
		// only one rule, check and update
		rule := rules[0]
		if rule.Hash != hash {
			// if same name, but different content, update
			return db.Model(&rule).Updates(i).Error
		}
	} else {
		// multiple rule in same name, delete all and create new
		if err := db.Where("rule_name = ?", i.RuleName).Unscoped().Delete(&schema.SyntaxFlowRule{}).Error; err != nil {
			return err
		}
		return db.Create(i).Error
	}
	return nil
}
