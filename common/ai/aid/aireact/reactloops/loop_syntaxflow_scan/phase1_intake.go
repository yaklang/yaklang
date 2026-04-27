package loop_syntaxflow_scan

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	sfu "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// syntaxFlowIntakeResult is the structured result from LiteForge for natural-language entry.
type syntaxFlowIntakeResult struct {
	TaskID      string
	ProjectPath string
	Reason      string
}

// extractSyntaxFlowScanIntake uses LiteForge to read task_id and/or an absolute project path from the user request.
func extractSyntaxFlowScanIntake(ctx context.Context, r aicommon.AIInvokeRuntime, userInput string) (*syntaxFlowIntakeResult, error) {
	promptTpl := `From the user request, extract a SyntaxFlow scan task_id (if any) and/or a local project directory to scan.

## User request
<|USER_INPUT_{{ .Nonce }}|>
{{ .UserInput }}
<|USER_INPUT_END_{{ .Nonce }}|>

## Rules
1. task_id: UUID or runtime id for an **existing** SyntaxFlow scan in the SSA project DB, if the user provided one.
2. project_path: a single local **absolute** directory (or file path whose parent is the project root) to start a new scan, if the user specified a path.
3. If neither is present, return empty strings.
4. Prefer task_id if both a new path and a task id to attach are clearly given, unless the user explicitly wants a new scan on disk.

Return structured fields.`

	rendered, err := utils.RenderTemplate(promptTpl, map[string]any{
		"Nonce":     utils.RandStringBytes(4),
		"UserInput": userInput,
	})
	if err != nil {
		return nil, utils.Wrap(err, "render syntaxflow intake prompt")
	}

	result, err := r.InvokeSpeedPriorityLiteForge(
		ctx,
		"extract-syntaxflow-scan-intake",
		rendered,
		[]aitool.ToolOption{
			aitool.WithStringParam("task_id",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("Existing SyntaxFlow scan task id, or empty")),
			aitool.WithStringParam("project_path",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("Absolute path to a local project to scan, or empty")),
			aitool.WithStringParam("reason",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("Brief justification")),
		},
		aicommon.WithGeneralConfigStreamableFieldWithNodeId("intent", "reason"),
	)
	if err != nil {
		return nil, err
	}
	out := &syntaxFlowIntakeResult{
		TaskID:      strings.TrimSpace(result.GetString("task_id")),
		ProjectPath: strings.TrimSpace(result.GetString("project_path")),
		Reason:      result.GetString("reason"),
	}
	log.Infof("[syntaxflow_scan] LiteForge intake: task_id=%q project_path=%q reason=%q", out.TaskID, out.ProjectPath, out.Reason)
	return out, nil
}

// clearStaleSyntaxFlowTaskID 在走「新扫 / 本地编译」路径时清空父 loop 上的 task_id，避免同一会话上一轮扫描遗留的
// syntaxflow_task_id 覆盖本次显式配置（否则 phase_compile 会误走 attach、不再 LoadPrograms/StartScan）。
func clearStaleSyntaxFlowTaskID(state *SyntaxFlowState, parentLoop *reactloops.ReActLoop) {
	state.SetTaskID("")
	if parentLoop != nil {
		parentLoop.Set(sfu.LoopVarSyntaxFlowTaskID, "")
	}
}

// runPhase1Intake resolves task_id, explicit sf_scan_config_json, or project_path (WithVar, then LiteForge). Updates state and parent loop vars.
//
// 语义与顺序：
// 1) **显式新扫**（`sf_scan_config_json` / `project_path` / LiteForge 项目路径）优先于只读 task 附着。
// 2) `syntaxflow_scan_session_mode` 或 `irify_syntaxflow#session_mode`：值为 `start` 表示**新扫**（忽略 irify 随附的 `task_id` 附件，仅可 WithVar 显式 task_id 仍会被 clear 掉）；值为 `attach` 表示**附着已有**。
// 3) 未显式设 session_mode 时，仍兼容仅带 `irify_syntaxflow/task_id` 的**附着**行为。
func runPhase1Intake(
	r aicommon.AIInvokeRuntime,
	state *SyntaxFlowState,
	parentLoop *reactloops.ReActLoop,
	task aicommon.AIStatefulTask,
) error {
	state.SetPhase(SyntaxFlowPhaseIntake)
	userInput := task.GetUserInput()

	sfu.SyncSyntaxFlowLoopVarsFromIrifyTask(parentLoop, task)
	mode := strings.ToLower(strings.TrimSpace(parentLoop.Get(sfu.LoopVarSyntaxFlowScanSessionMode)))
	if mode != "" {
		parentLoop.Set(sfu.LoopVarSyntaxFlowScanSessionMode, mode)
	}
	// 新扫：先清可能来自底座/上层的 task_id 变量，且后续不采纳 irify 的 task_id 附件
	if mode == sfu.SessionModeStart {
		clearStaleSyntaxFlowTaskID(state, parentLoop)
	}

	if j := strings.TrimSpace(parentLoop.Get(sfu.LoopVarSFScanConfigJSON)); j != "" {
		clearStaleSyntaxFlowTaskID(state, parentLoop)
		state.SetResolvedSFScanConfigJSON(j)
		state.SetConfigInferred("0")
		return nil
	}

	proj := strings.TrimSpace(parentLoop.Get(sfu.LoopVarProjectPath))
	if proj == "" {
		proj = strings.TrimSpace(parentLoop.Get("target_path")) // common alias
	}
	if proj != "" {
		clearStaleSyntaxFlowTaskID(state, parentLoop)
		if err := pathMustExistForScan(proj); err != nil {
			return err
		}
		j, err := BuildCodeScanJSONForLocalPath(proj)
		if err != nil {
			return fmt.Errorf("build scan config for project path %s: %w", proj, err)
		}
		state.SetResolvedSFScanConfigJSON(j)
		state.SetConfigInferred("1")
		parentLoop.Set(sfu.LoopVarSFScanConfigJSON, j)
		return nil
	}

	ex, err := extractSyntaxFlowScanIntake(task.GetContext(), r, userInput)
	if err != nil {
		log.Warnf("[syntaxflow_scan] LiteForge intake failed: %v", err)
	} else {
		if ex.ProjectPath != "" {
			clearStaleSyntaxFlowTaskID(state, parentLoop)
			if err := pathMustExistForScan(ex.ProjectPath); err != nil {
				return err
			}
			j, berr := BuildCodeScanJSONForLocalPath(ex.ProjectPath)
			if berr != nil {
				return fmt.Errorf("build scan config for extracted project path: %w", berr)
			}
			state.SetResolvedSFScanConfigJSON(j)
			state.SetConfigInferred("1")
			parentLoop.Set(sfu.LoopVarSFScanConfigJSON, j)
			return nil
		}
		// 新扫模式下不从自然语言采纳「已有 task_id」
		if ex.TaskID != "" && mode != sfu.SessionModeStart {
			state.SetTaskID(ex.TaskID)
			parentLoop.Set(sfu.LoopVarSyntaxFlowTaskID, ex.TaskID)
			return nil
		}
	}

	// 仅 attach / 未声明 start：接受 WithVar 与已在 Sync 中写入的 syntaxflow_task_id
	if id := strings.TrimSpace(parentLoop.Get(sfu.LoopVarSyntaxFlowTaskID)); id != "" {
		state.SetTaskID(id)
		parentLoop.Set(sfu.LoopVarSyntaxFlowTaskID, id)
		return nil
	}

	if mode == sfu.SessionModeStart {
		return fmt.Errorf("当前为**新扫**（`syntaxflow_scan_session_mode` 或 `irify_syntaxflow#session_mode` = `start`）：已忽略 `task_id` 随附/解析结果；请提供 `sf_scan_config_json`、`project_path` 或含**绝对**目录的自然语言，以进行本地编译与起扫。")
	}
	if mode == sfu.SessionModeAttach {
		return fmt.Errorf("当前为**附着**（`session_mode=attach`），但未解析到有效 `task_id`：请随任务附加 `irify_syntaxflow` + `task_id`，或设置 loop 变量 `syntaxflow_task_id`。")
	}
	return fmt.Errorf("missing scan input: set Loop var `syntaxflow_task_id` or `sf_scan_config_json` or `project_path` (or say an absolute project path in your message). Example: attach task id `syntaxflow_task_id=...` or new scan: `请扫描 /path/to/project`")
}

func pathMustExistForScan(p string) error {
	_, err := os.Stat(strings.TrimSpace(p))
	if err != nil {
		return fmt.Errorf("project path not accessible: %s: %w", p, err)
	}
	return nil
}

// --- scan session (DB + formatting) ---

const DefaultRiskSampleLimit = 100

// ScanSessionResult aggregates a SyntaxFlow scan task and SSA risks for that runtime (task_id == runtime_id).
type ScanSessionResult struct {
	TaskID     string
	ScanTask   *schema.SyntaxFlowScanTask
	TotalRisks int
	Risks      []*schema.SSARisk
	Preface    string
}

// LoadScanSessionResult loads task row + risk count + up to riskSampleLimit risks for AI preface.
func LoadScanSessionResult(db *gorm.DB, taskID string, riskSampleLimit int) (*ScanSessionResult, error) {
	if db == nil {
		return nil, utils.Error("nil db")
	}
	if taskID == "" {
		return nil, utils.Error("empty task_id")
	}
	if riskSampleLimit <= 0 {
		riskSampleLimit = DefaultRiskSampleLimit
	}

	_, tasks, err := yakit.QuerySyntaxFlowScanTask(db, &ypb.QuerySyntaxFlowScanTaskRequest{
		Filter: &ypb.SyntaxFlowScanTaskFilter{
			TaskIds: []string{taskID},
		},
		Pagination: &ypb.Paging{Page: 1, Limit: 1, OrderBy: "id", Order: "desc"},
	})
	if err != nil || len(tasks) == 0 {
		return nil, utils.Errorf("syntaxflow scan task not found: %v", err)
	}
	st := tasks[0]

	riskFilter := &ypb.SSARisksFilter{
		RuntimeID: []string{taskID},
	}
	totalRisks, err := yakit.QuerySSARiskCount(db, riskFilter)
	if err != nil {
		totalRisks = 0
	}

	paging := &ypb.Paging{Page: 1, Limit: int64(riskSampleLimit), OrderBy: "id", Order: "desc"}
	_, risks, qerr := yakit.QuerySSARisk(db, riskFilter, paging)
	if qerr != nil {
		risks = nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("SyntaxFlowScanTask: task_id=%s programs=%s status=%s kind=%s risk_count=%d rules=%d\n",
		st.TaskId, st.Programs, st.Status, string(st.Kind), st.RiskCount, st.RulesCount))
	sb.WriteString(FormatScanTaskProgressLine(st))
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("SSA risks for this runtime (total=%d), showing up to %d:\n", totalRisks, len(risks)))
	for i, rk := range risks {
		sb.WriteString(fmt.Sprintf("%d. risk id=%d sev=%s rule=%s title=%s\n",
			i+1, rk.ID, rk.Severity, utils.ShrinkTextBlock(rk.FromRule, 64), utils.ShrinkTextBlock(rk.Title, 100)))
	}

	return &ScanSessionResult{
		TaskID:     taskID,
		ScanTask:   st,
		TotalRisks: totalRisks,
		Risks:      risks,
		Preface:    sb.String(),
	}, nil
}

// FormatScanTaskProgressLine summarizes query progress from the task row.
func FormatScanTaskProgressLine(st *schema.SyntaxFlowScanTask) string {
	if st == nil {
		return ""
	}
	return fmt.Sprintf("Progress: total_query=%d success=%d failed=%d skip=%d (status=%s)\n",
		st.TotalQuery, st.SuccessQuery, st.FailedQuery, st.SkipQuery, st.Status)
}

// FormatSyntaxFlowScanEndReport is a one-line scan-end summary for pipeline / logs.
func FormatSyntaxFlowScanEndReport(st *schema.SyntaxFlowScanTask) string {
	if st == nil {
		return ""
	}
	return fmt.Sprintf(
		"【扫描终态】 task_id=%s status=%s reason=%q programs=%s kind=%s\n"+
			"【规则/Query】 rules_count=%d total_query=%d success=%d failed=%d skip=%d\n"+
			"【Risk 分级】 total=%d critical=%d high=%d warn=%d low=%d info=%d",
		st.TaskId, st.Status, st.Reason, st.Programs, string(st.Kind),
		st.RulesCount, st.TotalQuery, st.SuccessQuery, st.FailedQuery, st.SkipQuery,
		st.RiskCount, st.CriticalCount, st.HighCount, st.WarningCount, st.LowCount, st.InfoCount,
	)
}

// FormatSyntaxFlowScanEndReportMarkdownTable 扫描结束用户向表格（主对话与 P4 输入），避免以内部 key 作主结构。
func FormatSyntaxFlowScanEndReportMarkdownTable(st *schema.SyntaxFlowScanTask) string {
	if st == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("## 扫描任务行终态（汇总）\n\n")
	b.WriteString("| 字段 | 值 |\n| --- | --- |\n")
	fmt.Fprintf(&b, "| task_id | `%s` |\n", st.TaskId)
	fmt.Fprintf(&b, "| status | `%s` |\n", st.Status)
	fmt.Fprintf(&b, "| reason | %s |\n", escapeScanTableCell(st.Reason))
	fmt.Fprintf(&b, "| programs | %s |\n", escapeScanTableCell(st.Programs))
	fmt.Fprintf(&b, "| kind | `%s` |\n", string(st.Kind))
	b.WriteString("\n### Query 与规则批次\n\n")
	b.WriteString("| 指标 | 数值 |\n| --- | ---: |\n")
	fmt.Fprintf(&b, "| rules_count（批次数/规则配置相关） | %d |\n", st.RulesCount)
	fmt.Fprintf(&b, "| total_query | %d |\n", st.TotalQuery)
	fmt.Fprintf(&b, "| success | %d |\n", st.SuccessQuery)
	fmt.Fprintf(&b, "| failed | %d |\n", st.FailedQuery)
	fmt.Fprintf(&b, "| skip | %d |\n", st.SkipQuery)
	b.WriteString("\n**skip 说明**: " + SkipQueryProductHint + "\n\n")
	b.WriteString("### 风险分级汇总\n\n")
	b.WriteString("| 级别 | 条数 |\n| --- | ---: |\n")
	fmt.Fprintf(&b, "| total | %d |\n", st.RiskCount)
	fmt.Fprintf(&b, "| critical | %d |\n", st.CriticalCount)
	fmt.Fprintf(&b, "| high | %d |\n", st.HighCount)
	fmt.Fprintf(&b, "| warning | %d |\n", st.WarningCount)
	fmt.Fprintf(&b, "| low | %d |\n", st.LowCount)
	fmt.Fprintf(&b, "| info | %d |\n", st.InfoCount)
	return b.String()
}

func escapeScanTableCell(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "|", "¦")
	return s
}

// --- task validate ---

// ErrSyntaxFlowScanTaskNotFound 附着 task_id 在 SSA 工程库中无对应任务行时返回（errors.Is 可判断）。
var ErrSyntaxFlowScanTaskNotFound = errors.New("syntaxflow scan task not found in SSA DB")

// EnsureSyntaxFlowScanTaskExists 校验 `syntaxflow_scan_task` 中是否存在该 task_id（SSA runtime id）。
// 用于 **attach** 路径在编排层尽早失败，避免进入 phase 后才读库才报错。
func EnsureSyntaxFlowScanTaskExists(db *gorm.DB, taskID string) error {
	if db == nil {
		return fmt.Errorf("SSA 工程库未连接，无法校验 task_id")
	}
	tid := strings.TrimSpace(taskID)
	if tid == "" {
		return fmt.Errorf("task_id 为空，无法执行附着")
	}
	st, err := schema.GetSyntaxFlowScanTaskById(db, tid)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: task_id=%q 在库中无 SyntaxFlow 扫描任务行（请确认已落库或 id 非粘贴错误）: %v",
				ErrSyntaxFlowScanTaskNotFound, tid, err)
		}
		return fmt.Errorf("无法读取扫描任务行 task_id=%q: %w", tid, err)
	}
	if st == nil {
		return fmt.Errorf("%w: task_id=%q", ErrSyntaxFlowScanTaskNotFound, tid)
	}
	return nil
}
