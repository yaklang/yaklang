package loop_plan

import (
	"bytes"
	_ "embed"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func buildPlanInitTask(r aicommon.AIInvokeRuntime) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
		config := r.GetConfig()

		if config.GetConfigBool("DisableIntentRecognition") {
			log.Infof("plan: intent recognition disabled via config, skipping")
			return
		}

		loop.LoadingStatus("深度意图识别 / Deep intent recognition")
		log.Infof("plan: invoking deep intent recognition directly")

		deepResult := reactloops.ExecuteDeepIntentRecognition(r, loop, task)
		if deepResult != nil {
			reactloops.ApplyDeepIntentResult(r, loop, deepResult)
		} else {
			log.Infof("plan: deep intent recognition returned no result, proceeding normally")
		}
	}
}

var PLAN_DATA_KEY = "plan_data"
var PLAN_HELP_KEY = "plan_help"
var PLAN_ENHANCE_KEY = "plan_enhance"
var PLAN_ENHANCE_COUNT = "plan_enhance_count"
var PLAN_PROMPT_KEY = "plan_prompt" // Additional context for plan phase only

//go:embed prompts/output_example.txt
var outputExample string

//go:embed prompts/reactive_data.txt
var reactiveData string

//go:embed prompts/persistent_instruction.txt
var persistentInstruction string

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_PLAN,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			help := r.GetConfig().GetConfigString(PLAN_HELP_KEY)
			planPrompt := r.GetConfig().GetConfigString(PLAN_PROMPT_KEY)
		preset := []reactloops.ReActLoopOption{
			reactloops.WithAllowRAG(false),
			reactloops.WithAllowToolCall(false),
			reactloops.WithAllowAIForge(false),
			reactloops.WithAllowPlanAndExec(false),
			reactloops.WithInitTask(buildPlanInitTask(r)),
			reactloops.WithMaxIterations(int(r.GetConfig().GetMaxIterationCount())),
				reactloops.WithAllowUserInteract(r.GetConfig().GetAllowUserInteraction()),
				reactloops.WithPersistentContextProvider(func(loop *reactloops.ReActLoop, nonce string) (string, error) {
					return utils.RenderTemplate(persistentInstruction, map[string]any{
						"Nonce":      nonce,
						"UserInput":  loop.GetCurrentTask().GetUserInput(),
						"PlanPrompt": planPrompt,
					})
				}),
				reactloops.WithReflectionOutputExample(outputExample),
				reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
					currentPlan := loop.Get(PLAN_DATA_KEY)
					enhance := loop.Get(PLAN_ENHANCE_KEY)
					renderMap := map[string]any{
						"Plan":    currentPlan,
						"Help":    help,
						"Nonce":   nonce,
						"Enhance": enhance,
					}
					return utils.RenderTemplate(reactiveData, renderMap)
				}),
				generate(r),
				searchKnowledge(r),
			}
			preset = append(opts, preset...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_PLAN, r, preset...)
		},
		// Register metadata for better AI understanding
		reactloops.WithLoopDescription("Loop for generating and refining plans based on user requirements, with knowledge enhancement."),
		reactloops.WithLoopUsagePrompt("when user needs to create or refine a plan for a specific task, if need to search knowledge to enhance the plan, use search_knowledge action to get relevant information."),
	)
	if err != nil {
		log.Errorf("register reactloop: %v failed", schema.AI_REACT_LOOP_NAME_PLAN)
	}
}
