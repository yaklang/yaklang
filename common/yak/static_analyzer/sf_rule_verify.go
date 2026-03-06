package static_analyzer

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/result"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// SampleVerificationResult 规则与样例匹配验证结果（原 sfverify.VerifySFRuleMatchesSampleResult）
type SampleVerificationResult struct {
	Matched              bool           `json:"matched"`
	Message              string         `json:"message"`
	Error                string         `json:"error,omitempty"`
	AlertCount           int            `json:"alert_count,omitempty"`
	AlertDetails         map[string]int `json:"alert_details,omitempty"`
	QueryResultsFull     string         `json:"query_results_full,omitempty"`
	Suggestion           string         `json:"suggestion,omitempty"`
	ResultVarsDiagnostic map[string]int `json:"result_vars_diagnostic,omitempty"`
	DiagnosticHint       string         `json:"diagnostic_hint,omitempty"`
}

// SyntaxFlowCheckResult 合并语法检查与正例自检结果
type SyntaxFlowCheckResult struct {
	SyntaxErrors   []*result.StaticAnalyzeResult
	FormattedErrors string // 语法错误的富格式输出，供 AI 工具直接展示
	Sample         *SampleVerificationResult
}

// SyntaxFlowRuleCheckingWithSample 语法检查 + 正例自检（当 sampleCode、language 非空时）
// 仅编译一次：有语法错误时返回错误；无错误且无样例时返回空；无错误且有样例时复用已编译 frame 执行正例自检
func SyntaxFlowRuleCheckingWithSample(code, sampleCode, filename, language string) SyntaxFlowCheckResult {
	syntaxErrs, frame := syntaxFlowCompileAndCheck(code)
	if len(syntaxErrs) > 0 {
		return SyntaxFlowCheckResult{
			SyntaxErrors:    syntaxErrs,
			FormattedErrors: FormatSyntaxFlowErrors(code, syntaxErrs),
			Sample:          nil,
		}
	}
	if strings.TrimSpace(sampleCode) == "" || language == "" {
		return SyntaxFlowCheckResult{SyntaxErrors: nil, Sample: nil}
	}
	sample := verifySFRuleMatchesSampleWithFrame(frame, code, sampleCode, filename, language)
	return SyntaxFlowCheckResult{SyntaxErrors: nil, Sample: &sample}
}

// syntaxFlowCompileAndCheck 编译一次，返回语法错误（有则）或已编译的 frame（无错误时）
func syntaxFlowCompileAndCheck(code string) ([]*result.StaticAnalyzeResult, *sfvm.SFFrame) {
	vm := sfvm.NewSyntaxFlowVirtualMachine()
	frame, err := vm.Compile(code)
	if err == nil {
		return nil, frame
	}
	errs := vm.GetErrors()
	if errs == nil || len(errs) == 0 {
		return []*result.StaticAnalyzeResult{{
			Message:         fmt.Sprintf("基础语法错误（Syntax Error）：%v", err),
			Severity:        result.Error,
			StartLineNumber: 0,
			StartColumn:     0,
			EndLineNumber:   0,
			EndColumn:       1,
			From:            "compiler",
		}}, nil
	}
	var results []*result.StaticAnalyzeResult
	for _, e := range errs {
		results = append(results, &result.StaticAnalyzeResult{
			Message:         fmt.Sprintf("基础语法错误（Syntax Error）：%v", e.Message),
			Severity:        result.Error,
			StartLineNumber: int64(e.StartPos.GetLine()),
			StartColumn:     int64(e.StartPos.GetColumn()),
			EndLineNumber:   int64(e.EndPos.GetLine()),
			EndColumn:       int64(e.EndPos.GetColumn() + 1),
			From:            "compiler",
		})
	}
	return results, nil
}

// FormatSyntaxFlowErrors 将语法错误格式化为富文本，含上下文与可操作提示（heredoc、desc 等）
func FormatSyntaxFlowErrors(content string, errs []*result.StaticAnalyzeResult) string {
	if len(errs) == 0 {
		return ""
	}
	me := memedit.NewMemEditor(content)
	var buf bytes.Buffer
	sorted := make([]*result.StaticAnalyzeResult, len(errs))
	copy(sorted, errs)
	sort.Slice(sorted, func(i, j int) bool {
		si, sj := sorted[i], sorted[j]
		if si == nil || sj == nil {
			return false
		}
		if si.StartLineNumber != sj.StartLineNumber {
			return si.StartLineNumber < sj.StartLineNumber
		}
		return si.StartColumn < sj.StartColumn
	})
	errTextPreview := ""
	if len(sorted) > 0 && sorted[0] != nil {
		errTextPreview = sorted[0].Message
	}
	if strings.Contains(content, "<<<") && strings.Contains(errTextPreview, "mismatched input ':'") && strings.Contains(errTextPreview, "expecting") {
		buf.WriteString("【错误类型】heredoc 结束符格式错误\n")
		buf.WriteString("【原因】heredoc（如 <<<TEXT ... TEXT）的结束标识符有前导空格，未被识别，导致解析异常。\n")
		buf.WriteString("【修复】结束标识符必须单独占一行且行首无空格。错误：`    TEXT`。正确：换行后紧跟 `TEXT`。\n")
		buf.WriteString("------------------------\n")
	}
	maxShow := 3
	if len(sorted) < maxShow {
		maxShow = len(sorted)
	}
	for i := 0; i < maxShow; i++ {
		e := sorted[i]
		if e == nil {
			continue
		}
		buf.WriteString(e.Message + "\n")
		if e.StartLineNumber >= 0 && e.EndLineNumber >= 0 {
			markedErr := me.GetTextContextWithPrompt(
				memedit.NewRange(
					memedit.NewPosition(int(e.StartLineNumber), int(e.StartColumn)),
					memedit.NewPosition(int(e.EndLineNumber), int(e.EndColumn)),
				),
				3, e.Message,
			)
			if markedErr != "" {
				buf.WriteString(markedErr)
			}
		}
		buf.WriteString("------------------------\n")
	}
	if len(sorted) > maxShow {
		buf.WriteString("------------------------\n")
		buf.WriteString(fmt.Sprintf("还有 %d 个错误，建议先修复以上关键问题\n", len(sorted)-maxShow))
	}
	errText := buf.String()
	if strings.Contains(content, "desc(") && (strings.Contains(errText, "missing ')'") || strings.Contains(errText, "mismatched input ','")) {
		buf.WriteString("------------------------\n")
		buf.WriteString("【desc 格式提示】若错误位于 desc 块内：字段必须为 fieldName: value（冒号不可省略），字段间用换行分隔、禁止用逗号。参考 golang-template-ssti.sf 的 desc 写法。\n")
	}
	if strings.Contains(content, "<<<") && strings.Contains(errText, "mismatched input ':'") && strings.Contains(errText, "expecting") {
		buf.WriteString("------------------------\n")
		buf.WriteString("【heredoc 结束符错误】heredoc（如 desc: <<<TEXT ... TEXT）的结束标识符必须**单独占一行且行首无空格**。错误：`    TEXT`（有前导空格，不会被识别）。正确：换行后紧跟 `TEXT` 或 `DESC`，无任何前导空格或制表符。参考 golang-reflected-xss-gin-context.sf。\n")
	}
	return strings.TrimSpace(buf.String())
}

// verifySFRuleMatchesSampleWithFrame 使用已编译的 frame 执行正例自检，避免重复编译
func verifySFRuleMatchesSampleWithFrame(frame *sfvm.SFFrame, ruleContent, sampleCode, filename, language string) SampleVerificationResult {
	if ruleContent == "" || sampleCode == "" {
		return SampleVerificationResult{
			Matched: false,
			Message: "缺少必要参数：rule_content、sample_code 均不能为空",
			Error:   "invalid_args",
		}
	}
	lang, err := ssaconfig.ValidateLanguage(language)
	if err != nil {
		return SampleVerificationResult{
			Matched: false,
			Message: "不支持的语言: " + language + "。支持: golang, java, php, c, javascript, yak, python",
			Error:   err.Error(),
		}
	}
	if filename == "" {
		ext := lang.GetFileExt()
		if ext != "" {
			filename = "sample" + ext
		} else {
			filename = "sample"
		}
	}
	if !strings.Contains(filename, ".") && lang.GetFileExt() != "" {
		filename = strings.TrimSuffix(filename, "/") + lang.GetFileExt()
	}
	vfs := filesys.NewVirtualFs()
	vfs.AddFile(filename, sampleCode)
	progs, err := ssaapi.ParseProjectWithFS(vfs, ssaapi.WithLanguage(lang))
	if err != nil {
		return SampleVerificationResult{
			Matched: false,
			Message: "样例代码解析失败: " + err.Error(),
			Error:   err.Error(),
			Suggestion: "请确认 sample_code 为有效的 " + string(lang) + " 代码，且 filename 扩展名正确（如 .go/.java/.php/.c）",
		}
	}
	if len(progs) == 0 {
		return SampleVerificationResult{
			Matched: false,
			Message: "样例代码未能生成有效程序",
			Error:   "empty_program",
			Suggestion: "请检查 sample_code 是否包含可解析的入口（如 package main、class 等）",
		}
	}
	opts := []ssaapi.QueryOption{
		ssaapi.QueryWithPrograms(progs),
		ssaapi.QueryWithFrame(frame),
		ssaapi.QueryWithInitInputVar(progs[0]),
	}
	sfResult, err := ssaapi.QuerySyntaxflow(opts...)
	if err != nil {
		return SampleVerificationResult{
			Matched: false,
			Message: "规则执行失败: " + err.Error(),
			Error:   err.Error(),
			Suggestion: "请先调用 check-syntaxflow-syntax 检查规则语法，并确认规则中的 include/lib 引用是否可用",
		}
	}
	if len(sfResult.GetErrors()) > 0 {
		return SampleVerificationResult{
			Matched: false,
			Message: "规则执行有错误: " + strings.Join(sfResult.GetErrors(), "; "),
			Error:   strings.Join(sfResult.GetErrors(), "; "),
			Suggestion: "检查规则逻辑是否与样例中的 API/调用方式一致，如 source/sink 方法名、数据流路径等",
		}
	}
	alertVars := sfResult.GetAlertVariables()
	alertDetails := make(map[string]int)
	resultVarsDiagnostic := make(map[string]int)
	totalAlert := 0
	allVars := sfResult.GetAllVariable()
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
		vals := sfResult.GetValues(name)
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
		return SampleVerificationResult{
			Matched:          true,
			Message:          fmt.Sprintf("规则已正确匹配样例中的漏洞，触发 %d 处告警", totalAlert),
			AlertCount:       totalAlert,
			AlertDetails:     alertDetails,
			QueryResultsFull: sfResult.Dump(true),
		}
	}
	suggestion := "请检查：1) 规则的 source/sink 模式是否覆盖样例中的调用链；2) 数据流 #-> 是否与样例实际路径一致；3) 条件过滤 ?{} 是否过于严格"
	msg := "规则未在样例上触发告警，可能未正确匹配漏洞"
	var diagnosticHint string
	ruleAnalysis := parseRuleVarAnalysis(ruleContent)
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
			orderToUse := varOrder
			if len(orderToUse) == 0 {
				for name := range resultVarsDiagnostic {
					orderToUse = append(orderToUse, name)
				}
			}
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
		if firstZeroOrUndef != "" && (firstZeroOrUndef == "input" || firstZeroOrUndef == "source" || firstZeroOrUndef == "param" || firstZeroOrUndef == "gin" || firstZeroOrUndef == "context" || (len(varOrder) > 0 && varOrder[0] == firstZeroOrUndef)) {
			suggestion += " 若链首变量为 0 难以定位，可尝试拆分复合模式。若 include 相关变量为 0，可读取 syntaxflow-ai-training-materials/awesome-rule 中对应 lib 文件查看 lib 内部模式。"
		}
	}
	return SampleVerificationResult{
		Matched:              false,
		Message:              msg,
		Suggestion:           suggestion,
		ResultVarsDiagnostic: resultVarsDiagnostic,
		DiagnosticHint:       diagnosticHint,
	}
}

// --- rule parse (from sfverify/rule_parse.go) ---

type ruleVarAnalysis struct {
	Defined      map[string]bool
	Used         map[string]bool
	Undefined    []string
	Dependencies map[string][]string
}

var (
	reVarDefAs     = regexp.MustCompile(`as\s+\$([a-zA-Z0-9_]+)`)
	reIncludeAs    = regexp.MustCompile(`<include\s*\([^)]+\)\s*>\s*as\s+\$([a-zA-Z0-9_]+)`)
	reVarUsedDot   = regexp.MustCompile(`\$([a-zA-Z0-9_]+)\.`)
	reVarUsedSpace = regexp.MustCompile(`\$([a-zA-Z0-9_]+)(?:\s|\))`)
	reDataflowTo   = regexp.MustCompile(`#->\s*\$([a-zA-Z0-9_]+)`)
	reVarInPattern = regexp.MustCompile(`\$([a-zA-Z0-9_]+)`)
)

func parseRuleVarAnalysis(ruleContent string) *ruleVarAnalysis {
	a := &ruleVarAnalysis{
		Defined:      make(map[string]bool),
		Used:         make(map[string]bool),
		Dependencies: make(map[string][]string),
	}
	body := stripDescBlocks(ruleContent)
	for _, m := range reIncludeAs.FindAllStringSubmatch(body, -1) {
		if len(m) >= 2 && m[1] != "_" {
			a.Defined[m[1]] = true
		}
	}
	for _, m := range reVarDefAs.FindAllStringSubmatch(body, -1) {
		if len(m) >= 2 && m[1] != "_" {
			a.Defined[m[1]] = true
		}
	}
	for _, m := range reVarUsedDot.FindAllStringSubmatch(body, -1) {
		if len(m) >= 2 && m[1] != "_" {
			a.Used[m[1]] = true
		}
	}
	for _, m := range reVarUsedSpace.FindAllStringSubmatch(body, -1) {
		if len(m) >= 2 && m[1] != "_" {
			a.Used[m[1]] = true
		}
	}
	for _, m := range reDataflowTo.FindAllStringSubmatch(body, -1) {
		if len(m) >= 2 && m[1] != "_" {
			a.Used[m[1]] = true
		}
	}
	for _, m := range reVarInPattern.FindAllStringSubmatch(body, -1) {
		if len(m) >= 2 && m[1] != "_" {
			a.Used[m[1]] = true
		}
	}
	seenUndef := make(map[string]bool)
	for v := range a.Used {
		if !a.Defined[v] && !seenUndef[v] {
			a.Undefined = append(a.Undefined, v)
			seenUndef[v] = true
		}
	}
	reDepChain := regexp.MustCompile(`\$([a-zA-Z0-9_]+)\.([a-zA-Z0-9_*]+)\s*\([^)]*\)\s*as\s+\$([a-zA-Z0-9_]+)`)
	for _, m := range reDepChain.FindAllStringSubmatch(body, -1) {
		if len(m) >= 4 {
			base, target := m[1], m[3]
			if base != "_" && target != "_" {
				a.Dependencies[target] = appendUniq(a.Dependencies[target], base)
			}
		}
	}
	reDepSimple := regexp.MustCompile(`\$([a-zA-Z0-9_]+)\.([a-zA-Z0-9_*]+)\s+as\s+\$([a-zA-Z0-9_]+)`)
	for _, m := range reDepSimple.FindAllStringSubmatch(body, -1) {
		if len(m) >= 4 {
			base, target := m[1], m[3]
			if base != "_" && target != "_" {
				a.Dependencies[target] = appendUniq(a.Dependencies[target], base)
			}
		}
	}
	reDataflowDep := regexp.MustCompile(`\$([a-zA-Z0-9_]+)\s+#->[^;]*?as\s+\$([a-zA-Z0-9_]+)`)
	for _, m := range reDataflowDep.FindAllStringSubmatch(body, -1) {
		if len(m) >= 3 && m[1] != "_" && m[2] != "_" {
			a.Dependencies[m[2]] = appendUniq(a.Dependencies[m[2]], m[1])
		}
	}
	reParamInCall := regexp.MustCompile(`\$([a-zA-Z0-9_]+)\.[^;]*?\*\s*(?:#->\s*)?as\s+\$([a-zA-Z0-9_]+)`)
	for _, m := range reParamInCall.FindAllStringSubmatch(body, -1) {
		if len(m) >= 3 {
			base, target := m[1], m[2]
			if base != "_" && target != "_" {
				a.Dependencies[target] = appendUniq(a.Dependencies[target], base)
			}
		}
	}
	return a
}

func stripDescBlocks(s string) string {
	inDesc := false
	parenDepth := 0
	var out strings.Builder
	i := 0
	for i < len(s) {
		if i+4 <= len(s) && strings.ToLower(s[i:i+4]) == "desc" {
			inDesc = true
			parenDepth = 0
			i += 4
			for i < len(s) && (s[i] == ' ' || s[i] == '(') {
				if s[i] == '(' {
					parenDepth++
				}
				i++
			}
			continue
		}
		if inDesc {
			if s[i] == '(' {
				parenDepth++
			} else if s[i] == ')' {
				parenDepth--
				if parenDepth <= 0 {
					inDesc = false
				}
			}
			i++
			continue
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}

func appendUniq(slice []string, v string) []string {
	for _, x := range slice {
		if x == v {
			return slice
		}
	}
	return append(slice, v)
}

func buildBottomUpZeroChain(varOrder []string, diag map[string]int, deps map[string][]string, undefined map[string]bool) string {
	type link struct {
		varName string
		reason  string
	}
	var chain []link
	for i := len(varOrder) - 1; i >= 0; i-- {
		name := varOrder[i]
		cnt, ok := diag[name]
		if !ok {
			continue
		}
		if cnt == 0 {
			reason := ""
			if parents, has := deps[name]; has && len(parents) > 0 {
				for _, p := range parents {
					if undefined[p] {
						reason = "因 $" + p + " 未定义"
					} else if pc, pok := diag[p]; pok && pc == 0 {
						reason = "因依赖 $" + p + ":0"
					} else if !pok {
						reason = "因 $" + p + " 可能未定义"
					}
					if reason != "" {
						break
					}
				}
			}
			if reason == "" {
				if undefined[name] {
					reason = "变量未定义（如 include 缺少 as $" + name + "）"
				} else {
					reason = "其前模式/include 未匹配"
				}
			}
			chain = append(chain, link{name, reason})
		}
	}
	if len(chain) == 0 {
		return ""
	}
	var parts []string
	for j := len(chain) - 1; j >= 0; j-- {
		l := chain[j]
		parts = append(parts, fmt.Sprintf("$%s:0 ← %s", l.varName, l.reason))
	}
	return strings.Join(parts, "；")
}
