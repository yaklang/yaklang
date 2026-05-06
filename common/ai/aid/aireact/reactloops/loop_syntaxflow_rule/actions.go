package loop_syntaxflow_rule

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	sfu "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"
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
			res, err := sfu.RunRuleContentOnProgram(ctx, pn, ruleText)
			if err != nil {
				operator.Feedback(fmt.Sprintf("run_syntaxflow_rule_on_project failed: %v", err))
				operator.Continue()
				return
			}
			summary := sfu.FormatSyntaxFlowResultSummary(res)
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
