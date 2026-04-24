// 代码扫描阶段用户向 Markdown（# / ## 章节），对齐 finalize/HTTP 的材料组织方式；避免把内部 loop 名当作主标题。
// Per-rule 逐条成功/失败/跳过：DB 仅聚合，完整「逐规则点名」需引擎侧落库/序列化（方案 B）。v1 以任务行数字 + 风险 FromRule 去重名称为准。
package loop_syntaxflow_scan

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	sfutil "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

const (
	// SkipQueryProductHint 为对外说明中「skip」的共性原因（不暴露引擎内部名）。
	SkipQueryProductHint = "当某条规则与目标语言/工程形态不一致时，引擎会跳过该规则（不视为执行失败）。"

	scanConfigJSONMaxRunes = 8000
)

// AppendUserStageLog 在 interpret 子环上累加**用户向**阶段块，供 P4 物化与报告输入引用。
func AppendUserStageLog(loop *reactloops.ReActLoop, block string) {
	if loop == nil {
		return
	}
	block = strings.TrimSpace(block)
	if block == "" {
		return
	}
	prev := strings.TrimSpace(loop.Get(sfutil.LoopVarSFUserStageLog))
	if prev == "" {
		loop.Set(sfutil.LoopVarSFUserStageLog, block)
		return
	}
	loop.Set(sfutil.LoopVarSFUserStageLog, prev+"\n\n---\n\n"+block)
}

// OrchestratorParentTaskID 与编排父任务 Id 对齐（用于主对话去重键）；无则回退 fallback。
func OrchestratorParentTaskID(loop *reactloops.ReActLoop, fallback string) string {
	if loop == nil {
		return strings.TrimSpace(fallback)
	}
	if s := strings.TrimSpace(loop.Get("sf_orchestrator_parent_task_id")); s != "" {
		return s
	}
	return strings.TrimSpace(fallback)
}

// BuildScanStagePhase0Intake 阶段 0：输入与下一步（本地静态编译后接扫描）。
func BuildScanStagePhase0Intake(projectPath, configInferred, userHint, orchestratorTaskID string) string {
	var b strings.Builder
	b.WriteString("# 代码扫描·阶段 0\n\n")
	b.WriteString("## 你的目标与范围\n\n")
	if h := strings.TrimSpace(userHint); h != "" {
		b.WriteString(h)
		b.WriteString("\n\n")
	}
	b.WriteString("## 当前输入\n\n")
	if p := strings.TrimSpace(projectPath); p != "" {
		fmt.Fprintf(&b, "- **项目/路径**: `%s`\n", p)
	} else {
		b.WriteString("- **项目/路径**: （本回合由扫描配置 JSON 描述；未从编排单独填写 project_path 变量）\n")
	}
	if t := strings.TrimSpace(orchestratorTaskID); t != "" {
		fmt.Fprintf(&b, "- **编排任务 Id（便于对账）**: `%s`\n", t)
	}
	ci := strings.TrimSpace(configInferred)
	if ci == "1" {
		b.WriteString("- **配置来源**: 已根据本机目录推断为最小化 code-scan 配置\n")
	} else if ci == "0" {
		b.WriteString("- **配置来源**: 使用显式提供的 code-scan 配置\n")
	}
	b.WriteString("\n## 下一步\n\n")
	b.WriteString("将先在本地对目标工程做**静态程序分析用的编译（SSA）**，随后再启动**后台代码扫描**（规则与 Query 由配置与引擎决定）。\n\n")
	b.WriteString("> 若你选择**附着**已有 `task_id` 而非新扫，可跳过「本地起扫」路径；该分支的完整产品文案可后续接 intake。\n")
	return b.String()
}

// BuildScanStagePhase0Attach 附着已有 task_id 时的阶段 0 说明。
func BuildScanStagePhase0Attach(taskID, orchestratorTaskID string) string {
	return fmt.Sprintf(
		"# 代码扫描·阶段 0\n\n## 附着已有任务\n\n"+
			"将使用已有**扫描任务/会话** **task_id / runtime_id**: `%s`，**不再**在本地做 SSA 编译，也不会再调用 `StartScanInBackground` 起新扫。\n\n"+
			"- **编排侧任务 Id（对账用）**: `%s`\n",
		strings.TrimSpace(taskID), strings.TrimSpace(orchestratorTaskID),
	)
}

// BuildScanStagePhase1CompileStart 阶段 1 开始：配置摘要。
func BuildScanStagePhase1CompileStart(codeScanJSON string) string {
	var b strings.Builder
	b.WriteString("# 代码扫描·阶段 1\n\n")
	b.WriteString("## 开始静态编译\n\n")
	b.WriteString("已根据下列「扫描配置（code-scan 族 JSON）」在本地建立 SSA 程序。大型仓库可能**较慢**（数分钟级），期间会按需推送心跳。\n\n")
	b.WriteString("### 配置 JSON（已截断）\n\n```json\n")
	b.WriteString(utils.ShrinkTextBlock(strings.TrimSpace(codeScanJSON), scanConfigJSONMaxRunes))
	b.WriteString("\n```\n")
	return b.String()
}

// BuildScanStagePhase1CompileHeartbeat 编译超过约 3 分钟仍无结果时的心beat。
func BuildScanStagePhase1CompileHeartbeat(elapsed time.Duration, seq int) string {
	return fmt.Sprintf(
		"# 代码扫描·阶段 1\n\n## 仍在编译中（心跳 %d）\n\n已耗时约 **%s**（大仓库或全量分析可能更久；编译完成后会给出程序规模表）。\n",
		seq, durationHuman(elapsed),
	)
}

// BuildScanStagePhase1CompileFailed 编译/加载 program 失败。
func BuildScanStagePhase1CompileFailed(errText string) string {
	return "# 代码扫描·阶段 1\n\n## 静态编译未成功\n\n```\n" +
		utils.ShrinkTextBlock(errText, 2000) + "\n```\n"
}

// BuildScanStagePhase1CompileDone 编译成功结束表：每 program 行数/文件数/耗时。
func BuildScanStagePhase1CompileDone(progs []*ssaapi.Program, durationMs int64) string {
	var b strings.Builder
	b.WriteString("# 代码扫描·阶段 1\n\n## 静态编译完成\n\n")
	if durationMs > 0 {
		fmt.Fprintf(&b, "总耗时: **%d ms**。\n\n", durationMs)
	}
	b.WriteString("### 程序与规模\n\n")
	b.WriteString("| 项目名/Program | 代码行数（约） | 源文件数（约） |\n| --- | ---: | ---: |\n")
	if len(progs) == 0 {
		b.WriteString("| （无 program） | 0 | 0 |\n")
	} else {
		for _, p := range progs {
			name := "（未命名）"
			lines := 0
			files := 0
			if p != nil {
				if n := p.GetProgramName(); n != "" {
					name = n
				}
				lines = p.TotalLines()
				if o := p.GetOverlay(); o != nil {
					files = o.GetFileCount()
				}
			}
			fmt.Fprintf(&b, "| %s | %d | %d |\n", escapeMDCell(name), lines, files)
		}
	}
	b.WriteString("\n*文件数来自 overlay 汇总；无 overlay 时可能为 0。*\n")
	return b.String()
}

// BuildScanStagePhase2ScanStart 阶段 2：后台任务已起。
func BuildScanStagePhase2ScanStart(taskID string) string {
	return fmt.Sprintf(
		"# 代码扫描·阶段 2\n\n## 扫描已启动\n\n"+
			"后台扫描任务已创建：\n\n"+
			"- **任务/会话 Id（`task_id` / runtime_id）**: `%s`\n\n"+
			"随后将轮询任务进度与**风险入库**情况，并在有增量或定周期时向本对话推送摘要；扫描结束后仍可能在短时间内继续收敛风险行数，直到计数稳定再进入成稿阶段。\n",
		strings.TrimSpace(taskID),
	)
}

// BuildScanStagePhase2Progress 扫描执行中或终态后等待收敛时的一帧用户向摘要。
func BuildScanStagePhase2Progress(
	st *schema.SyntaxFlowScanTask,
	res *ScanSessionResult,
	frameIdx int,
	riskStabilizing bool,
) string {
	scanRunEnded := st != nil && st.Status != schema.SYNTAXFLOWSCAN_EXECUTING
	var b strings.Builder
	b.WriteString("# 代码扫描·阶段 2\n\n## 任务与进度快照\n\n")
	if st != nil {
		fmt.Fprintf(&b, "- **task_id**: `%s`\n", st.TaskId)
		fmt.Fprintf(&b, "- **扫描侧状态（是否仍在执行）**: %s\n", scanRunningHuman(st.Status))
		fmt.Fprintf(&b, "- **任务行 risk_count（聚合）**: %d\n", st.RiskCount)
		b.WriteString("\n### Query 进度\n\n")
		b.WriteString("| 指标 | 数值 |\n| --- | ---: |\n")
		fmt.Fprintf(&b, "| 已登记 Query 数 | %d |\n", st.TotalQuery)
		fmt.Fprintf(&b, "| 成功/命中 | %d |\n", st.SuccessQuery)
		fmt.Fprintf(&b, "| 失败 | %d |\n", st.FailedQuery)
		fmt.Fprintf(&b, "| 跳过 | %d |\n", st.SkipQuery)
		b.WriteString("\n**关于「跳过」数字**: " + SkipQueryProductHint + "\n\n")
	} else {
		b.WriteString("（暂无法读任务行；仅展示抽样风险表）\n\n")
	}
	if riskStabilizing && scanRunEnded {
		b.WriteString("### 风险侧收敛中\n\n扫描任务行已**非 executing**，但数据库中的**风险行数**仍可能短时间波动。正在**连续 2 次轮询**读数一致后，再认为风险已收敛、可成稿。\n\n")
	}
	rulesLine, table := buildRiskSampleTableAndRules(res, 24)
	if rulesLine != "" {
		b.WriteString("### 本批可见规则名（FromRule 去重，抽样）\n\n")
		b.WriteString(rulesLine)
		b.WriteString("\n\n")
	}
	b.WriteString("### 风险表（本批展示）\n\n")
	b.WriteString(table)
	fmt.Fprintf(&b, "\n*本帧编号: %d。风险分级：Critical / High 将优先在解读中强调。*\n", frameIdx)
	return b.String()
}

// BuildScanStagePhase2ScanFinishedTable 阶段 2 结束：与扫描终态行一致的表格（Markdown）。
func BuildScanStagePhase2ScanFinishedTable(st *schema.SyntaxFlowScanTask) string {
	var b strings.Builder
	b.WriteString("# 代码扫描·阶段 2\n\n")
	b.WriteString("## 扫描结束（任务行终态）\n\n")
	b.WriteString(FormatSyntaxFlowScanEndReportMarkdownTable(st))
	b.WriteString("\n")
	b.WriteString(perRuleV1Footnote())
	return b.String()
}

func scanRunningHuman(status string) string {
	if status == schema.SYNTAXFLOWSCAN_EXECUTING {
		return "是（`executing`）"
	}
	if status == "" {
		return "未知"
	}
	return "否，当前状态为 `" + status + "`"
}

func durationHuman(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	s := int(d.Round(time.Second).Seconds())
	if s < 120 {
		return fmt.Sprintf("%d 秒", s)
	}
	return fmt.Sprintf("%d 分 %d 秒", s/60, s%60)
}

func escapeMDCell(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "|", "¦")
}

func perRuleV1Footnote() string {
	return "\n> **逐规则**成功/失败/跳过**名单**：当前版本仅见任务行**汇总数字**与风险行中的 **FromRule** 抽样。若产品需要「每条规则一行」的完整审计，需要引擎/存储扩展（计划中的方案 B），不在本轮交付范围。\n"
}

// DistinctFromRulesFromRisks 从 risk 行抽取非空、去重后的规则名（用于 v1 展示）。
func DistinctFromRulesFromRisks(risks []*schema.SSARisk) []string {
	if len(risks) == 0 {
		return nil
	}
	set := make(map[string]struct{})
	for _, r := range risks {
		if r == nil {
			continue
		}
		if n := strings.TrimSpace(r.FromRule); n != "" {
			set[n] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for n := range set {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// buildRiskSampleTableAndRules 返回 (规则 bullet 行, markdown 表)。
func buildRiskSampleTableAndRules(res *ScanSessionResult, maxRows int) (rulesLine, table string) {
	if res == nil {
		return "", "（当前未取到本批风险样本。若总风险为 0，或尚未落库/查询失败，会显示为「暂无已入库风险」。）\n"
	}
	if res.TotalRisks == 0 && len(res.Risks) == 0 {
		return "", "**当前未产出/未查询到入 SSA 风险行（或本 runtime 为 0）。**\n"
	}
	rules := DistinctFromRulesFromRisks(res.Risks)
	if len(rules) > 0 {
		var rb strings.Builder
		for _, name := range rules {
			fmt.Fprintf(&rb, "- `%s`\n", escapeMDCell(name))
		}
		rulesLine = strings.TrimSpace(rb.String())
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("> 与 DB 同口径的 runtime 下风险**约** %d 条；下表为抽样最多 %d 行。\n\n", res.TotalRisks, maxRows))
	b.WriteString("| 严重级别 | 规则（FromRule，抽样） | 标题（截断） |\n| --- | --- | --- |\n")
	rows := res.Risks
	if maxRows > 0 && len(rows) > maxRows {
		rows = rows[:maxRows]
	}
	if len(rows) == 0 {
		b.WriteString("| — | — | 暂无行样本 |\n")
		return rulesLine, b.String()
	}
	for _, rk := range rows {
		if rk == nil {
			continue
		}
		tit := utils.ShrinkTextBlock(rk.Title, 120)
		fr := utils.ShrinkTextBlock(rk.FromRule, 80)
		fmt.Fprintf(&b, "| %s | %s | %s |\n", escapeMDCell(string(rk.Severity)), escapeMDCell(fr), escapeMDCell(tit))
	}
	return rulesLine, b.String()
}

// WaitForSyntaxFlowReportGate 在 P4 前阻塞，直到 `sf_scan_risk_converged=1` 或 ctx 取消/或超时。超时后返回（P4 仍由上游调用，可另打 timeline 提示）。
func WaitForSyntaxFlowReportGate(ctx context.Context, loop *reactloops.ReActLoop) {
	if loop == nil {
		return
	}
	if strings.TrimSpace(loop.Get(sfutil.LoopVarSFRiskConverged)) == "1" {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	deadline := time.Now().Add(45 * time.Minute)
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		if strings.TrimSpace(loop.Get(sfutil.LoopVarSFRiskConverged)) == "1" {
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
