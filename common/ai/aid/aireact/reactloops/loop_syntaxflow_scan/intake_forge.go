package loop_syntaxflow_scan

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// SyntaxFlowScanIntakeResult is the structured LiteForge output for syntaxflow_scan P1 intake.
type SyntaxFlowScanIntakeResult struct {
	TaskID      string
	ProjectPath string
	Reason      string
	RuleHint    string `json:"rule_hint,omitempty"` // optional future fields
}

// ExtractSyntaxFlowScanIntake reads task_id and/or absolute project_path from natural language (LiteForge single step).
func ExtractSyntaxFlowScanIntake(ctx context.Context, r aicommon.AIInvokeRuntime, userInput string) (*SyntaxFlowScanIntakeResult, error) {
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
5. rule_hint: optional short hint if user mentioned a rule name, severity, or scan focus.

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
			aitool.WithStringParam("rule_hint",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("Optional rule name/focus snippet")),
			aitool.WithStringParam("reason",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("Brief justification")),
		},
		aicommon.WithGeneralConfigStreamableFieldWithNodeId("intent", "reason"),
	)
	if err != nil {
		return nil, err
	}
	out := &SyntaxFlowScanIntakeResult{
		TaskID:      strings.TrimSpace(result.GetString("task_id")),
		ProjectPath: strings.TrimSpace(result.GetString("project_path")),
		RuleHint:    strings.TrimSpace(result.GetString("rule_hint")),
		Reason:      result.GetString("reason"),
	}
	log.Infof("[syntaxflow_scan] LiteForge intake: task_id=%q project_path=%q rule_hint=%q reason=%q",
		out.TaskID, out.ProjectPath, out.RuleHint, out.Reason)
	return out, nil
}
