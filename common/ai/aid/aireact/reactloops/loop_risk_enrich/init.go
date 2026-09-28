package loop_risk_enrich

import (
	"bytes"
	_ "embed"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

//go:embed instruction.txt
var instruction string

func init() {
	err := reactloops.RegisterLoopFactory(schema.AI_REACT_LOOP_ACTION_RISK_ENRICH,
		func(invoker aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			preset := []reactloops.ReActLoopOption{
				reactloops.WithAllowToolCall(false),
				reactloops.WithAllowAIForge(false),
				reactloops.WithAllowPlanAndExec(false),
				reactloops.WithMaxIterations(20),
				reactloops.WithInitTask(buildInitTask(invoker)),
				reactloops.WithPersistentInstruction(instruction),
				reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedback *bytes.Buffer, nonce string) (string, error) {
					return "User request: " + loop.GetCurrentTask().GetUserInput() +
						"\nFixed attached risk context:\n" + loop.Get("risk_enrich_context") +
						"\nBoundary inventory:\n" + loop.Get("risk_enrich_inventory") +
						"\nSearches already tried:\n" + searchHistoryPrompt(loop) +
						"\nInspected evidence assessments:\n" + assessmentPrompt(loop) +
						"\nPrevious action feedback: " + feedback.String(), nil
				}),
				surveyAction(invoker),
				searchFlowAction(invoker),
				searchPortAction(invoker),
				inspectFlowAction(invoker),
				recordAssessmentAction(),
				attachFlowAction(invoker),
				fillPortAction(invoker),
			}
			preset = append(preset, opts...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_ACTION_RISK_ENRICH, invoker, preset...)
		},
		reactloops.WithLoopDescription("Investigate a specific risk's related traffic and ports; optionally enrich its stored evidence."),
		reactloops.WithLoopDescriptionZh("风险补全：根据指定风险关联流量和端口，可选择展示或显式写回证据。"),
		reactloops.WithLoopUsagePrompt("Use with exactly one attached resource of type risk_id (value: an existing risk ID). Initialization binds the risk and its runtime/session scope; the agent investigates evidence without choosing a risk ID."),
		reactloops.WithVerboseName("Risk Enrichment"),
		reactloops.WithVerboseNameZh("风险补全"),
	)
	if err != nil {
		log.Errorf("register risk enrichment loop: %v", err)
	}
}

func userRequestedWrite(input string) bool {
	input = strings.ToLower(input)
	for _, phrase := range []string{"不要写入", "不要保存", "不要补全", "只看", "只找", "只展示", "仅展示", "仅查询", "read only", "read-only", "do not update"} {
		if strings.Contains(input, phrase) {
			return false
		}
	}
	for _, word := range []string{"补全", "补充", "完善", "更新风险", "写入", "保存", "关联到风险", "attach", "persist", "enrich", "update risk"} {
		if strings.Contains(input, word) {
			return true
		}
	}
	return false
}
