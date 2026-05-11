// Phase 1: irify_syntaxflow attachments + LiteForge → intake signals committed to state/loop vars.
package loop_syntaxflow_scan

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	sfu "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// ParseSyntaxFlowScanIntakeSignalsFromAttachments 读取 irify_syntaxflow 上 session_mode / task_id / sf_scan_config_json。
// 同 key 多条时，最后一条非空胜出。
func ParseSyntaxFlowScanIntakeSignalsFromAttachments(task aicommon.AIStatefulTask) SyntaxFlowScanIntakeSignals {
	var out SyntaxFlowScanIntakeSignals
	if task == nil {
		return out
	}
	for _, a := range task.GetAttachedDatas() {
		if a == nil || a.Type != sfu.IrifyTypeSyntaxFlow {
			continue
		}
		switch a.Key {
		case sfu.IrifyKeySessionMode:
			out.Mode = ParseSyntaxFlowScanSessionMode(a.Value)
		case sfu.IrifyKeyTaskID:
			if v := strings.TrimSpace(a.Value); v != "" {
				out.TaskID = v
			}
		case sfu.IrifyKeySFScanConfigJSON:
			if v := strings.TrimSpace(a.Value); v != "" {
				out.SFScanConfigJSON = v
			}
		}
	}
	return out
}

// ForgeSyntaxFlowScanIntakeSignals 从用户自然语言抽取负载；Mode 保持 none，提交时由 commitScanFromIntakeSignals 写入 Attach/Start。
func ForgeSyntaxFlowScanIntakeSignals(ctx context.Context, r aicommon.AIInvokeRuntime, userInput string) (SyntaxFlowScanIntakeSignals, error) {
	var zero SyntaxFlowScanIntakeSignals

	promptTpl := `From the user message, extract at most ONE primary kind of SyntaxFlow scan intake (see rules).

## User message
<|USER_INPUT_{{ .Nonce }}|>
{{ .UserInput }}
<|USER_INPUT_END_{{ .Nonce }}|>

## Rules
1. task_id: an **existing** SyntaxFlow scan runtime id / UUID in SSA (attach to that scan). Not a file path.
2. project_path: a single local **absolute** directory (or file under a project) to start a **new** scan.
3. sf_scan_config_json: the **full** code-scan JSON body (same as yak code-scan --config). Only if the user pasted near-valid JSON.
4. Prefer leaving all three empty if ambiguous or if the user gave multiple conflicting signals.
5. At most one of task_id / project_path / sf_scan_config_json should be non-empty.

Return structured fields.`

	rendered, err := utils.RenderTemplate(promptTpl, map[string]any{
		"Nonce":     utils.RandStringBytes(4),
		"UserInput": userInput,
	})
	if err != nil {
		return zero, utils.Wrap(err, "render syntaxflow intake forge prompt")
	}

	result, err := r.InvokeSpeedPriorityLiteForge(
		ctx,
		"extract-syntaxflow-scan-intake",
		rendered,
		[]aitool.ToolOption{
			aitool.WithStringParam("task_id",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("Existing scan task id or empty")),
			aitool.WithStringParam("project_path",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("Absolute project path for new scan or empty")),
			aitool.WithStringParam("sf_scan_config_json",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("Full code-scan JSON string or empty")),
			aitool.WithStringParam("reason",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("Brief justification")),
		},
		aicommon.WithGeneralConfigStreamableFieldWithNodeId("intent", "reason"),
	)
	if err != nil {
		return zero, err
	}

	out := SyntaxFlowScanIntakeSignals{
		TaskID:           strings.TrimSpace(result.GetString("task_id")),
		ProjectPath:      strings.TrimSpace(result.GetString("project_path")),
		SFScanConfigJSON: strings.TrimSpace(result.GetString("sf_scan_config_json")),
		Reason:           result.GetString("reason"),
	}
	log.Infof("[syntaxflow_scan] intake forge: task_id=%q path=%q json_len=%d reason=%q",
		out.TaskID, out.ProjectPath, len(out.SFScanConfigJSON), out.Reason)
	return out, nil
}

func requireLocalPathForScan(p string) error {
	p = strings.TrimSpace(p)
	if p == "" {
		return fmt.Errorf("项目路径为空")
	}
	ok, err := utils.PathExists(p)
	if err != nil {
		return fmt.Errorf("检查路径失败 %q: %w", p, err)
	}
	if !ok {
		return fmt.Errorf("路径不存在或不可访问: %s", p)
	}
	return nil
}

// commitNewScan 写入新扫：Mode=start，清空 task_id；可仅含 code-scan JSON、或仅含本地项目路径（由 P2 经 ssa_compile 探测+编译）。
func commitNewScan(state *SyntaxFlowState, loop *reactloops.ReActLoop, sfJSON string, configInferred string, intakeProjectPath string) error {
	sfJSON = strings.TrimSpace(sfJSON)
	intakeProjectPath = strings.TrimSpace(intakeProjectPath)
	if sfJSON == "" && intakeProjectPath == "" {
		return fmt.Errorf("新扫需要 sf_scan_config_json 与项目路径至少其一")
	}
	state.SetTaskID("")
	state.SetSessionMode(SyntaxFlowSessionModeStart)
	state.SetProjectPath(intakeProjectPath)
	loop.Set(sfu.LoopVarSyntaxFlowScanSessionMode, sfu.SessionModeStart)
	loop.Set(sfu.LoopVarSFScanConfigJSON, sfJSON)
	state.SetSFScanConfigJSON(sfJSON)
	inferred := strings.TrimSpace(configInferred)
	if inferred == "" {
		inferred = "0"
	}
	state.SetConfigInferred(inferred)
	return nil
}

// commitAttachScan 写入附着：task_id 存在性在 runPhase1Intake 结束前统一校验（validateAttachTaskAfterPhase1）。
func commitAttachScan(state *SyntaxFlowState, loop *reactloops.ReActLoop, taskID string) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("附着需要非空的 syntaxflow_task_id")
	}
	state.SetProjectPath("")
	state.SetSessionMode(SyntaxFlowSessionModeAttach)
	loop.Set(sfu.LoopVarSyntaxFlowScanSessionMode, sfu.SessionModeAttach)
	state.SetTaskID(taskID)
	loop.Set(sfu.LoopVarSyntaxFlowTaskID, taskID)
	return nil
}

// commitAttachScanWithOptionalResolvedJSON 附着并可选附带一份配置 JSON（仅存 state.SFScanConfigJSON，不参与 compile 主路径）。
func commitAttachScanWithOptionalResolvedJSON(state *SyntaxFlowState, loop *reactloops.ReActLoop, taskID string, optionalJSON string) error {
	if err := commitAttachScan(state, loop, taskID); err != nil {
		return err
	}
	if j := strings.TrimSpace(optionalJSON); j != "" {
		state.SetSFScanConfigJSON(j)
		state.SetConfigInferred("0")
	}
	return nil
}

// errIntakeSignalsUnmatched 表示 Mode 为 none 且 task_id / JSON / path 均未形成可提交负载；调用方（如再走 LiteForge）。
var errIntakeSignalsUnmatched = errors.New("syntaxflow intake: no matching signals")

// commitScanFromIntakeSignals 将 intake 落到 state/loop：显式 Mode 走 attach/start；Mode==none 时按 task_id → JSON → project_path 推导并写入对应 Mode 与路径快照。
// 成功提交返回 nil；无可行负载返回 [errIntakeSignalsUnmatched]；其余为校验/数据库/路径等错误。
// fromIrifyAttachment：为 true 时，无显式 Mode 下若同时给出 task_id 与 sf_scan_config_json 则报错（须显式 session_mode）；LiteForge 路径传 false。
func commitScanFromIntakeSignals(state *SyntaxFlowState, loop *reactloops.ReActLoop, s SyntaxFlowScanIntakeSignals, fromIrifyAttachment bool) error {
	switch s.Mode {
	case SyntaxFlowSessionModeNone:
		break // 见下方推导：成功后 Mode 必为 attach 或 start
	case SyntaxFlowSessionModeAttach:
		loop.Set(sfu.LoopVarSyntaxFlowScanSessionMode, sfu.SessionModeAttach)
		return commitAttachScanWithOptionalResolvedJSON(state, loop, s.TaskID, s.SFScanConfigJSON)
	case SyntaxFlowSessionModeStart:
		loop.Set(sfu.LoopVarSyntaxFlowScanSessionMode, sfu.SessionModeStart)
		j := strings.TrimSpace(s.SFScanConfigJSON)
		pp := strings.TrimSpace(s.ProjectPath)
		if j != "" {
			return commitNewScan(state, loop, j, "0", pp)
		}
		if pp == "" {
			return fmt.Errorf("session_mode=start 需要 sf_scan_config_json 或 project_path")
		}
		if err := requireLocalPathForScan(pp); err != nil {
			return err
		}
		return commitNewScan(state, loop, "", "1", pp)
	default:
		return fmt.Errorf("附件 session_mode 无效（仅 attach 或 start）")
	}

	if fromIrifyAttachment && strings.TrimSpace(s.TaskID) != "" && strings.TrimSpace(s.SFScanConfigJSON) != "" {
		return fmt.Errorf("附件同时含 task_id 与 sf_scan_config_json，请补充 irify_syntaxflow#session_mode")
	}
	if id := strings.TrimSpace(s.TaskID); id != "" {
		return commitAttachScan(state, loop, id)
	}
	if j := strings.TrimSpace(s.SFScanConfigJSON); j != "" {
		return commitNewScan(state, loop, j, "0", "")
	}
	if pp := strings.TrimSpace(s.ProjectPath); pp != "" {
		if err := requireLocalPathForScan(pp); err != nil {
			return err
		}
		return commitNewScan(state, loop, "", "1", pp)
	}
	return errIntakeSignalsUnmatched
}

// validateAttachTaskAfterPhase1 附着模式下校验 task_id 在 SSA 工程库存在（含「附着 + 可选 JSON」等路径）。
func validateAttachTaskAfterPhase1(state *SyntaxFlowState) error {
	if state.GetSessionMode() != SyntaxFlowSessionModeAttach {
		return nil
	}
	tid := strings.TrimSpace(state.GetTaskID())
	if tid == "" {
		return fmt.Errorf("附着模式但 task_id 为空")
	}
	db := sfu.GetSSADB()
	if db == nil {
		return fmt.Errorf("SSA 工程库未连接，无法校验 task_id")
	}
	return EnsureSyntaxFlowScanTaskExists(db, tid)
}

// runPhase1Intake 仅从 irify_syntaxflow 附件与 LiteForge 解析入参。
// 新扫时清空 state.TaskID；不向 loop 做「清理」型写入。
func runPhase1Intake(
	r aicommon.AIInvokeRuntime,
	state *SyntaxFlowState,
	parentLoop *reactloops.ReActLoop,
	task aicommon.AIStatefulTask,
) error {
	state.SetPhase(SyntaxFlowPhaseIntake)

	ctx := task.GetContext()
	if ctx == nil {
		ctx = context.Background()
	}
	userInput := task.GetUserInput()

	attached := ParseSyntaxFlowScanIntakeSignalsFromAttachments(task)
	if err := commitScanFromIntakeSignals(state, parentLoop, attached, true); err != nil {
		if !errors.Is(err, errIntakeSignalsUnmatched) {
			return err
		}
	} else {
		return validateAttachTaskAfterPhase1(state)
	}

	forged, err := ForgeSyntaxFlowScanIntakeSignals(ctx, r, userInput)
	if err != nil {
		return fmt.Errorf("irify_syntaxflow 无扫描负载且 LiteForge 失败: %w", err)
	}

	if err := commitScanFromIntakeSignals(state, parentLoop, forged, false); err != nil {
		if errors.Is(err, errIntakeSignalsUnmatched) {
			log.Infof("[syntaxflow_scan] intake forge empty: reason=%q", forged.Reason)
			return fmt.Errorf("irify_syntaxflow 无扫描参数，用户输入也未给出 task_id、路径或 sf_scan_config_json（可补充附件 session_mode）")
		}
		return err
	}
	state.SetIntakeReason(forged.Reason)
	return validateAttachTaskAfterPhase1(state)
}
