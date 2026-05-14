package loop_syntaxflow_scan

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	sfu "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// runPhaseReportGenerating runs after risk convergence: materialises data and delegates to report_generating sub-loop.
func runPhaseReportGenerating(
	r aicommon.AIInvokeRuntime,
	scanLoop *reactloops.ReActLoop,
	parentTask aicommon.AIStatefulTask,
) {
	if r == nil || scanLoop == nil || parentTask == nil {
		return
	}
	baseDir := scanLoop.GetLoopContentDir("syntaxflow_scan")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		log.Warnf("[syntaxflow_scan] report: mkdir: %v", err)
		r.AddToTimeline("syntaxflow_scan", "终局报告：创建工作目录失败: "+err.Error())
		return
	}
	inPath := filepath.Join(baseDir, "syntaxflow_scan_report_input.md")
	outPath := filepath.Join(baseDir, "syntaxflow_scan_report.md")

	// Collect per-batch markdown files.
	batchesDir := filepath.Join(baseDir, "batches")
	batchFiles := collectBatchFiles(batchesDir)

	inputBody := buildSyntaxflowReportInputMarkdown(scanLoop, parentTask, batchFiles)
	if err := os.WriteFile(inPath, []byte(inputBody), 0o644); err != nil {
		log.Warnf("[syntaxflow_scan] report: write input: %v", err)
		r.AddToTimeline("syntaxflow_scan", "终局报告：写入 input 失败: "+err.Error())
		return
	}
	if err := os.WriteFile(outPath, []byte(""), 0o644); err != nil {
		log.Warnf("[syntaxflow_scan] report: touch output: %v", err)
		return
	}

	writePrompt := buildSyntaxflowReportWritePrompt(scanLoop, inPath, outPath, batchFiles)
	reportLoop, err := reactloops.CreateLoopByName(
		schema.AI_REACT_LOOP_NAME_REPORT_GENERATING,
		r,
		reactloops.WithMaxIterations(math.MaxInt32),
		reactloops.WithAllowRAG(false),
		reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false),
		reactloops.WithAllowUserInteract(false),
		reactloops.WithInitTask(func(innerLoop *reactloops.ReActLoop, innerTask aicommon.AIStatefulTask, innerOp *reactloops.InitTaskOperator) {
			innerLoop.Set("report_filename", outPath)
			innerLoop.Set("full_report_code", "")
			innerLoop.Set("user_requirements", writePrompt)
			innerLoop.Set("collected_references", "")

			var filesHint strings.Builder
			filesHint.WriteString("### SyntaxFlow 扫描报告输入（**必须先读**）\n")
			filesHint.WriteString(fmt.Sprintf("- %s\n", inPath))
			if len(batchFiles) > 0 {
				filesHint.WriteString("\n### SSA Risk 逐批分析报告（顺序读取）\n")
				for _, f := range batchFiles {
					filesHint.WriteString(fmt.Sprintf("- %s\n", f))
				}
			}

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
	if err != nil || len(strings.TrimSpace(string(content))) == 0 {
		log.Warnf("[syntaxflow_scan] report empty or unreadable, writing fallback")
		fallback := generateSyntaxflowScanFallbackReport(inPath, batchFiles)
		if writeErr := os.WriteFile(outPath, []byte(fallback), 0o644); writeErr != nil {
			log.Warnf("[syntaxflow_scan] report: write fallback: %v", writeErr)
			r.AddToTimeline("syntaxflow_scan", "终局报告：输出文件未生成或为空: "+outPath)
			return
		}
		content = []byte(fallback)
		r.AddToTimeline("syntaxflow_scan", "终局报告：AI 子环未写出完整内容，已自动汇编输入与批次文件: "+outPath)
	}
	if len(content) > 0 {
		preview := utils.ShrinkTextBlock(string(content), 3000)
		r.AddToTimeline("syntaxflow_scan", fmt.Sprintf("终局报告已写入: %s（%d 字节）\n前略:\n%s", outPath, len(content), preview))
		parentID := parentTask.GetId()
		EmitSyntaxFlowStageMarkdown(scanLoop, parentID, "p4_report_done", "终局·SyntaxFlow 扫描报告已生成", fmt.Sprintf(
			"**输出文件**: `%s`（%d 字节）\n\n**预览（截断）**:\n```markdown\n%s\n```",
			outPath, len(content), preview,
		))
	}
}

func generateSyntaxflowScanFallbackReport(inPath string, batchFiles []string) string {
	var b strings.Builder
	b.WriteString("# SyntaxFlow 扫描报告（自动生成）\n\n")
	b.WriteString("> `report_generating` 子环未产出完整 AI 报告，以下为基于已落盘输入与批次文件的自动汇编稿。\n\n")
	if data, err := os.ReadFile(inPath); err == nil && len(data) > 0 {
		b.WriteString("## 扫描输入概要\n\n")
		b.Write(data)
		b.WriteString("\n\n")
	}
	if len(batchFiles) == 0 {
		b.WriteString("（无 SSA Risk 批次文件。）\n")
		return b.String()
	}
	b.WriteString("## 逐批 Risk Overview\n\n")
	for _, f := range batchFiles {
		b.WriteString(fmt.Sprintf("### %s\n\n", filepath.Base(f)))
		if data, err := os.ReadFile(f); err == nil {
			b.Write(data)
		} else {
			fmt.Fprintf(&b, "（读取失败: %v）\n", err)
		}
		b.WriteString("\n\n")
	}
	return b.String()
}

// collectBatchFiles returns sorted batch_*.md paths from batchesDir, or nil if none.
func collectBatchFiles(batchesDir string) []string {
	entries, err := os.ReadDir(batchesDir)
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, "batch_") && strings.HasSuffix(name, ".md") {
			files = append(files, filepath.Join(batchesDir, name))
		}
	}
	sort.Strings(files)
	return files
}

func buildSyntaxflowReportInputMarkdown(scanLoop *reactloops.ReActLoop, parentTask aicommon.AIStatefulTask, batchFiles []string) string {
	var b strings.Builder
	b.WriteString("# SyntaxFlow 扫描报告：引擎输入\n\n")
	b.WriteString("## 用户原始目标\n\n")
	b.WriteString(utils.ShrinkTextBlock(parentTask.GetUserInput(), 8000))
	b.WriteString("\n\n## 各阶段用户向 `sf_scan_user_stage_log`\n\n")
	b.WriteString(utils.ShrinkTextBlock(scanLoop.Get(sfu.LoopVarSFUserStageLog), 12000))
	b.WriteString("\n\n## 扫描行终态与 pipeline\n\n")
	b.WriteString("### 终局表 / 行摘要 `sf_scan_scan_end_summary`\n\n```\n")
	b.WriteString(utils.ShrinkTextBlock(scanLoop.Get(sfu.LoopVarSFScanEndSummary), 6000))
	b.WriteString("\n```\n\n### pipeline 摘要 `sf_scan_pipeline_summary`\n\n```\n")
	b.WriteString(utils.ShrinkTextBlock(scanLoop.Get(sfu.LoopVarSFPipelineSummary), 6000))
	b.WriteString("\n```\n\n## 引擎键值（摘要）\n\n")
	b.WriteString(fmt.Sprintf(
		"- task_id: %s\n- session_mode: %s\n- sf_scan_risk_converged: %s\n- 批次文件数: %d\n",
		scanLoop.Get(sfu.LoopVarSyntaxFlowTaskID),
		scanLoop.Get(sfu.LoopVarSyntaxFlowScanSessionMode),
		scanLoop.Get(sfu.LoopVarSFRiskConverged),
		len(batchFiles),
	))
	b.WriteString("\n### 编译元信息 `sf_scan_compile_meta`（截断）\n\n```\n")
	b.WriteString(utils.ShrinkTextBlock(scanLoop.Get(sfu.LoopVarSFCompileMeta), 2000))
	b.WriteString("\n```\n\n## 中间发现 / 解读日志（截断）\n\n### `sf_scan_interpret_log`\n\n```\n")
	b.WriteString(utils.ShrinkTextBlock(scanLoop.Get(LoopVarInterpretLog), 3000))
	b.WriteString("\n```\n\n## 批次文件目录\n\n")
	if len(batchFiles) == 0 {
		b.WriteString("（无批次文件——扫描未产生 SSA Risk 或批次处理未完成）\n")
	} else {
		b.WriteString(fmt.Sprintf("共 %d 个批次文件，位于 `batches/` 子目录，报告生成时须顺序读取：\n\n", len(batchFiles)))
		for i, f := range batchFiles {
			b.WriteString(fmt.Sprintf("%d. `%s`\n", i+1, f))
		}
	}
	return b.String()
}

func buildSyntaxflowReportWritePrompt(scanLoop *reactloops.ReActLoop, inPath, outPath string, batchFiles []string) string {
	tid := scanLoop.Get(sfu.LoopVarSyntaxFlowTaskID)
	batchSection := "\n（本次扫描无 SSA Risk 批次文件，报告仅含 pipeline 统计与扫描终态信息）\n"
	if len(batchFiles) > 0 {
		batchSection = fmt.Sprintf(
			"\n批次分析文件位于 `%s`（共 %d 个 batch_*.md）。请通过 read_reference_file 按文件名顺序读取；完整列表见输入概要或 available_files。\n",
			filepath.Dir(batchFiles[0]),
			len(batchFiles),
		)
	}
	return fmt.Sprintf(`你是安全报告撰写助手。请**仅**根据输入文件与批次文件撰写一份**完整**的 SyntaxFlow 静态扫描结果报告（Markdown），保存到已指定的 report_filename。

## 必须阅读的文件
- 输入概要: %s
- 输出: %s
%s
## 任务与约束
- **task_id / runtime_id**: %s
- 报告 **必须** 覆盖 pipeline 摘要、扫描统计、以及所有批次文件中的 risk 信息；**不得** 编造未在批次文件中出现的 risk_id。
- **步骤**：1) 读取输入概要文件；2) 按顺序用 read_reference_file 逐批次读取每个 batch_*.md；3) 将所有信息合并写入报告文件。
- 以批次文件中的数据为准，勿省略任何批次中出现的 risk 条目。
- 每一步必须输出合法 JSON 行并带 @action（例如 read_reference_file、write_section）；禁止只输出说明文字。

## 建议章节
1. 执行摘要（项目/范围/结论）
2. 扫描与编译阶段（来自 pipeline/compile 元信息）
3. 规则/Query/命中与任务行统计
4. 风险总览与分级（按批次整合，逐条或分组，勿遗漏已列条目）
5. 修复与验证建议
6. 局限与后续

直接写入最终 Markdown 到报告路径。`, inPath, outPath, batchSection, tid)
}
