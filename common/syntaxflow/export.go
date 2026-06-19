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

// ExecRule 在已编译的程序上执行一条 SyntaxFlow 规则（导出名为 syntaxflow.ExecRule）
// 参数:
//   - r: SyntaxFlow 规则对象
//   - prog: 已经过 ssa 编译的程序对象
//   - opts: 查询可选项，如 syntaxflow.withContext / syntaxflow.withCache
//
// 返回值:
//   - SyntaxFlow 执行结果对象
//   - 错误信息
//
// Example:
// ```
// // 编译代码后执行规则（示意性示例，需要先有 schema.SyntaxFlowRule 对象）
// prog = ssa.Parse("a = 1; println(a)")~
// // rule 通常来自 syntaxflow.QuerySyntaxFlowRules 的查询结果
//
//	for rule := range syntaxflow.QuerySyntaxFlowRules("*") {
//	    result = syntaxflow.ExecRule(rule, prog)~
//	    break
//	}
//
// ```
func ExecRule(r *schema.SyntaxFlowRule, prog *ssaapi.Program, opts ...ssaapi.QueryOption) (*ssaapi.SyntaxFlowResult, error) {
	res, err := prog.SyntaxFlowRule(r, opts...)
	if err != nil {
		return nil, err
	}
	return res, nil
}

type QueryRulesOption func(*gorm.DB) *gorm.DB

// QuerySyntaxFlowRules 按规则名模糊查询内置/已保存的 SyntaxFlow 规则（导出名为 syntaxflow.QuerySyntaxFlowRules）
// 参数:
//   - name: 规则名关键字，支持模糊匹配，传 "*" 匹配全部
//   - opts: 可选的数据库查询条件
//
// 返回值:
//   - SyntaxFlow 规则对象的 channel，可使用 for-range 遍历
//
// Example:
// ```
// // 遍历名称包含 "xss" 的规则
// count = 0
//
//	for rule := range syntaxflow.QuerySyntaxFlowRules("xss") {
//	    count++
//	    if count > 3 { break }
//	}
//
// println(count)
// ```
func QuerySyntaxFlowRules(name string, opts ...QueryRulesOption) chan *schema.SyntaxFlowRule {
	db := consts.GetGormProfileDatabase()
	db = bizhelper.FuzzQueryLike(db, "rule_name", name)
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
	"MergeBeautificationResults": MergeBeautificationResultsForYak,
	// 扫描任务 / 项目核对导出统一收敛到高层聚合入口。
	"RunSyntaxFlowProjectScanCheck": RunSyntaxFlowProjectScanCheck,
}
