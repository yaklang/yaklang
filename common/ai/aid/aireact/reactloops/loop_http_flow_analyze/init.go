package loop_http_flow_analyze

import (
	"bytes"
	_ "embed"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/persistent_instruction.txt
var persistentInstruction string

//go:embed prompts/reactive_data.txt
var reactiveData string

//go:embed prompts/reflection_output_example.txt
var outputExample string

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_ACTION_HTTP_FLOW_ANALYZE,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {

			preset := []reactloops.ReActLoopOption{
				reactloops.WithAllowRAG(true),
				reactloops.WithAllowToolCall(true),
				reactloops.WithAllowAIForge(false),
				reactloops.WithAllowPlanAndExec(false),
				reactloops.WithMaxIterations(10),
				reactloops.WithAllowUserInteract(r.GetConfig().GetAllowUserInteraction()),
				reactloops.WithPersistentInstruction(persistentInstruction),
				reactloops.WithReflectionOutputExample(outputExample),
				reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
					renderMap := map[string]any{
						"Nonce":            nonce,
						"UserInput":        loop.GetCurrentTask().GetUserInput(),
						"LastQuerySummary": loop.Get("last_query_summary"),
						"LastMatchSummary": loop.Get("last_match_summary"),
						"CurrentFlow":      loop.Get("current_flow"),
						"FeedbackMessages": feedbacker.String(),
					}
					return utils.RenderTemplate(reactiveData, renderMap)
				}),
				getHTTPFlowDetailAction(r),
				filterAndMatchHTTPFlowsAction(r),
				matchHTTPFlowsWithSimpleMatcherAction(r),
				BuildOnPostIterationHook(r),
			}
			preset = append(preset, opts...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_ACTION_HTTP_FLOW_ANALYZE, r, preset...)
		},
		reactloops.WithLoopDescription("Analyze captured HTTP flows from Yakit by querying, inspecting details, and applying matchers to highlight interesting traffic."),
		reactloops.WithLoopUsagePrompt("Use when you need to investigate HTTP traffic. Start with filter_and_match_http_flows to narrow data, use get_http_flow_detail for specific packets."),
	)
	if err != nil {
		log.Errorf("register reactloop: %v failed: %v", schema.AI_REACT_LOOP_ACTION_HTTP_FLOW_ANALYZE, err)
	}
}
