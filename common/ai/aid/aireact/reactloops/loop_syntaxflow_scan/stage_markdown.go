package loop_syntaxflow_scan

import (
	"fmt"
	"hash/fnv"
	"strings"
	"sync"

	sfutil "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	// NodeIDSyntaxFlowStageReport 主对话区流式 Markdown（与 EVENT_TYPE_STRUCTURED 的 progress 节点区分）
	NodeIDSyntaxFlowStageReport = "syntaxflow_scan_stage_report"
)

var stageMarkdownOnce sync.Map

func dedupKey(taskID, phaseKey string) string {
	if taskID == "" {
		taskID = "_"
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(taskID + "|" + phaseKey))
	return fmt.Sprintf("%x", h.Sum32())
}

// EmitSyntaxFlowStageMarkdown 向主对话区推送**引擎组装**的阶段性 Markdown（与 EmitSyntaxFlowScanPhase 的 JSON 进度互补）。
// parentTaskID 优先用编排父任务的 Id（P2 时尚未绑定 interpret 的 currentTask）；空则回退 GetCurrentTask()。
// phaseKey 用于去重（同 parentTask+phase 只发一次）。title/body 会截断。
func EmitSyntaxFlowStageMarkdown(loop *reactloops.ReActLoop, parentTaskID, phaseKey, title, body string) {
	if loop == nil {
		return
	}
	taskID := strings.TrimSpace(parentTaskID)
	if taskID == "" {
		if task := loop.GetCurrentTask(); task != nil {
			taskID = task.GetId()
		}
	}
	if _, loaded := stageMarkdownOnce.LoadOrStore(dedupKey(taskID, phaseKey), true); loaded {
		return
	}
	em := loop.GetEmitter()
	if em == nil {
		return
	}
	t := strings.TrimSpace(title)
	if t == "" {
		t = "SyntaxFlow 阶段"
	}
	b := strings.TrimSpace(body)
	if b == "" {
		b = "（无正文）"
	}
	doc := fmt.Sprintf("## %s\n\n%s\n", t, utils.ShrinkTextBlock(b, 12000))
	if _, err := em.EmitTextMarkdownStreamEvent(
		NodeIDSyntaxFlowStageReport,
		strings.NewReader(doc),
		taskID,
		func() {},
	); err != nil {
		log.Debugf("syntaxflow stage markdown: %v", err)
	}
}

// EmitSyntaxFlowUserStageMarkdown 用户向、章节化长文档：写入 `sfutil.LoopVarSFUserStageLog` 并推主对话；`phaseKey` 须唯一以允许心跳等重复型帧去重不冲突（如 p1_hb_3）。
// fullDocument 可含以 `#` 开头的顶级标题，不再包一层 `##`。
func EmitSyntaxFlowUserStageMarkdown(loop *reactloops.ReActLoop, parentTaskID, phaseKey, fullDocument string) {
	if loop == nil {
		return
	}
	taskID := strings.TrimSpace(parentTaskID)
	if taskID == "" {
		if task := loop.GetCurrentTask(); task != nil {
			taskID = task.GetId()
		}
	}
	if _, loaded := stageMarkdownOnce.LoadOrStore(dedupKey(taskID, phaseKey), true); loaded {
		return
	}
	doc := strings.TrimSpace(fullDocument)
	if doc == "" {
		return
	}
	AppendUserStageLog(loop, doc)
	if len(doc) > 16000 {
		doc = utils.ShrinkTextBlock(doc, 16000) + "\n"
	} else {
		doc += "\n"
	}
	em := loop.GetEmitter()
	if em == nil {
		return
	}
	if _, err := em.EmitTextMarkdownStreamEvent(
		NodeIDSyntaxFlowStageReport,
		strings.NewReader(doc),
		taskID,
		func() {},
	); err != nil {
		log.Debugf("syntaxflow user stage markdown: %v", err)
	}
}

// EngineSnapshotBodyForInterpret 进入解读/报告物化时使用的**确定性**纯文本块（不依赖模型）。
func EngineSnapshotBodyForInterpret(loop *reactloops.ReActLoop) string {
	if loop == nil {
		return ""
	}
	return fmt.Sprintf(
		"- **task_id / runtime_id**: %s\n"+
			"- **session_mode**（attach/解读说明）: %s\n"+
			"- **config 推断**（sf_scan_config_inferred 1=路径推断）: %s\n"+
			"- **sf_scan_final_report_due**（1=终局大报告）: %s\n"+
			"- **sf_scan_risk_converged**（1=风险侧可成稿）: %s\n\n"+
			"### 各阶段用户向累计 `sf_scan_user_stage_log`（截断）\n```\n%s\n```\n\n"+
			"### 编译/管线 `sf_scan_compile_meta`\n```\n%s\n```\n\n"+
			"### pipeline 摘要 `sf_scan_pipeline_summary`\n```\n%s\n```\n\n"+
			"### 扫描行终态 `sf_scan_scan_end_summary`（若已有）\n```\n%s\n```\n\n"+
			"### preface 头（截断）\n```\n%s\n```\n\n"+
			"### risk 列表头（若已有）\n- total_hint: %s\n```\n%s\n```\n",
		loop.Get(sfutil.LoopVarSyntaxFlowTaskID),
		loop.Get(sfutil.LoopVarSyntaxFlowScanSessionMode),
		loop.Get("sf_scan_config_inferred"),
		loop.Get(sfutil.LoopVarSFFinalReportDue),
		loop.Get(sfutil.LoopVarSFRiskConverged),
		utils.ShrinkTextBlock(loop.Get(sfutil.LoopVarSFUserStageLog), 8000),
		utils.ShrinkTextBlock(loop.Get(sfutil.LoopVarSFCompileMeta), 2000),
		utils.ShrinkTextBlock(loop.Get(sfutil.LoopVarSFPipelineSummary), 8000),
		utils.ShrinkTextBlock(loop.Get(sfutil.LoopVarSFScanEndSummary), 4000),
		utils.ShrinkTextBlock(loop.Get("sf_scan_review_preface"), 8000),
		loop.Get("ssa_risk_total_hint"),
		utils.ShrinkTextBlock(loop.Get("ssa_risk_list_summary"), 8000),
	)
}
