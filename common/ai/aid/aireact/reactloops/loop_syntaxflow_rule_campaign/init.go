package loop_syntaxflow_rule_campaign

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	sfs "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_services"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func newSubTaskWithInput(parent aicommon.AIStatefulTask, name, userInput string) aicommon.AIStatefulTask {
	subID := fmt.Sprintf("%s-%s", parent.GetId(), name)
	return aicommon.NewSubTaskBase(parent, subID, userInput, true)
}

func campaignSelectorJSON(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) string {
	if loop != nil {
		if s := strings.TrimSpace(loop.Get("sf_campaign_project_selector_json")); s != "" {
			return s
		}
	}
	ui := strings.TrimSpace(task.GetUserInput())
	if ui != "" && ui[0] == '{' {
		var raw map[string]any
		if json.Unmarshal([]byte(ui), &raw) == nil {
			if _, ok := raw["project_paths"]; ok {
				return ui
			}
		}
	}
	for _, ln := range strings.Split(ui, "\n") {
		ln = strings.TrimSpace(ln)
		if strings.HasPrefix(ln, "{") {
			var raw map[string]any
			if json.Unmarshal([]byte(ln), &raw) == nil {
				if _, ok := raw["project_paths"]; ok {
					return ln
				}
			}
		}
	}
	return `{"project_paths":[]}`
}

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_SYNTAXFLOW_RULE_CAMPAIGN,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			preset := []reactloops.ReActLoopOption{
				reactloops.WithInitTask(func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
					ui := utils.ShrinkTextBlock(task.GetUserInput(), 4000)
					r.AddToTimeline("syntaxflow_rule_campaign", "[campaign] start: "+ui)

					rulePrompt := `You author or refine one SyntaxFlow rule file for a **multi-project campaign**. After saving, bulk scans will run this rule across project_paths from the selector JSON (when non-empty).

User / campaign brief:
` + ui

					ruleLoop, err := reactloops.CreateLoopByName(schema.AI_REACT_LOOP_NAME_WRITE_SYNTAXFLOW, r)
					if err != nil {
						op.Failed(err)
						return
					}
					if err := ruleLoop.ExecuteWithExistedTask(newSubTaskWithInput(task, "campaign_write_rule", rulePrompt)); err != nil {
						log.Warnf("[syntaxflow_rule_campaign] write_syntaxflow_rule: %v", err)
					}
					rulePath := strings.TrimSpace(ruleLoop.Get("sf_filename"))
					if rulePath == "" {
						r.AddToTimeline("syntaxflow_rule_campaign", "[campaign] no sf_filename from write_syntaxflow_rule; bulk step skipped")
						loop.Set("sf_bulk_campaign_id", "")
						op.Done()
						return
					}
					loop.Set("sf_campaign_rule_path", rulePath)

					sel := campaignSelectorJSON(loop, task)
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
					defer cancel()
					var bulk sfs.BulkScanService
					cid, err := bulk.RunRuleAcrossProjects(ctx, rulePath, sel)
					if err != nil {
						r.AddToTimeline("syntaxflow_rule_campaign", fmt.Sprintf("[campaign] bulk error: %v", err))
						loop.Set("sf_bulk_campaign_id", "")
					} else {
						loop.Set("sf_bulk_campaign_id", cid)
						r.AddToTimeline("syntaxflow_rule_campaign", fmt.Sprintf("[campaign] bulk started campaign_id=%s selector=%s",
							cid, utils.ShrinkTextBlock(sel, 800)))
					}
					op.Done()
				}),
				reactloops.WithMaxIterations(1),
				reactloops.WithAllowToolCall(false),
				reactloops.WithAllowRAG(false),
			}
			preset = append(preset, opts...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_SYNTAXFLOW_RULE_CAMPAIGN, r, preset...)
		},
		reactloops.WithVerboseName("SyntaxFlow · Rule rollout campaign"),
		reactloops.WithLoopDescription("Orchestrates write_syntaxflow_rule then BulkScanService.RunRuleAcrossProjects for each project path in the selector JSON."),
	)
	if err != nil {
		log.Errorf("register reactloop %v failed: %v", schema.AI_REACT_LOOP_NAME_SYNTAXFLOW_RULE_CAMPAIGN, err)
	}
}
