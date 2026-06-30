package loop_yaklangcode

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/ziputil"
)

// buildInitTask creates the initialization task handler with file detection and initial code search
func buildInitTask(r aicommon.AIInvokeRuntime, holder *searcherHolder, installCfg *aikbInstallConfig) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
		emitter := r.GetConfig().GetEmitter()
		attachedDatas := task.GetAttachedDatas()

		// 关键依赖自动安装: AIKB(grep/rag) 缺失则阻塞下载(带进度), 成功回填 holder, 失败降级。
		// yak-skills 缺失则后台安装并刷新 AutoSkillLoader。详见 ensureDependencies。
		ensureDependencies(r, loop, task, holder, installCfg)

		// 从 holder 读取(可能已被上面的自动安装回填)的最新搜索器
		docSearcher := holder.getGrep()
		ragSearcher := holder.getRAG()
		reactloops.RunAttachedExtraResourcesInit(r, loop, attachedDatas)
		editorCtx := initYaklangEditorContextFromAttached(r, loop, attachedDatas)
		if editorCtx == nil {
			editorCtx = &aicommon.YaklangEditorContext{}
		}
		aicommon.EnrichYaklangEditorContextFromUserInput(editorCtx, task.GetUserInput())
		if editorCtx.HasEditorFile() {
			loop.Set("editor_file_path", editorCtx.EditorFile)
		} else {
			loop.Set("editor_file_path", "")
		}
		createMode := editorCtx.IsCreateMode()
		if createMode {
			log.Infof("create mode: no editor target file; will emit op=create at loop end")
		}
		hasAttachedPath := editorCtx.HasEditorFile()
		attachedPath := ""
		workspacePath := ""
		if editorCtx != nil {
			if editorCtx.HasEditorFile() {
				attachedPath = editorCtx.EditorFile
			}
			if editorCtx.HasWorkspace() {
				workspacePath = editorCtx.WorkspacePath
			}
		}

		hasGrepSearcher := docSearcher != nil
		hasRAGSearcher := ragSearcher != nil
		hasSearcher := hasGrepSearcher || hasRAGSearcher
		if hasSearcher {
			loop.Set("aikb_available", "true")
		} else {
			loop.Set("aikb_available", "false")
		}

		needLiteforge := hasSearcher || !hasAttachedPath

		analyzeOpts := yaklangAnalyzeRequirementOptions{
			userInput:       task.GetUserInput(),
			hasAttachedPath: hasAttachedPath,
			createMode:      createMode,
			attachedPath:    attachedPath,
			workspacePath:   workspacePath,
			hasGrepSearcher: hasGrepSearcher,
			hasRAGSearcher:  hasRAGSearcher,
		}

		var (
			existed           string
			reason            string
			searchPatterns    []string
			semanticQuestions []string
			coreLibraries     []string
		)

		if needLiteforge {
			renderedPrompt := buildYaklangAnalyzeRequirementPrompt(analyzeOpts)
			toolOptions := buildYaklangAnalyzeRequirementToolOptions(analyzeOpts, hasSearcher)

			forgeOptions := []aicommon.GeneralKVConfigOption{}
			if hasSearcher {
				forgeOptions = append(forgeOptions, aicommon.WithGeneralConfigStreamableFieldWithNodeId("init-search-code-sample", "reason"))
			}

			reactloops.EmitStatus(loop, "开始分析用户需求... / Analyzing user requirements...")
			step1Result, err := r.InvokeSpeedPriorityLiteForge(
				task.GetContext(),
				"analyze-requirement-and-search",
				renderedPrompt,
				toolOptions,
				forgeOptions...,
			)
			if err != nil {
				log.Errorf("failed to invoke liteforge step 1: %v", err)
				operator.Failed(utils.Errorf("failed to analyze requirement: %v", err))
				return
			}

			if hasAttachedPath {
				existed = step1Result.GetString("existed_filepath")
			}
			reason = step1Result.GetString("reason")
			coreLibraries = step1Result.GetStringSlice("core_libraries")
			if hasSearcher {
				searchPatterns = step1Result.GetStringSlice("search_patterns")
				semanticQuestions = step1Result.GetStringSlice("semantic_questions")
			}
		} else {
			log.Infof("skip liteforge file detection: target path already attached (%s)", attachedPath)
		}

		// PIN 接口: 把 AI 选定的核心库(及搜索关键字派生的库)的权威函数签名锁进反应数据,
		// 让模型上手即有正确签名(参数类型/个数), 从源头减少类型/参数/猜名错误。yakdoc 始终可用,
		// 因此即使 AIKB 缺失也能 PIN。详见 BuildPinnedAPISection。
		pinnedLibs := CollectPinnedLibraries(coreLibraries, searchPatterns)
		if pinned := BuildPinnedAPISection(pinnedLibs); pinned != "" {
			loop.Set("pinned_apis", pinned)
			loop.Set("pinned_libraries", strings.Join(pinnedLibs, ","))
			reactloops.EmitStatus(loop, fmt.Sprintf("已锁定核心库 API: %s / Pinned core library APIs: %s",
				strings.Join(pinnedLibs, ", "), strings.Join(pinnedLibs, ", ")))
			log.Infof("pinned core library APIs for libs: %v (%d bytes)", pinnedLibs, len(pinned))
		}

		userRequirements := utils.MustRenderTemplate(`<|USER_REQUIREMENTS_{{.nonce}}|>
{{.data}}
---
{{.reason}}
<|USER_REQUIREMENTS_END_{{.nonce}}|>

`, map[string]any{
			"data":   task.GetUserInput(),
			"reason": reason,
			"nonce":  utils.RandStringBytes(4),
		})

		log.Infof("identified search_patterns count: %d, semantic_questions count: %d, has_attached_path: %v, has_searcher: %v",
			len(searchPatterns), len(semanticQuestions), hasAttachedPath, hasSearcher)

		var initialSamples string
		var allHits []SampleHit

		if hasSearcher {
			// Step 2.1: Grep
			if docSearcher != nil && len(searchPatterns) > 0 {
				log.Infof("init task step 2.1: grep searching code samples with %d patterns", len(searchPatterns))
				reactloops.EmitStatus(loop, "开始搜索相关代码样例... / Searching for relevant code examples...")

				patternTotal := 0
				for _, pattern := range searchPatterns {
					if pattern != "" {
						patternTotal++
					}
				}
				searchedCount := 0

				for idx, pattern := range searchPatterns {
					if pattern == "" {
						continue
					}
					log.Infof("grep searching pattern %d/%d: %s", idx+1, len(searchPatterns), pattern)

					grepOpts := []ziputil.GrepOption{
						ziputil.WithGrepCaseSensitive(false),
						ziputil.WithContext(15),
					}

					results, err := docSearcher.GrepRegexp(pattern, grepOpts...)
					if err != nil {
						results, err = docSearcher.GrepSubString(pattern, grepOpts...)
					}
					if err != nil || len(results) == 0 {
						log.Infof("no grep results found for pattern: %s", pattern)
						continue
					}

					searchedCount++
					reactloops.EmitStatus(loop, fmt.Sprintf(
						"Grep 搜索 %d/%d / Grep search %d/%d",
						searchedCount, patternTotal, searchedCount, patternTotal,
					))

					hits := GrepResultsToSampleHits(pattern, results, grepMaxHitsPerPattern)
					allHits = append(allHits, hits...)
				}
			}

			// Step 2.2: RAG semantic search
			if ragSearcher != nil && len(semanticQuestions) > 0 {
				log.Infof("init task step 2.2: semantic searching code samples with %d questions", len(semanticQuestions))
				topN := 20
				scoreThreshold := 0.4
				type ResultKey struct {
					DocID string
				}
				allResultsMap := make(map[ResultKey]rag.SearchResult)

				questionTotal := 0
				for _, question := range semanticQuestions {
					if question != "" {
						questionTotal++
					}
				}
				searchedQuestions := 0

				for idx, question := range semanticQuestions {
					if question == "" {
						continue
					}

					log.Infof("semantic searching question %d/%d: %s", idx+1, len(semanticQuestions), question)
					searchedQuestions++
					reactloops.EmitStatus(loop, fmt.Sprintf(
						"语义搜索 %d/%d / Semantic search %d/%d",
						searchedQuestions, questionTotal, searchedQuestions, questionTotal,
					))

					results, err := ragSearcher.QueryTopN(question, topN, scoreThreshold)
					if err != nil {
						log.Errorf("semantic search failed for question '%s': %v", question, err)
						continue
					}

					log.Infof("semantic search found %d results for question: %s", len(results), question)

					for _, result := range results {
						var docID string
						if result.KnowledgeBaseEntry != nil {
							docID = fmt.Sprintf("kb_%d_%s", result.KnowledgeBaseEntry.ID, result.KnowledgeBaseEntry.KnowledgeTitle)
						} else if result.Document != nil {
							docID = result.Document.ID
						} else {
							continue
						}

						key := ResultKey{DocID: docID}
						existing, exists := allResultsMap[key]
						if !exists || result.Score > existing.Score {
							allResultsMap[key] = *result
						}
					}

					questionHits := make([]rag.SearchResult, 0, len(results))
					for _, result := range results {
						questionHits = append(questionHits, *result)
					}
					sort.Slice(questionHits, func(i, j int) bool {
						return questionHits[i].Score > questionHits[j].Score
					})
					allHits = append(allHits, RAGResultsToSampleHits(question, questionHits, ragMaxHits)...)
				}
			}

			// Step 2.3: rank, trim, optional rare LLM compress
			if len(allHits) > 0 {
				var searchQueryBuilder strings.Builder
				searchQueryBuilder.WriteString(userRequirements)
				searchQueryBuilder.WriteString("\n\n【搜索模式】\n")
				if len(searchPatterns) > 0 {
					searchQueryBuilder.WriteString("Grep Patterns: ")
					searchQueryBuilder.WriteString(strings.Join(searchPatterns, ", "))
					searchQueryBuilder.WriteString("\n")
				}
				if len(semanticQuestions) > 0 {
					searchQueryBuilder.WriteString("Semantic Questions: ")
					searchQueryBuilder.WriteString(strings.Join(semanticQuestions, ", "))
				}
				searchQuery := searchQueryBuilder.String()

				ctx := task.GetContext()
				reactloops.EmitStatus(loop, "压缩样例中 / Compressing samples...")
				initialSamples = FinalizeSearchResults(ctx, allHits, searchQuery, r)
				log.Infof("initial samples finalized, hit count: %d, final size: %d bytes", len(allHits), len(initialSamples))

				if initialSamples != "" {
					manifest := NewSearchManifest(searchPatterns, semanticQuestions)
					loop.Set("initial_code_samples", initialSamples)
					loop.Set("init_search_manifest", manifest.JSON())
					loop.Set("init_samples_ready", "true")

					reactloops.EmitStatus(loop, "样例准备完成 / Samples ready")
					summary, reference := reactloops.SpillLongContent(loop, "init_yaklang_samples", initialSamples)
					reactloops.EmitActionLog(loop, "yaklang-init-search",
						fmt.Sprintf("初始化代码样例: %s (%d 条命中) / Init code samples: %s (%d hits)",
							utils.ByteSize(uint64(len(initialSamples))), len(allHits),
							utils.ByteSize(uint64(len(initialSamples))), len(allHits)),
						reference,
					)
					r.AddToTimeline("initial_code_samples", fmt.Sprintf(
						"初始化代码样例 (%s, %d hits)\n%s",
						utils.ByteSize(uint64(len(initialSamples))), len(allHits), summary,
					))
				}
			} else {
				log.Infof("no search hits collected from any searcher")
			}
		} else {
			log.Infof("skip init search: AIKB unavailable")
		}

		finalizeYaklangInitFileTarget(r, loop, emitter, operator, editorCtx, existed)
	}
}
