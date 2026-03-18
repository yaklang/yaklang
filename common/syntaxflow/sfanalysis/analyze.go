package sfanalysis

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/result"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type compileOutcome struct {
	frame        *sfvm.SFFrame
	syntaxErrors []*result.StaticAnalyzeResult
	rawErrors    antlr4util.SourceCodeErrors
	compileErr   error
}

func Analyze(ctx context.Context, code string, opts Options) *Report {
	if ctx == nil {
		ctx = context.Background()
	}
	opts = normalizeOptions(opts)

	isBlank := strings.TrimSpace(code) == ""
	report := &Report{
		Profile: opts.Profile,
		Code:    code,
		IsBlank: isBlank,
	}

	// Blank draft rules are common while authoring. Treat them as an empty program rather than a
	// syntax error, while still keeping them identifiable via `Report.IsBlank`.
	compileContent := code
	if isBlank {
		compileContent = "\n"
	}

	if opts.VerifySampleCode && isBlank && strings.TrimSpace(opts.SampleCode) != "" && opts.SampleLanguage != "" {
		report.Sample = &SampleVerificationResult{
			Matched:    false,
			Message:    "规则内容为空白，无法执行正例自检",
			Error:      ProblemTypeBlankRule,
			Suggestion: "请先编写非空规则后再进行正例验证",
		}
	}

	select {
	case <-ctx.Done():
		return report
	default:
	}

	compiled := compileSyntaxFlow(compileContent)
	report.SyntaxErrors = compiled.syntaxErrors
	if opts.NeedFormattedSyntax && len(compiled.syntaxErrors) > 0 {
		report.FormattedSyntaxErrors = FormatSyntaxFlowErrors(code, compiled.syntaxErrors)
	}
	if len(compiled.syntaxErrors) > 0 {
		if needsQualityResult(opts) {
			report.Quality = newQualityResult()
			report.Quality.Score -= SyntaxErrorPenalty
			appendSyntaxProblems(report.Quality, compiled)
			calculateGradeAndSummary(report.Quality)
		}
		return report
	}

	report.Frame = compiled.frame

	if opts.VerifyEmbeddedTests {
		report.EmbeddedVerify = runEmbeddedVerifyWithFrame(compiled.frame, opts.VerifyOptions...)
	}

	if opts.VerifySampleCode && !isBlank {
		sample := verifySFRuleMatchesSampleWithFrame(compiled.frame, code, opts.SampleCode, opts.SampleFilename, opts.SampleLanguage)
		report.Sample = &sample
	}

	if needsQualityResult(opts) {
		report.Quality = analyzeQuality(compiled.frame, report.EmbeddedVerify, isBlank)
	}

	return report
}

func normalizeOptions(opts Options) Options {
	if opts.Profile == "" {
		opts = DefaultOptions(ProfileEditor)
	}
	if opts.VerifyEmbeddedTests && len(opts.VerifyOptions) == 0 {
		opts.VerifyOptions = DefaultVerifyOptions(opts.Profile)
	}
	return opts
}

func needsQualityResult(opts Options) bool {
	return opts.CheckMetadata || opts.CheckRuleLogic || opts.NeedScore
}

func newQualityResult() *SyntaxFlowRuleAnalyzeResult {
	return &SyntaxFlowRuleAnalyzeResult{
		Score:    MaxScore,
		MaxScore: MaxScore,
		Problems: make([]SyntaxFlowRuleProblem, 0),
	}
}

func analyzeQuality(frame *sfvm.SFFrame, verify *EmbeddedVerifyReport, isBlank bool) *SyntaxFlowRuleAnalyzeResult {
	result := newQualityResult()
	checkDescriptionCompleteness(result, frame)
	checkTestData(result, frame, verify)
	checkRuleLogic(result, frame, isBlank)
	calculateGradeAndSummary(result)
	return result
}

func appendSyntaxProblems(result *SyntaxFlowRuleAnalyzeResult, compiled compileOutcome) {
	if len(compiled.rawErrors) > 0 {
		for _, e := range compiled.rawErrors {
			result.Problems = append(result.Problems, SyntaxFlowRuleProblem{
				Type:        ProblemTypeSyntaxError,
				Severity:    Error,
				Description: fmt.Sprintf("语法错误: %s", e.Error()),
				Suggestion:  "请检查规则语法是否正确",
				Range: &ypb.Range{
					StartLine:   int64(e.StartPos.GetLine()),
					StartColumn: int64(e.StartPos.GetColumn()),
					EndLine:     int64(e.EndPos.GetLine()),
					EndColumn:   int64(e.EndPos.GetColumn()),
				},
			})
		}
		return
	}

	description := "语法错误"
	if compiled.compileErr != nil {
		description = fmt.Sprintf("语法错误: %s", compiled.compileErr.Error())
	}
	result.Problems = append(result.Problems, SyntaxFlowRuleProblem{
		Type:        ProblemTypeSyntaxError,
		Severity:    Error,
		Description: description,
		Suggestion:  "请检查规则语法是否正确",
	})
}

func compileSyntaxFlow(code string) compileOutcome {
	compileContent := code
	// Normalize fully blank draft content so it can be parsed as an empty program and NOT be treated
	// as a syntax error.
	if strings.TrimSpace(compileContent) == "" {
		compileContent = "\n"
	}

	vm := sfvm.NewSyntaxFlowVirtualMachine()
	frame, err := vm.Compile(compileContent)
	if err == nil {
		return compileOutcome{
			frame: frame,
		}
	}

	errs := vm.GetCompileErrors()
	if errs == nil || len(errs) == 0 {
		return compileOutcome{
			compileErr: err,
			syntaxErrors: []*result.StaticAnalyzeResult{{
				Message:         fmt.Sprintf("基础语法错误（Syntax Error）：%v", err),
				Severity:        result.Error,
				StartLineNumber: 0,
				StartColumn:     0,
				EndLineNumber:   0,
				EndColumn:       1,
				From:            "compiler",
			}},
		}
	}

	ret := compileOutcome{
		compileErr:   err,
		rawErrors:    errs,
		syntaxErrors: make([]*result.StaticAnalyzeResult, 0, len(errs)),
	}
	for _, e := range errs {
		ret.syntaxErrors = append(ret.syntaxErrors, &result.StaticAnalyzeResult{
			Message:         fmt.Sprintf("基础语法错误（Syntax Error）：%v", e.Message),
			Severity:        result.Error,
			StartLineNumber: int64(e.StartPos.GetLine()),
			StartColumn:     int64(e.StartPos.GetColumn()),
			EndLineNumber:   int64(e.EndPos.GetLine()),
			EndColumn:       int64(e.EndPos.GetColumn() + 1),
			From:            "compiler",
		})
	}
	return ret
}

func checkDescriptionCompleteness(result *SyntaxFlowRuleAnalyzeResult, frame *sfvm.SFFrame) {
	if frame.GetRule().Description == "" {
		result.Problems = append(result.Problems, SyntaxFlowRuleProblem{
			Type:        ProblemTypeLackDescriptionField,
			Severity:    Warning,
			Description: "缺少必要的描述字段: description",
			Suggestion:  "建议在 desc() 中添加 description 字段",
		})
		result.Score -= MissingDescriptionPenalty
	}

	if frame.GetRule().Solution == "" {
		result.Problems = append(result.Problems, SyntaxFlowRuleProblem{
			Type:        ProblemTypeLackSolutionField,
			Severity:    Warning,
			Description: "缺少必要的描述字段: solution",
			Suggestion:  "建议在 desc() 中添加 solution 字段",
		})
		result.Score -= MissingSolutionPenalty
	}
}

func checkTestData(result *SyntaxFlowRuleAnalyzeResult, frame *sfvm.SFFrame, verify *EmbeddedVerifyReport) {
	positiveTests, err := frame.ExtractVerifyFilesystemAndLanguage()
	if err != nil || len(positiveTests) == 0 {
		result.Problems = append(result.Problems, SyntaxFlowRuleProblem{
			Type:        ProblemTypeMissingPositiveTestData,
			Severity:    Warning,
			Description: "缺少正例测试数据（用于验证规则能够匹配问题代码）",
			Suggestion:  "建议在 desc() 中添加验证测试文件",
		})
		result.Score -= MissingPositiveTestPenalty
	}

	negativeTests, err := frame.ExtractNegativeFilesystemAndLanguage()
	if err != nil || len(negativeTests) == 0 {
		result.Problems = append(result.Problems, SyntaxFlowRuleProblem{
			Type:        ProblemTypeMissingNegativeTestData,
			Severity:    Warning,
			Description: "缺少反例测试数据（用于验证规则不会对正常代码误报）",
			Suggestion:  "建议在 desc() 中添加反例测试文件",
		})
		result.Score -= MissingNegativeTestPenalty
	}

	if verify != nil && verify.Error != nil {
		result.Problems = append(result.Problems, SyntaxFlowRuleProblem{
			Type:        ProblemTypeTestCaseNotPass,
			Severity:    Error,
			Description: "验证规则的测试案例无法通过",
			Suggestion:  "检查规则与内嵌正反例是否一致",
		})
		result.Score -= VerifyTestCaseNotPassPenalty
	}
}

func checkRuleLogic(result *SyntaxFlowRuleAnalyzeResult, frame *sfvm.SFFrame, isBlank bool) {
	if len(frame.GetRule().AlertDesc) > 0 {
		return
	}

	severity := Error
	// For a completely blank draft rule, missing alert is expected during authoring; treat it as a
	// warning (while still scoring it to 0).
	if isBlank {
		severity = Warning
	}

	result.Problems = append(result.Problems, SyntaxFlowRuleProblem{
		Type:        ProblemTypeMissingAlert,
		Severity:    severity,
		Description: "缺少告警语句",
		Suggestion:  "规则应该包含 alert 语句来产生检测结果",
	})
	result.Score -= MissingAlertPenalty
}

func calculateGradeAndSummary(result *SyntaxFlowRuleAnalyzeResult) {
	if result.Score < 0 {
		result.Score = 0
	}

	var errorCount, warningCount, infoCount int
	for _, problem := range result.Problems {
		switch problem.Severity {
		case Error:
			errorCount++
		case Warning:
			warningCount++
		case Info:
			infoCount++
		}
	}

	log.Info(fmt.Sprintf("规则质量评分: %d/%d, 发现 %d 个错误, %d 个警告, %d 个建议",
		result.Score, result.MaxScore, errorCount, warningCount, infoCount))
}

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
		buf.WriteString("【heredoc 结束符错误】heredoc（如 desc: <<<TEXT ... TEXT）的结束标识符必须单独占一行且行首无空格。错误：`    TEXT`（有前导空格，不会被识别）。正确：换行后紧跟 `TEXT` 或 `DESC`，无任何前导空格或制表符。参考 golang-reflected-xss-gin-context.sf。\n")
	}
	return strings.TrimSpace(buf.String())
}

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
			Matched:    false,
			Message:    "样例代码解析失败: " + err.Error(),
			Error:      err.Error(),
			Suggestion: "请确认 sample_code 为有效的 " + string(lang) + " 代码，且 filename 扩展名正确（如 .go/.java/.php/.c）",
		}
	}
	if len(progs) == 0 {
		return SampleVerificationResult{
			Matched:    false,
			Message:    "样例代码未能生成有效程序",
			Error:      "empty_program",
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
			Matched:    false,
			Message:    "规则执行失败: " + err.Error(),
			Error:      err.Error(),
			Suggestion: "请先修复规则语法，并确认规则中的 include/lib 引用是否可用",
		}
	}
	if len(sfResult.GetErrors()) > 0 {
		return SampleVerificationResult{
			Matched:    false,
			Message:    "规则执行有错误: " + strings.Join(sfResult.GetErrors(), "; "),
			Error:      strings.Join(sfResult.GetErrors(), "; "),
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
			QueryResultsFull: sfResult.String(),
		}
	}

	hint := "没有任何 alert 命中，通常表示 source/sink 链路没有贯通。"
	if len(resultVarsDiagnostic) > 0 {
		hint = "请优先检查匹配数为 0 的变量，它通常是断链位置。"
	}
	return SampleVerificationResult{
		Matched:              false,
		Message:              "规则未能匹配样例中的漏洞",
		ResultVarsDiagnostic: resultVarsDiagnostic,
		DiagnosticHint:       hint,
		Suggestion:           "根据 result_vars_diagnostic 中各变量匹配数量调整规则，再重新验证",
	}
}
