package loop_report_generating

import (
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

// reportFinishEvent 供 Yakit 渲染报告文件卡片：路径 + 短 Markdown（正文以磁盘文件为准）。
type reportFinishEvent struct {
	ReportPath      string `json:"report_path"`
	Title           string `json:"title,omitempty"`
	SummaryMarkdown string `json:"summary_markdown,omitempty"`
}

func buildReportFinishHook() reactloops.ReActLoopOption {
	return reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, _ int, _ aicommon.AIStatefulTask, isDone bool, _ any, _ *reactloops.OnPostIterationOperator) {
		if !isDone {
			return
		}
		emitReportFinish(loop)
	})
}

// emitReportFinish 在 report_generating 子 loop 收尾时投递终稿标记（与 http_fuzz_request_change 同类 EmitJSON）。
// 模式同 loop_http_flow_analyze 的 buildPostIterationHook：仅 WithOnPostIteraction + isDone，不改 loopinfra。
func emitReportFinish(loop *reactloops.ReActLoop) {
	reportPath := strings.TrimSpace(loop.Get("report_filename"))
	if reportPath == "" {
		log.Infof("report_generating: skip report_finish (no report file)")
		return
	}

	content := strings.TrimSpace(loop.Get("full_report_code"))
	if content == "" {
		if raw, err := os.ReadFile(reportPath); err == nil {
			content = strings.TrimSpace(string(raw))
		}
	}
	title, summary := buildReportFinishPreview(content)
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(reportPath), filepath.Ext(reportPath))
	}

	emitter := loop.GetEmitter()
	if emitter == nil {
		return
	}

	_, err := emitter.EmitJSON(schema.EVENT_TYPE_REPORT_FINISH, reportFinishEventNode, reportFinishEvent{
		ReportPath:      reportPath,
		Title:           title,
		SummaryMarkdown: summary,
	})
	if err != nil {
		log.Warnf("report_generating: emit report_finish failed: %v", err)
		return
	}
	log.Infof("report_generating: emitted report_finish for %s", reportPath)
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
	summary = utils.ShrinkTextBlock(content, reportFinishSummaryMaxChars)
	return title, summary
}
