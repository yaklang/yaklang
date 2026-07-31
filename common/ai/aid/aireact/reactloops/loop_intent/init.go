package loop_intent

import (
	_ "embed"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

//go:embed prompts/persistent_instruction.txt
var instruction string

//go:embed prompts/output_example.txt
var outputExample string

//go:embed prompts/reactive_data.txt
var reactiveData string

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_INTENT,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			preset := []reactloops.ReActLoopOption{
				reactloops.WithDisableTodoSnapshot(true),
				reactloops.WithDisableLoopPerception(true),
				reactloops.WithAllowRAG(false),
				reactloops.WithAllowAIForge(false),
				reactloops.WithAllowPlanAndExec(false),
				reactloops.WithAllowToolCall(false),
				reactloops.WithAllowUserInteract(false),
				reactloops.WithUseSpeedPriorityAICallback(true),
				reactloops.WithInitTask(buildInitTask(r)),
				reactloops.WithMaxIterations(0), // init-only loop: no ReAct iterations needed
				reactloops.WithDisableIncreaseIteration(true),
				reactloops.WithPersistentInstruction(instruction),
				reactloops.WithReflectionOutputExample(outputExample),
			}
			preset = append(opts, preset...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_INTENT, r, preset...)
		},
		reactloops.WithLoopDescription("Intent recognition and context enrichment mode: analyzes user input to identify intent, search for relevant tools/forges/skills, and produce context enrichment for the main loop."),
		reactloops.WithLoopDescriptionZh("意图识别与上下文增强模式：分析用户输入、检索相关工具、蓝图和技能，并为主循环补充上下文。"),
		reactloops.WithLoopUsagePrompt("Used internally when user input is medium-to-large scale and requires deep intent decomposition and capability matching before the main loop can proceed effectively."),
		reactloops.WithLoopOutputExample(`
* When internal pre-routing needs deep intent decomposition:
  {"@action": "intent", "human_readable_thought": "I should analyze user intent deeply and return structured capability recommendations for the main loop"}
`),
		reactloops.WithLoopIsHidden(true),
		reactloops.WithVerboseName("Intent Recognition"),
		reactloops.WithVerboseNameZh("意图识别（内部）"),
	)
	if err != nil {
		log.Errorf("register reactloop %s failed: %v", schema.AI_REACT_LOOP_NAME_INTENT, err)
	}
}

// getLanguageFromConfig reads language preference from AICallerConfigIf.
// Default is "zh" (Chinese) if not set or not accessible.
func getLanguageFromConfig(r aicommon.AIInvokeRuntime) string {
	config := r.GetConfig()
	// Try to get language via GetLanguage() if the concrete type supports it
	if langGetter, ok := config.(interface{ GetLanguage() string }); ok {
		if lang := langGetter.GetLanguage(); lang != "" {
			return lang
		}
	}
	// Fallback: check KeyValueConfig
	if lang := config.GetConfigString("language"); lang != "" {
		return lang
	}
	return "zh"
}

// buildInitTask creates the init handler for the intent recognition loop.
//
// The simplified loop_intent runs entirely in InitTask:
//  1. Single AI call to generate intent_summary + search_keywords.
//  2. Local BM25 capability search (tools, forges, skills, focus modes) — no AI.
//  3. Conditional second AI call for capability recommendation (only when matched results > threshold).
//  4. Set loop variables and exit via op.Done().
func buildInitTask(r aicommon.AIInvokeRuntime) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
		userQuery := task.GetUserInput()

		// Store user query and language in loop context
		loop.Set("user_query", userQuery)
		loop.Set("language", getLanguageFromConfig(r))
		loop.Set("search_results", "")
		loop.Set("intent_analysis", "")
		loop.Set("recommended_tools", "")
		loop.Set("recommended_forges", "")
		loop.Set("context_enrichment", "")

		r.AddToTimeline("intent_init", "Intent recognition loop initialized for deep analysis")
		log.Infof("intent recognition loop initialized for query: %s", userQuery)

		// Run the full intent recognition pipeline (1-2 AI calls max)
		runIntentRecognition(r, loop)

		operator.Done()
	}
}
