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

// WithOpenRuleWriterFromScanAction gives task_id + risk list digest in tool feedback/timeline only (no loop vars; nothing else reads seeds here).
func WithOpenRuleWriterFromScanAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"open_rule_writer_from_scan",
		"Publishes syntaxflow_task_id and a truncated ssa_risk_list_summary in Feedback for handoff to write_syntaxflow_rule (or use derive_rule_seed_from_risk). Does not mutate loop vars.",
		[]aitool.ToolOption{},
		func(_ *reactloops.ReActLoop, _ *aicommon.Action) error { return nil },
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			tid := strings.TrimSpace(loop.Get(sfu.LoopVarSyntaxFlowTaskID))
			sum := utils.ShrinkTextBlock(strings.TrimSpace(loop.Get("ssa_risk_list_summary")), 12000)
			hint := fmt.Sprintf("[open_rule_writer_from_scan] task_id=%s\n\nTruncated risk list digest (paste if needed):\n%s", tid, sum)
			r.AddToTimeline("syntaxflow_scan", utils.ShrinkTextBlock(hint, 4000))
			operator.Feedback(hint)
			operator.Continue()
		},
	)
}

// WithOpenCodeAuditFromScanAction records handoff for syntaxflow_code_audit in Feedback/timeline only (no loop mutation; orchestrator-owned keys stay unchanged).
func WithOpenCodeAuditFromScanAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"open_code_audit_from_scan",
		"Reminder to follow up with syntaxflow_code_audit: repeats syntaxflow_task_id and optional project_path in Feedback only (irify_syntaxflow attachment in the new task).",
		[]aitool.ToolOption{
			aitool.WithStringParam("project_path", aitool.WithParam_Description("Optional explicit project path for the audit follow-up session.")),
		},
		func(_ *reactloops.ReActLoop, _ *aicommon.Action) error { return nil },
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			tid := strings.TrimSpace(loop.Get(sfu.LoopVarSyntaxFlowTaskID))
			var hint string
			if p := strings.TrimSpace(action.GetString("project_path")); p != "" {
				hint = fmt.Sprintf("[open_code_audit_from_scan] Use syntaxflow_code_audit with syntaxflow_task_id=%q (attach irify_syntaxflow), project_path=%q.", tid, p)
			} else {
				hint = fmt.Sprintf("[open_code_audit_from_scan] Use syntaxflow_code_audit with syntaxflow_task_id=%q (attach irify_syntaxflow).", tid)
			}
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
		"Reminder: use the ssa-read-file tool from the tool panel with program_name + path (+ optional line range). Feedback and timeline only; file bytes remain from tools.",
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
		func(_ *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			msg := fmt.Sprintf("[read_ssa_project_file] Invoke **ssa-read-file** tool manually: program_name=%q path=%q start_line=%d limit=%d",
				action.GetString("program_name"), action.GetString("path"), action.GetInt("start_line"), action.GetInt("limit"))
			r.AddToTimeline("syntaxflow_scan", msg)
			operator.Feedback(msg + "\n(ReAct loop keeps policy: file bytes come from tools, not this handler.)")
			operator.Continue()
		},
	)
}

// SFAuditCodeSearchHint is appended to SyntaxFlow code-audit rule prompts so the model greps the tree before writing rules.
func SFAuditCodeSearchHint() string {
	return `

【代码搜索 / 专注阅读】在编写或修改 SyntaxFlow 规则前，请使用 grep、read_file、find_file 在已探索的项目路径内缩小 Source/Sink 与框架入口，避免仅凭猜测写规则；优先在可疑目录（handler、controller、router）上缩小范围后再 grep。`
}
