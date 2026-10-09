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

// SummaryMarkdown is the existing frontend card body field. It contains the
// saved Markdown or a caller's overview, bounded for inline display.
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
	content := string(raw)
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return "", utils.Error("报告文件为空")
	}
	if expected := strings.TrimSpace(loop.Get("full_report_code")); expected != "" && expected != trimmed {
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
	title := buildReportFinishTitle(content)
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(reportPath), filepath.Ext(reportPath))
	}

	// Read the final artifact, then bound display copies only. GEN_REPORT bodies
	// and the complete saved report remain intact, including after later edits.
	markdown := reportDisplayMarkdown(content)
	if err := EmitReportFinish(loop, reportPath, title, markdown); err != nil {
		log.Warnf("report_generating: emit report_finish failed: %v", err)
		return
	}
	loop.Set("result_report_path", reportPath)
	loop.Set("result_summary", markdown)
	if task := loop.GetCurrentTask(); task != nil {
		result := markdown + "\n\n报告文件：" + reportPath
		task.SetResult(result)
		_, _ = loop.GetEmitter().EmitResultAfterStream("result", result, true)
	}
}

// EmitReportFinish is also used by a parent focus mode after its report child
// succeeds. Apply the display budget to both standalone reports and parent
// overviews. The complete report remains accessible through the saved file.
func EmitReportFinish(loop *reactloops.ReActLoop, reportPath, title, markdown string) error {
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
	markdown = reportDisplayMarkdown(markdown)
	title = reportDisplayTitle(title)
	if _, err := emitter.EmitPinFilename(reportPath); err != nil {
		return err
	}
	if _, err := emitter.EmitJSON(schema.EVENT_TYPE_REPORT_FINISH, reportFinishEventNode, reportFinishEvent{
		ReportPath: reportPath, Title: title, SummaryMarkdown: markdown,
	}); err != nil {
		return err
	}
	// The generic report card's open button currently only works in code-audit
	// pages. Preserve the entire saved report in the existing reference viewer:
	// it loads on click as plain text, rather than inline Markdown in the chat.
	reactloops.EmitActionLog(loop, reportFinishEventNode, "完整报告已保存: "+reportPath, string(content))
	if invoker := loop.GetInvoker(); invoker != nil {
		invoker.AddToTimeline("report_finish", fmt.Sprintf(
			"Report finished: %s\nTitle: %s\nContent:\n%s",
			reportPath, title, markdown,
		))
	}
	return nil
}

func buildReportFinishTitle(content string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
		}
	}
	return ""
}
