package loop_explore_filesystem

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// buildInitTask creates the initialization task handler for filesystem exploration
func buildInitTask(r aicommon.AIInvokeRuntime) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) error {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) error {
		emitter := r.GetConfig().GetEmitter()

		log.Infof("explore_filesystem init: analyzing user requirements")

		// Use LiteForge to analyze user requirements and extract exploration parameters
		promptTemplate := `
你的目标是分析用户需求，提取代码探索的关键信息：

【任务1：识别探索目标路径】
从用户输入中识别需要探索的目录或文件路径。
- 如果用户提到具体路径（如 "/path/to/project"），提取该路径
- 如果没有明确路径，使用当前工作目录

【任务2：明确探索目标】
分析用户想要了解什么：
- 找到特定函数/类/接口的实现
- 理解代码调用关系
- 分析项目结构
- 查找特定模式或关键词

【任务3：生成初始搜索模式】
根据用户需求，生成 2-4 个初始搜索模式（search_patterns），用于首次探索：
- 函数名模式：如 "func.*HandleRequest"
- 关键词模式：如 "toolcall|tool_call"
- 结构模式：如 "type.*struct"

<|USER_INPUT_{{ .nonce }}|>
{{ .data }}
<|USER_INPUT_END_{{ .nonce }}|>
`

		renderedPrompt := utils.MustRenderTemplate(
			promptTemplate,
			map[string]any{
				"nonce": utils.RandStringBytes(4),
				"data":  task.GetUserInput(),
			})

		initResult, err := r.InvokeLiteForge(
			task.GetContext(),
			"analyze-exploration-requirements",
			renderedPrompt,
			[]aitool.ToolOption{
				aitool.WithStringParam("target_path",
					aitool.WithParam_Description("The filesystem path to explore (directory or file)"),
					aitool.WithParam_Required(true)),
				aitool.WithStringParam("exploration_goal",
					aitool.WithParam_Description("A clear description of what the user wants to find or understand"),
					aitool.WithParam_Required(true)),
				aitool.WithStringArrayParam("search_patterns",
					aitool.WithParam_Description("2-4 initial search patterns for grep exploration"),
					aitool.WithParam_Required(true)),
				aitool.WithStringParam("reason",
					aitool.WithParam_Description("Explain your analysis of the user's exploration requirements"),
					aitool.WithParam_Required(true)),
			},
			aicommon.WithGeneralConfigStreamableFieldWithNodeId("init-explore-filesystem", "reason"),
		)

		if err != nil {
			log.Errorf("failed to invoke liteforge for exploration init: %v", err)
			return utils.Errorf("failed to analyze exploration requirements: %v", err)
		}

		targetPath := initResult.GetString("target_path")
		explorationGoal := initResult.GetString("exploration_goal")
		searchPatterns := initResult.GetStringSlice("search_patterns")
		reason := initResult.GetString("reason")

		log.Infof("explore_filesystem init: target_path=%s, goal=%s, patterns=%v",
			targetPath, explorationGoal, searchPatterns)

		// Store exploration context in loop state
		loop.Set("target_path", targetPath)
		loop.Set("exploration_goal", explorationGoal)
		loop.Set("exploration_findings", "") // Will be populated during exploration
		loop.Set("initial_patterns", strings.Join(searchPatterns, ","))

		// Set output directory for artifacts from config
		outputDir := r.GetConfig().GetConfigString("explore_output_directory")
		if outputDir == "" {
			// Default to target_path/.explore_results if not configured
			outputDir = targetPath + "/.explore_results"
		}
		loop.Set("output_directory", outputDir)
		log.Infof("explore_filesystem: artifact output directory set to: %s", outputDir)

		// Emit exploration context
		emitter.EmitThoughtStream(task.GetIndex(), "Exploration initialized:\n"+
			"- Target Path: "+targetPath+"\n"+
			"- Goal: "+explorationGoal+"\n"+
			"- Initial Patterns: "+strings.Join(searchPatterns, ", ")+"\n"+
			"- Analysis: "+reason)

		r.AddToTimeline("exploration_init", utils.MustRenderTemplate(`
Exploration Context Initialized:
- Target Path: {{ .targetPath }}
- Goal: {{ .goal }}
- Initial Patterns: {{ .patterns }}
- Analysis: {{ .reason }}
`, map[string]any{
			"targetPath": targetPath,
			"goal":       explorationGoal,
			"patterns":   strings.Join(searchPatterns, ", "),
			"reason":     reason,
		}))

		return nil
	}
}
