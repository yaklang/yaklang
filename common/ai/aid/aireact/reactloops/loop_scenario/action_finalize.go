package loop_scenario

import (
	"fmt"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

var finalizeScenarioAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return makeFinalizeScenarioAction(r)
}

func makeFinalizeScenarioAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	desc := "识别当前情景并输出后续检索所需的搜索语句。/ Identify the current scenario and generate search queries for downstream retrieval."

	toolOpts := []aitool.ToolOption{
		aitool.WithStringParam("scenario_summary",
			aitool.WithParam_Description("简短情景标签，只描述当前任务所处情景。不要复述原请求，不要解释分析过程。/ Short scenario label describing the current operating scenario only."),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringArrayParamEx("tool_search_queries",
			[]aitool.PropertyOption{
				aitool.WithParam_Description("用于搜索相关工具/能力的搜索语句列表。/ Search queries for related tools or capabilities."),
			},
		),
		aitool.WithStringArrayParamEx("knowledge_search_queries",
			[]aitool.PropertyOption{
				aitool.WithParam_Description("用于搜索相关知识库/文档的搜索语句列表。/ Search queries for relevant knowledge bases or documents."),
			},
		),
		aitool.WithStringArrayParamEx("memory_search_queries",
			[]aitool.PropertyOption{
				aitool.WithParam_Description("用于搜索相关记忆的搜索语句列表。/ Search queries for relevant memories."),
			},
		),
		aitool.WithStringArrayParamEx("tags",
			[]aitool.PropertyOption{
				aitool.WithParam_Description("用于后续检索的标签列表。/ Tags for downstream retrieval."),
			},
		),
		aitool.WithStringArrayParamEx("questions",
			[]aitool.PropertyOption{
				aitool.WithParam_Description("用于后续检索的关键问题列表。/ Key questions for downstream retrieval."),
			},
		),
	}

	return reactloops.WithRegisterLoopActionWithStreamField(
		"finalize_scenario",
		desc,
		toolOpts,
		[]*reactloops.LoopStreamField{
			{AINodeId: "scenario", FieldName: "scenario_summary", StreamHandler: scenarioSummaryStreamHandler},
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			if strings.TrimSpace(action.GetString("scenario_summary")) == "" {
				return utils.Error("scenario_summary is required")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			scenarioSummary := compactScenarioSummary(action.GetString("scenario_summary"))

			toolQueries := normalizeScenarioItems(action.GetStringSlice("tool_search_queries"))
			knowledgeQueries := normalizeScenarioItems(action.GetStringSlice("knowledge_search_queries"))
			memoryQueries := normalizeScenarioItems(action.GetStringSlice("memory_search_queries"))
			tags := normalizeScenarioItems(action.GetStringSlice("tags"))
			questions := normalizeScenarioItems(action.GetStringSlice("questions"))

			if len(toolQueries) == 0 && len(knowledgeQueries) == 0 && len(memoryQueries) == 0 {
				toolQueries, knowledgeQueries, memoryQueries = buildFallbackScenarioQueries(loop, scenarioSummary)
			}

			mergedQueries := mergeScenarioSearchQueries(toolQueries, knowledgeQueries, memoryQueries)
			if len(questions) == 0 {
				questions = append([]string{}, mergedQueries...)
			}

			loop.Set("scenario_summary", scenarioSummary)
			loop.Set("scenario_analysis", scenarioSummary)
			loop.Set("scenario_tool_search_queries", strings.Join(toolQueries, "\n"))
			loop.Set("scenario_knowledge_search_queries", strings.Join(knowledgeQueries, "\n"))
			loop.Set("scenario_memory_search_queries", strings.Join(memoryQueries, "\n"))
			loop.Set("scenario_search_queries", strings.Join(mergedQueries, "\n"))
			loop.Set("task_retrieval_tags", strings.Join(tags, "\n"))
			loop.Set("task_retrieval_questions", strings.Join(questions, "\n"))
			loop.Set("task_retrieval_target", scenarioSummary)

			r.AddToTimeline("scenario_finalized", fmt.Sprintf("情景识别完成：%s", scenarioSummary))
			log.Infof("scenario loop finalized: summary=%s, tool_queries=%d, knowledge_queries=%d, memory_queries=%d",
				utils.ShrinkString(scenarioSummary, 120), len(toolQueries), len(knowledgeQueries), len(memoryQueries))
			op.Exit()
		},
	)
}

func compactScenarioSummary(summary string) string {
	summary = strings.TrimSpace(summary)
	summary = strings.TrimPrefix(summary, "当前情景：")
	summary = strings.TrimPrefix(summary, "情景：")
	summary = strings.TrimPrefix(summary, "Scenario:")
	summary = strings.TrimSpace(summary)
	if len([]rune(summary)) > 32 {
		summary = string([]rune(summary)[:32])
	}
	return strings.TrimSpace(summary)
}

func normalizeScenarioItems(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func mergeScenarioSearchQueries(groups ...[]string) []string {
	var merged []string
	for _, group := range groups {
		merged = append(merged, group...)
	}
	return normalizeScenarioItems(merged)
}

type scenarioValueGetter interface {
	Get(key string) string
}

func buildFallbackScenarioQueries(loop scenarioValueGetter, scenarioSummary string) ([]string, []string, []string) {
	userQuery := strings.TrimSpace(loop.Get("user_query"))
	intentAnalysis := strings.TrimSpace(loop.Get("upstream_intent_analysis"))

	base := normalizeScenarioItems([]string{
		scenarioSummary,
		userQuery,
		intentAnalysis,
	})
	if len(base) == 0 {
		base = []string{"当前任务相关情景"}
	}

	toolQueries := normalizeScenarioItems([]string{
		base[0] + " 工具",
		base[0] + " 能力",
		userQuery,
	})
	knowledgeQueries := normalizeScenarioItems([]string{
		base[0] + " 知识",
		base[0] + " 解决方案",
		userQuery,
	})
	memoryQueries := normalizeScenarioItems([]string{
		base[0],
		base[0] + " 经验",
		userQuery,
	})
	return toolQueries, knowledgeQueries, memoryQueries
}

func scenarioSummaryStreamHandler(fieldReader io.Reader, emitWriter io.Writer) {
	content, err := io.ReadAll(fieldReader)
	if err != nil {
		return
	}
	_, _ = emitWriter.Write([]byte(compactScenarioSummary(string(content))))
}
