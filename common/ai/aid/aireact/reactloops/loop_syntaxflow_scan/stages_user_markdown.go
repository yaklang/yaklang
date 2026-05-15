// 代码扫描阶段用户向 Markdown（# / ## 章节），对齐 finalize/HTTP 的材料组织方式；避免把内部 loop 名当作主标题。
// Per-rule 逐条成功/失败/跳过：DB 仅聚合，完整「逐规则点名」需引擎侧落库/序列化（方案 B）。v1 以任务行数字 + 风险 FromRule 去重名称为准。
package loop_syntaxflow_scan

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	sfutil "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

const scanConfigJSONMaxRunes = 8000

// AppendUserStageLog 在编排环上累加**用户向**阶段块，供 P4 物化与报告输入引用。
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
// parentTaskID 优先用编排任务 Id；空则回退 GetCurrentTask()。
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

// EmitSyntaxFlowUserStageMarkdown 用户向、章节化长文档：写入 `sfu.LoopVarSFUserStageLog` 并推主对话；`phaseKey` 须唯一以允许心跳等重复型帧去重不冲突（如 p1_hb_3）。
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

// OrchestratorParentTaskID 返回用于阶段 Markdown 去重的任务 Id；传入 task.GetId() 作为 fallback。
func OrchestratorParentTaskID(_ *reactloops.ReActLoop, fallback string) string {
	return strings.TrimSpace(fallback)
}

// BuildScanStagePhase0Intake 阶段 0：输入与下一步。
func BuildScanStagePhase0Intake(mode SyntaxFlowScanSessionMode, projectPath, projectName, programName, userHint, orchestratorTaskID string) string {
	var b strings.Builder
	b.WriteString("# 代码扫描·阶段 0\n\n")
	b.WriteString("## 你的目标与范围\n\n")
	if h := strings.TrimSpace(userHint); h != "" {
		b.WriteString(h)
		b.WriteString("\n\n")
	}
	b.WriteString("## 当前输入\n\n")
	fmt.Fprintf(&b, "- **会话模式**: `%s`\n", mode.String())
	switch mode {
	case SyntaxFlowSessionModeCompileScan:
		if p := strings.TrimSpace(projectPath); p != "" {
			fmt.Fprintf(&b, "- **项目/路径**: `%s`\n", p)
		}
		if pn := strings.TrimSpace(projectName); pn != "" {
			fmt.Fprintf(&b, "- **项目名称（project_name）**: `%s`\n", pn)
		}
		b.WriteString("- **配置来源**: 将按本地路径匹配已有 SSA 项目，或自动探测并创建项目配置\n")
		b.WriteString("\n## 下一步\n\n")
		b.WriteString("将先在本地对目标工程做**静态程序分析用的编译（SSA）**，随后再启动**后台代码扫描**。\n")
	case SyntaxFlowSessionModeProgramScan:
		fmt.Fprintf(&b, "- **Program 名称**: `%s`\n", strings.TrimSpace(programName))
		b.WriteString("\n## 下一步\n\n")
		b.WriteString("将跳过 SSA 编译，直接对已编译 Program 启动**后台代码扫描**。\n")
	default:
		b.WriteString("\n## 下一步\n\n")
		b.WriteString("将按已提交的会话模式继续编排。\n")
	}
	if t := strings.TrimSpace(orchestratorTaskID); t != "" {
		fmt.Fprintf(&b, "\n- **编排任务 Id（便于对账）**: `%s`\n", t)
	}
	return b.String()
}

// BuildScanStagePhase0ProgramScan program 模式：跳过编译，直接扫描。
func BuildScanStagePhase0ProgramScan(programName, userHint, orchestratorTaskID string) string {
	return BuildScanStagePhase0Intake(SyntaxFlowSessionModeProgramScan, "", "", programName, userHint, orchestratorTaskID)
}

// BuildScanStagePhase0ConfigResolve 阶段 0：配置解析结果（显式 JSON / 复用项目 / 自动探测新建）。
func BuildScanStagePhase0ConfigResolve(out *sfutil.CodeScanConfigResolveOutcome) string {
	var b strings.Builder
	b.WriteString("# 代码扫描·阶段 0\n\n")
	b.WriteString("## 配置解析结果\n\n")
	if out == nil || out.Config == nil {
		b.WriteString("（未能解析出有效配置。）\n")
		return b.String()
	}
	switch out.Source {
	case sfutil.CodeScanConfigSourceExistingProject:
		b.WriteString("- **解析方式**: 已匹配到已有 SSA 项目，直接复用其配置\n")
	case sfutil.CodeScanConfigSourceAutoDetectNew:
		b.WriteString("- **解析方式**: 未找到匹配项目，已自动探测工程并创建/同步 SSA 项目配置\n")
	default:
		b.WriteString("- **解析方式**: 未知\n")
	}
	if p := strings.TrimSpace(out.ResolvedPath); p != "" {
		fmt.Fprintf(&b, "- **归一化路径**: `%s`\n", p)
	}
	if pn := strings.TrimSpace(out.ProjectName); pn != "" {
		fmt.Fprintf(&b, "- **项目名称**: `%s`\n", pn)
	}
	if out.ProjectID > 0 {
		fmt.Fprintf(&b, "- **项目 Id**: `%d`\n", out.ProjectID)
	}
	b.WriteString("\n## 下一步\n\n")
	b.WriteString("已得到可用于编译的 code-scan 配置，即将进入 SSA 编译阶段。\n")
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
	res *sfutil.ScanSessionResult,
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
func buildRiskSampleTableAndRules(res *sfutil.ScanSessionResult, maxRows int) (rulesLine, table string) {
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
