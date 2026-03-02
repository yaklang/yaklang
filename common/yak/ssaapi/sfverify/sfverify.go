package sfverify

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func init() {
	ssaapi.RegisterExport("VerifySFRuleMatchesSample", func(ruleContent, sampleCode, filename, language string) map[string]any {
		res := VerifySFRuleMatchesSample(ruleContent, sampleCode, filename, language)
		out := map[string]any{
			"matched": res.Matched,
			"message": res.Message,
		}
		if res.Error != "" {
			out["error"] = res.Error
		}
		if res.AlertCount > 0 {
			out["alert_count"] = res.AlertCount
			out["alert_details"] = res.AlertDetails
		}
		if res.QueryResultsFull != "" {
			out["query_results_full"] = res.QueryResultsFull
		}
		if res.Suggestion != "" {
			out["suggestion"] = res.Suggestion
		}
		if len(res.ResultVarsDiagnostic) > 0 {
			out["result_vars_diagnostic"] = res.ResultVarsDiagnostic
		}
		if res.DiagnosticHint != "" {
			out["diagnostic_hint"] = res.DiagnosticHint
		}
		return out
	})
}

// VerifySFRuleMatchesSampleResult 规则与样例匹配验证结果
type VerifySFRuleMatchesSampleResult struct {
	Matched               bool           `json:"matched"`                 // 规则是否在样例上触发告警（即正确匹配漏洞）
	Message               string         `json:"message"`                 // 人类可读的验证结果描述
	Error                 string         `json:"error,omitempty"`         // 错误类型或详细信息
	AlertCount            int            `json:"alert_count,omitempty"`   // 触发告警的数量（matched 时有效）
	AlertDetails          map[string]int `json:"alert_details,omitempty"`  // 各 alert 变量及其匹配数量，便于调试
	QueryResultsFull      string         `json:"query_results_full,omitempty"` // matched 时：完整查询结果（含变量、位置、源代码上下文）
	Suggestion            string         `json:"suggestion,omitempty"`    // 未匹配时的修复建议
	ResultVarsDiagnostic  map[string]int `json:"result_vars_diagnostic,omitempty"` // 未匹配时：所有变量（含中间变量）的匹配数量，辅助定位数据流断点
	DiagnosticHint        string         `json:"diagnostic_hint,omitempty"` // 未匹配时：解读变量链，指出首个为 0 的变量及修复方向
}

// VerifySFRuleMatchesSample 静态检测：SyntaxFlow 规则是否能正确匹配用户提供的漏洞样例。
// 将 sample 作为虚拟项目解析，对该项目执行规则扫描，若规则产生告警则判定为匹配。
// 参数: ruleContent 完整 .sf 规则文本；sampleCode 漏洞样例代码；filename 样例文件名（如 vuln.go）；language 语言（如 golang、java、php、c）。
func VerifySFRuleMatchesSample(ruleContent, sampleCode, filename, language string) VerifySFRuleMatchesSampleResult {
	if ruleContent == "" || sampleCode == "" {
		return VerifySFRuleMatchesSampleResult{
			Matched: false,
			Message: "缺少必要参数：rule_content、sample_code 均不能为空",
			Error:   "invalid_args",
		}
	}
	lang, err := ssaconfig.ValidateLanguage(language)
	if err != nil {
		return VerifySFRuleMatchesSampleResult{
			Matched: false,
			Message: "不支持的语言: " + language + "。支持: golang, java, php, c, javascript, yak, python",
			Error:   err.Error(),
		}
	}
	// 若 filename 为空，根据语言推断默认文件名
	if filename == "" {
		ext := lang.GetFileExt()
		if ext != "" {
			filename = "sample" + ext
		} else {
			filename = "sample"
		}
	}
	// 若 filename 无扩展名或扩展名与语言不匹配，尝试补充
	if !strings.Contains(filename, ".") && lang.GetFileExt() != "" {
		filename = strings.TrimSuffix(filename, "/") + lang.GetFileExt()
	}
	vfs := filesys.NewVirtualFs()
	vfs.AddFile(filename, sampleCode)
	progs, err := ssaapi.ParseProjectWithFS(vfs, ssaapi.WithLanguage(lang))
	if err != nil {
		return VerifySFRuleMatchesSampleResult{
			Matched: false,
			Message: "样例代码解析失败: " + err.Error(),
			Error:   err.Error(),
			Suggestion: "请确认 sample_code 为有效的 " + string(lang) + " 代码，且 filename 扩展名正确（如 .go/.java/.php/.c）",
		}
	}
	if len(progs) == 0 {
		return VerifySFRuleMatchesSampleResult{
			Matched: false,
			Message: "样例代码未能生成有效程序",
			Error:   "empty_program",
			Suggestion: "请检查 sample_code 是否包含可解析的入口（如 package main、class 等）",
		}
	}
	opts := []ssaapi.QueryOption{ssaapi.QueryWithInitInputVar(progs[0])}
	result, err := progs.SyntaxFlowWithError(ruleContent, opts...)
	if err != nil {
		return VerifySFRuleMatchesSampleResult{
			Matched: false,
			Message: "规则执行失败: " + err.Error(),
			Error:   err.Error(),
			Suggestion: "请先调用 check-syntaxflow-syntax 检查规则语法，并确认规则中的 include/lib 引用是否可用",
		}
	}
	if len(result.GetErrors()) > 0 {
		return VerifySFRuleMatchesSampleResult{
			Matched: false,
			Message: "规则执行有错误: " + strings.Join(result.GetErrors(), "; "),
			Error:   strings.Join(result.GetErrors(), "; "),
			Suggestion: "检查规则逻辑是否与样例中的 API/调用方式一致，如 source/sink 方法名、数据流路径等",
		}
	}
	alertVars := result.GetAlertVariables()
	alertDetails := make(map[string]int)
	resultVarsDiagnostic := make(map[string]int)
	totalAlert := 0
	// 收集所有变量（含中间变量）以定位数据流断点
	allVars := result.GetAllVariable()
	if allVars != nil {
		allVars.ForEach(func(name string, value any) {
			if name == "_" {
				return
			}
			n := 0
			if v, ok := value.(int); ok {
				n = v
			}
			resultVarsDiagnostic[name] = n
		})
	}
	for _, name := range alertVars {
		vals := result.GetValues(name)
		n := 0
		if vals != nil {
			n = len(vals)
		}
		if n > 0 {
			alertDetails[name] = n
			totalAlert += n
		}
	}
	if totalAlert > 0 {
		queryResultsFull := result.Dump(true) // 完整查询结果：含 alert 变量、位置、源代码上下文
		return VerifySFRuleMatchesSampleResult{
			Matched:          true,
			Message:          fmt.Sprintf("规则已正确匹配样例中的漏洞，触发 %d 处告警", totalAlert),
			AlertCount:       totalAlert,
			AlertDetails:     alertDetails,
			QueryResultsFull: queryResultsFull,
		}
	}
	// 未匹配时提供详细诊断信息，辅助 AI 修改规则
	suggestion := "请检查：1) 规则的 source/sink 模式是否覆盖样例中的调用链；2) 数据流 #-> 是否与样例实际路径一致；3) 条件过滤 ?{} 是否过于严格"
	msg := "规则未在样例上触发告警，可能未正确匹配漏洞"
	var diagnosticHint string

	// 解析规则中的变量定义、使用及依赖关系
	ruleAnalysis := parseRuleVarAnalysis(ruleContent)

	// 将未定义变量加入 resultVarsDiagnostic（标记为 0），便于诊断
	for _, undef := range ruleAnalysis.Undefined {
		if _, ok := resultVarsDiagnostic[undef]; !ok {
			resultVarsDiagnostic[undef] = 0
		}
	}

	if len(resultVarsDiagnostic) > 0 {
		var diagParts []string
		var varOrder []string
		if allVars != nil {
			allVars.ForEach(func(name string, _ any) {
				if name != "_" {
					varOrder = append(varOrder, name)
				}
			})
		}
		// 确保未定义变量也出现在链中（插到前面）
		seenInOrder := make(map[string]bool)
		for _, n := range varOrder {
			seenInOrder[n] = true
		}
		for _, undef := range ruleAnalysis.Undefined {
			if !seenInOrder[undef] {
				varOrder = append([]string{undef}, varOrder...)
				seenInOrder[undef] = true
			}
		}
		if len(varOrder) == 0 {
			for name := range resultVarsDiagnostic {
				varOrder = append(varOrder, name)
			}
		}
		for _, name := range varOrder {
			if cnt, ok := resultVarsDiagnostic[name]; ok {
				mark := ""
				for _, u := range ruleAnalysis.Undefined {
					if u == name {
						mark = " [未定义]"
						break
					}
				}
				diagParts = append(diagParts, fmt.Sprintf("$%s:%d%s", name, cnt, mark))
			}
		}
		if len(diagParts) == 0 {
			for name, cnt := range resultVarsDiagnostic {
				mark := ""
				for _, u := range ruleAnalysis.Undefined {
					if u == name {
						mark = " [未定义]"
						break
					}
				}
				diagParts = append(diagParts, fmt.Sprintf("$%s:%d%s", name, cnt, mark))
			}
		}
		msg = fmt.Sprintf("规则未在样例上触发告警。变量链: %s。数量为 0 表示数据流未到达该变量（其前模式未匹配或 #-> 路径断裂）。标注 [未定义] 表示变量被使用但未定义。", strings.Join(diagParts, " → "))

		// 优先处理未定义变量，并展示从下往上的依赖链（根因多为未定义）
		if len(ruleAnalysis.Undefined) > 0 {
			undefList := strings.Join(func() []string {
				ss := make([]string, len(ruleAnalysis.Undefined))
				for i, u := range ruleAnalysis.Undefined {
					ss[i] = "$" + u
				}
				return ss
			}(), "、")
			includeHint := ""
			for _, u := range ruleAnalysis.Undefined {
				if strings.Contains(ruleContent, "<include") && strings.Contains(ruleContent, "$"+u+".") {
					includeHint = fmt.Sprintf(" <include('...')> 缺少 as $%s，正确写法：<include('golang-gin-context')> as $%s。", u, u)
					break
				}
			}
			undefSet := make(map[string]bool)
			for _, u := range ruleAnalysis.Undefined {
				undefSet[u] = true
			}
			chain := buildBottomUpZeroChain(varOrder, resultVarsDiagnostic, ruleAnalysis.Dependencies, undefSet)
			chainHint := ""
			if chain != "" {
				chainHint = fmt.Sprintf(" 【从下往上】%s，根因：%s 未定义导致后续变量均为 0。", chain, undefList)
			}
			diagnosticHint = fmt.Sprintf("【未定义变量】%s 被使用但未定义。规则中不应存在未定义的变量。%s%s 请为所有被引用的变量提供定义（如 include 需带 as $var）。", undefList, chainHint, includeHint)
		} else {
			// 无未定义变量时，从下往上分析依赖链
			orderToUse := varOrder
			if len(orderToUse) == 0 {
				for name := range resultVarsDiagnostic {
					orderToUse = append(orderToUse, name)
				}
			}
			// 构建从下往上的断点链：从 0 的变量追溯到根本原因
			chain := buildBottomUpZeroChain(orderToUse, resultVarsDiagnostic, ruleAnalysis.Dependencies, nil)
			if chain != "" {
				diagnosticHint = fmt.Sprintf("【从下往上分析】%s 数量为 0 表示其前的模式未匹配或依赖的变量为 0。根因多为链首变量（如 include 输出）未正确匹配。建议：1) 检查 <include> 是否带 as $var；2) 对照样例确认方法名、包路径；3) 链首为 0 时可拆分复合模式逐段验证。", chain)
			} else {
				firstZeroVar := ""
				for _, name := range orderToUse {
					if cnt, ok := resultVarsDiagnostic[name]; ok && cnt == 0 {
						firstZeroVar = name
						break
					}
				}
				if firstZeroVar != "" {
					diagnosticHint = fmt.Sprintf("【断点】$%s 为 0：其前的模式/include 未匹配样例中的 API。建议：1) 对照样例代码确认方法名、包路径；2) 检查 <include> 是否选对框架并带 as $var；3) 若为链首变量，简化模式或用 .methodName 精确匹配样例中的调用", firstZeroVar)
				} else {
					diagnosticHint = "所有变量均有值但无告警：检查 alert 变量是否在数据流末尾，以及 #-> 连接是否完整"
				}
			}
		}

		suggestion = "根据 diagnostic_hint 与变量链修改规则。理解变量依赖关系：$param 依赖 $context（来自 $context.Query(* as $param)），$context 依赖 $gin（来自 $gin.Context as $context），$gin 来自 include。若链首变量未定义或为 0，后续变量必然为 0。"
		suggestion += " 优先简化：若迭代多次未通过，回归 initial_rule_samples 中的参考规则，用最小模式验证后再扩展。"
		firstZeroOrUndef := ""
		undefSet := make(map[string]bool)
		for _, u := range ruleAnalysis.Undefined {
			undefSet[u] = true
		}
		for _, name := range varOrder {
			if undefSet[name] {
				firstZeroOrUndef = name
				break
			}
			if cnt, ok := resultVarsDiagnostic[name]; ok && cnt == 0 {
				firstZeroOrUndef = name
				break
			}
		}
		if firstZeroOrUndef != "" && (firstZeroOrUndef == "input" || firstZeroOrUndef == "source" || firstZeroOrUndef == "param" || firstZeroOrUndef == "gin" || firstZeroOrUndef == "context" || len(varOrder) > 0 && varOrder[0] == firstZeroOrUndef) {
			suggestion += " 若链首变量为 0 难以定位，可尝试拆分复合模式。若 include 相关变量为 0，可读取 syntaxflow-ai-training-materials/awesome-rule 中对应 lib 文件查看 lib 内部模式。"
		}
	}
	return VerifySFRuleMatchesSampleResult{
		Matched:              false,
		Message:              msg,
		Suggestion:           suggestion,
		ResultVarsDiagnostic: resultVarsDiagnostic,
		DiagnosticHint:       diagnosticHint,
	}
}
