package loop_yaklangcode

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/ziputil"
)

// buildInitTask creates the initialization task handler with file detection and initial code search
func buildInitTask(r aicommon.AIInvokeRuntime, docSearcher *ziputil.ZipGrepSearcher, ragSearcher *rag.RAGSystem) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
		emitter := r.GetConfig().GetEmitter()
		attachedDatas := task.GetAttachedDatas()
		reactloops.RunAttachedExtraResourcesInit(r, loop, attachedDatas)
		editorCtx := initYaklangEditorContextFromAttached(r, loop, attachedDatas)
		hasAttachedPath := editorCtx != nil && editorCtx.HasEditorFile()
		codePreviewOnly := !hasAttachedPath
		setYaklangCodePreviewOnly(loop, codePreviewOnly)
		if codePreviewOnly {
			log.Infof("code preview mode: no editor target file; generated code will not bind to disk paths")
		}
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

		// Step 1: 分析用户需求，生成搜索关键字和判断文件路径
		log.Infof("init task step 1: analyzing user requirements and generating search patterns")

		hasGrepSearcher := docSearcher != nil
		hasRAGSearcher := ragSearcher != nil
		hasSearcher := hasGrepSearcher || hasRAGSearcher
		needLiteforge := hasSearcher || !hasAttachedPath

		analyzeOpts := yaklangAnalyzeRequirementOptions{
			userInput:        task.GetUserInput(),
			hasAttachedPath:  hasAttachedPath,
			codePreviewOnly:  codePreviewOnly,
			attachedPath:     attachedPath,
			workspacePath:    workspacePath,
			hasGrepSearcher:  hasGrepSearcher,
			hasRAGSearcher:   hasRAGSearcher,
		}

		var (
			existed           string
			reason            string
			searchPatterns    []string
			semanticQuestions []string
		)

		if needLiteforge {
			renderedPrompt := buildYaklangAnalyzeRequirementPrompt(analyzeOpts)
			toolOptions := buildYaklangAnalyzeRequirementToolOptions(analyzeOpts, hasSearcher)

			forgeOptions := []aicommon.GeneralKVConfigOption{}
			if hasSearcher {
				forgeOptions = append(forgeOptions, aicommon.WithGeneralConfigStreamableFieldWithNodeId("init-search-code-sample", "reason"))
			}

			loop.LoadingStatus("开始分析用户需求... / Analyzing user requirements...")
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
			searchPatterns = step1Result.GetStringSlice("search_patterns")
			semanticQuestions = step1Result.GetStringSlice("semantic_questions")
		} else {
			log.Infof("skip liteforge file detection: target path already attached (%s)", attachedPath)
		}
		for _, question := range semanticQuestions {
			emitter.EmitDefaultStreamEvent("thought", bytes.NewBufferString(question), task.GetIndex())
		}
		if len(searchPatterns) > 0 {
			emitter.EmitDefaultStreamEvent("thought", bytes.NewBufferString(strings.Join(searchPatterns, ",")), task.GetIndex())
		}

		var userRequirements = utils.MustRenderTemplate(`<|USER_REQUIREMENTS_{{.nonce}}|>
{{.data}}
---
{{.reason}}
<|USER_REQUIREMENTS_END_{{.nonce}}|>

`, map[string]any{
			"data":   task.GetUserInput(),
			"reason": reason,
			"nonce":  utils.RandStringBytes(4),
		})

		log.Infof("identified search_patterns count: %d, semantic_questions count: %d, has_attached_path: %v",
			len(searchPatterns), len(semanticQuestions), hasAttachedPath)

		// Step 2: 执行代码样例搜索（Grep + RAG 语义搜索）
		var initialSamples string
		var allSearchResults strings.Builder

		// Step 2.1: 执行 Grep 搜索（如果有 docSearcher）
		if docSearcher != nil && len(searchPatterns) > 0 {
			log.Infof("init task step 2.1: grep searching code samples with %d patterns", len(searchPatterns))
			loop.LoadingStatus("开始搜索相关代码样例... / Searching for relevant code examples...")

			var grepResults strings.Builder
			searchedCount := 0
			for idx, pattern := range searchPatterns {
				if pattern == "" {
					continue
				}
				log.Infof("grep searching pattern %d/%d: %s", idx+1, len(searchPatterns), pattern)

				// 执行 grep 搜索
				grepOpts := []ziputil.GrepOption{
					ziputil.WithGrepCaseSensitive(false),
					ziputil.WithContext(15),
				}

				results, err := docSearcher.GrepRegexp(pattern, grepOpts...)
				if err != nil {
					// 如果正则失败，尝试子字符串搜索
					results, err = docSearcher.GrepSubString(pattern, grepOpts...)
				}

				if err != nil || len(results) == 0 {
					log.Infof("no grep results found for pattern: %s", pattern)
					continue
				}

				var singleResult bytes.Buffer
				var singleResultStreamId string
				pr, pw := utils.NewPipe()
				if event, _ := emitter.EmitDefaultStreamEvent("thought", pr, task.GetIndex()); event != nil {
					singleResultStreamId = event.GetStreamEventWriterId()
				}

				pw.WriteString("[Searching]: ")
				pw.WriteString(pattern)
				pw.WriteString("... \n ")
				pw.WriteString(fmt.Sprintf("结果[%v]条, ", len(results)))

				header := fmt.Sprintf("\n=== Grep Pattern: %s (Found %d matches) ===\n", pattern, len(results))
				searchedCount++
				grepResults.WriteString(header)
				singleResult.WriteString(header)

				for i, result := range results {
					l := fmt.Sprintf("\n--- [%d] %s:%d ---\n", i+1, result.FileName, result.LineNumber)
					grepResults.WriteString(l)
					singleResult.WriteString(l)

					if len(result.ContextBefore) > 0 {
						for _, line := range result.ContextBefore {
							text := fmt.Sprintf("  %s\n", line)
							singleResult.WriteString(text)
							grepResults.WriteString(text)
						}
					}

					text := fmt.Sprintf(">>> %s\n", result.Line)
					grepResults.WriteString(text)
					singleResult.WriteString(text)

					if len(result.ContextAfter) > 0 {
						for _, line := range result.ContextAfter {
							text := fmt.Sprintf("  %s\n", line)
							grepResults.WriteString(text)
							singleResult.WriteString(text)
						}
					}
				}

				pw.WriteString(" Size: " + utils.ByteSize(uint64(singleResult.Len())) + "\n")
				emitter.EmitTextReferenceMaterial(singleResultStreamId, singleResult.String())
				pw.Close()
			}

			if searchedCount > 0 {
				rawResults := grepResults.String()
				log.Infof("grep collected %d bytes of search results", len(rawResults))
				allSearchResults.WriteString(rawResults)
			} else {
				log.Infof("no grep search results found for any pattern")
			}
		}

		// Step 2.2: 执行 RAG 语义搜索（如果有 ragSearcher）
		if ragSearcher != nil && len(semanticQuestions) > 0 {
			log.Infof("init task step 2.2: semantic searching code samples with %d questions", len(semanticQuestions))
			topN := 20            // 每个问题返回20个结果
			scoreThreshold := 0.4 // 相似度阈值
			type ResultKey struct {
				DocID string
			}
			allResultsMap := make(map[ResultKey]rag.SearchResult)

			for idx, question := range semanticQuestions {
				if question == "" {
					continue
				}

				log.Infof("semantic searching question %d/%d: %s", idx+1, len(semanticQuestions), question)

				pr, pw := utils.NewPipe()
				var singleResultStreamId string
				if event, _ := emitter.EmitDefaultStreamEvent("thought", pr, task.GetIndex()); event != nil {
					singleResultStreamId = event.GetStreamEventWriterId()
				}

				pw.WriteString("[Searching] 语义搜索: ")
				pw.WriteString(question)
				pw.WriteString("... \n ")

				// 执行语义搜索
				results, err := ragSearcher.QueryTopN(question, topN, scoreThreshold)
				if err != nil {
					pw.WriteString("No Results Found.\n")
					pw.Close()
					log.Errorf("semantic search failed for question '%s': %v", question, err)
					continue
				}

				pw.WriteString(fmt.Sprintf("结果[%v]条; ", len(results)))

				log.Infof("semantic search found %d results for question: %s", len(results), question)

				var singleResult bytes.Buffer
				// 合并结果，使用文档ID去重，保留最高分数
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
						singleResult.WriteString(result.GetContent())
					}
				}
				emitter.EmitTextReferenceMaterial(singleResultStreamId, singleResult.String())
				pw.Close()
			}

			// 将 map 转换为切片并格式化
			if len(allResultsMap) > 0 {
				var ragResults strings.Builder
				ragResults.WriteString(fmt.Sprintf("\n=== Semantic Search Results (Found %d unique matches) ===\n", len(allResultsMap)))

				displayCount := 0
				for _, result := range allResultsMap {
					ragResults.WriteString(fmt.Sprintf("\n--- [%d] Score: %.3f ---\n", displayCount+1, result.Score))
					var content string
					if result.KnowledgeBaseEntry != nil {
						content = result.KnowledgeBaseEntry.KnowledgeDetails
					} else if result.Document != nil {
						content = result.Document.Content
					}

					ragResults.WriteString(content)
					ragResults.WriteString("\n")
					displayCount++
				}

				rawResults := ragResults.String()
				log.Infof("semantic search collected %d bytes of results", len(rawResults))
				log.Infof("semantic search results: \n%s", rawResults)
				allSearchResults.WriteString(rawResults)
			} else {
				log.Infof("no semantic search results found for any question")
			}
		}

		// Step 2.3: 合并并压缩所有搜索结果
		if allSearchResults.Len() > 0 {
			rawCombinedResults := allSearchResults.String()
			log.Infof("total collected %d bytes of combined search results, attempting compression", len(rawCombinedResults))

			// 构建搜索查询字符串，包含用户需求
			var searchQueryBuilder strings.Builder
			searchQueryBuilder.WriteString(userRequirements)
			searchQueryBuilder.WriteString("\n\n【搜索模式】\n")
			if len(searchPatterns) > 0 {
				searchQueryBuilder.WriteString("Grep Patterns: ")
				for idx, pattern := range searchPatterns {
					if idx > 0 {
						searchQueryBuilder.WriteString(", ")
					}
					searchQueryBuilder.WriteString(pattern)
				}
				searchQueryBuilder.WriteString("\n")
			}
			if len(semanticQuestions) > 0 {
				searchQueryBuilder.WriteString("Semantic Questions: ")
				for idx, question := range semanticQuestions {
					if idx > 0 {
						searchQueryBuilder.WriteString(", ")
					}
					searchQueryBuilder.WriteString(question)
				}
			}
			searchQuery := searchQueryBuilder.String()

			// 使用 invoker 的压缩接口精选代码片段
			ctx := task.GetContext()
			compressedSamples, err := r.CompressLongTextWithDestination(ctx, rawCombinedResults, searchQuery, 10*1024) // 压缩到 10KB
			if err != nil {
				log.Warnf("failed to compress search results: %v, using raw results", err)
				// 压缩失败时使用原始结果的截断版本
				initialSamples = utils.ShrinkTextBlock(rawCombinedResults, 10*1024)
			} else {
				initialSamples = compressedSamples
			}

			if initialSamples != "" {
				if event, _ := emitter.EmitThoughtStream(task.GetIndex(), "压缩完成，压缩后样本大小为: "+utils.ByteSize(uint64(len(initialSamples)))); event != nil {
					emitter.EmitTextReferenceMaterial(event.GetStreamEventWriterId(), initialSamples)
				}
				r.AddToTimeline("initial_code_samples", initialSamples)
				log.Infof("initial samples collected and compressed successfully, final size: %d bytes", len(initialSamples))
			}
		} else {
			log.Infof("no search results found from any searcher")
		}

		// Step 3: 处理文件路径与初始代码（附件选区优先于磁盘读取）
		finalizeYaklangInitFileTarget(r, loop, emitter, operator, editorCtx, existed)
	}
}
