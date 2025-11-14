package loop_yaklangcode

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/ziputil"
)

// buildInitTask creates the initialization task handler with file detection and initial code search
func buildInitTask(r aicommon.AIInvokeRuntime, docSearcher *ziputil.ZipGrepSearcher, ragSearcher *rag.RAGSystem) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) error {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) error {
		emitter := r.GetConfig().GetEmitter()

		// Step 1: 分析用户需求，生成搜索关键字和判断文件路径
		log.Infof("init task step 1: analyzing user requirements and generating search patterns")

		// 判断是否需要搜索代码样例
		hasSearcher := docSearcher != nil || ragSearcher != nil

		// 使用模板构建动态 prompt
		promptTemplate := `
你的目标是分析用户需求，完成以下任务：

【任务1：判断文件操作类型】
判断这是创建新文件还是修改已有文件：
- 如果用户明确提到文件路径（如"修改 /tmp/test.yak"），则是修改已有文件
- 如果用户只描述功能需求，没有提到具体文件，则是创建新文件
{{ if .hasGrepSearcher }}
【任务2：生成精确代码搜索关键字(Grep模式)】
根据用户需求，生成 2-4 个搜索模式（search_patterns），用于在 Yaklang 代码样例库中进行精确文本搜索：

搜索模式类型：
1. 函数名搜索：如 "servicescan\\.Scan", "poc\\.Get", "str\\.Split"
2. 关键词搜索：如 "端口扫描", "HTTP请求", "JSON解析"
3. 混合搜索：如 "mitm.*证书", "fuzz.*参数"

注意事项：
- 优先使用函数名搜索（使用 \\.  转义点号）
- 每个pattern要具体且相关，避免过于宽泛
- 如果涉及多个功能点，可以为每个功能点生成一个pattern
- 搜索模式需要是正则表达式或关键词
{{ end }}{{ if .hasRAGSearcher }}
【任务{{ if .hasGrepSearcher }}3{{ else }}2{{ end }}：生成语义搜索问题(RAG向量搜索)】
根据用户需求，生成 2-4 个完整的问题（semantic_questions），用于语义向量搜索相关代码样例：

问题格式要求：
1. 必须是完整的主谓宾句式
2. 禁止使用代词（它、这个、那个等）
3. 明确指明 Yaklang 语言
4. 每个问题要从不同角度描述需求

问题示例：
✅ Good: "Yaklang中如何发送HTTP请求？"
✅ Good: "Yaklang中如何进行端口扫描？"
✅ Good: "Yaklang中如何处理JSON数据？"
✅ Good: "Yaklang中如何调用爬虫功能？"
❌ Bad: "如何发送请求？" - 缺少主语
❌ Bad: "它如何使用？" - 使用代词
❌ Bad: "端口扫描" - 不完整句式
{{ end }}
<|USER_INPUT_{{ .nonce }}|>
{{ .data }}
<|USER_INPUT_END_{{ .nonce }}|>
`

		// 构建动态 tool options
		toolOptions := []aitool.ToolOption{
			aitool.WithBoolParam("create_new_file", aitool.WithParam_Description("Is this task to create a new file or modify an existing file? If user mentions specific file path, set to false."), aitool.WithParam_Required(true)),
			aitool.WithStringParam("existed_filepath", aitool.WithParam_Description("Only when create_new_file is false. The file path to modify.")),
		}

		// 根据 docSearcher 是否存在添加 search_patterns
		if docSearcher != nil {
			toolOptions = append(toolOptions,
				aitool.WithStringArrayParam("search_patterns", aitool.WithParam_Description("2-4 search patterns for finding relevant Yaklang code examples. Each pattern should be a regex or keyword."), aitool.WithParam_Required(true)),
			)
		}

		// 根据 ragSearcher 是否存在添加 semantic_questions
		if ragSearcher != nil {
			toolOptions = append(toolOptions,
				aitool.WithStringArrayParam("semantic_questions", aitool.WithParam_Description("2-4 complete questions for semantic search of Yaklang code examples. Each question must be a complete sentence with subject-predicate-object structure and explicitly mention 'Yaklang'."), aitool.WithParam_Required(true)),
			)
		}

		// 只有在有搜索器时才添加 reason 参数
		if hasSearcher {
			toolOptions = append(toolOptions,
				aitool.WithStringParam("reason", aitool.WithParam_Description("Explain your decision and why these search patterns/questions are chosen."), aitool.WithParam_Required(true)),
			)
		}

		// 渲染 prompt 模板
		renderedPrompt := utils.MustRenderTemplate(
			promptTemplate,
			map[string]any{
				"nonce":           utils.RandStringBytes(4),
				"data":            task.GetUserInput(),
				"hasGrepSearcher": docSearcher != nil,
				"hasRAGSearcher":  ragSearcher != nil,
			})

		// 构建 InvokeLiteForge 选项
		forgeOptions := []aicommon.GeneralKVConfigOption{}
		if hasSearcher {
			forgeOptions = append(forgeOptions, aicommon.WithGeneralConfigStreamableFieldWithNodeId("init-search-code-sample", "reason"))
		}

		step1Result, err := r.InvokeLiteForge(
			task.GetContext(),
			"analyze-requirement-and-search",
			renderedPrompt,
			toolOptions,
			forgeOptions...,
		)
		if err != nil {
			log.Errorf("failed to invoke liteforge step 1: %v", err)
			return utils.Errorf("failed to analyze requirement: %v", err)
		}

		createNewFile := step1Result.GetBool("create_new_file")
		existed := step1Result.GetString("existed_filepath")
		reason := step1Result.GetString("reason")
		searchPatterns := step1Result.GetStringSlice("search_patterns")
		semanticQuestions := step1Result.GetStringSlice("semantic_questions")
		for _, question := range searchPatterns {
			emitter.EmitDefaultStreamEvent("semantic_questions", bytes.NewBufferString(question), task.GetIndex())
		}

		var userRequirements = utils.MustRenderTemplate(`<|USER_REQUIREMENTS_{{.nonce}}|>
{{.data}}
---
{{.reason}}
<|USER_REQUIREMENTS_END_{{.nonce}}|>

`, map[string]any{
			"data":   task.GetUserInput(),
			"reason": reason,
		})

		log.Infof("identified create_new_file: %v, search_patterns count: %d, semantic_questions count: %d",
			createNewFile, len(searchPatterns), len(semanticQuestions))

		// Step 2: 执行代码样例搜索（Grep + RAG 语义搜索）
		var initialSamples string
		var allSearchResults strings.Builder

		// Step 2.1: 执行 Grep 搜索（如果有 docSearcher）
		if docSearcher != nil && len(searchPatterns) > 0 {
			log.Infof("init task step 2.1: grep searching code samples with %d patterns", len(searchPatterns))
			emitter.EmitThoughtStream(task.GetIndex(), "Searching for relevant code examples using grep patterns...")

			var grepResults strings.Builder
			searchedCount := 0
			maxPatterns := 4 // 最多搜索4个pattern
			if len(searchPatterns) > maxPatterns {
				searchPatterns = searchPatterns[:maxPatterns]
			}

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

				searchedCount++
				grepResults.WriteString(fmt.Sprintf("\n=== Grep Pattern: %s (Found %d matches) ===\n", pattern, len(results)))

				// 限制每个pattern的结果数量
				maxResultsPerPattern := 10
				displayCount := len(results)
				if displayCount > maxResultsPerPattern {
					displayCount = maxResultsPerPattern
				}

				for i := 0; i < displayCount; i++ {
					result := results[i]
					grepResults.WriteString(fmt.Sprintf("\n--- [%d] %s:%d ---\n", i+1, result.FileName, result.LineNumber))

					if len(result.ContextBefore) > 0 {
						for _, line := range result.ContextBefore {
							grepResults.WriteString(fmt.Sprintf("  %s\n", line))
						}
					}

					grepResults.WriteString(fmt.Sprintf(">>> %s\n", result.Line))

					if len(result.ContextAfter) > 0 {
						for _, line := range result.ContextAfter {
							grepResults.WriteString(fmt.Sprintf("  %s\n", line))
						}
					}
				}

				if len(results) > maxResultsPerPattern {
					grepResults.WriteString(fmt.Sprintf("\n... (%d more results not shown)\n", len(results)-maxResultsPerPattern))
				}
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
			emitter.EmitThoughtStream(task.GetIndex(), "Searching for relevant code examples using semantic questions...")

			maxQuestions := 4 // 最多搜索4个问题
			if len(semanticQuestions) > maxQuestions {
				semanticQuestions = semanticQuestions[:maxQuestions]
			}

			topN := 20            // 每个问题返回20个结果
			scoreThreshold := 0.3 // 相似度阈值

			type ResultKey struct {
				DocID string
			}
			allResultsMap := make(map[ResultKey]rag.SearchResult)

			for idx, question := range semanticQuestions {
				if question == "" {
					continue
				}

				log.Infof("semantic searching question %d/%d: %s", idx+1, len(semanticQuestions), question)

				// 执行语义搜索
				results, err := ragSearcher.QueryTopN(question, topN, scoreThreshold)
				if err != nil {
					log.Errorf("semantic search failed for question '%s': %v", question, err)
					continue
				}

				log.Infof("semantic search found %d results for question: %s", len(results), question)

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
					}
				}
			}

			// 将 map 转换为切片并格式化
			if len(allResultsMap) > 0 {
				var ragResults strings.Builder
				ragResults.WriteString(fmt.Sprintf("\n=== Semantic Search Results (Found %d unique matches) ===\n", len(allResultsMap)))

				// 限制显示数量
				maxDisplay := 15
				displayCount := 0
				for _, result := range allResultsMap {
					if displayCount >= maxDisplay {
						break
					}

					ragResults.WriteString(fmt.Sprintf("\n--- [%d] Score: %.3f ---\n", displayCount+1, result.Score))

					var content string
					if result.KnowledgeBaseEntry != nil {
						content = result.KnowledgeBaseEntry.KnowledgeDetails
					} else if result.Document != nil {
						content = result.Document.Content
					}

					// 限制单个结果的长度
					if len(content) > 800 {
						content = content[:800] + "\n[... content truncated ...]"
					}

					ragResults.WriteString(content)
					ragResults.WriteString("\n")
					displayCount++
				}

				if len(allResultsMap) > maxDisplay {
					ragResults.WriteString(fmt.Sprintf("\n... (%d more results not shown)\n", len(allResultsMap)-maxDisplay))
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

			// 使用压缩功能精选代码片段，明确目标是保持与用户需求的相关性
			// userRequirements 已经包含了用户输入和 reason，作为上下文传递
			initialSamples = compressSearchResults(rawCombinedResults, searchQuery, userRequirements, r, nil, 8, 5, 20, "【精选初始代码样例】", true)

			if initialSamples != "" {
				emitter.EmitThoughtStream(task.GetIndex(), "Found and compressed relevant code samples:\n"+initialSamples)
				r.AddToTimeline("initial_code_samples", initialSamples)
				log.Infof("initial samples collected and compressed successfully, final size: %d bytes", len(initialSamples))
			}
		} else {
			log.Infof("no search results found from any searcher")
		}

		// Step 3: 处理文件路径
		if !createNewFile || existed != "" {
			targetPath := existed
			log.Infof("identified target path: %s", targetPath)
			filename := utils.GetFirstExistedFile(targetPath)
			if filename == "" {
				createFileErr := os.WriteFile(targetPath, []byte(""), 0644)
				if createFileErr != nil {
					return utils.Errorf("not found existed file and cannot create file to disk, failed: %v", createFileErr)
				}
				filename = targetPath
			}
			content, _ := os.ReadFile(targetPath)
			if len(content) > 0 {
				log.Infof("identified target file: %s, file size: %v", targetPath, len(content))
				loop.Set("full_code", string(content))
			}
			emitter.EmitPinFilename(filename)
			loop.Set("filename", filename)
			return nil
		}

		// 创建新文件
		filename := r.EmitFileArtifactWithExt("gen_code", ".yak", "")
		emitter.EmitPinFilename(filename)
		loop.Set("filename", filename)
		return nil
	}
}
