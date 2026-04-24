package loop_syntaxflow_scan

import (
	"fmt"

	sfutil "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// ApplyFinalReportContextWhenScanAlreadyDone runs when wireSession sees scan not executing (no background poll):
// end summary, large-page risk overview, merged preface, sf_scan_final_report_due=1 — same intent as poll终态.
func ApplyFinalReportContextWhenScanAlreadyDone(
	loop *reactloops.ReActLoop,
	r aicommon.AIInvokeRuntime,
	db *gorm.DB,
	task aicommon.AIStatefulTask,
	taskID string,
	res *ScanSessionResult,
) {
	if loop == nil || r == nil || db == nil || taskID == "" || res == nil || res.ScanTask == nil {
		return
	}
	st := res.ScanTask
	if st.Status == schema.SYNTAXFLOWSCAN_EXECUTING {
		return
	}
	endText := FormatSyntaxFlowScanEndReport(st)
	loop.Set(sfutil.LoopVarSFScanEndSummary, endText)
	AppendSFPipelineLine(loop, "【2·结束】"+endText)

	filterRT := &ypb.SSARisksFilter{RuntimeID: []string{taskID}}
	lim := int64(500)
	if c, e := yakit.QuerySSARiskCount(db, filterRT); e == nil && c > 0 && int64(c) < lim {
		lim = int64(c)
	}
	if lim < 1 {
		lim = 100
	}
	ApplySSARiskOverviewToInterpret(loop, r, db, task, taskID, filterRT, lim)
	AppendSFPipelineLine(loop, fmt.Sprintf("【4·全量风险列表】初载已结束任务：最多 %d 条", lim))
	AppendSfScanInterpretLog(loop, r, taskID, "init: 扫描已非 executing，已灌入终态总结与全表风险抽样")

	pipe := loop.Get(sfutil.LoopVarSFPipelineSummary)
	prev := loop.Get("sf_scan_review_preface")
	full := "【==== 大总结用数据：须纳入终局报告 ====】\n\n" +
		"【A·各阶段 pipeline】\n" + pipe + "\n\n" +
		"【B·扫描行终态】\n" + endText + "\n\n" +
		"【C·上文会话摘要 + risk 样例】\n" + prev
	loop.Set("sf_scan_review_preface", full)
	loop.Set(sfutil.LoopVarSFFinalReportDue, "1")
	// 无后台 poll 时，任务行已终态、一次性读入即可视为可成稿
	loop.Set(sfutil.LoopVarSFRiskConverged, "1")
}
