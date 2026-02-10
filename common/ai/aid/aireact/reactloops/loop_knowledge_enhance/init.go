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
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
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
				reactloops.WithAllowToolCall(false),
				reactloops.WithInitTask(buildInitTask(r)),
				reactloops.WithMaxIterations(int(r.GetConfig().GetMaxIterationCount())),
				reactloops.WithAllowUserInteract(r.GetConfig().GetAllowUserInteraction()),
				reactloops.WithPersistentInstruction(instruction),
				reactloops.WithReflectionOutputExample(outputExample),
				reactloops.WithMaxIterations(3), // 支持多轮单条搜索
				reactloops.WithActionFilter(func(action *reactloops.LoopAction) bool {
					allowActionNames := []string{
						"search_knowledge_semantic",
						"search_knowledge_keyword",
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
					attachedResources := loop.Get("attached_resources")
					searchResults := loop.Get("search_results_summary")
					searchHistory := loop.Get("search_history")
					nextMovementsSummary := loop.Get("next_movements_summary")
					artifactsSummary := buildArtifactsSummary(loop)
					// 已加载的知识库列表，用于在 prompt 中展示
					loadedKnowledgeBases := loop.Get("knowledge_bases")

					renderMap := map[string]any{
						"UserQuery":            userQuery,
						"AttachedResources":    attachedResources,
						"SearchResults":        searchResults,
						"SearchHistory":        searchHistory,
						"NextMovementsSummary": nextMovementsSummary,
						"ArtifactsSummary":     artifactsSummary,
						"LoadedKnowledgeBases": loadedKnowledgeBases,
						"Nonce":                nonce,
					}
					return utils.RenderTemplate(reactiveData, renderMap)
				}),
				// Register actions: semantic search, keyword search and final summary
				searchKnowledgeSemanticAction(r),
				searchKnowledgeKeywordAction(r),
				finalSummaryAction(r),
				// Register post-iteration hook for final document generation (triggered on loop exit)
				BuildOnPostIterationHook(r),
			}
			preset = append(opts, preset...)
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
		reactloops.WithLoopIsHidden(false),
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
func buildInitTask(r aicommon.AIInvokeRuntime) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
		// Get user input from task
		userQuery := task.GetUserInput()

		// Get attached resources from task
		attachedDatas := task.GetAttachedDatas()

		// Parse and format attached resources
		var resourcesInfo strings.Builder
		var knowledgeBases []string
		includeAllKnowledgeBases := false
		autoSelectAllKnowledgeBases := false
		var files []string
		var aiTools []string
		var aiForges []string
		var knowledgeCoreSummary string

		for _, data := range attachedDatas {
			switch data.Type {
			case aicommon.CONTEXT_PROVIDER_TYPE_KNOWLEDGE_BASE:
				if data.Key == aicommon.CONTEXT_PROVIDER_KEY_SYSTEM_FLAG {
					if data.Value == aicommon.CONTEXT_PROVIDER_VALUE_ALL_KNOWLEDGE_BASE {
						includeAllKnowledgeBases = true
						if autoSelectAllKnowledgeBases {
							autoSelectAllKnowledgeBases = false
							log.Warn("@auto_select_knowledge_base is already set, override")
						}
						continue
					}
					if strings.HasPrefix(data.Value, aicommon.CONTEXT_PROVIDER_VALUE_AUTO_SELECT_KNOWLEDGE_BASE) {
						autoSelectAllKnowledgeBases = true
						if includeAllKnowledgeBases {
							includeAllKnowledgeBases = false
							log.Warn("@all_knowledge_base is already set, override")
							continue
						}
					}
				}
				knowledgeBases = append(knowledgeBases, data.Value)
			case aicommon.CONTEXT_PROVIDER_TYPE_FILE:
				files = append(files, data.Value)
			case aicommon.CONTEXT_PROVIDER_TYPE_AITOOL:
				aiTools = append(aiTools, data.Value)
			case aicommon.CONTEXT_PROVIDER_TYPE_AIFORGE:
				aiForges = append(aiForges, data.Value)
			}
		}

		if includeAllKnowledgeBases {
			allKBNames, err := yakit.GetKnowledgeBaseNameList(consts.GetGormProfileDatabase())
			if err != nil {
				log.Warnf("failed to load all knowledge base names: %v", err)
			} else {
				knowledgeBases = append(knowledgeBases, allKBNames...)
			}
		}

		if autoSelectAllKnowledgeBases {
			knowledgeBases = []string{}
		}

		knowledgeBases = dedupStrings(knowledgeBases)
		log.Infof("start to get knowledge base selected: %v", knowledgeBases)

		if len(knowledgeBases) <= 0 {
			log.Info("no knowledge bases found, start to select via invoker")
			// Use the invoker to select knowledge bases
			selectResult, err := r.SelectKnowledgeBase(loop.GetConfig().GetContext(), userQuery)
			if err != nil {
				log.Warnf("failed to select knowledge bases: %v", err)
				operator.Failed(utils.Errorf("failed to select knowledge bases: %v", err))
				return
			}
			if len(selectResult.KnowledgeBases) > 0 {
				knowledgeBases = append(knowledgeBases, selectResult.KnowledgeBases...)
			}
			log.Infof("selected %d knowledge bases: %v, reason: %s", len(selectResult.KnowledgeBases), selectResult.KnowledgeBases, selectResult.Reason)
		}
		knowledgeBases = dedupStrings(knowledgeBases)

		if len(knowledgeBases) <= 0 {
			operator.Failed(utils.Errorf("no knowledge bases found"))
			return
		}

		resourcesInfo.WriteString("### 知识库 (Knowledge Bases)\n")
		if includeAllKnowledgeBases {
			resourcesInfo.WriteString("用户本次回答涉及知识库，列表如下：\n")
		}
		for _, kb := range knowledgeBases {
			resourcesInfo.WriteString(fmt.Sprintf("- %s\n", kb))
		}
		resourcesInfo.WriteString("\n")

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

		loopDataDir := loop.GetLoopContentDir("data")
		if loopDataDir != "" {
			var markdown strings.Builder
			markdown.WriteString("# Attached Resources\n\n")
			markdown.WriteString("## User Query\n\n")
			markdown.WriteString(userQuery)
			markdown.WriteString("\n\n## Resources\n\n")
			markdown.WriteString(resourcesInfo.String())

			filename := filepath.Join(loopDataDir, fmt.Sprintf("attached_resources_%s.md", utils.DatetimePretty2()))
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
		// Default: Continue with normal loop execution
		operator.Continue()
	}
}

func dedupStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
