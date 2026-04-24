package loop_syntaxflow_scan

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	sfu "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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
