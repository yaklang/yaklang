package loop_report_generating

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

const reportFinishEventNode = "report-finish"

type reportFinishEvent struct {
	ReportPath string `json:"report_path"`
}

func buildReportFinishHook() reactloops.ReActLoopOption {
	return reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, _ int, _ aicommon.AIStatefulTask, isDone bool, _ any, _ *reactloops.OnPostIterationOperator) {
		if !isDone {
			return
		}
		emitReportFinish(loop)
	})
}

// emitReportFinish 在 report_generating 子 loop 收尾时投递终稿标记（与 http_fuzz_request_change 同类）。
// 不重复流式 markdown，不表示会话结束；前端按 Type=report_finish + NodeId=report-finish 识别终稿 UI。
func emitReportFinish(loop *reactloops.ReActLoop) {
	reportPath := strings.TrimSpace(loop.Get("report_filename"))
	if reportPath == "" {
		log.Infof("report_generating: skip report_finish (no report file)")
		return
	}

	emitter := loop.GetEmitter()
	if emitter == nil {
		return
	}

	_, err := emitter.EmitJSON(schema.EVENT_TYPE_REPORT_FINISH, reportFinishEventNode, reportFinishEvent{
		ReportPath: reportPath,
	})
	if err != nil {
		log.Warnf("report_generating: emit report_finish failed: %v", err)
		return
	}
	log.Infof("report_generating: emitted report_finish for %s", reportPath)
}
