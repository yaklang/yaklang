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
var PLAN_PROMPT_KEY = "plan_prompt"
var PLAN_FILE_RESULTS_KEY = "plan_file_results"
var PLAN_WEB_RESULTS_KEY = "plan_web_results"

const PlanMaxIterations = 5

var infoGatheringActions = []string{
	"search_knowledge", "read_file", "find_files", "grep_text",
	"web_search", schema.AI_REACT_LOOP_ACTION_LOADING_SKILLS,
}

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
			allowedActions := []string{
				"plan", "search_knowledge",
				"read_file", "find_files", "grep_text",
				"web_search",
				schema.AI_REACT_LOOP_ACTION_LOADING_SKILLS,
			}
			if r.GetConfig().GetAllowUserInteraction() {
				allowedActions = append(allowedActions, schema.AI_REACT_LOOP_ACTION_ASK_FOR_CLARIFICATION)
			}
			preset := []reactloops.ReActLoopOption{
				reactloops.WithAllowRAG(false),
				reactloops.WithAllowToolCall(false),
				reactloops.WithAllowAIForge(false),
				reactloops.WithAllowPlanAndExec(false),
				reactloops.WithInitTask(buildPlanInitTask(r)),
				reactloops.WithMaxIterations(int(r.GetConfig().GetMaxIterationCount())),
				reactloops.WithMaxIterations(PlanMaxIterations),
				reactloops.WithAllowUserInteract(r.GetConfig().GetAllowUserInteraction()),
				reactloops.WithActionFilter(func(action *reactloops.LoopAction) bool {
					for _, name := range allowedActions {
						if action.ActionType == name {
							return true
						}
					}
					return false
				}),
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
					fileResults := loop.Get(PLAN_FILE_RESULTS_KEY)
					webResults := loop.Get(PLAN_WEB_RESULTS_KEY)
					currentIter := loop.GetCurrentIterationIndex()
					maxIter := loop.GetMaxIterations()
					isLastIteration := currentIter+1 >= maxIter
					if isLastIteration {
						for _, name := range infoGatheringActions {
							loop.RemoveAction(name)
						}
						log.Infof("plan loop: last iteration (%d/%d), removed all info-gathering actions, forcing plan output", currentIter+1, maxIter)
					}
					renderMap := map[string]any{
						"Plan":            currentPlan,
						"Help":            help,
						"Nonce":           nonce,
						"Enhance":         enhance,
						"FileResults":     fileResults,
						"WebResults":      webResults,
						"IsLastIteration": isLastIteration,
					}
					return utils.RenderTemplate(reactiveData, renderMap)
				}),
				generate(r),
				searchKnowledge(r),
				readFileAction(r),
				findFilesAction(r),
				grepTextAction(r),
				webSearchAction(r),
			}
			preset = append(opts, preset...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_PLAN, r, preset...)
		},
		reactloops.WithLoopDescription("Loop for generating and refining plans based on user requirements, with multi-source information gathering (knowledge base, file system, internet) and structured thinking frameworks (SMART, Six Thinking Hats, SWOT)."),
		reactloops.WithLoopUsagePrompt("when user needs to create or refine a plan for a specific task. Supports searching knowledge, reading local files, grepping code, internet search, and loading skills to produce comprehensive plans."),
		reactloops.WithLoopOutputExample(`
* When the user asks for a clear executable plan:
  {"@action": "plan", "human_readable_thought": "I should break down the goal into actionable subtasks and refine the plan with supporting knowledge"}
* When needing to understand project structure before planning:
  {"@action": "find_files", "dir": "/project/root", "pattern": "*.go"}
* When needing external best practices:
  {"@action": "web_search", "query": "best practices for microservice architecture"}
`),

		reactloops.WithVerboseName("Plan Builder"),
		reactloops.WithVerboseNameZh("任务规划"),
	)
	if err != nil {
		log.Errorf("register reactloop: %v failed", schema.AI_REACT_LOOP_NAME_PLAN)
	}
}
