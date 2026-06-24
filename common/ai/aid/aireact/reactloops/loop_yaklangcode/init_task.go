package loop_yaklangcode

import (
	"bytes"
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
func buildInitTask(r aicommon.AIInvokeRuntime, docSearcher *ziputil.ZipGrepSearcher, ragSearcher *rag.RAGSystem) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
		emitter := r.GetConfig().GetEmitter()
		attachedDatas := task.GetAttachedDatas()
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
			if hasSearcher {
				searchPatterns = step1Result.GetStringSlice("search_patterns")
				semanticQuestions = step1Result.GetStringSlice("semantic_questions")
			}
		} else {
			log.Infof("skip liteforge file detection: target path already attached (%s)", attachedPath)
		}
		for _, question := range semanticQuestions {
			emitter.EmitDefaultStreamEvent("thought", bytes.NewBufferString(question), task.GetIndex())
		}
		if len(searchPatterns) > 0 {
			emitter.EmitDefaultStreamEvent("thought", bytes.NewBufferString(strings.Join(searchPatterns, ",")), task.GetIndex())
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
				loop.LoadingStatus("开始搜索相关代码样例... / Searching for relevant code examples...")

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
					singleResult.WriteString(header)

					hits := GrepResultsToSampleHits(pattern, results, grepMaxHitsPerPattern)
					allHits = append(allHits, hits...)

					for i, hit := range hits {
						l := fmt.Sprintf("\n--- [%d] %s:%d ---\n", i+1, hit.FileName, hit.Line)
						singleResult.WriteString(l)
						singleResult.WriteString(hit.Content)
						singleResult.WriteString("\n")
					}

					pw.WriteString(" Size: " + utils.ByteSize(uint64(singleResult.Len())) + "\n")
					emitter.EmitTextReferenceMaterial(singleResultStreamId, singleResult.String())
					pw.Close()
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

					for _, result := range results {
						singleResult.WriteString(result.GetContent())
					}
					emitter.EmitTextReferenceMaterial(singleResultStreamId, singleResult.String())
					pw.Close()
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
				initialSamples = FinalizeSearchResults(ctx, allHits, searchQuery, r)
				log.Infof("initial samples finalized, hit count: %d, final size: %d bytes", len(allHits), len(initialSamples))

				if initialSamples != "" {
					manifest := NewSearchManifest(searchPatterns, semanticQuestions)
					loop.Set("initial_code_samples", initialSamples)
					loop.Set("init_search_manifest", manifest.JSON())
					loop.Set("init_samples_ready", "true")

					if event, _ := emitter.EmitThoughtStream(task.GetIndex(), "预检索完成，样本大小: "+utils.ByteSize(uint64(len(initialSamples)))); event != nil {
						emitter.EmitTextReferenceMaterial(event.GetStreamEventWriterId(), initialSamples)
					}
					r.AddToTimeline("initial_code_samples", initialSamples)
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
