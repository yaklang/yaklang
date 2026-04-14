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
var PLAN_RECON_RESULTS_KEY = "plan_recon_results"
var PLAN_FACTS_KEY = "plan_facts"
var PLAN_EVIDENCE_KEY = "plan_evidence"
var PLAN_DOCUMENT_KEY = "plan_document"

const PlanMaxIterations = 4

var infoGatheringActions = []string{
	"search_knowledge", "read_file", "find_files", "grep_text",
	"web_search", "scan_port", "simple_crawler",
	schema.AI_REACT_LOOP_ACTION_LOADING_SKILLS,
}

//go:embed prompts/output_example.txt
var outputExample string

//go:embed prompts/reactive_data.txt
var reactiveData string

//go:embed prompts/persistent_instruction.txt
var persistentInstruction string

//go:embed prompts/guidance_document.txt
var guidanceDocumentPrompt string

//go:embed prompts/plan_from_document.txt
var planFromDocumentPrompt string

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_PLAN,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			help := r.GetConfig().GetConfigString(PLAN_HELP_KEY)
			planPrompt := r.GetConfig().GetConfigString(PLAN_PROMPT_KEY)
			allowedActions := []string{
				"finish_exploration", "search_knowledge",
				"read_file", "find_files", "grep_text",
				"web_search", "scan_port", "simple_crawler",
				"output_facts",
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
				reactloops.WithAITagFieldWithAINodeId(PlanFactsAITagName, PlanFactsFieldName, PlanFactsAINodeID, aicommon.TypeTextMarkdown),
				reactloops.WithInitTask(buildPlanInitTask(r)),
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
					enhance := loop.Get(PLAN_ENHANCE_KEY)
					fileResults := loop.Get(PLAN_FILE_RESULTS_KEY)
					webResults := loop.Get(PLAN_WEB_RESULTS_KEY)
					reconResults := loop.Get(PLAN_RECON_RESULTS_KEY)
					currentIter := loop.GetCurrentIterationIndex()
					maxIter := loop.GetMaxIterations()
					isLastIteration := currentIter+1 >= maxIter
					if isLastIteration {
						for _, name := range infoGatheringActions {
							loop.RemoveAction(name)
						}
						log.Infof("plan loop: last iteration (%d/%d), removed all info-gathering actions, forcing finish_exploration", currentIter+1, maxIter)
					}
					renderMap := map[string]any{
						"Help":            help,
						"Nonce":           nonce,
						"Enhance":         enhance,
						"FileResults":     fileResults,
						"WebResults":      webResults,
						"ReconResults":    reconResults,
						"Facts":           loop.Get(PLAN_FACTS_KEY),
						"Evidence":        getLoopTaskEvidenceDocument(loop),
						"IsLastIteration": isLastIteration,
					}
					return utils.RenderTemplate(reactiveData, renderMap)
				}),
				buildPlanPostIterationHook(r),
				finishExploration(r),
				outputFactsAction(r),
				searchKnowledge(r),
				readFileAction(r),
				findFilesAction(r),
				grepTextAction(r),
				webSearchAction(r),
				scanPortAction(r),
				simpleCrawlerAction(r),
			}
			preset = append(opts, preset...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_PLAN, r, preset...)
		},
		reactloops.WithLoopDescription("Loop for generating and refining plans based on user requirements, with multi-source information gathering (knowledge base, file system, internet) and structured thinking frameworks (SMART, Six Thinking Hats, SWOT)."),
		reactloops.WithLoopDescriptionZh("任务规划模式：围绕用户目标生成和细化执行计划，结合知识库、文件系统和互联网信息完成规划。"),
		reactloops.WithLoopUsagePrompt("when user needs to create or refine a plan for a specific task. Supports searching knowledge, reading local files, grepping code, internet search, port scanning, web crawling, and loading skills to produce comprehensive plans."),
		reactloops.WithLoopOutputExample(`
* When you have gathered enough information and are ready to finalize:
  {"@action": "finish_exploration", "human_readable_thought": "I have collected sufficient facts and evidence to generate a comprehensive guidance document and plan"}
* When needing to understand project structure before planning:
  {"@action": "find_files", "dir": "/project/root", "pattern": "*.go"}
* When needing external best practices:
  {"@action": "web_search", "query": "best practices for microservice architecture"}
* When needing to discover open ports and services on a target:
  {"@action": "scan_port", "request": "scan 192.168.1.1 for common web ports 80,443,8080-8090"}
* When needing to map web application attack surface:
  {"@action": "simple_crawler", "request": "crawl https://target.example.com with depth 2 to discover pages and endpoints"}
`),

		reactloops.WithVerboseName("Plan Builder"),
		reactloops.WithVerboseNameZh("任务规划"),
	)
	if err != nil {
		log.Errorf("register reactloop: %v failed", schema.AI_REACT_LOOP_NAME_PLAN)
	}
}
