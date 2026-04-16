package reactloops

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

type DeepScenarioResult struct {
	ScenarioSummary        string
	ScenarioAnalysis       string
	ToolSearchQueries      string
	KnowledgeSearchQueries string
	MemorySearchQueries    string
	ScenarioSearchQueries  string
	Tags                   string
	Questions              string
}

func ExecuteDeepScenarioRecognition(r aicommon.AIInvokeRuntime, loop *ReActLoop, task aicommon.AIStatefulTask) *DeepScenarioResult {
	if r == nil || task == nil {
		return nil
	}

	scenarioTask := aicommon.NewStatefulTaskBase(
		task.GetId()+"_scenario",
		task.GetUserInput(),
		r.GetConfig().GetContext(),
		r.GetConfig().GetEmitter(),
	)

	originOptions := r.GetConfig().OriginOptions()
	var opts []any
	for _, option := range originOptions {
		opts = append(opts, option)
	}

	var scenarioLoop *ReActLoop
	opts = append(opts, WithOnLoopInstanceCreated(func(l *ReActLoop) {
		scenarioLoop = l
		if loop != nil {
			l.Set("current_memories", utils.ShrinkString(loop.GetCurrentMemoriesContent(), 3000))
			l.Set("upstream_intent_analysis", loop.Get("intent_analysis"))
			l.Set("upstream_context_enrichment", loop.Get("intent_context_enrichment"))
			l.Set("upstream_recommended_tools", loop.Get("intent_recommended_tools"))
			l.Set("upstream_recommended_forges", loop.Get("intent_recommended_forges"))
		}
		if configWithTimeline, ok := r.GetConfig().(interface{ GetTimeline() *aicommon.Timeline }); ok {
			if timeline := configWithTimeline.GetTimeline(); timeline != nil {
				l.Set("timeline_snapshot", utils.ShrinkString(timeline.Dump(), 4000))
			}
		}
	}), WithNoEndLoadingStatus(true), WithUseSpeedPriorityAICallback(true))

	_, err := r.ExecuteLoopTaskIF(schema.AI_REACT_LOOP_NAME_SCENARIO, scenarioTask, opts...)
	if err != nil {
		log.Warnf("deep scenario recognition failed: %v", err)
		return nil
	}
	if scenarioLoop == nil {
		log.Warnf("deep scenario recognition: scenario loop reference is nil")
		return nil
	}

	result := &DeepScenarioResult{
		ScenarioSummary:        scenarioLoop.Get("scenario_summary"),
		ScenarioAnalysis:       scenarioLoop.Get("scenario_analysis"),
		ToolSearchQueries:      scenarioLoop.Get("scenario_tool_search_queries"),
		KnowledgeSearchQueries: scenarioLoop.Get("scenario_knowledge_search_queries"),
		MemorySearchQueries:    scenarioLoop.Get("scenario_memory_search_queries"),
		ScenarioSearchQueries:  scenarioLoop.Get("scenario_search_queries"),
		Tags:                   scenarioLoop.Get("task_retrieval_tags"),
		Questions:              scenarioLoop.Get("task_retrieval_questions"),
	}

	ApplyTaskRetrievalInfoToTask(task, result.Tags, result.Questions, scenarioLoop.Get("task_retrieval_target"))

	log.Infof("deep scenario recognition completed: summary=%d bytes, tool_queries=%d bytes, knowledge_queries=%d bytes, memory_queries=%d bytes",
		len(result.ScenarioSummary), len(result.ToolSearchQueries), len(result.KnowledgeSearchQueries), len(result.MemorySearchQueries))

	return result
}

func ApplyDeepScenarioResult(r aicommon.AIInvokeRuntime, loop *ReActLoop, result *DeepScenarioResult) {
	if r == nil || loop == nil || result == nil {
		return
	}

	if result.ScenarioSummary != "" {
		loop.Set("scenario_summary", result.ScenarioSummary)
		loop.Set("scenario_analysis", result.ScenarioAnalysis)
		r.AddToTimeline("scenario_analysis", "情景识别："+utils.ShrinkString(result.ScenarioSummary, 120))
	}
	if result.ToolSearchQueries != "" {
		loop.Set("scenario_tool_search_queries", result.ToolSearchQueries)
	}
	if result.KnowledgeSearchQueries != "" {
		loop.Set("scenario_knowledge_search_queries", result.KnowledgeSearchQueries)
	}
	if result.MemorySearchQueries != "" {
		loop.Set("scenario_memory_search_queries", result.MemorySearchQueries)
	}
	if result.ScenarioSearchQueries != "" {
		loop.Set("scenario_search_queries", result.ScenarioSearchQueries)
	}

	target := strings.TrimSpace(result.ScenarioSummary)
	if target == "" {
		target = strings.TrimSpace(result.ScenarioAnalysis)
	}
	ApplyTaskRetrievalInfoToTask(loop.GetCurrentTask(), result.Tags, result.Questions, target)
}
