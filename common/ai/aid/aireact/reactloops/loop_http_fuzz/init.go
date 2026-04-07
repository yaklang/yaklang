package loop_http_fuzz

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/persistent_instruction.txt
var instruction string

//go:embed prompts/reactive_data.txt
var reactiveData string

//go:embed prompts/reflection_output_example.txt
var outputExample string

const LoopHTTPFuzzName = "http_fuzz"

func init() {
	err := reactloops.RegisterLoopFactory(
		LoopHTTPFuzzName,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			preset := []reactloops.ReActLoopOption{
				reactloops.WithAllowRAG(false),
				reactloops.WithAllowToolCall(true),
				reactloops.WithAllowPlanAndExec(false),
				reactloops.WithMaxIterations(int(r.GetConfig().GetMaxIterationCount())),
				reactloops.WithAllowUserInteract(r.GetConfig().GetAllowUserInteraction()),
				reactloops.WithPersistentInstruction(instruction),
				reactloops.WithReflectionOutputExample(outputExample),
				reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
					renderMap := map[string]any{
						"UserGoal":               currentUserGoal(loop),
						"RequestProfile":         toPrettyJSON(loop.GetVariable(stateRequestProfile)),
						"ParameterInventory":     toPrettyJSON(loop.GetVariable(stateParameterInventory)),
						"HighValueTargets":       toPrettyJSON(loop.GetVariable(stateHighValueTargets)),
						"TestPlan":               toPrettyJSON(loop.GetVariable(stateTestPlan)),
						"BaselineFingerprint":    toPrettyJSON(loop.GetVariable(stateBaselineFingerprint)),
						"CoverageMap":            toPrettyJSON(loop.GetVariable(stateCoverageMap)),
						"LastMutation":           toPrettyJSON(loop.GetVariable(stateLastMutation)),
						"LastBatchResults":       toPrettyJSON(loop.GetVariable(stateLastBatchResults)),
						"AnomalyCandidates":      toPrettyJSON(loop.GetVariable(stateAnomalyCandidates)),
						"ConfirmedFindings":      toPrettyJSON(loop.GetVariable(stateConfirmedFindings)),
						"NextRecommendedActions": toPrettyJSON(loop.GetVariable(stateNextRecommendedActions)),
						"FeedbackMessages":       feedbacker.String(),
						"Nonce":                  nonce,
					}
					return utils.RenderTemplate(reactiveData, renderMap)
				}),
				loadHTTPRequestAction(r),
				inspectRequestSurfaceAction(r),
				mutateTargetAction(r),
				executeTestBatchAction(r),
				runGenericVulnTestAction(r),
				runWeakPasswordTestAction(r),
				runIdentifierEnumerationAction(r),
				runSensitiveInfoExposureTestAction(r),
				runEncodingBypassTestAction(r),
				commitFindingAction(r),
			}
			preset = append(preset, opts...)
			return reactloops.NewReActLoop(LoopHTTPFuzzName, r, preset...)
		},
		reactloops.WithLoopDescription("AI-guided HTTP manual fuzzing loop with request surface analysis, scenario planning, batch execution, and finding collection."),
		reactloops.WithLoopDescriptionZh("HTTP 手工发包安全测试模式：先分析请求面，再按场景执行 fuzz，沉淀异常与 finding。"),
		reactloops.WithVerboseName("HTTP Fuzz"),
		reactloops.WithVerboseNameZh("HTTP 发包测试"),
		reactloops.WithLoopUsagePrompt("Use when user wants to manually fuzz an HTTP request for security testing. Start with load_http_request, inspect_request_surface, then use mutate_target / execute_test_batch or scenario actions."),
		reactloops.WithLoopOutputExample(`
* When user requests focused HTTP security fuzzing:
  {"@action": "http_fuzz", "human_readable_thought": "先分析请求面并规划测试"}
`),
	)
	if err != nil {
		log.Errorf("register reactloop[%s] failed: %v", LoopHTTPFuzzName, err)
	}
}

func currentUserGoal(loop *reactloops.ReActLoop) string {
	if loop == nil {
		return ""
	}
	if goal := loop.Get(stateUserGoal); goal != "" {
		return goal
	}
	task := loop.GetCurrentTask()
	if task == nil {
		return ""
	}
	return task.GetUserInput()
}

func toPrettyJSON(v any) string {
	if utils.IsNil(v) {
		return "(empty)"
	}
	switch ret := v.(type) {
	case string:
		if ret == "" {
			return "(empty)"
		}
		return ret
	case fmt.Stringer:
		return ret.String()
	default:
		raw, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return utils.InterfaceToString(v)
		}
		return string(raw)
	}
}
