package loop_scenario

import (
	"bytes"
	_ "embed"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/persistent_instruction.txt
var instruction string

//go:embed prompts/output_example.txt
var outputExample string

//go:embed prompts/reactive_data.txt
var reactiveData string

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_SCENARIO,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			preset := []reactloops.ReActLoopOption{
				reactloops.WithAllowRAG(false),
				reactloops.WithAllowAIForge(false),
				reactloops.WithAllowPlanAndExec(false),
				reactloops.WithAllowToolCall(false),
				reactloops.WithAllowUserInteract(false),
				reactloops.WithUseSpeedPriorityAICallback(),
				reactloops.WithInitTask(buildInitTask(r)),
				reactloops.WithMaxIterations(1),
				reactloops.WithPersistentInstruction(instruction),
				reactloops.WithReflectionOutputExample(outputExample),
				reactloops.WithActionFilter(func(action *reactloops.LoopAction) bool {
					return action != nil && action.ActionType == "finalize_scenario"
				}),
				reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
					renderMap := map[string]any{
						"UserQuery":                 loop.Get("user_query"),
						"OriginUserInput":           loop.Get("origin_user_input"),
						"TaskSummary":               loop.Get("task_summary"),
						"TaskFocusMode":             loop.Get("task_focus_mode"),
						"TaskRetrievalTarget":       loop.Get("task_retrieval_target"),
						"TaskRetrievalTags":         loop.Get("task_retrieval_tags"),
						"TaskRetrievalQuestions":    loop.Get("task_retrieval_questions"),
						"AttachedResources":         loop.Get("attached_resources"),
						"CurrentMemories":           loop.Get("current_memories"),
						"TimelineSnapshot":          loop.Get("timeline_snapshot"),
						"UserInputHistory":          loop.Get("user_input_history"),
						"UpstreamIntentAnalysis":    loop.Get("upstream_intent_analysis"),
						"UpstreamContextEnrichment": loop.Get("upstream_context_enrichment"),
						"UpstreamRecommendedTools":  loop.Get("upstream_recommended_tools"),
						"UpstreamRecommendedForges": loop.Get("upstream_recommended_forges"),
						"ScenarioAnalysis":          loop.Get("scenario_analysis"),
						"Language":                  loop.Get("language"),
						"Nonce":                     nonce,
					}
					return utils.RenderTemplate(reactiveData, renderMap)
				}),
				finalizeScenarioAction(r),
			}
			preset = append(opts, preset...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_SCENARIO, r, preset...)
		},
		reactloops.WithLoopDescription("Scenario recognition mode: infer the current operating scenario from task context, memories, timeline, and prior intent signals, then produce search queries for tools, knowledge, and memories."),
		reactloops.WithLoopDescriptionZh("情景识别模式：从任务上下文、当前记忆、时间线和既有意图信号中识别当前情景，并生成用于工具、知识和记忆检索的搜索语句。"),
		reactloops.WithLoopUsagePrompt("Used internally when the system needs a compact scenario understanding plus multiple retrieval queries before searching tools, knowledge bases, or memories."),
		reactloops.WithLoopOutputExample(`
* When internal pre-routing needs scenario recognition:
  {"@action": "scenario", "human_readable_thought": "I should identify the current scenario and generate retrieval queries for tools, knowledge, and memories"}
`),
		reactloops.WithLoopIsHidden(true),
		reactloops.WithVerboseName("Scenario Recognition"),
		reactloops.WithVerboseNameZh("情景识别（内部）"),
	)
	if err != nil {
		log.Errorf("register reactloop %s failed: %v", schema.AI_REACT_LOOP_NAME_SCENARIO, err)
	}
}

func buildInitTask(r aicommon.AIInvokeRuntime) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
		if loop == nil || task == nil {
			operator.Failed(utils.Error("scenario init requires loop and task"))
			return
		}

		loop.Set("language", getLanguageFromConfig(r))
		loop.Set("user_query", task.GetUserInput())
		loop.Set("origin_user_input", task.GetOriginUserInput())
		loop.Set("task_summary", task.GetSummary())
		loop.Set("task_focus_mode", task.GetFocusMode())
		loop.Set("attached_resources", formatAttachedResources(task.GetAttachedDatas()))
		loop.Set("scenario_analysis", "")

		if info := task.GetTaskRetrievalInfo(); info != nil {
			loop.Set("task_retrieval_target", strings.TrimSpace(info.Target))
			loop.Set("task_retrieval_tags", strings.Join(normalizeScenarioItems(info.Tags), "\n"))
			loop.Set("task_retrieval_questions", strings.Join(normalizeScenarioItems(info.Questions), "\n"))
		}

		if formatter, ok := r.GetConfig().(interface{ FormatUserInputHistory() string }); ok {
			loop.Set("user_input_history", utils.ShrinkString(formatter.FormatUserInputHistory(), 2500))
		}
		if timelineGetter, ok := r.GetConfig().(interface{ GetTimeline() *aicommon.Timeline }); ok {
			if timeline := timelineGetter.GetTimeline(); timeline != nil && loop.Get("timeline_snapshot") == "" {
				loop.Set("timeline_snapshot", utils.ShrinkString(timeline.Dump(), 4000))
			}
		}

		r.AddToTimeline("scenario_init", "Scenario recognition loop initialized")
		operator.Continue()
	}
}

func getLanguageFromConfig(r aicommon.AIInvokeRuntime) string {
	config := r.GetConfig()
	if langGetter, ok := config.(interface{ GetLanguage() string }); ok {
		if lang := langGetter.GetLanguage(); lang != "" {
			return lang
		}
	}
	if lang := config.GetConfigString("language"); lang != "" {
		return lang
	}
	return "zh"
}

func formatAttachedResources(resources []*aicommon.AttachedResource) string {
	if len(resources) == 0 {
		return ""
	}
	var buf strings.Builder
	for _, resource := range resources {
		if resource == nil {
			continue
		}
		buf.WriteString("- ")
		buf.WriteString(strings.TrimSpace(resource.Type))
		if resource.Key != "" {
			buf.WriteString(" [")
			buf.WriteString(strings.TrimSpace(resource.Key))
			buf.WriteString("]")
		}
		if resource.Value != "" {
			buf.WriteString(": ")
			buf.WriteString(strings.TrimSpace(resource.Value))
		}
		buf.WriteString("\n")
	}
	return strings.TrimSpace(buf.String())
}
