package loop_syntaxflow_rule

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/ziputil"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// buildInitTask creates the initialization task handler with file detection and initial rule sample search
func buildInitTask(r aicommon.AIInvokeRuntime, docSearcher *ziputil.ZipGrepSearcher, ragSearcher *rag.RAGSystem) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
		emitter := r.GetConfig().GetEmitter()

		// Step 1: 分析用户需求，生成搜索关键字和判断文件路径
		log.Infof("init task step 1: analyzing user requirements and generating search patterns for SyntaxFlow rules")

		hasSearcher := docSearcher != nil || ragSearcher != nil

		promptTemplate := `
你的目标是分析用户需求，完成以下任务：

【任务1：判断文件操作类型】
判断这是创建新规则文件还是修改已有规则文件：
- 如果用户明确提到文件路径（如"修改 /tmp/rule.sf"），则是修改已有文件
- 如果用户只描述漏洞检测需求，没有提到具体文件，则是创建新文件

【任务1.5：判断是否提供漏洞样例并提取】
若用户消息中包含漏洞代码样例（如 markdown 代码块、以 go/java/php 等语言标注的代码块），则 has_code_sample=true，并必须提取：
- extracted_sample_code: 代码块内的原始代码（不含 markdown 代码块标记包裹）
- sample_language: 根据代码块语言标注推断，golang/java/php/c/javascript/yak/python
- sample_filename: 虚拟文件名且带正确后缀，如 handler.go、Main.java、vuln.php。Go 代码常用 xxx.go，Java 用 Main.java 或 XxxController.java
若没有代码块或仅为描述性需求，则 has_code_sample=false，上述三字段留空。
{{ if .hasGrepSearcher }}
【任务2：生成精确规则搜索关键字(Grep模式)】
根据用户需求，生成 2-4 个搜索模式（search_patterns），用于在 SyntaxFlow 规则样例库中进行精确文本搜索：

搜索模式类型：
1. 规则名搜索：如 "rule\\(\"sql\", "rule\\(\"xss"
2. 关键词搜索：如 "SQL注入", "XSS", "SSRF"
3. 语法搜索：如 "dataflow\\.", "desc\\(", "sink\\("
4. 若用户提供了漏洞样例(has_code_sample=true)，建议增加能命中带测试用例的规则示例：如 "'file://", "alert_high", "alert_mid", "desc\\(.*lang:"

注意事项：
- 优先使用规则语法相关搜索（使用 \\. 转义点号、\\( 转义括号）
- 每个pattern要具体且相关，避免过于宽泛
- 如果涉及多种漏洞类型，可以为每种生成一个pattern
{{ end }}{{ if .hasRAGSearcher }}
【任务{{ if .hasGrepSearcher }}3{{ else }}2{{ end }}：生成语义搜索问题(RAG向量搜索)】
根据用户需求，生成 2-4 个完整的问题（semantic_questions），用于语义向量搜索相关 SyntaxFlow 规则样例：

问题格式要求：
1. 必须是完整的主谓宾句式
2. 禁止使用代词（它、这个、那个等）
3. 明确指明 SyntaxFlow 或规则
4. 每个问题要从不同角度描述漏洞检测需求

问题示例：
✅ Good: "SyntaxFlow中如何检测SQL注入漏洞？"
✅ Good: "SyntaxFlow中如何编写dataflow数据流追踪规则？"
✅ Good: "如何用SyntaxFlow检测XSS跨站脚本？"
✅ Good: "SyntaxFlow规则如何定义source和sink？"
❌ Bad: "如何检测？" - 缺少主语
❌ Bad: "它怎么写？" - 使用代词
❌ Bad: "SQL注入" - 不完整句式
{{ end }}
<|USER_INPUT_{{ .nonce }}|>
{{ .data }}
<|USER_INPUT_END_{{ .nonce }}|>
`

		toolOptions := []aitool.ToolOption{
			aitool.WithBoolParam("create_new_file", aitool.WithParam_Description("Is this task to create a new rule file or modify an existing file? If user mentions specific file path, set to false."), aitool.WithParam_Required(true)),
			aitool.WithStringParam("existed_filepath", aitool.WithParam_Description("Only when create_new_file is false. The .sf file path to modify.")),
			aitool.WithBoolParam("has_code_sample", aitool.WithParam_Description("True if user provided vulnerability code sample (e.g. markdown code block). When true, must extract sample, save to file, embed in rule, and call verify-syntaxflow-rule-against-sample.")),
			aitool.WithStringParam("extracted_sample_code", aitool.WithParam_Description("When has_code_sample=true. The raw vulnerability code extracted from user's markdown code block, without the ``` wrapper.")),
			aitool.WithStringParam("sample_language", aitool.WithParam_Description("When has_code_sample=true. Language: golang, java, php, c, javascript, yak, python.")),
			aitool.WithStringParam("sample_filename", aitool.WithParam_Description("When has_code_sample=true. Virtual filename for the sample, e.g. vuln.go, handler.go, Main.java. Should have correct extension for the language.")),
		}
		if docSearcher != nil {
			toolOptions = append(toolOptions,
				aitool.WithStringArrayParam("search_patterns", aitool.WithParam_Description("2-4 search patterns for finding relevant SyntaxFlow rule examples. Each pattern should be a regex or keyword."), aitool.WithParam_Required(false)),
			)
		}
		if ragSearcher != nil {
			toolOptions = append(toolOptions,
				aitool.WithStringArrayParam("semantic_questions", aitool.WithParam_Description("2-4 complete questions for semantic search of SyntaxFlow rule examples. Each question must be a complete sentence with subject-predicate-object structure and explicitly mention 'SyntaxFlow' or rule."), aitool.WithParam_Required(false)),
			)
		}
		if hasSearcher {
			toolOptions = append(toolOptions,
				aitool.WithStringParam("reason", aitool.WithParam_Description("Explain your decision and why these search patterns/questions are chosen."), aitool.WithParam_Required(false)),
			)
		}

		renderedPrompt := utils.MustRenderTemplate(
			promptTemplate,
			map[string]any{
				"nonce":           utils.RandStringBytes(4),
				"data":            task.GetUserInput(),
				"hasGrepSearcher": docSearcher != nil,
				"hasRAGSearcher":  ragSearcher != nil,
			})

		forgeOptions := []aicommon.GeneralKVConfigOption{}
		if hasSearcher {
			forgeOptions = append(forgeOptions, aicommon.WithGeneralConfigStreamableFieldWithNodeId("init-search-rule-sample", "reason"))
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

		createNewFile := step1Result.GetBool("create_new_file")
		existed := step1Result.GetString("existed_filepath")
		reason := step1Result.GetString("reason")
		hasCodeSample := step1Result.GetBool("has_code_sample")
		extractedSampleCode := step1Result.GetString("extracted_sample_code")
		sampleLanguage := step1Result.GetString("sample_language")
		sampleFilename := step1Result.GetString("sample_filename")
		searchPatterns := step1Result.GetStringSlice("search_patterns")
		semanticQuestions := step1Result.GetStringSlice("semantic_questions")

		// 当用户提供漏洞样例时：保存到文件，供规则嵌入和 verify 工具使用
		if hasCodeSample && extractedSampleCode != "" {
			lang, _ := ssaconfig.ValidateLanguage(sampleLanguage)
			ext := lang.GetFileExt()
			if ext == "" {
				ext = ".go"
			}
			if sampleFilename == "" || !strings.Contains(sampleFilename, ".") {
				sampleFilename = "sample" + ext
			}
			samplePath := r.EmitFileArtifactWithExt("gen_sample", ext, extractedSampleCode)
			loop.Set("sf_sample_filepath", samplePath)
			loop.Set("sf_sample_code", extractedSampleCode)
			loop.Set("sf_sample_language", sampleLanguage)
			loop.Set("sf_sample_filename", sampleFilename)
			r.AddToTimeline("vulnerability_sample", fmt.Sprintf("【漏洞样例】\n保存路径: %s\n虚拟文件名: %s\n语言: %s\n\n代码内容:\n%s", samplePath, sampleFilename, sampleLanguage, extractedSampleCode))
			log.Infof("saved vulnerability sample to %s (lang=%s, virtual_filename=%s)", samplePath, sampleLanguage, sampleFilename)
		}
		for _, question := range semanticQuestions {
			emitter.EmitDefaultStreamEvent("thought", bytes.NewBufferString(question), task.GetIndex())
		}
		if len(searchPatterns) > 0 {
			emitter.EmitDefaultStreamEvent("thought", bytes.NewBufferString(strings.Join(searchPatterns, ",")), task.GetIndex())
		}

		sampleHint := ""
		if hasCodeSample {
			embedFilename := sampleFilename
			if embedFilename == "" {
				embedFilename = "sample.go"
			}
			embedLang := sampleLanguage
			if embedLang == "" {
				embedLang = "golang"
			}
			sampleHint = "\n【重要】用户提供了漏洞代码样例，已保存到 sf_sample_filepath。生成规则时务必：\n" +
				"1) 测试样例必须放在**规则末尾的第二个 desc() 块**中（第一个 desc 仅含 title/type/level 等元数据，不含 file://）。参考 golang-template-ssti.sf、golang-reflected-xss-gin-context.sf 的结构：\n" +
				"   desc(lang: " + embedLang + ", alert_high: 1, 'file://" + embedFilename + "': <<<UNSAFE\n<样例代码完整内容>\nUNSAFE)\n" +
				"2) 生成后必须调用 verify-syntaxflow-rule-against-sample 工具（path=sf_filename, sample_code=sf_sample_code, filename=sf_sample_filename, language=sf_sample_language）验证。若 matched=false，需 modify_rule 修复后再次验证直至 matched=true。"
		}
		var userRequirements = utils.MustRenderTemplate(`<|USER_REQUIREMENTS_{{.nonce}}|>
{{.data}}
---
{{.reason}}{{.sample_hint}}
<|USER_REQUIREMENTS_END_{{.nonce}}|>
`, map[string]any{
			"data":        task.GetUserInput(),
			"reason":      reason,
			"sample_hint": sampleHint,
			"nonce":       utils.RandStringBytes(4),
		})

		log.Infof("identified create_new_file: %v, has_code_sample: %v, search_patterns count: %d, semantic_questions count: %d",
			createNewFile, hasCodeSample, len(searchPatterns), len(semanticQuestions))

		if hasCodeSample {
			loop.Set("sf_has_code_sample", true)
			emitter.EmitDefaultStreamEvent("thought", bytes.NewBufferString("用户提供了漏洞代码样例，生成规则后需调用 verify-syntaxflow-rule-against-sample 验证匹配。"), task.GetIndex())
		}

		// Step 2: 执行规则样例搜索（Grep + RAG 语义搜索）
		var initialSamples string
		var allSearchResults strings.Builder

		if docSearcher != nil && len(searchPatterns) > 0 {
			log.Infof("init task step 2.1: grep searching rule samples with %d patterns", len(searchPatterns))
			loop.LoadingStatus("开始搜索相关规则样例... / Searching for relevant rule examples...")

			var grepResults strings.Builder
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
							singleResult.WriteString(text)
							grepResults.WriteString(text)
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

		if ragSearcher != nil && len(semanticQuestions) > 0 {
			log.Infof("init task step 2.2: semantic searching rule samples with %d questions", len(semanticQuestions))
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
						singleResult.WriteString(result.GetContent())
					}
				}
				emitter.EmitTextReferenceMaterial(singleResultStreamId, singleResult.String())
				pw.Close()
			}

			if len(allResultsMap) > 0 {
				// Re-rank: 提高可执行规则(.sf)权重，降低对纯 desc 长文本的依赖
				type scoredResult struct {
					result rag.SearchResult
					score  float64
				}
				scored := make([]scoredResult, 0, len(allResultsMap))
				for _, result := range allResultsMap {
					content := result.GetContent()
					boost := 0.0
					// 1. 元数据标记为可执行规则 -> 显著加分
					if result.Document != nil && result.Document.Metadata != nil {
						if st, ok := result.Document.Metadata["search_type"]; ok && utils.InterfaceToString(st) == "SyntaxFlow可执行规则" {
							boost += 0.25
						}
					}
					// 2. 内容包含可执行语法(desc、#->、alert等) -> 加分
					hasDesc := strings.Contains(content, "desc(")
					hasFlow := strings.Contains(content, "#->") || strings.Contains(content, "alert")
					if hasDesc && hasFlow {
						boost += 0.15
					} else if hasFlow || hasDesc {
						boost += 0.05
					}
					// 3. 纯长文档且无可执行语法 -> 减分，减少对长 desc 的过度依赖
					if len(content) > 1500 && !hasDesc && !hasFlow {
						boost -= 0.12
					}
					scored = append(scored, scoredResult{result, result.Score + boost})
				}
				sort.Slice(scored, func(i, j int) bool { return scored[i].score > scored[j].score })

				var ragResults strings.Builder
				ragResults.WriteString(fmt.Sprintf("\n=== Semantic Search Results (Found %d unique matches) ===\n", len(scored)))

				displayCount := 0
				for _, sr := range scored {
					result := sr.result
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
				allSearchResults.WriteString(rawResults)
			} else {
				log.Infof("no semantic search results found for any question")
			}
		}

		// Step 2.3: 合并并压缩所有搜索结果
		if allSearchResults.Len() > 0 {
			rawCombinedResults := allSearchResults.String()
			log.Infof("total collected %d bytes of combined search results, attempting compression", len(rawCombinedResults))

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

			ctx := task.GetContext()
			compressedSamples, err := r.CompressLongTextWithDestination(ctx, rawCombinedResults, searchQuery, 10*1024)
			if err != nil {
				log.Warnf("failed to compress search results: %v, using raw results", err)
				initialSamples = utils.ShrinkTextBlock(rawCombinedResults, 10*1024)
			} else {
				initialSamples = compressedSamples
			}

			if initialSamples != "" {
				if event, _ := emitter.EmitThoughtStream(task.GetIndex(), "压缩完成，压缩后样本大小为: "+utils.ByteSize(uint64(len(initialSamples)))); event != nil {
					emitter.EmitTextReferenceMaterial(event.GetStreamEventWriterId(), initialSamples)
				}
				r.AddToTimeline("initial_rule_samples", initialSamples)
				log.Infof("initial rule samples collected and compressed successfully, final size: %d bytes", len(initialSamples))
			}
		} else {
			log.Infof("no search results found from any searcher")
		}

		// Step 3: 处理文件路径
		if existed != "" {
			targetPath := existed
			log.Infof("identified target path: %s", targetPath)
			filename := utils.GetFirstExistedFile(targetPath)
			if filename == "" {
				createFileErr := os.WriteFile(targetPath, []byte(""), 0644)
				if createFileErr != nil {
					operator.Failed(utils.Errorf("cannot create file to disk, failed: %v", createFileErr))
					return
				}
				filename = targetPath
			}
			content, _ := os.ReadFile(targetPath)
			if len(content) > 0 {
				log.Infof("identified target file: %s, file size: %v", targetPath, len(content))
				loop.Set("full_sf_code", string(content))
			}
			emitter.EmitPinFilename(filename)
			loop.Set("sf_filename", filename)
			operator.Continue()
			return
		}

		filename := r.EmitFileArtifactWithExt("gen_code", ".sf", "")
		emitter.EmitPinFilename(filename)
		loop.Set("sf_filename", filename)
		operator.Continue()
	}
}
