package syntaxflow

import (
	"context"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
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

// MergeCompletionResultsForYak 供 Yak 调用，将 desc/alert forge 输出合并到规则内容。descMap/alertMap 为 map[string]any。
func MergeCompletionResultsForYak(descMap, alertMap any, ruleContent string) (string, error) {
	descParams := aitool.InvokeParams(utils.InterfaceToGeneralMap(descMap))
	alertParams := aitool.InvokeParams(utils.InterfaceToGeneralMap(alertMap))
	return MergeCompletionResults(descParams, alertParams, ruleContent)
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
	"QuerySyntaxFlowRules":   QuerySyntaxFlowRules,
	"MergeCompletionResults": MergeCompletionResultsForYak,
	// 扫描任务 / 项目核对导出统一收敛到高层聚合入口。
	"RunSyntaxFlowProjectScanCheck": RunSyntaxFlowProjectScanCheck,
}
