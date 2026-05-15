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

// ParseSyntaxFlowScanIntakeSignalsFromAttachments 读取 irify_syntaxflow 上 session_mode / task_id / project_path / project_name / program_name。
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
		case sfu.IrifyKeyProjectPath:
			if v := strings.TrimSpace(a.Value); v != "" {
				out.ProjectPath = v
			}
		case sfu.IrifyKeyProjectName:
			if v := strings.TrimSpace(a.Value); v != "" {
				out.ProjectName = v
			}
		case sfu.IrifyKeyProgramName:
			if v := strings.TrimSpace(a.Value); v != "" {
				out.ProgramName = v
			}
		}
	}
	return out
}

// ForgeSyntaxFlowScanIntakeSignals 从用户自然语言抽取负载；Mode 保持 none，提交时由 commitScanFromIntakeSignals 写入 attach/compile_scan/program。
func ForgeSyntaxFlowScanIntakeSignals(ctx context.Context, r aicommon.AIInvokeRuntime, userInput string) (SyntaxFlowScanIntakeSignals, error) {
	var zero SyntaxFlowScanIntakeSignals

	promptTpl := `From the user message, extract at most ONE primary kind of SyntaxFlow scan intake (see rules).

## User message
<|USER_INPUT_{{ .Nonce }}|>
{{ .UserInput }}
<|USER_INPUT_END_{{ .Nonce }}|>

## Rules
1. task_id: an **existing** SyntaxFlow scan runtime id / UUID in SSA (attach to that scan). Not a file path.
2. program_name: an **existing** compiled SSA Program name to scan directly (no compile).
3. project_path: a single local **absolute** directory (or file under a project) to compile then scan.
4. project_name: optional SSA project name when project_path is given (helps match an existing SSAProject row).
5. Prefer leaving all fields empty if ambiguous or if the user gave multiple conflicting primary signals.
6. At most one of task_id / program_name / project_path should be non-empty.

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
			aitool.WithStringParam("program_name",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("Existing compiled SSA program name or empty")),
			aitool.WithStringParam("project_path",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("Absolute project path for compile+scan or empty")),
			aitool.WithStringParam("project_name",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("Optional SSA project name when project_path is set")),
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
		TaskID:      strings.TrimSpace(result.GetString("task_id")),
		ProgramName: strings.TrimSpace(result.GetString("program_name")),
		ProjectName: strings.TrimSpace(result.GetString("project_name")),
		ProjectPath: strings.TrimSpace(result.GetString("project_path")),
	}
	log.Infof("[syntaxflow_scan] intake forge: task_id=%q program_name=%q project_name=%q path=%q",
		out.TaskID, out.ProgramName, out.ProjectName, out.ProjectPath)
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

// errIntakeSignalsUnmatched 表示 Mode 为 none 且 task_id / program_name / project_path 均未形成可提交负载。
var errIntakeSignalsUnmatched = errors.New("syntaxflow intake: no matching signals")

func normalizeCommittedIntakeSignals(s SyntaxFlowScanIntakeSignals) (SyntaxFlowScanIntakeSignals, error) {
	s.TaskID = strings.TrimSpace(s.TaskID)
	s.ProjectPath = strings.TrimSpace(s.ProjectPath)
	s.ProjectName = strings.TrimSpace(s.ProjectName)
	s.ProgramName = strings.TrimSpace(s.ProgramName)

	mode := s.Mode
	if mode == SyntaxFlowSessionModeNone {
		switch {
		case s.TaskID != "":
			mode = SyntaxFlowSessionModeAttach
		case s.ProgramName != "":
			mode = SyntaxFlowSessionModeProgramScan
		case s.ProjectPath != "":
			mode = SyntaxFlowSessionModeCompileScan
		default:
			return SyntaxFlowScanIntakeSignals{}, errIntakeSignalsUnmatched
		}
	}

	switch mode {
	case SyntaxFlowSessionModeAttach:
		if s.TaskID == "" {
			return SyntaxFlowScanIntakeSignals{}, fmt.Errorf("附着需要非空的 syntaxflow_task_id")
		}
		return SyntaxFlowScanIntakeSignals{Mode: mode, TaskID: s.TaskID}, nil
	case SyntaxFlowSessionModeCompileScan:
		if s.ProjectPath == "" {
			return SyntaxFlowScanIntakeSignals{}, fmt.Errorf("compile_scan 需要非空的 project_path")
		}
		if err := requireLocalPathForScan(s.ProjectPath); err != nil {
			return SyntaxFlowScanIntakeSignals{}, err
		}
		return SyntaxFlowScanIntakeSignals{
			Mode:        mode,
			ProjectPath: s.ProjectPath,
			ProjectName: s.ProjectName,
		}, nil
	case SyntaxFlowSessionModeProgramScan:
		if s.ProgramName == "" {
			return SyntaxFlowScanIntakeSignals{}, fmt.Errorf("program 模式需要非空的 program_name")
		}
		return SyntaxFlowScanIntakeSignals{Mode: mode, ProgramName: s.ProgramName}, nil
	default:
		return SyntaxFlowScanIntakeSignals{}, fmt.Errorf("附件 session_mode 无效（仅 attach、compile_scan 或 program）")
	}
}

// commitScanFromIntakeSignals 将 intake 规范化后一次性写入 state.SyntaxFlowScanIntakeSignals。
func commitScanFromIntakeSignals(state *SyntaxFlowState, s SyntaxFlowScanIntakeSignals) error {
	sig, err := normalizeCommittedIntakeSignals(s)
	if err != nil {
		return err
	}
	state.SetIntakeSignals(sig)
	return nil
}

// syncLoopIntakeState 仅把 attach 所需的 loop 键同步给后续 action / 报告；其余 intake 只读 state。
func syncLoopIntakeState(loop *reactloops.ReActLoop, state *SyntaxFlowState) {
	if loop == nil || state == nil {
		return
	}
	if wire := state.GetSessionMode().WireValue(); wire != "" {
		loop.Set(sfu.LoopVarSyntaxFlowScanSessionMode, wire)
	}
	if state.GetSessionMode() == SyntaxFlowSessionModeAttach {
		loop.Set(sfu.LoopVarSyntaxFlowTaskID, state.GetTaskID())
	}
}

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

func validateProgramAfterPhase1(state *SyntaxFlowState) error {
	if state.GetSessionMode() != SyntaxFlowSessionModeProgramScan {
		return nil
	}
	programName := strings.TrimSpace(state.GetProgramName())
	if programName == "" {
		return fmt.Errorf("program 模式但 program_name 为空")
	}
	if _, err := sfu.LoadCompiledProgramsByName(programName); err != nil {
		return err
	}
	return nil
}

// runPhase1Intake 仅从 irify_syntaxflow 附件与 LiteForge 解析入参。
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
	if err := commitScanFromIntakeSignals(state, attached); err != nil {
		if !errors.Is(err, errIntakeSignalsUnmatched) {
			return err
		}
	} else {
		syncLoopIntakeState(parentLoop, state)
		if err := validateAttachTaskAfterPhase1(state); err != nil {
			return err
		}
		return validateProgramAfterPhase1(state)
	}

	forged, err := ForgeSyntaxFlowScanIntakeSignals(ctx, r, userInput)
	if err != nil {
		return fmt.Errorf("irify_syntaxflow 无扫描负载且 LiteForge 失败: %w", err)
	}

	if err := commitScanFromIntakeSignals(state, forged); err != nil {
		if errors.Is(err, errIntakeSignalsUnmatched) {
			log.Infof("[syntaxflow_scan] intake forge empty")
			return fmt.Errorf("irify_syntaxflow 无扫描参数，用户输入也未给出 task_id、program_name 或 project_path（可补充附件 session_mode）")
		}
		return err
	}
	syncLoopIntakeState(parentLoop, state)
	if err := validateAttachTaskAfterPhase1(state); err != nil {
		return err
	}
	return validateProgramAfterPhase1(state)
}
