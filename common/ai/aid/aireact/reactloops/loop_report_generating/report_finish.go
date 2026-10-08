package loop_report_generating

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const reportFinishEventNode = "report-finish"

const reportFinishSummaryMaxChars = 480

// reportFinishEvent renders a coherent overview and a link to the full artifact.
type reportFinishEvent struct {
	ReportPath      string `json:"report_path"`
	Title           string `json:"title,omitempty"`
	SummaryMarkdown string `json:"summary_markdown,omitempty"`
}

func withReportFinishValidation() reactloops.ReActLoopOption {
	return func(loop *reactloops.ReActLoop) {
		finish, err := loop.GetActionHandler("finish")
		if err != nil {
			return
		}
		copy := *finish
		copy.ActionHandler = func(l *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			if _, err := readSavedReport(l); err != nil {
				op.Fail(err)
				return
			}
			finish.ActionHandler(l, action, op)
		}
		reactloops.WithOverrideLoopAction(&copy)(loop)
	}
}

func readSavedReport(loop *reactloops.ReActLoop) (string, error) {
	path := strings.TrimSpace(loop.Get("report_filename"))
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", utils.Errorf("报告文件未保存: %v", err)
	}
	content := strings.TrimSpace(string(raw))
	if content == "" {
		return "", utils.Error("报告文件为空")
	}
	if expected := strings.TrimSpace(loop.Get("full_report_code")); expected != "" && expected != content {
		return "", utils.Error("报告内容尚未完整保存")
	}
	return content, nil
}

func buildReportFinishHook() reactloops.ReActLoopOption {
	return reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, _ int, task aicommon.AIStatefulTask, isDone bool, reason any, _ *reactloops.OnPostIterationOperator) {
		if !isDone || !utils.IsNil(reason) || task.IsUserCancelled() || task.GetContext().Err() != nil {
			return
		}
		last := loop.GetLastAction()
		if last == nil || last.ActionType != "finish" {
			return
		}
		loop.Set("report_finished", "true")
		if loop.Get("internal_report_output") == "true" {
			return
		}
		emitReportFinish(loop)
	})
}

func emitReportFinish(loop *reactloops.ReActLoop) {
	reportPath := strings.TrimSpace(loop.Get("report_filename"))
	if reportPath == "" {
		log.Infof("report_generating: skip report_finish (no report file)")
		return
	}

	content, err := readSavedReport(loop)
	if err != nil {
		log.Warnf("report_generating: skip report_finish (report not saved: %s, %v)", reportPath, err)
		return
	}
	title, summary := buildReportFinishPreview(content)
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(reportPath), filepath.Ext(reportPath))
	}

	if provided := strings.TrimSpace(loop.Get("report_summary_markdown")); provided != "" {
		summary = provided
	}
	if err := EmitReportFinish(loop, reportPath, title, summary); err != nil {
		log.Warnf("report_generating: emit report_finish failed: %v", err)
		return
	}
	loop.Set("result_report_path", reportPath)
	loop.Set("result_summary", summary)
	if task := loop.GetCurrentTask(); task != nil {
		result := summary + "\n\n报告文件：" + reportPath
		task.SetResult(result)
		_, _ = loop.GetEmitter().EmitResultAfterStream("result", result, true)
	}
}

// EmitReportFinish is also used by a parent focus mode after its report child
// succeeds. The caller supplies a user-facing summary, never a head/tail slice.
func EmitReportFinish(loop *reactloops.ReActLoop, reportPath, title, summary string) error {
	content, err := os.ReadFile(reportPath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(content)) == "" {
		return utils.Error("报告文件为空")
	}
	emitter := loop.GetEmitter()
	if emitter == nil {
		return utils.Error("report emitter is nil")
	}
	if _, err := emitter.EmitPinFilename(reportPath); err != nil {
		return err
	}
	if _, err := emitter.EmitJSON(schema.EVENT_TYPE_REPORT_FINISH, reportFinishEventNode, reportFinishEvent{
		ReportPath: reportPath, Title: title, SummaryMarkdown: summary,
	}); err != nil {
		return err
	}
	// The generic report card's open button currently only works in code-audit
	// pages. Preserve the existing reference viewer with the entire saved report.
	reactloops.EmitActionLog(loop, reportFinishEventNode, "完整报告已保存: "+reportPath, string(content))
	if invoker := loop.GetInvoker(); invoker != nil {
		invoker.AddToTimeline("report_finish", fmt.Sprintf(
			"Report finished: %s\nTitle: %s\nSummary:\n%s",
			reportPath, title, summary,
		))
	}
	return nil
}

func buildReportFinishPreview(content string) (title, summary string) {
	if content == "" {
		return "", ""
	}
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			title = strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
			break
		}
	}
	// Select a complete prose paragraph. Never splice the beginning and end of
	// Markdown: that merges unrelated sections and breaks tables/code fences.
	inFence := false
	for _, paragraph := range strings.Split(content, "\n\n") {
		paragraph = strings.TrimSpace(paragraph)
		if strings.Contains(paragraph, "```") || strings.Contains(paragraph, "~~~") {
			if (strings.Count(paragraph, "```")+strings.Count(paragraph, "~~~"))%2 != 0 {
				inFence = !inFence
			}
			continue
		}
		if inFence || paragraph == "" || strings.HasPrefix(paragraph, "#") ||
			strings.HasPrefix(paragraph, "|") || strings.HasPrefix(paragraph, "- ") ||
			len([]rune(paragraph)) > reportFinishSummaryMaxChars {
			continue
		}
		summary = paragraph + "\n\n完整内容见报告文件。"
		break
	}
	if summary == "" {
		summary = "报告已生成，完整内容见报告文件。"
	}
	return title, summary
}
