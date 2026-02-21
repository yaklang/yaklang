package loop_internet_research

import (
	"bytes"
	_ "embed"
	"fmt"
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
		schema.AI_REACT_LOOP_NAME_INTERNET_RESEARCH,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			preset := []reactloops.ReActLoopOption{
				reactloops.WithAllowRAG(false),
				reactloops.WithAllowAIForge(false),
				reactloops.WithAllowPlanAndExec(false),
				reactloops.WithAllowToolCall(false),
				reactloops.WithInitTask(buildInitTask(r)),
				reactloops.WithMaxIterations(int(r.GetConfig().GetMaxIterationCount())),
				reactloops.WithAllowUserInteract(r.GetConfig().GetAllowUserInteraction()),
				reactloops.WithPersistentInstruction(instruction),
				reactloops.WithReflectionOutputExample(outputExample),
				reactloops.WithMaxIterations(5),
				reactloops.WithActionFilter(func(action *reactloops.LoopAction) bool {
					allowActionNames := []string{
						"web_search",
						"read_url",
						"final_summary",
					}
					for _, actionName := range allowActionNames {
						if action.ActionType == actionName {
							return true
						}
					}
					return false
				}),
				reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
					userQuery := loop.Get("user_query")
					searchResults := loop.Get("search_results_summary")
					searchHistory := loop.Get("search_history")
					nextMovementsSummary := loop.Get("next_movements_summary")
					artifactsSummary := buildArtifactsSummary(loop)

					renderMap := map[string]any{
						"UserQuery":            userQuery,
						"SearchResults":        searchResults,
						"SearchHistory":        searchHistory,
						"NextMovementsSummary": nextMovementsSummary,
						"ArtifactsSummary":     artifactsSummary,
						"Nonce":                nonce,
					}
					return utils.RenderTemplate(reactiveData, renderMap)
				}),
				webSearchAction(r),
				readURLAction(r),
				finalSummaryAction(r),
				BuildOnPostIterationHook(r),
			}
			preset = append(opts, preset...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_INTERNET_RESEARCH, r, preset...)
		},
		reactloops.WithLoopDescription("互联网调研模式：通过互联网搜索引擎收集、阅读、分析和整合信息，生成全面的调研报告。"),
		reactloops.WithLoopUsagePrompt(`当用户需要从互联网获取实时信息、调研某个主题、或验证某些事实时使用此流程。
AI会通过多轮搜索和页面阅读，从互联网收集相关信息并生成调研报告。`),
		reactloops.WithLoopOutputExample(`
* 当需要从互联网搜索信息时：
  {"@action": "internet_research", "human_readable_thought": "需要从互联网搜索和收集与用户问题相关的最新信息"}
`),
		reactloops.WithLoopIsHidden(false),
	)
	if err != nil {
		log.Errorf("register reactloop: %v failed: %v", schema.AI_REACT_LOOP_NAME_INTERNET_RESEARCH, err)
	}
}

func buildArtifactsSummary(loop *reactloops.ReActLoop) string {
	var artifacts []string
	maxIterations := loop.GetCurrentIterationIndex()
	if maxIterations <= 0 {
		maxIterations = 5
	}

	for iteration := 1; iteration <= maxIterations+1; iteration++ {
		for queryIdx := 1; queryIdx <= 20; queryIdx++ {
			artifactFile := loop.Get(fmt.Sprintf("artifact_round_%d_%d", iteration, queryIdx))
			if artifactFile != "" {
				artifacts = append(artifacts, artifactFile)
			}
		}
	}

	if len(artifacts) == 0 {
		return ""
	}

	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("已保存 %d 个调研结果文件：\n", len(artifacts)))
	for i, filename := range artifacts {
		summary.WriteString(fmt.Sprintf("  %d. %s\n", i+1, filename))
	}
	return summary.String()
}

func buildInitTask(r aicommon.AIInvokeRuntime) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
		userQuery := task.GetUserInput()

		loop.Set("user_query", userQuery)
		loop.Set("search_results_summary", "")
		loop.Set("search_history", "")
		loop.Set("search_count", "0")

		r.AddToTimeline("task_initialized", fmt.Sprintf("Internet research task initialized: %s", userQuery))
		operator.Continue()
	}
}
