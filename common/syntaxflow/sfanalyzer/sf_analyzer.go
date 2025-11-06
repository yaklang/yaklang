package sfanalyzer

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// SyntaxFlowAnalyzer SyntaxFlow规则分析器
type SyntaxFlowAnalyzer struct {
	ruleContent string
}

// SyntaxFlowRuleAnalyzeResult 完整分析结果
type SyntaxFlowRuleAnalyzeResult struct {
	Score    int                     `json:"score"`
	MaxScore int                     `json:"max_score"`
	Problems []SyntaxFlowRuleProblem `json:"problems"`
}

// SyntaxFlowRuleProblem 检测到的问题
type SyntaxFlowRuleProblem struct {
	Type        string     `json:"type"`
	Severity    string     `json:"severity"`
	Description string     `json:"description"`
	Suggestion  string     `json:"suggestion"`
	Range       *ypb.Range `json:"range"`
}

// NewSyntaxFlowAnalyzer 创建新的分析器
func NewSyntaxFlowAnalyzer(ruleContent string) *SyntaxFlowAnalyzer {
	return &SyntaxFlowAnalyzer{
		ruleContent: ruleContent,
	}
}

// GetResponse 获取分析响应
func (s *SyntaxFlowRuleAnalyzeResult) GetResponse() *ypb.SmokingEvaluatePluginResponse {
	res := &ypb.SmokingEvaluatePluginResponse{
		Score: int64(s.Score),
	}
	res.Results = make([]*ypb.SmokingEvaluateResult, 0, len(s.Problems))
	for _, problem := range s.Problems {
		result := &ypb.SmokingEvaluateResult{
			Item:       problem.Type,
			Suggestion: problem.Suggestion,

			Range:    problem.Range,
			Severity: problem.Severity,
		}
		res.Results = append(res.Results, result)
	}
	return res
}

// Analyze 执行完整分析
func (s *SyntaxFlowAnalyzer) Analyze() *SyntaxFlowRuleAnalyzeResult {
	result := &SyntaxFlowRuleAnalyzeResult{
		Score:    MaxScore,
		MaxScore: MaxScore,
		Problems: []SyntaxFlowRuleProblem{},
	}

	// 基础语法检查(SF规则能否通过编译)
	frame := s.checkBasicSyntax(result)

	if result.Score == 0 || frame == nil {
		return result
	}

	// 检查描述信息完整性
	s.checkDescriptionCompleteness(result, frame)

	// 检查是否包含测试数据
	s.checkTestData(result, frame)

	// 检查规则逻辑完整性
	s.checkRuleLogic(result, frame)

	// 计算最终等级和总结
	s.calculateGradeAndSummary(result)

	return result
}

// checkBasicSyntax 基础语法检查
func (s *SyntaxFlowAnalyzer) checkBasicSyntax(result *SyntaxFlowRuleAnalyzeResult) *sfvm.SFFrame {
	addSourceCodeError := func(errs antlr4util.SourceCodeErrors) {
		for _, e := range errs {
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
	}
	// 使用sfvm编译检查语法
	vm := sfvm.NewSyntaxFlowVirtualMachine()
	frame, err := vm.Compile(s.ruleContent)
	if err != nil {
		result.Score -= SyntaxErrorPenalty // 语法错误直接扣100分

		if errs := vm.GetCompileErrors(); errs != nil {
			addSourceCodeError(errs)
			return nil
		}

		result.Problems = append(result.Problems, SyntaxFlowRuleProblem{
			Type:        ProblemTypeSyntaxError,
			Severity:    Error,
			Description: fmt.Sprintf("语法错误: %s", err.Error()),
			Suggestion:  "请检查规则语法是否正确",
			Range:       nil,
		})
		return nil
	}
	return frame
}

// checkDescriptionCompleteness 检查描述信息完整性
func (s *SyntaxFlowAnalyzer) checkDescriptionCompleteness(result *SyntaxFlowRuleAnalyzeResult, frame *sfvm.SFFrame) {
	// 按固定顺序检查必要的描述字段，确保问题列表顺序一致

	// 1. 检查description字段
	hasDetailedDescription := frame.GetRule().Description != ""
	if !hasDetailedDescription {
		result.Problems = append(result.Problems, SyntaxFlowRuleProblem{
			Type:        ProblemTypeLackDescriptionField,
			Severity:    Warning,
			Description: "缺少必要的描述字段: description",
			Suggestion:  "建议在desc()中添加description字段",
		})
		result.Score -= MissingDescriptionPenalty
	}

	// 2. 检查solution字段
	hasDetailedSolution := frame.GetRule().Solution != ""
	if !hasDetailedSolution {
		result.Problems = append(result.Problems, SyntaxFlowRuleProblem{
			Type:        ProblemTypeLackSolutionField,
			Severity:    Warning,
			Description: "缺少必要的描述字段: solution",
			Suggestion:  "建议在desc()中添加solution字段",
		})
		result.Score -= MissingSolutionPenalty
	}
}

// checkTestData 检查是否包含测试数据
func (s *SyntaxFlowAnalyzer) checkTestData(result *SyntaxFlowRuleAnalyzeResult, frame *sfvm.SFFrame) {
	positiveTests, err := frame.ExtractVerifyFilesystemAndLanguage()
	if err != nil || positiveTests == nil || len(positiveTests) == 0 {
		result.Problems = append(result.Problems, SyntaxFlowRuleProblem{
			Type:        ProblemTypeMissingPositiveTestData,
			Severity:    Warning,
			Description: "缺少正例测试数据（用于验证规则能够匹配问题代码）",
			Suggestion:  "建议在desc()中添加验证测试文件",
		})
		result.Score -= MissingPositiveTestPenalty // 缺少验证测试数据扣15分
	}
	negativeTests, err := frame.ExtractNegativeFilesystemAndLanguage()
	if err != nil || negativeTests == nil || len(negativeTests) == 0 {
		result.Problems = append(result.Problems, SyntaxFlowRuleProblem{
			Type:        ProblemTypeMissingNegativeTestData,
			Severity:    Warning,
			Description: "缺少反例测试数据（用于验证规则不会对正常代码误报）",
			Suggestion:  "建议在desc()中添加反例测试文件",
		})
		result.Score -= MissingNegativeTestPenalty // 缺少反例测试数据扣5分
	}
	if (positiveTests != nil && len(positiveTests) > 0) || (negativeTests != nil && len(negativeTests) > 0) {
		err = evaluateVerifyFilesystemWithRule(frame.GetRule())
		log.Info(err)
		if err != nil {
			result.Problems = append(result.Problems, SyntaxFlowRuleProblem{
				Type:        ProblemTypeTestCaseNotPass,
				Severity:    Error,
				Description: "验证规则的测试按例无法通过",
				Suggestion:  "检查规则和测试案例是否符合预期",
			})
			result.Score -= VerifyTestCaseNotPassPenalty // 测试用例不通过直接0分
		}
	}
}

// checkRuleLogic 检查规则逻辑完整性
func (s *SyntaxFlowAnalyzer) checkRuleLogic(result *SyntaxFlowRuleAnalyzeResult, frame *sfvm.SFFrame) {
	// 检查是否有alert语句
	hasAlert := len(frame.GetRule().AlertDesc) > 0
	if !hasAlert {
		result.Problems = append(result.Problems, SyntaxFlowRuleProblem{
			Type:        ProblemTypeMissingAlert,
			Severity:    Error,
			Description: "缺少告警语句",
			Suggestion:  "规则应该包含alert语句来产生检测结果",
		})
		result.Score -= MissingAlertPenalty // 规则不包括alert直接0分
	}
}

// calculateGradeAndSummary 计算等级和总结
func (s *SyntaxFlowAnalyzer) calculateGradeAndSummary(result *SyntaxFlowRuleAnalyzeResult) {
	// 确保分数不低于0
	if result.Score < 0 {
		result.Score = 0
	}

	// 计算等级
	//result.Grade = GetGrade(result.Score)

	// 生成总结
	errorCount := 0
	warningCount := 0
	infoCount := 0

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

// BatchAnalyze 批量分析多个规则
func BatchAnalyze(rules map[string]string) map[string]*SyntaxFlowRuleAnalyzeResult {
	results := make(map[string]*SyntaxFlowRuleAnalyzeResult)

	for name, content := range rules {
		analyzer := NewSyntaxFlowAnalyzer(content)
		results[name] = analyzer.Analyze()

		log.Infof("分析规则 %s: 得分 %d/100", name, results[name].Score)
	}

	return results
}

// evaluateVerifyFilesystemWithRule 用于验证规则中内嵌的正反例测试是否符合预期结果
func evaluateVerifyFilesystemWithRule(rule *schema.SyntaxFlowRule) error {
	frame, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(rule.Content)
	if err != nil {
		return err
	}
	verifyFs, err := frame.ExtractVerifyFilesystemAndLanguage()
	if err != nil {
		return err
	}
	log.Infof("unsafe filesystem start")
	if verifyFs == nil {
		log.Errorf("no positive filesystem found in rule: %s", rule.RuleName)
	}

	for _, f := range verifyFs {
		err = checkWithFS(f.GetVirtualFs(), func(p ssaapi.Programs) error {
			// Use the program as the init input var,so that the lib rule which have `$input` can be tested.
			result, err := p.SyntaxFlowWithError(rule.Content, ssaapi.QueryWithInitInputVar(p[0]))
			if err != nil {
				return utils.Errorf("syntax flow content failed: %v", err)
			}
			if err := checkResult(f, rule, result); err != nil {
				return err
			}

			return nil
		}, ssaapi.WithLanguage(f.GetLanguage()))
		if err != nil {
			return err
		}
	}

	check := func(result *ssaapi.SyntaxFlowResult) error {
		if len(result.GetAlertVariables()) > 0 {
			for _, name := range result.GetAlertVariables() {
				vals := result.GetValues(name)
				return utils.Errorf("alert symbol table not empty, have: %v: %v", name, vals)
			}
		}
		return nil
	}

	verifyFs, _ = frame.ExtractNegativeFilesystemAndLanguage()
	if verifyFs == nil {
		log.Errorf("no positive filesystem found in rule: %s", rule.RuleName)
	}
	log.Debug("safe filesystem start")
	for _, f := range verifyFs {
		err = checkWithFS(f.GetVirtualFs(), func(programs ssaapi.Programs) error {
			result, err := programs.SyntaxFlowWithError(rule.Content, ssaapi.QueryWithEnableDebug(), ssaapi.QueryWithInitInputVar(programs[0]))
			if err != nil {
				return utils.Errorf("syntax flow content failed: %v", err)
			}
			if err := check(result); err != nil {
				return utils.Errorf("check content failed: %v", err)
			}
			result2, err := programs.SyntaxFlowRule(rule, ssaapi.QueryWithEnableDebug(), ssaapi.QueryWithInitInputVar(programs[0]))
			if err != nil {
				return utils.Errorf("syntax flow rule failed: %v", err)
			}
			if err := check(result2); err != nil {
				return utils.Errorf("check rule failed: %v", err)
			}
			return nil
		}, ssaapi.WithLanguage(f.GetLanguage()))
		if err != nil {
			return err
		}
	}
	return nil
}

// checkWithFS 辅助函数遍历FS
func checkWithFS(fs fi.FileSystem, handler func(ssaapi.Programs) error, opt ...ssaconfig.Option) error {
	prog, err := ssaapi.ParseProjectWithFS(fs, opt...)
	if err != nil {
		return err
	}
	err = handler(prog)
	return err
}

// checkResult 辅助函数验证测试例子
func checkResult(verifyFs *sfvm.VerifyFileSystem, rule *schema.SyntaxFlowRule, result *ssaapi.SyntaxFlowResult) (errs error) {
	defer func() {
		if errs != nil {
			fs := verifyFs.GetVirtualFs()
			builder := &strings.Builder{}
			entrys, err := fs.ReadDir(".")
			if err != nil {
				return
			}
			for _, entry := range entrys {
				if entry.IsDir() {
					continue
				}
				fileName := entry.Name()
				builder.WriteString(fileName)
				builder.WriteString(" | ")
			}
			errs = utils.Wrapf(errs, "checkResult failed in file: %s", builder.String())
		}
	}()
	result.Show(sfvm.WithShowAll())
	if len(result.GetErrors()) > 0 {
		for _, e := range result.GetErrors() {
			errs = utils.JoinErrors(errs, utils.Errorf("syntax flow failed: %v", e))
		}
		return utils.Errorf("syntax flow failed: %v", strings.Join(result.GetErrors(), "\n"))
	}
	if len(result.GetAlertVariables()) <= 0 {
		errs = utils.JoinErrors(errs, utils.Errorf("alert symbol table is empty"))
		return errs
	}
	if rule.AllowIncluded {
		libOutput := result.GetValues("output")
		if libOutput == nil {
			errs = utils.JoinErrors(errs, utils.Errorf("lib: %v is not exporting output in `alert`", result.Name()))
		}
		if len(libOutput) <= 0 {
			errs = utils.JoinErrors(errs, utils.Errorf("lib: %v is not exporting output in `alert` (empty result)", result.Name()))
		}
	}
	var (
		alertCount = 0
		alert_high = 0
		alert_mid  = 0
		alert_info = 0
	)

	for _, name := range result.GetAlertVariables() {
		alertCount += len(result.GetValues(name))
		count := len(result.GetValues(name))
		if info, b := result.GetAlertInfo(name); b {
			switch info.Severity {
			case "mid", "m", "middle":
				alert_mid += count
			case "high", "h":
				alert_high += count
			case "info", "low":
				alert_info += count
			}
		}
	}
	if alertCount <= 0 {
		errs = utils.JoinErrors(errs, utils.Errorf("alert symbol table is empty"))
		return
	}
	result.Show()

	ret := verifyFs.GetExtraInfoInt("alert_min", "vuln_min", "alertMin", "vulnMin")
	if ret > 0 {
		if alertCount < ret {
			errs = utils.JoinErrors(errs, utils.Errorf("alert symbol table is less than alert_min config: %v actual got: %v", ret, alertCount))
			return
		}
	}
	maxNum := verifyFs.GetExtraInfoInt("alert_max", "vuln_max", "alertMax", "vulnMax")
	if maxNum > 0 {
		if alertCount > maxNum {
			errs = utils.JoinErrors(errs, utils.Errorf("alert symbol table is more than alert_max config: %v actual got: %v", maxNum, alertCount))
			return
		}
	}
	num := verifyFs.GetExtraInfoInt("alert_exact", "alertExact", "vulnExact", "alert_num", "vulnNum")
	if num > 0 {
		if alertCount != num {
			errs = utils.JoinErrors(errs, utils.Errorf("alert symbol table is not equal alert_exact config: %v, actual got: %v", num, alertCount))
			return
		}
	}
	high := verifyFs.GetExtraInfoInt("alert_high", "alertHigh", "vulnHigh")
	if high > 0 {
		if alert_high != high {
			errs = utils.JoinErrors(errs, utils.Errorf("alert symbol table is less than alert_high config: %v, actual got: %v", high, alert_high))
			return
		}
	}
	mid := verifyFs.GetExtraInfoInt("alert_mid", "alertMid", "vulnMid")
	if mid > 0 {
		if alert_mid < mid {
			errs = utils.JoinErrors(errs, utils.Errorf("alert symbol table is less than alert_mid config: %v, actual got: %v", mid, alert_mid))
			return
		}
	}
	low := verifyFs.GetExtraInfoInt("alert_low", "alertMid", "vulnMid", "alert_info")
	if low > 0 {
		if alert_info < low {
			errs = utils.JoinErrors(errs, utils.Errorf("alert symbol table is less than alert_low config: %v, actual got: %v", low, alert_info))
			return
		}
	}

	return
}
