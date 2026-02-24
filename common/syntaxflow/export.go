package syntaxflow

import (
	"context"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func ExecRule(r *schema.SyntaxFlowRule, prog *ssaapi.Program, opts ...ssaapi.QueryOption) (*ssaapi.SyntaxFlowResult, error) {
	res, err := prog.SyntaxFlowRule(r, opts...)
	if err != nil {
		return nil, err
	}
	return res, nil
}

type QueryRulesOption func(*gorm.DB) *gorm.DB

func QuerySyntaxFlowRules(name string, opts ...QueryRulesOption) chan *schema.SyntaxFlowRule {
	db := consts.GetGormProfileDatabase()
	db = bizhelper.FuzzQueryLike(db, "rule_name", name)
	for _, opt := range opts {
		db = opt(db)
	}
	return sfdb.YieldSyntaxFlowRules(db, context.Background())
}

// QuerySyntaxFlowRulesByKeyword 按规则名称、英文标题、中文标题模糊查询，支持中文/英文输入
func QuerySyntaxFlowRulesByKeyword(keyword string, opts ...QueryRulesOption) chan *schema.SyntaxFlowRule {
	db := consts.GetGormProfileDatabase().Model(&schema.SyntaxFlowRule{})
	if keyword != "" {
		like := "%" + keyword + "%"
		db = db.Where("rule_name LIKE ? OR title LIKE ? OR title_zh LIKE ?", like, like, like)
	}
	for _, opt := range opts {
		db = opt(db)
	}
	return sfdb.YieldSyntaxFlowRules(db, context.Background())
}

var Exports = map[string]any{
	"ExecRule":       ExecRule,
	"withExecTaskID": ssaapi.QueryWithTaskID,
	"withExecDebug":  ssaapi.QueryWithEnableDebug,
	"withProcess":    ssaapi.QueryWithProcessCallback,
	"withContext":    ssaapi.QueryWithContext,
	"withCache":      ssaapi.QueryWithUseCache,
	"withSave": func() ssaapi.QueryOption {
		return ssaapi.QueryWithSave(schema.SFResultKindQuery)
	},
	"withSearch": func() ssaapi.QueryOption {
		return ssaapi.QueryWithSave(schema.SFResultKindSearch)
	},
	"QuerySyntaxFlowRules":       QuerySyntaxFlowRules,
	"QuerySyntaxFlowRulesByKeyword": QuerySyntaxFlowRulesByKeyword,
}
