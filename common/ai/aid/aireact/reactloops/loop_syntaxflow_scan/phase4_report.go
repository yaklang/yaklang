package loop_syntaxflow_scan

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	sfu "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// WaitForSyntaxFlowReportGate 在 P4 前阻塞，直到 `sf_scan_risk_converged=1` 或 ctx 取消/或超时。超时后返回（P4 仍由上游调用，可另打 timeline 提示）。
func WaitForSyntaxFlowReportGate(ctx context.Context, loop *reactloops.ReActLoop) {
	if loop == nil {
		return
	}
	if strings.TrimSpace(loop.Get(sfu.LoopVarSFRiskConverged)) == "1" {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	deadline := time.Now().Add(45 * time.Minute)
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		if strings.TrimSpace(loop.Get(sfu.LoopVarSFRiskConverged)) == "1" {
			return
		}
		if time.Now().After(deadline) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

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
	loop.Set(sfu.LoopVarSFScanEndSummary, endText)
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

	pipe := loop.Get(sfu.LoopVarSFPipelineSummary)
	prev := loop.Get("sf_scan_review_preface")
	full := "【==== 大总结用数据：须纳入终局报告 ====】\n\n" +
		"【A·各阶段 pipeline】\n" + pipe + "\n\n" +
		"【B·扫描行终态】\n" + endText + "\n\n" +
		"【C·上文会话摘要 + risk 样例】\n" + prev
	loop.Set("sf_scan_review_preface", full)
	loop.Set(sfu.LoopVarSFFinalReportDue, "1")
	// 无后台 poll 时，任务行已终态、一次性读入即可视为可成稿
	loop.Set(sfu.LoopVarSFRiskConverged, "1")
}

// runPhaseReportGenerating 在解读子环结束后，物化数据并委托 report_generating 子环落盘终稿（与 code_security_audit/phase4 同构，软失败）。
func runPhaseReportGenerating(
	r aicommon.AIInvokeRuntime,
	interpretLoop *reactloops.ReActLoop,
	parentTask aicommon.AIStatefulTask,
) {
	if r == nil || interpretLoop == nil || parentTask == nil {
		return
	}
	baseDir := interpretLoop.GetLoopContentDir("syntaxflow_scan")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		log.Warnf("[syntaxflow_scan] report: mkdir: %v", err)
		r.AddToTimeline("syntaxflow_scan", "终局报告：创建工作目录失败: "+err.Error())
		return
	}
	inPath := filepath.Join(baseDir, "syntaxflow_scan_report_input.md")
	outPath := filepath.Join(baseDir, "syntaxflow_scan_report.md")

	inputBody := buildSyntaxflowReportInputMarkdown(interpretLoop, parentTask)
	if err := os.WriteFile(inPath, []byte(inputBody), 0o644); err != nil {
		log.Warnf("[syntaxflow_scan] report: write input: %v", err)
		r.AddToTimeline("syntaxflow_scan", "终局报告：写入 input 失败: "+err.Error())
		return
	}
	if err := os.WriteFile(outPath, []byte(""), 0o644); err != nil {
		log.Warnf("[syntaxflow_scan] report: touch output: %v", err)
		return
	}

	writePrompt := buildSyntaxflowReportWritePrompt(interpretLoop, inPath, outPath)
	reportLoop, err := reactloops.CreateLoopByName(
		schema.AI_REACT_LOOP_NAME_REPORT_GENERATING,
		r,
		reactloops.WithMaxIterations(math.MaxInt32),
		reactloops.WithAllowUserInteract(false),
		reactloops.WithInitTask(func(innerLoop *reactloops.ReActLoop, innerTask aicommon.AIStatefulTask, innerOp *reactloops.InitTaskOperator) {
			innerLoop.Set("report_filename", outPath)
			innerLoop.Set("full_report_code", "")
			innerLoop.Set("user_requirements", writePrompt)
			innerLoop.Set("collected_references", "")

			var filesHint strings.Builder
			filesHint.WriteString("### SyntaxFlow 扫描报告输入（**必须先读**）\n")
			filesHint.WriteString(fmt.Sprintf("- %s\n", inPath))
			innerLoop.Set("available_files", filesHint.String())
			innerLoop.Set("available_knowledge_bases", "")
			innerLoop.Set("is_modify_mode", "false")
			if em := innerLoop.GetEmitter(); em != nil {
				if _, err := em.EmitPinFilename(outPath); err != nil {
					log.Debugf("[syntaxflow_scan] report: pin: %v", err)
				}
			}
			innerOp.Continue()
		}),
	)
	if err != nil {
		log.Warnf("[syntaxflow_scan] report: CreateLoopByName: %v", err)
		r.AddToTimeline("syntaxflow_scan", "终局报告子环创建失败: "+err.Error())
		return
	}
	sub := aicommon.NewSubTaskBase(parentTask, "syntaxflow_scan_report", writePrompt, true)
	if err := reportLoop.ExecuteWithExistedTask(sub); err != nil {
		log.Warnf("[syntaxflow_scan] report: Execute: %v", err)
		r.AddToTimeline("syntaxflow_scan", "终局报告子环执行异常: "+err.Error())
	}
	content, err := os.ReadFile(outPath)
	if err == nil && len(content) > 0 {
		preview := utils.ShrinkTextBlock(string(content), 3000)
		r.AddToTimeline("syntaxflow_scan", fmt.Sprintf("终局报告已写入: %s（%d 字节）\n前略:\n%s", outPath, len(content), preview))
		parentID := strings.TrimSpace(interpretLoop.Get(loopVarOrchestratorParentTaskID))
		if parentID == "" {
			parentID = parentTask.GetId()
		}
		EmitSyntaxFlowStageMarkdown(interpretLoop, parentID, "p4_report_done", "终局·SyntaxFlow 扫描报告已生成", fmt.Sprintf(
			"**输出文件**: `%s`（%d 字节）\n\n**预览（截断）**:\n```markdown\n%s\n```",
			outPath, len(content), preview,
		))
	} else {
		r.AddToTimeline("syntaxflow_scan", "终局报告：输出文件未生成或为空: "+outPath)
	}
}

func buildSyntaxflowReportInputMarkdown(interpretLoop *reactloops.ReActLoop, parentTask aicommon.AIStatefulTask) string {
	var b strings.Builder
	b.WriteString("# SyntaxFlow 扫描报告：引擎输入\n\n")
	b.WriteString("以 **风险总览 + 扫描行终态 + 用户向阶段** 为主；`sf_scan_findings_doc` / `sf_scan_interpret_log` 仅附截断样例，完整以数据库与 `reload_ssa_risk_overview` 等工具为准。\n\n")
	b.WriteString("## 用户原始目标\n\n")
	b.WriteString(utils.ShrinkTextBlock(parentTask.GetUserInput(), 8000))
	b.WriteString("\n\n## 各阶段用户向 `sf_scan_user_stage_log`\n\n")
	b.WriteString(utils.ShrinkTextBlock(interpretLoop.Get(sfu.LoopVarSFUserStageLog), 12000))
	b.WriteString("\n\n## Risk 总览（`ssa_risk_list_summary` / `ssa_risk_total_hint`）\n\n")
	b.WriteString("- **total_hint**: ")
	b.WriteString(interpretLoop.Get("ssa_risk_total_hint"))
	b.WriteString("\n\n")
	b.WriteString(utils.ShrinkTextBlock(interpretLoop.Get("ssa_risk_list_summary"), 12000))
	b.WriteString("\n\n## 扫描行终态与 pipeline\n\n")
	b.WriteString("### 终局表 / 行摘要 `sf_scan_scan_end_summary`\n\n```\n")
	b.WriteString(utils.ShrinkTextBlock(interpretLoop.Get(sfu.LoopVarSFScanEndSummary), 6000))
	b.WriteString("\n```\n\n### pipeline 摘要 `sf_scan_pipeline_summary`\n\n```\n")
	b.WriteString(utils.ShrinkTextBlock(interpretLoop.Get(sfu.LoopVarSFPipelineSummary), 6000))
	b.WriteString("\n```\n\n## 引擎键值（摘要）\n\n")
	b.WriteString(fmt.Sprintf(
		"- task_id: %s\n- session_mode: %s\n- sf_scan_final_report_due: %s\n- sf_scan_risk_converged: %s\n",
		interpretLoop.Get(sfu.LoopVarSyntaxFlowTaskID),
		interpretLoop.Get(sfu.LoopVarSyntaxFlowScanSessionMode),
		interpretLoop.Get(sfu.LoopVarSFFinalReportDue),
		interpretLoop.Get(sfu.LoopVarSFRiskConverged),
	))
	b.WriteString("\n### 编译元信息 `sf_scan_compile_meta`（截断）\n\n```\n")
	b.WriteString(utils.ShrinkTextBlock(interpretLoop.Get(sfu.LoopVarSFCompileMeta), 2000))
	b.WriteString("\n```\n\n## 中间发现 / 解读（截断样例）\n\n")
	b.WriteString("### `sf_scan_findings_doc`\n\n```\n")
	b.WriteString(utils.ShrinkTextBlock(interpretLoop.Get("sf_scan_findings_doc"), 2000))
	b.WriteString("\n```\n\n### `sf_scan_interpret_log`\n\n```\n")
	b.WriteString(utils.ShrinkTextBlock(interpretLoop.Get(LoopVarInterpretLog), 3000))
	b.WriteString("\n```\n")
	return b.String()
}

func buildSyntaxflowReportWritePrompt(interpretLoop *reactloops.ReActLoop, inPath, outPath string) string {
	tid := interpretLoop.Get(sfu.LoopVarSyntaxFlowTaskID)
	return fmt.Sprintf(`你是安全报告撰写助手。请**仅**根据输入文件与引擎数据撰写一份**完整**的 SyntaxFlow 静态扫描结果报告（Markdown），保存到已指定的 report_filename（已由引擎创建）。

## 必须阅读的文件
- 输入: %s
- 输出: %s

## 任务与约束
- **task_id / runtime_id**: %s
- 报告 must 覆盖 pipeline 与扫描统计、风险总览、以及输入中的 risk/interpret 信息；**不得**编造未在输入中出现的 risk_id。
- 与对话中的 interpret 摘要可对账，**以本文件与 DB 一致的数据为准**。
- 使用 read_reference_file 等工具读取 %s 后再写入报告文件。

## 建议章节
1. 执行摘要（项目/范围/结论）
2. 扫描与编译阶段（来自 pipeline/compile 元信息）
3. 规则/Query/命中与任务行统计
4. 风险总览与分级（与 ssa_risk 列表一致时逐条或分组，勿遗漏已列条目标识）
5. 修复与验证建议
6. 局限与后续

直接写入最终 Markdown 到报告路径。`, inPath, outPath, tid, inPath)
}
