package loop_syntaxflow_scan

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	sfu "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// WithHandoffSyntaxFlowAuditAnalystAction runs the syntaxflow_audit_analyst child loop once with current scan context.
func WithHandoffSyntaxFlowAuditAnalystAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"handoff_syntaxflow_audit_analyst",
		"Runs the **syntaxflow_audit_analyst** sub-loop once with the current task_id and risk digest (blocking until the child loop exits).",
		[]aitool.ToolOption{
			aitool.WithStringParam("extra_context", aitool.WithParam_Description("Optional focus or question for the analyst.")),
		},
		func(_ *reactloops.ReActLoop, _ *aicommon.Action) error { return nil },
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			if loop == nil {
				operator.Continue()
				return
			}
			parent := loop.GetCurrentTask()
			if parent == nil {
				operator.Feedback("handoff_syntaxflow_audit_analyst: no current task on loop")
				operator.Continue()
				return
			}
			tid := strings.TrimSpace(loop.Get(sfu.LoopVarSyntaxFlowTaskID))
			if tid == "" {
				tid = strings.TrimSpace(loop.Get("sf_scan_task_id"))
			}
			ui := fmt.Sprintf("SyntaxFlow audit analyst handoff.\ntask_id=%s\nssa_risk_overview_preface (truncated):\n%s\nssa_risk_list_summary (truncated):\n%s\nExtra:\n%s",
				tid,
				utils.ShrinkTextBlock(loop.Get("ssa_risk_overview_preface"), 8000),
				utils.ShrinkTextBlock(loop.Get("ssa_risk_list_summary"), 12000),
				strings.TrimSpace(action.GetString("extra_context")),
			)
			child, err := reactloops.CreateLoopByName(schema.AI_REACT_LOOP_NAME_SYNTAXFLOW_AUDIT_ANALYST, r)
			if err != nil {
				operator.Feedback(fmt.Sprintf("handoff_syntaxflow_audit_analyst: CreateLoopByName: %v", err))
				operator.Continue()
				return
			}
			child.Set(sfu.LoopVarSyntaxFlowTaskID, tid)
			child.Set("ssa_risk_overview_preface", loop.Get("ssa_risk_overview_preface"))

			subID := fmt.Sprintf("%s-audit-analyst", parent.GetId())
			sub := aicommon.NewSubTaskBase(parent, subID, ui, true)
			if err := child.ExecuteWithExistedTask(sub); err != nil {
				operator.Feedback(fmt.Sprintf("handoff_syntaxflow_audit_analyst: child loop error: %v", err))
			} else {
				operator.Feedback("[handoff_syntaxflow_audit_analyst] child loop finished.")
			}
			operator.Continue()
		},
	)
}

// WithOpenReviewForRiskAction sets ssa_risk_id and records a handoff hint (user switches focus mode in Yakit to ssa_risk_review).
func WithOpenReviewForRiskAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"open_review_for_risk",
		"Focus a single SSA risk id for deep review. Sets loop var ssa_risk_id and reminds to open or stay in ssa_risk_review focus mode; does not start another loop by itself.",
		[]aitool.ToolOption{
			aitool.WithIntegerParam("risk_id", aitool.WithParam_Description("SSA Risk primary key."), aitool.WithParam_Required(true)),
		},
		func(_ *reactloops.ReActLoop, action *aicommon.Action) error {
			if action.GetInt("risk_id") <= 0 {
				return utils.Error("risk_id must be positive")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			id := int64(action.GetInt("risk_id"))
			loop.Set(sfu.LoopVarSSARiskID, fmt.Sprintf("%d", id))
			msg := fmt.Sprintf("[open_review_for_risk] Set ssa_risk_id=%d — switch to SSA Risk Review focus or call reload_ssa_risk in that mode.", id)
			r.AddToTimeline("syntaxflow_scan", msg)
			operator.Feedback(msg)
			operator.Continue()
		},
	)
}

// WithOpenRuleWriterFromScanAction packs scan-linked context for a subsequent write_syntaxflow_rule session.
func WithOpenRuleWriterFromScanAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"open_rule_writer_from_scan",
		"Snapshot task id + overview summary into sf_rule_seed_from_scan_* vars for handing off to rule writing (paste into new write_syntaxflow_rule task or IRify focus).",
		[]aitool.ToolOption{},
		func(_ *reactloops.ReActLoop, _ *aicommon.Action) error { return nil },
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			tid := strings.TrimSpace(loop.Get(sfu.LoopVarSyntaxFlowTaskID))
			if tid == "" {
				tid = strings.TrimSpace(loop.Get("sf_scan_task_id"))
			}
			sum := strings.TrimSpace(loop.Get("ssa_risk_list_summary"))
			loop.Set("sf_rule_seed_scan_task_id", tid)
			loop.Set("sf_rule_seed_scan_risk_digest", utils.ShrinkTextBlock(sum, 12000))
			hint := fmt.Sprintf("[open_rule_writer_from_scan] task_id=%s — use write_syntaxflow_rule with seed vars sf_rule_seed_scan_* or combine with derive_rule_seed_from_risk.", tid)
			r.AddToTimeline("syntaxflow_scan", hint)
			operator.Feedback(hint)
			operator.Continue()
		},
	)
}

// WithOpenCodeAuditFromScanAction records handoff for syntaxflow_code_audit with current task id.
func WithOpenCodeAuditFromScanAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"open_code_audit_from_scan",
		"Prepare loop vars for SyntaxFlow code audit: attaches syntaxflow_task_id hint for a follow-up syntaxflow_code_audit task.",
		[]aitool.ToolOption{
			aitool.WithStringParam("project_path", aitool.WithParam_Description("Optional explicit project path override for audit orchestrator.")),
		},
		func(_ *reactloops.ReActLoop, _ *aicommon.Action) error { return nil },
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			tid := strings.TrimSpace(loop.Get(sfu.LoopVarSyntaxFlowTaskID))
			if tid == "" {
				tid = strings.TrimSpace(loop.Get("sf_scan_task_id"))
			}
			loop.Set(sfu.LoopVarSyntaxFlowTaskID, tid)
			if p := strings.TrimSpace(action.GetString("project_path")); p != "" {
				loop.Set(sfu.LoopVarProjectPath, p)
			}
			hint := fmt.Sprintf("[open_code_audit_from_scan] Use syntaxflow_code_audit with task_id=%s (irify_syntaxflow attachment) and project_path if set.", tid)
			r.AddToTimeline("syntaxflow_scan", hint)
			operator.Feedback(hint)
			operator.Continue()
		},
	)
}

// WithReadSSAProjectFileAction reads a source file slice via an existing aitool if registered; placeholder uses feedback only.
func WithReadSSAProjectFileAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"read_ssa_project_file",
		"Reminder: use the ssa-read-file tool from the tool panel with program_name + path (+ optional line range). This action records intent on the loop only.",
		[]aitool.ToolOption{
			aitool.WithStringParam("program_name", aitool.WithParam_Required(true)),
			aitool.WithStringParam("path", aitool.WithParam_Required(true)),
			aitool.WithIntegerParam("start_line", aitool.WithParam_Description("Optional 1-based start line.")),
			aitool.WithIntegerParam("limit", aitool.WithParam_Description("Optional line limit.")),
		},
		func(_ *reactloops.ReActLoop, action *aicommon.Action) error {
			if strings.TrimSpace(action.GetString("program_name")) == "" || strings.TrimSpace(action.GetString("path")) == "" {
				return utils.Error("program_name and path are required")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			msg := fmt.Sprintf("[read_ssa_project_file] Invoke **ssa-read-file** tool manually: program_name=%q path=%q start_line=%d limit=%d",
				action.GetString("program_name"), action.GetString("path"), action.GetInt("start_line"), action.GetInt("limit"))
			if loop != nil {
				loop.Set("ssa_read_file_last_hint", msg)
			}
			r.AddToTimeline("syntaxflow_scan", msg)
			operator.Feedback(msg + "\n(ReAct loop keeps policy: file bytes come from tools, not this handler.)")
			operator.Continue()
		},
	)
}
