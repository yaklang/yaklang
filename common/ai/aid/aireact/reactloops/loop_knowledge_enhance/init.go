package loop_knowledge_enhance

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
		schema.AI_REACT_LOOP_NAME_KNOWLEDGE_ENHANCE,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			preset := []reactloops.ReActLoopOption{
				reactloops.WithAllowRAG(true),
				reactloops.WithAllowToolCall(true),
				reactloops.WithInitTask(buildInitTask(r)),
				reactloops.WithMaxIterations(int(r.GetConfig().GetMaxIterationCount())),
				reactloops.WithAllowUserInteract(r.GetConfig().GetAllowUserInteraction()),
				reactloops.WithPersistentInstruction(instruction),
				reactloops.WithReflectionOutputExample(outputExample),
				reactloops.WithMaxIterations(3),
				reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
					userQuery := loop.Get("user_query")
					attachedResources := loop.Get("attached_resources")
					searchResults := loop.Get("search_results")
					searchHistory := loop.Get("search_history")

					feedbacks := feedbacker.String()

					renderMap := map[string]any{
						"UserQuery":         userQuery,
						"AttachedResources": attachedResources,
						"SearchResults":     searchResults,
						"SearchHistory":     searchHistory,
						"FeedbackMessages":  feedbacks,
						"Nonce":             nonce,
					}
					return utils.RenderTemplate(reactiveData, renderMap)
				}),
				// Register actions
				searchKnowledgeAction(r),
			}
			preset = append(preset, opts...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_KNOWLEDGE_ENHANCE, r, preset...)
		},
		// Register metadata for better AI understanding
		reactloops.WithLoopDescription("附加资源信息收集模式：根据用户问题从附加的资源（知识库、文件、AI工具、AI蓝图）中收集相关信息，用于后续回答。"),
		reactloops.WithLoopUsagePrompt(`当用户附加了资源（知识库、文件等）时使用此流程收集信息。
AI会根据用户问题从附加资源中尽可能多地收集相关信息，这些信息将用于后续的回答环节。`),
		reactloops.WithLoopOutputExample(`
* 当需要从附加资源中收集信息时：
  {"@action": "knowledge_enhance", "human_readable_thought": "需要从用户附加的资源中收集与问题相关的信息"}
`),
	)
	if err != nil {
		log.Errorf("register reactloop: %v failed: %v", schema.AI_REACT_LOOP_NAME_KNOWLEDGE_ENHANCE, err)
	}
}

// DefaultKnowledgeSampleCount 默认获取的知识库样本数量
const DefaultKnowledgeSampleCount = 10

// buildInitTask creates the initial task for the knowledge enhance loop
func buildInitTask(r aicommon.AIInvokeRuntime) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) error {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) error {
		// Get user input from task
		userQuery := task.GetUserInput()

		// Get attached resources from task
		attachedDatas := task.GetAttachedDatas()

		// Parse and format attached resources
		var resourcesInfo strings.Builder
		var knowledgeBases []string
		var files []string
		var aiTools []string
		var aiForges []string

		for _, data := range attachedDatas {
			switch data.Type {
			case aicommon.CONTEXT_PROVIDER_TYPE_KNOWLEDGE_BASE:
				knowledgeBases = append(knowledgeBases, data.Value)
			case aicommon.CONTEXT_PROVIDER_TYPE_FILE:
				files = append(files, data.Value)
			case aicommon.CONTEXT_PROVIDER_TYPE_AITOOL:
				aiTools = append(aiTools, data.Value)
			case aicommon.CONTEXT_PROVIDER_TYPE_AIFORGE:
				aiForges = append(aiForges, data.Value)
			}
		}

		// Build attached resources info string
		if len(knowledgeBases) > 0 {
			resourcesInfo.WriteString("### 知识库 (Knowledge Bases)\n")
			for _, kb := range knowledgeBases {
				resourcesInfo.WriteString(fmt.Sprintf("- %s\n", kb))
			}
			resourcesInfo.WriteString("\n")

			// 获取知识库样本数据，帮助 AI 了解知识库的领域和内容
			ctx := loop.GetConfig().GetContext()
			sampleData, err := r.EnhanceKnowledgeGetRandomN(ctx, DefaultKnowledgeSampleCount, knowledgeBases...)
			if err != nil {
				log.Warnf("failed to get knowledge base samples: %v", err)
			} else if sampleData != "" {
				resourcesInfo.WriteString("### 知识库样本内容 (Knowledge Base Samples)\n")
				resourcesInfo.WriteString("以下是知识库中的部分知识条目，帮助你了解知识库的领域和内容，便于后续搜索：\n\n")
				resourcesInfo.WriteString(sampleData)
				resourcesInfo.WriteString("\n")
			}
		}

		if len(files) > 0 {
			resourcesInfo.WriteString("### 文件 (Files)\n")
			for _, f := range files {
				resourcesInfo.WriteString(fmt.Sprintf("- %s\n", f))
			}
			resourcesInfo.WriteString("\n")
		}

		if len(aiTools) > 0 {
			resourcesInfo.WriteString("### AI工具 (AI Tools)\n")
			for _, t := range aiTools {
				resourcesInfo.WriteString(fmt.Sprintf("- %s\n", t))
			}
			resourcesInfo.WriteString("\n")
		}

		if len(aiForges) > 0 {
			resourcesInfo.WriteString("### AI蓝图 (AI Forges/Blueprints)\n")
			for _, f := range aiForges {
				resourcesInfo.WriteString(fmt.Sprintf("- %s\n", f))
			}
			resourcesInfo.WriteString("\n")
		}

		// Initialize loop context
		loop.Set("user_query", userQuery)
		loop.Set("attached_resources", resourcesInfo.String())
		loop.Set("knowledge_bases", strings.Join(knowledgeBases, ","))
		loop.Set("files", strings.Join(files, ","))
		loop.Set("ai_tools", strings.Join(aiTools, ","))
		loop.Set("ai_forges", strings.Join(aiForges, ","))
		loop.Set("search_results", "")
		loop.Set("search_history", "")

		r.AddToTimeline("task_initialized", fmt.Sprintf("Knowledge enhance task initialized with %d attached resources: %s", len(attachedDatas), userQuery))
		return nil
	}
}
