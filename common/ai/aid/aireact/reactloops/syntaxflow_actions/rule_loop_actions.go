package syntaxflow_actions

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	sfs "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_services"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

// WithRunSyntaxFlowRuleOnProjectAction runs current or given .sf rule text against an SSA program (by program_name).
func WithRunSyntaxFlowRuleOnProjectAction(_ aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"run_syntaxflow_rule_on_project",
		"Execute SyntaxFlow rule text against one compiled SSA program_name. Prefer rule_path; otherwise falls back to current loop full_sf_code. Does not persist scan tasks unless save_result=1.",
		[]aitool.ToolOption{
			aitool.WithStringParam("program_name", aitool.WithParam_Description("SSA Program name to query (LoadProgramRegexp)."), aitool.WithParam_Required(true)),
			aitool.WithStringParam("rule_path", aitool.WithParam_Description("Path to .sf file; if empty uses full_sf_code from loop.")),
			aitool.WithStringParam("syntaxflow_code", aitool.WithParam_Description("Inline rule when rule_path empty and loop has no file.")),
			aitool.WithIntegerParam("save_result", aitool.WithParam_Description("1 to note intent to save (engine still in-process only).")),
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			if strings.TrimSpace(action.GetString("program_name")) == "" {
				return utils.Error("program_name is required")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			pn := strings.TrimSpace(action.GetString("program_name"))
			var ruleText string
			if p := strings.TrimSpace(action.GetString("rule_path")); p != "" {
				raw, err := os.ReadFile(p)
				if err != nil {
					operator.Feedback(fmt.Sprintf("run_syntaxflow_rule_on_project: read rule: %v", err))
					operator.Continue()
					return
				}
				ruleText = string(raw)
			} else if s := strings.TrimSpace(action.GetString("syntaxflow_code")); s != "" {
				ruleText = s
			} else if loop != nil {
				ruleText = strings.TrimSpace(loop.Get("full_sf_code"))
			}
			if ruleText == "" {
				operator.Feedback("run_syntaxflow_rule_on_project: provide rule_path, syntaxflow_code, or full_sf_code on loop")
				operator.Continue()
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			res, err := sfs.RunRuleContentOnProgram(ctx, pn, ruleText)
			if err != nil {
				operator.Feedback(fmt.Sprintf("run_syntaxflow_rule_on_project failed: %v", err))
				operator.Continue()
				return
			}
			summary := sfs.FormatSyntaxFlowResultSummary(res)
			if loop != nil {
				loop.Set("sf_rule_trial_summary", utils.ShrinkTextBlock(summary, 8000))
			}
			if action.GetInt("save_result") != 0 {
				summary += "\n(note: save_result requests persistence via normal Yakit scan workflows; hook not wired here.)\n"
			}
			operator.Feedback("[run_syntaxflow_rule_on_project]\n" + summary)
			operator.Continue()
		},
	)
}

// WithOpenCampaignFromRuleAction records handoff to syntaxflow_rule_campaign / batch scan (user runs separate focus mode).
func WithOpenCampaignFromRuleAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"open_campaign_from_rule",
		"Pack rule path + optional project selector for a follow-up batch scan (syntaxflow_rule_campaign or project_batch_scan focus in Yakit).",
		[]aitool.ToolOption{
			aitool.WithStringParam("rule_path", aitool.WithParam_Description("Path to validated .sf file.")),
			aitool.WithStringParam("project_selector_json", aitool.WithParam_Description("Optional JSON describing language / search / all projects.")),
		},
		func(_ *reactloops.ReActLoop, _ *aicommon.Action) error { return nil },
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			rp := strings.TrimSpace(action.GetString("rule_path"))
			if rp == "" && loop != nil {
				rp = strings.TrimSpace(loop.Get("sf_filename"))
			}
			if loop != nil {
				loop.Set("sf_campaign_rule_path", rp)
				loop.Set("sf_campaign_project_selector_json", strings.TrimSpace(action.GetString("project_selector_json")))
			}
			msg := fmt.Sprintf("[open_campaign_from_rule] rule_path=%q — open **syntaxflow_rule_campaign** or **project_batch_scan** with these loop vars / IRify attachments.", rp)
			r.AddToTimeline("write_syntaxflow_rule", msg)
			operator.Feedback(msg)
			operator.Continue()
		},
	)
}

// WithListSSAProjectsAction lists SSA projects from profile DB.
func WithListSSAProjectsAction(_ aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"list_ssa_projects",
		"List SSA projects (paged) from the profile database via QuerySSAProject.",
		[]aitool.ToolOption{
			aitool.WithStringParam("search", aitool.WithParam_Description("Fuzzy search keyword.")),
			aitool.WithStringParam("language", aitool.WithParam_Description("Filter by language token.")),
			aitool.WithIntegerParam("limit", aitool.WithParam_Description("Page size (default 30).")),
		},
		func(_ *reactloops.ReActLoop, _ *aicommon.Action) error { return nil },
		func(_ *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			text, err := sfs.ListProjectsText(action.GetString("search"), action.GetString("language"), action.GetInt("limit"))
			if err != nil {
				operator.Feedback(fmt.Sprintf("list_ssa_projects: %v", err))
				operator.Continue()
				return
			}
			operator.Feedback(text)
			operator.Continue()
		},
	)
}

// WithResolveSSAProjectsAction is a stub resolver — echoes selector for now.
func WithResolveSSAProjectsAction(_ aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"resolve_ssa_projects",
		"Resolve project selection JSON into a concrete list (placeholder: returns raw selector).",
		[]aitool.ToolOption{
			aitool.WithStringParam("selector_json", aitool.WithParam_Description("JSON blob for filters.")),
		},
		func(_ *reactloops.ReActLoop, _ *aicommon.Action) error { return nil },
		func(_ *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			operator.Feedback("[resolve_ssa_projects] Not fully implemented; selector was:\n" + strings.TrimSpace(action.GetString("selector_json")))
			operator.Continue()
		},
	)
}

// WithCompileSSAProjectAction reminds to use code-scan compile path (deterministic compile is via syntaxflow_scan / yak CLI).
func WithCompileSSAProjectAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"compile_ssa_project",
		"Record compile intent for a project path or id; full compile uses syntaxflow_scan new-scan path or yak code-scan.",
		[]aitool.ToolOption{
			aitool.WithStringParam("project_path", aitool.WithParam_Description("Local path to compile.")),
			aitool.WithIntegerParam("project_id", aitool.WithParam_Description("Optional SSA project id.")),
		},
		func(_ *reactloops.ReActLoop, _ *aicommon.Action) error { return nil },
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			msg := fmt.Sprintf("[compile_ssa_project] path=%q id=%d — use **syntaxflow_scan** with project_path / sf_scan_config_json for in-process compile.",
				action.GetString("project_path"), action.GetInt("project_id"))
			if loop != nil {
				loop.Set("sf_compile_hint", msg)
			}
			r.AddToTimeline("project", msg)
			operator.Feedback(msg)
			operator.Continue()
		},
	)
}

// WithProjectBatchScanHintAction hints batch scan orchestrator.
func WithProjectBatchScanHintAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"project_batch_scan",
		"Document intent to run project_batch_scan orchestrator with current selector vars (placeholder handoff).",
		[]aitool.ToolOption{},
		func(_ *reactloops.ReActLoop, _ *aicommon.Action) error { return nil },
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			_ = action
			msg := "[project_batch_scan] Open **project_batch_scan** focus mode in Yakit with irify attachments; orchestrator wiring is server-side."
			if loop != nil {
				loop.Set("sf_project_batch_scan_hint", "1")
			}
			r.AddToTimeline("project", msg)
			operator.Feedback(msg)
			operator.Continue()
		},
	)
}
