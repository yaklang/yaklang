package loop_knowledge_enhance

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
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
				reactloops.WithAllowRAG(false),
				reactloops.WithAllowAIForge(false),
				reactloops.WithAllowPlanAndExec(false),
				reactloops.WithAllowToolCall(true),
				reactloops.WithInitTask(buildInitTask(r)),
				reactloops.WithMaxIterations(int(r.GetConfig().GetMaxIterationCount())),
				reactloops.WithAllowUserInteract(r.GetConfig().GetAllowUserInteraction()),
				reactloops.WithPersistentInstruction(instruction),
				reactloops.WithReflectionOutputExample(outputExample),
				reactloops.WithMaxIterations(2),
				reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
					userQuery := loop.Get("user_query")
					attachedResources := loop.Get("attached_resources")
					searchResults := loop.Get("search_results")
					searchHistory := loop.Get("search_history")
					nextMovementsSummary := loop.Get("next_movements_summary")
					artifactsSummary := buildArtifactsSummary(loop)

					renderMap := map[string]any{
						"UserQuery":            userQuery,
						"AttachedResources":    attachedResources,
						"SearchResults":        searchResults,
						"SearchHistory":        searchHistory,
						"NextMovementsSummary": nextMovementsSummary,
						"ArtifactsSummary":     artifactsSummary,
						"Nonce":                nonce,
					}
					return utils.RenderTemplate(reactiveData, renderMap)
				}),
				// Register actions: semantic and keyword search variants
				searchKnowledgeSemanticAction(r),
				searchKnowledgeKeywordAction(r),
				// Register post-iteration hook for final document generation
				BuildOnPostIterationHook(r),
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

// buildArtifactsSummary collects artifact filenames from loop context
func buildArtifactsSummary(loop *reactloops.ReActLoop) string {
	var artifacts []string
	maxIterations := loop.GetCurrentIterationIndex()
	if maxIterations <= 0 {
		maxIterations = 5 // check at least 5 iterations
	}

	for iteration := 1; iteration <= maxIterations+1; iteration++ {
		for queryIdx := 1; queryIdx <= 20; queryIdx++ { // Support up to 20 queries per iteration
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
	summary.WriteString(fmt.Sprintf("已保存 %d 个知识查询结果文件：\n", len(artifacts)))
	for i, filename := range artifacts {
		summary.WriteString(fmt.Sprintf("  %d. %s\n", i+1, filename))
	}
	return summary.String()
}

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
		var knowledgeCoreSummary string

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
			if task != nil && !utils.IsNil(task.GetContext()) {
				ctx = task.GetContext()
			}
			sampleData, err := r.EnhanceKnowledgeGetRandomN(ctx, DefaultKnowledgeSampleCount, knowledgeBases...)
			if err != nil {
				log.Warnf("failed to get knowledge base samples: %v", err)
			} else if sampleData != "" {
				// 使用 LiteForge 总结核心内容（多维度，最多 5 条）
				const maxSummaryInputSize = 12 * 1024 // 12KB
				summaryInput := sampleData
				if len(summaryInput) > maxSummaryInputSize {
					summaryInput = summaryInput[:maxSummaryInputSize] + "\n\n[... 内容已截断 ...]"
				}

				summaryPrompt := utils.MustRenderTemplate(`请根据以下知识库样本内容，提炼知识库的主要核心内容，从不同维度总结。
要求：
1. 输出不超过 5 条
2. 每条包含“维度”和“核心内容”
3. 用简洁中文表达
4. 只根据样本内容，不要编造

<|KNOWLEDGE_SAMPLES_{{ .Nonce }}|>
{{ .Samples }}
<|KNOWLEDGE_SAMPLES_END_{{ .Nonce }}|>
`, map[string]any{
					"Nonce":   utils.RandStringBytes(4),
					"Samples": summaryInput,
				})

				summaryResult, sumErr := r.InvokeLiteForge(
					ctx,
					"summarize-knowledge-core",
					summaryPrompt,
					[]aitool.ToolOption{
						aitool.WithStructArrayParam(
							"core_summaries",
							[]aitool.PropertyOption{
								aitool.WithParam_Description("知识库核心内容多维度总结，最多 5 条"),
								aitool.WithParam_Required(true),
							},
							nil,
							aitool.WithStringParam("dimension", aitool.WithParam_Description("总结维度，如领域/主题/场景/方法/概念等")),
							aitool.WithStringParam("summary", aitool.WithParam_Description("该维度下的核心内容总结")),
						),
					},
				)
				if sumErr != nil {
					log.Warnf("failed to summarize knowledge core content: %v", sumErr)
				} else if summaryResult != nil {
					coreItems := summaryResult.GetInvokeParamsArray("core_summaries")
					if len(coreItems) > 5 {
						coreItems = coreItems[:5]
					}
					if len(coreItems) > 0 {
						var summaryBuilder strings.Builder
						for i, item := range coreItems {
							dimension := strings.TrimSpace(item.GetString("dimension"))
							summary := strings.TrimSpace(item.GetString("summary"))
							if summary == "" {
								continue
							}
							if dimension == "" {
								dimension = fmt.Sprintf("维度%d", i+1)
							}
							summaryBuilder.WriteString(fmt.Sprintf("- %s：%s\n", dimension, summary))
						}
						knowledgeCoreSummary = strings.TrimSpace(summaryBuilder.String())
						if knowledgeCoreSummary != "" {
							resourcesInfo.WriteString("### 知识库核心内容总结 (Knowledge Core Summary)\n")
							resourcesInfo.WriteString("以下为基于样本数据的核心内容多维度总结\n")
							resourcesInfo.WriteString(knowledgeCoreSummary)
							resourcesInfo.WriteString("\n\n")
						}
					}
				}

				// 检查样本数据大小，控制在30k字节以内
				const maxSampleSize = 30 * 1024 // 30KB
				if len(sampleData) > maxSampleSize {
					log.Infof("knowledge base samples too large (%d bytes), chunking and emitting to artifacts", len(sampleData))

					// 将大内容分块处理，每块最大10k字节
					const chunkSize = 10 * 1024 // 10KB per chunk
					chunks := make([]string, 0)
					currentChunk := strings.Builder{}
					lines := strings.Split(sampleData, "\n")

					for _, line := range lines {
						// 如果添加这行会导致当前块超过限制，开始新块
						if currentChunk.Len()+len(line)+1 > chunkSize && currentChunk.Len() > 0 {
							chunks = append(chunks, currentChunk.String())
							currentChunk.Reset()
						}
						currentChunk.WriteString(line)
						currentChunk.WriteString("\n")
					}

					// 添加最后一块
					if currentChunk.Len() > 0 {
						chunks = append(chunks, currentChunk.String())
					}

					// 为每个块创建 artifacts
					emitter := loop.GetEmitter()
					artifactFilenames := make([]string, 0, len(chunks))

					for i, chunk := range chunks {
						filename := r.EmitFileArtifactWithExt(fmt.Sprintf("kb_samples_chunk_%d", i+1), ".txt", "")
						emitter.EmitPinFilename(filename)
						artifactFilenames = append(artifactFilenames, filename)

						// 写入文件内容
						err := os.WriteFile(filename, []byte(chunk), 0644)
						if err != nil {
							log.Warnf("failed to write knowledge base sample chunk %d: %v", i+1, err)
						}
					}

					resourcesInfo.WriteString("### 知识库样本内容 (Knowledge Base Samples)\n")
					resourcesInfo.WriteString(fmt.Sprintf("知识库样本数据较大（%d 字节），已分块存储到 artifacts 中：\n", len(sampleData)))
					for i, filename := range artifactFilenames {
						resourcesInfo.WriteString(fmt.Sprintf("- 样本块 %d: %s\n", i+1, filename))
					}
					resourcesInfo.WriteString("\n请根据需要查看相关 artifacts 获取完整样本内容。\n\n")
				} else {
					resourcesInfo.WriteString("### 知识库样本内容 (Knowledge Base Samples)\n")
					resourcesInfo.WriteString("以下是知识库中的部分知识条目，帮助你了解知识库的领域和内容，便于后续搜索：\n\n")
					resourcesInfo.WriteString(sampleData)
					resourcesInfo.WriteString("\n")
				}
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

		loopDir := loop.Get("loop_directory")
		if loopDir != "" {
			var markdown strings.Builder
			markdown.WriteString("# Attached Resources\n\n")
			markdown.WriteString("## User Query\n\n")
			markdown.WriteString(userQuery)
			markdown.WriteString("\n\n## Resources\n\n")
			markdown.WriteString(resourcesInfo.String())

			filename := filepath.Join(loopDir, fmt.Sprintf("attached_resources_%s.md", utils.DatetimePretty2()))
			if err := os.WriteFile(filename, []byte(markdown.String()), 0644); err != nil {
				log.Warnf("failed to write knowledge enhance resources markdown: %v", err)
			} else {
				if emitter := loop.GetEmitter(); emitter != nil {
					emitter.EmitPinFilename(filename)
				}
			}
		}

		// Initialize loop context
		loop.Set("user_query", userQuery)
		loop.Set("attached_resources", resourcesInfo.String())
		loop.Set("knowledge_bases", strings.Join(knowledgeBases, ","))
		loop.Set("files", strings.Join(files, ","))
		loop.Set("ai_tools", strings.Join(aiTools, ","))
		loop.Set("ai_forges", strings.Join(aiForges, ","))
		loop.Set("knowledge_core_summary", knowledgeCoreSummary)
		loop.Set("search_results", "")
		loop.Set("search_history", "")

		r.AddToTimeline("task_initialized", fmt.Sprintf("Knowledge enhance task initialized with %d attached resources: %s", len(attachedDatas), userQuery))
		return nil
	}
}
