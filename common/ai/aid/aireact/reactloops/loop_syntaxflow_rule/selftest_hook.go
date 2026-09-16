package loop_syntaxflow_rule

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loopinfra"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfanalysis"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
)

const (
	syntaxflowNodeSampleSelfTest = "syntaxflow-sample-self-test"

	loopVarSfVerifyMatched     = "sf_verify_matched"
	loopVarSfVerifyLastFeedback = "sf_verify_last_feedback"
)

// resetSfVerifyMatchedAfterCodeChange clears sample self-test results whenever the rule changes.
// Previous sf_verify_matched must not survive write/modify/insert/delete; only
// postSyntaxCleanHook (after lint pass) may set it again.
func resetSfVerifyMatchedAfterCodeChange(loop interface {
	Set(string, any)
}) {
	if loop == nil {
		return
	}
	loop.Set(loopVarSfVerifyMatched, "")
	loop.Set(loopVarSfVerifyLastFeedback, "")
}

// buildSyntaxFlowPostSyntaxCleanSelfTestHook runs positive-sample self-test after static lint passes.
func buildSyntaxFlowPostSyntaxCleanSelfTestHook(r aicommon.AIInvokeRuntime) loopinfra.PostSyntaxCleanHook {
	return func(loop *reactloops.ReActLoop, op *reactloops.LoopActionHandlerOperator) (string, bool) {
		if loop == nil || op == nil {
			return "", false
		}

		hasSample := utils.InterfaceToBoolean(loop.Get("sf_has_code_sample"))
		if !hasSample {
			resetSfVerifyMatchedAfterCodeChange(loop)
			return "", false
		}

		if utils.InTestcase() {
			log.Infof("skip syntaxflow sample self-test: testcase mode")
			return "", false
		}

		ruleCode := strings.TrimSpace(loop.Get("full_sf_code"))
		sampleCode := strings.TrimSpace(loop.Get("sf_sample_code"))
		sampleFilename := strings.TrimSpace(loop.Get("sf_sample_filename"))
		sampleLanguage := strings.TrimSpace(loop.Get("sf_sample_language"))
		if sampleCode == "" || sampleLanguage == "" {
			feedback := "【正例自检跳过】已标记有正例，但 sf_sample_code 或 sf_sample_language 为空，无法自动自检。请补全样例后重新 write_rule/modify_rule。"
			loop.Set(loopVarSfVerifyMatched, "false")
			loop.Set(loopVarSfVerifyLastFeedback, feedback)
			r.AddToTimeline("sf_selftest_skipped", feedback)
			emitSyntaxFlowSelfTestSkipped(loop, "missing sample_code/language")
			return feedback, true
		}

		emitSyntaxFlowSelfTestStart(loop, sampleFilename, sampleLanguage)
		res := static_analyzer.SyntaxFlowRuleCheckingWithSample(ruleCode, sampleCode, sampleFilename, sampleLanguage)
		if len(res.SyntaxErrors) > 0 {
			// Lint already passed; treat unexpected compile errors as block.
			feedback := "【正例自检失败】规则在样例检查阶段出现语法错误：\n" + res.FormattedErrors
			loop.Set(loopVarSfVerifyMatched, "false")
			loop.Set(loopVarSfVerifyLastFeedback, feedback)
			emitSyntaxFlowSelfTestFinish(loop, false, feedback)
			r.AddToTimeline("sf_selftest_failed", utils.ShrinkTextBlock(feedback, 1024))
			return feedback, true
		}

		feedback, matched := applySampleSelfTestResult(loop, res.Sample)
		emitSyntaxFlowSelfTestFinish(loop, matched, feedback)
		if matched {
			r.AddToTimeline("sf_selftest_passed", "positive sample matched")
			log.Infof("syntaxflow sample self-test passed: file=%s lang=%s", sampleFilename, sampleLanguage)
			return "", false
		}
		r.AddToTimeline("sf_selftest_failed", utils.ShrinkTextBlock(feedback, 1024))
		log.Warnf("syntaxflow sample self-test failed: file=%s lang=%s", sampleFilename, sampleLanguage)
		return feedback, true
	}
}

// applySampleSelfTestResult stores verify flag / feedback from a sample verification result.
// Returns (feedback, matched).
func applySampleSelfTestResult(loop interface {
	Set(string, any)
}, sample *sfanalysis.SampleVerificationResult) (string, bool) {
	if loop == nil {
		return "【正例自检失败】内部错误：loop 为空", false
	}
	if sample == nil {
		feedback := "【正例自检失败】未返回样例验证结果，请检查 sample_code / language 后重试。"
		loop.Set(loopVarSfVerifyMatched, "false")
		loop.Set(loopVarSfVerifyLastFeedback, feedback)
		return feedback, false
	}
	if sample.Matched {
		loop.Set(loopVarSfVerifyMatched, "true")
		loop.Set(loopVarSfVerifyLastFeedback, "")
		return "", true
	}
	feedback := formatSampleSelfTestFailure(sample)
	loop.Set(loopVarSfVerifyMatched, "false")
	loop.Set(loopVarSfVerifyLastFeedback, feedback)
	return feedback, false
}

func formatSampleSelfTestFailure(sample *sfanalysis.SampleVerificationResult) string {
	if sample == nil {
		return "【正例自检未通过】无诊断信息。"
	}
	var b strings.Builder
	b.WriteString("【正例自检未通过】规则未能匹配正例（file://、UNSAFE）。请根据下方诊断修改规则后重试。\n\n")
	if sample.Message != "" {
		b.WriteString("message: ")
		b.WriteString(sample.Message)
		b.WriteByte('\n')
	}
	if sample.Error != "" {
		b.WriteString("error: ")
		b.WriteString(sample.Error)
		b.WriteByte('\n')
	}
	if sample.DiagnosticHint != "" {
		b.WriteString("\ndiagnostic_hint:\n")
		b.WriteString(sample.DiagnosticHint)
		b.WriteByte('\n')
	}
	if len(sample.ResultVarsDiagnostic) > 0 {
		b.WriteString("\nresult_vars_diagnostic:\n")
		keys := make([]string, 0, len(sample.ResultVarsDiagnostic))
		for k := range sample.ResultVarsDiagnostic {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString(fmt.Sprintf("  %s: %d\n", k, sample.ResultVarsDiagnostic[k]))
		}
	}
	if sample.Suggestion != "" {
		b.WriteString("\nsuggestion: ")
		b.WriteString(sample.Suggestion)
		b.WriteByte('\n')
	} else if len(sample.ResultVarsDiagnostic) > 0 {
		b.WriteString("\nsuggestion: 根据 result_vars_diagnostic 中为 0 的变量定位断点；标注 [未定义] 的变量需补全定义（如 include 必须带 as $var）。\n")
	}
	b.WriteString("\n【下一步】使用 modify_rule 修复后，语法通过时系统将自动重新做正例自检；也可主动调用 check-syntaxflow-syntax 传入 path/sample_code/filename/language。")
	return b.String()
}

func emitSyntaxFlowSelfTestStart(loop *reactloops.ReActLoop, filename, language string) {
	if loop == nil {
		return
	}
	line := fmt.Sprintf(
		"运行正例自检: %s (%s) / Running sample self-test: %s (%s)",
		filename, language, filename, language,
	)
	reactloops.EmitActionLog(loop, syntaxflowNodeSampleSelfTest, line)
	reactloops.EmitStatusI18n(loop, "正例自检中", "Running sample self-test...")
}

func emitSyntaxFlowSelfTestFinish(loop *reactloops.ReActLoop, matched bool, feedback string) {
	if loop == nil {
		return
	}
	var finishLine string
	var reference string
	if matched {
		finishLine = "正例自检通过 / Sample self-test passed"
		reactloops.EmitStatusI18n(loop, "正例自检通过", "Sample self-test passed")
	} else {
		errSummary := utils.ShrinkTextBlock(strings.TrimSpace(feedback), 256)
		finishLine = fmt.Sprintf("正例自检失败 — %s / Sample self-test failed — %s", errSummary, errSummary)
		if strings.TrimSpace(feedback) != "" {
			_, reference = reactloops.SpillLongContent(loop, "syntaxflow_selftest_feedback", feedback)
		}
		reactloops.EmitStatusI18n(loop, "正例自检失败，修复中", "Sample self-test failed, fixing...")
	}
	reactloops.EmitActionLog(loop, syntaxflowNodeSampleSelfTest, finishLine, reference)
}

func emitSyntaxFlowSelfTestSkipped(loop *reactloops.ReActLoop, reason string) {
	if loop == nil || strings.TrimSpace(reason) == "" {
		return
	}
	line := fmt.Sprintf("跳过正例自检: %s / Skipped sample self-test: %s", reason, reason)
	reactloops.EmitActionLog(loop, syntaxflowNodeSampleSelfTest, line)
	reactloops.EmitStatusI18n(loop, "正例自检跳过", "Sample self-test skipped")
}
