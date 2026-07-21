package syntaxflow

import (
	"context"

	"github.com/yaklang/gorm"
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

// withSave 让 SyntaxFlow 查询结果以"查询(Query)"类型保存到数据库（导出名为 syntaxflow.withSave）
// 作为 syntaxflow.ExecRule 等查询接口的可选项，保存后可后续按结果 ID 复查
//
// 返回值:
//   - SyntaxFlow 查询可选项
//
// Example:
// ```
// opt = syntaxflow.withSave()
// assert opt != nil, "withSave should return a query option"
// ```
func withSave() ssaapi.QueryOption {
	return ssaapi.QueryWithSave(schema.SFResultKindQuery)
}

// withSearch 让 SyntaxFlow 查询结果以"搜索(Search)"类型保存到数据库（导出名为 syntaxflow.withSearch）
// 与 syntaxflow.withSave 类似，但结果归类为搜索场景，便于区分用途
//
// 返回值:
//   - SyntaxFlow 查询可选项
//
// Example:
// ```
// opt = syntaxflow.withSearch()
// assert opt != nil, "withSearch should return a query option"
// ```
func withSearch() ssaapi.QueryOption {
	return ssaapi.QueryWithSave(schema.SFResultKindSearch)
}

var Exports = map[string]any{
	"ExecRule":       ExecRule,
	"withExecTaskID": ssaapi.QueryWithTaskID,
	"withExecDebug":  ssaapi.QueryWithEnableDebug,
	"withProcess":    ssaapi.QueryWithProcessCallback,
	"withContext":    ssaapi.QueryWithContext,
	"withCache":      ssaapi.QueryWithUseCache,
	"withSave":   withSave,
	"withSearch": withSearch,
	"QuerySyntaxFlowRules":       QuerySyntaxFlowRules,
	"MergeBeautificationResults": MergeBeautificationResultsForYak,
	// 扫描任务 / 项目核对导出统一收敛到高层聚合入口。
	"RunSyntaxFlowProjectScanCheck": RunSyntaxFlowProjectScanCheck,
}
