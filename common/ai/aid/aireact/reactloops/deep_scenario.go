package reactloops

import (
	_ "embed"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
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

type scenarioRecognitionPromptData struct {
	UserQuery                 string
	OriginUserInput           string
	TaskSummary               string
	TaskFocusMode             string
	TaskRetrievalTarget       string
	TaskRetrievalTags         string
	TaskRetrievalQuestions    string
	AttachedResources         string
	CurrentMemories           string
	TimelineSnapshot          string
	UserInputHistory          string
	UpstreamIntentAnalysis    string
	UpstreamContextEnrichment string
	UpstreamRecommendedTools  string
	UpstreamRecommendedForges string
	Language                  string
}

//go:embed prompts/scenario_recognition.txt
var scenarioRecognitionPromptTemplate string

func ExecuteDeepScenarioRecognition(r aicommon.AIInvokeRuntime, loop *ReActLoop, task aicommon.AIStatefulTask) *DeepScenarioResult {
	result, err := RecognizeScenarioViaLiteForge(r, loop, task)
	if err != nil {
		log.Warnf("deep scenario recognition failed: %v", err)
		return nil
	}
	if result == nil {
		return nil
	}
	ApplyTaskRetrievalInfoToTask(task, result.Tags, result.Questions, result.ScenarioSummary)
	log.Infof("deep scenario recognition completed: summary=%d bytes, tool_queries=%d bytes, knowledge_queries=%d bytes, memory_queries=%d bytes",
		len(result.ScenarioSummary), len(result.ToolSearchQueries), len(result.KnowledgeSearchQueries), len(result.MemorySearchQueries))
	return result
}

func RecognizeScenarioViaLiteForge(r aicommon.AIInvokeRuntime, loop *ReActLoop, task aicommon.AIStatefulTask) (*DeepScenarioResult, error) {
	if r == nil || task == nil {
		return nil, nil
	}

	prompt, err := BuildScenarioRecognitionPrompt(r, loop, task)
	if err != nil {
		return nil, err
	}

	action, err := r.InvokeSpeedPriorityLiteForge(
		task.GetContext(),
		"scenario-recognition",
		prompt,
		[]aitool.ToolOption{
			aitool.WithStringParam("scenario_summary",
				aitool.WithParam_Description("Short scenario label describing the current operating scenario only."),
				aitool.WithParam_Required(true),
			),
			aitool.WithStringArrayParamEx("tool_search_queries", []aitool.PropertyOption{
				aitool.WithParam_Description("Search queries for related tools, blueprints, skills, or focus modes."),
			}),
			aitool.WithStringArrayParamEx("knowledge_search_queries", []aitool.PropertyOption{
				aitool.WithParam_Description("Search queries for relevant knowledge bases, documents, methods, or solutions."),
			}),
			aitool.WithStringArrayParamEx("memory_search_queries", []aitool.PropertyOption{
				aitool.WithParam_Description("Search queries for relevant memories, prior experience, or similar cases."),
			}),
			aitool.WithStringArrayParamEx("tags", []aitool.PropertyOption{
				aitool.WithParam_Description("Tags for downstream retrieval."),
			}),
			aitool.WithStringArrayParamEx("questions", []aitool.PropertyOption{
				aitool.WithParam_Description("Key questions for downstream retrieval."),
			}),
		},
		aicommon.WithGeneralConfigStreamableFieldWithNodeId("scenario", "scenario_summary"),
	)
	if err != nil {
		return nil, err
	}
	if action == nil {
		return nil, nil
	}

	scenarioSummary := CompactScenarioSummary(action.GetString("scenario_summary"))
	toolQueries := NormalizeScenarioItems(action.GetStringSlice("tool_search_queries"))
	knowledgeQueries := NormalizeScenarioItems(action.GetStringSlice("knowledge_search_queries"))
	memoryQueries := NormalizeScenarioItems(action.GetStringSlice("memory_search_queries"))
	tags := NormalizeScenarioItems(action.GetStringSlice("tags"))
	questions := NormalizeScenarioItems(action.GetStringSlice("questions"))
	if len(toolQueries) == 0 && len(knowledgeQueries) == 0 && len(memoryQueries) == 0 {
		toolQueries, knowledgeQueries, memoryQueries = BuildFallbackScenarioQueries(&scenarioPromptValueGetter{
			values: map[string]string{
				"user_query":               task.GetUserInput(),
				"upstream_intent_analysis": "",
			},
		}, scenarioSummary)
	}
	mergedQueries := MergeScenarioSearchQueries(toolQueries, knowledgeQueries, memoryQueries)
	if len(questions) == 0 {
		questions = append([]string{}, mergedQueries...)
	}

	return &DeepScenarioResult{
		ScenarioSummary:        scenarioSummary,
		ScenarioAnalysis:       scenarioSummary,
		ToolSearchQueries:      strings.Join(toolQueries, "\n"),
		KnowledgeSearchQueries: strings.Join(knowledgeQueries, "\n"),
		MemorySearchQueries:    strings.Join(memoryQueries, "\n"),
		ScenarioSearchQueries:  strings.Join(mergedQueries, "\n"),
		Tags:                   strings.Join(tags, "\n"),
		Questions:              strings.Join(questions, "\n"),
	}, nil
}

func BuildScenarioRecognitionPrompt(r aicommon.AIInvokeRuntime, loop *ReActLoop, task aicommon.AIStatefulTask) (string, error) {
	data := scenarioRecognitionPromptData{
		UserQuery:       task.GetUserInput(),
		OriginUserInput: task.GetOriginUserInput(),
		TaskSummary:     task.GetSummary(),
		TaskFocusMode:   task.GetFocusMode(),
		Language:        getScenarioLanguage(r),
	}
	if info := task.GetTaskRetrievalInfo(); info != nil {
		data.TaskRetrievalTarget = strings.TrimSpace(info.Target)
		data.TaskRetrievalTags = strings.Join(NormalizeScenarioItems(info.Tags), "\n")
		data.TaskRetrievalQuestions = strings.Join(NormalizeScenarioItems(info.Questions), "\n")
	}
	data.AttachedResources = formatScenarioAttachedResources(task.GetAttachedDatas())
	if loop != nil {
		data.CurrentMemories = utils.ShrinkString(loop.GetCurrentMemoriesContent(), 3000)
		data.UpstreamIntentAnalysis = loop.Get("intent_analysis")
		data.UpstreamContextEnrichment = loop.Get("intent_context_enrichment")
		data.UpstreamRecommendedTools = loop.Get("intent_recommended_tools")
		data.UpstreamRecommendedForges = loop.Get("intent_recommended_forges")
	}
	if formatter, ok := r.GetConfig().(interface{ FormatUserInputHistory() string }); ok {
		data.UserInputHistory = utils.ShrinkString(formatter.FormatUserInputHistory(), 2500)
	}
	if timelineGetter, ok := r.GetConfig().(interface{ GetTimeline() *aicommon.Timeline }); ok {
		if timeline := timelineGetter.GetTimeline(); timeline != nil {
			data.TimelineSnapshot = utils.ShrinkString(timeline.Dump(), 4000)
		}
	}
	return utils.RenderTemplate(scenarioRecognitionPromptTemplate, data)
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

func getScenarioLanguage(r aicommon.AIInvokeRuntime) string {
	if r == nil || r.GetConfig() == nil {
		return "zh"
	}
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

func formatScenarioAttachedResources(resources []*aicommon.AttachedResource) string {
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

type scenarioValueGetter interface {
	Get(key string) string
}

type scenarioPromptValueGetter struct {
	values map[string]string
}

func (s *scenarioPromptValueGetter) Get(key string) string {
	if s == nil {
		return ""
	}
	return s.values[key]
}

func CompactScenarioSummary(summary string) string {
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

func NormalizeScenarioItems(values []string) []string {
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

func MergeScenarioSearchQueries(groups ...[]string) []string {
	var merged []string
	for _, group := range groups {
		merged = append(merged, group...)
	}
	return NormalizeScenarioItems(merged)
}

func BuildFallbackScenarioQueries(loop scenarioValueGetter, scenarioSummary string) ([]string, []string, []string) {
	userQuery := strings.TrimSpace(loop.Get("user_query"))
	intentAnalysis := strings.TrimSpace(loop.Get("upstream_intent_analysis"))
	base := NormalizeScenarioItems([]string{
		scenarioSummary,
		userQuery,
		intentAnalysis,
	})
	if len(base) == 0 {
		base = []string{"当前任务相关情景"}
	}
	toolQueries := NormalizeScenarioItems([]string{
		base[0] + " 工具",
		base[0] + " 能力",
		userQuery,
	})
	knowledgeQueries := NormalizeScenarioItems([]string{
		base[0] + " 知识",
		base[0] + " 解决方案",
		userQuery,
	})
	memoryQueries := NormalizeScenarioItems([]string{
		base[0],
		base[0] + " 经验",
		userQuery,
	})
	return toolQueries, knowledgeQueries, memoryQueries
}
