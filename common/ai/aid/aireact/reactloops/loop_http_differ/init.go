package loop_http_differ

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

//go:embed prompts/persistent_instruction.txt
var instruction string

//go:embed prompts/reactive_data.txt
var reactiveData string

//go:embed prompts/reflection_output_example.txt
var outputExample string

// LoopHTTPDifferName is the name of the HTTP differ loop
const LoopHTTPDifferName = "http_differ"

func init() {
	err := reactloops.RegisterLoopFactory(
		LoopHTTPDifferName,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			// 创建预设选项
			preset := []reactloops.ReActLoopOption{
				reactloops.WithAllowRAG(false),
				reactloops.WithAllowToolCall(true),
				reactloops.WithInitTask(buildInitTask(r)),
				reactloops.WithMaxIterations(int(r.GetConfig().GetMaxIterationCount())),
				reactloops.WithAllowUserInteract(r.GetConfig().GetAllowUserInteraction()),
				reactloops.WithPersistentInstruction(instruction),
				reactloops.WithReflectionOutputExample(outputExample),
				reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
					originalRequest := loop.Get("original_request")
					lastRequest := loop.Get("last_request")
					lastResponse := loop.Get("last_response")
					diffResult := loop.Get("diff_result")
					securityKnowledge := loop.Get("security_knowledge")

					renderMap := map[string]any{
						"OriginalRequest":   originalRequest,
						"LastRequest":       lastRequest,
						"LastResponse":      lastResponse,
						"DiffResult":        diffResult,
						"SecurityKnowledge": securityKnowledge,
						"Nonce":             nonce,
						"FeedbackMessages":  feedbacker.String(),
					}
					return utils.RenderTemplate(reactiveData, renderMap)
				}),
				// Register set_http_request action (must be called first)
				setHTTPRequestAction(r),
				// Register fuzz actions
				fuzzMethodAction(r),
				fuzzPathAction(r),
				fuzzHeaderAction(r),
				fuzzGetParamsAction(r),
				fuzzBodyAction(r),
				fuzzCookieAction(r),
			}
			preset = append(preset, opts...)
			return reactloops.NewReActLoop(LoopHTTPDifferName, r, preset...)
		},
		reactloops.WithLoopDescription("HTTP request fuzzing and response diff analysis for security testing"),
		reactloops.WithLoopUsagePrompt("Use when user wants to fuzz HTTP requests and analyze response differences. First use 'set_http_request' to set the target request, then use fuzz actions (fuzz_method, fuzz_path, fuzz_header, fuzz_get_params, fuzz_body, fuzz_cookie) to test"),
		reactloops.WithLoopOutputExample(`
* When user requests to fuzz HTTP request:
  {"@action": "http_differ", "human_readable_thought": "I need to fuzz HTTP request parameters to find vulnerabilities"}
`),
	)
	if err != nil {
		log.Errorf("register reactloop: %v failed: %v", LoopHTTPDifferName, err)
	}
}

// defaultSecurityKBCollectionName is the default collection name for security knowledge base
var defaultSecurityKBCollectionName = "security_testing_kb"

// buildInitTask creates the initialization task handler
func buildInitTask(r aicommon.AIInvokeRuntime) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) error {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) error {
		emitter := r.GetConfig().GetEmitter()
		config := r.GetConfig()

		// Step 1: 分析用户需求，生成搜索关键字和语义问题
		log.Infof("init task step 1: analyzing user requirements and generating search patterns")

		analysisPrompt := `
你的目标是分析用户的安全测试需求，完成以下任务：

【任务1：生成精确搜索关键词】
根据用户需求，生成 2-5 个搜索关键词（search_keywords），用于在安全知识库中进行精确文本搜索：

关键词类型：
1. 漏洞类型关键词：如 "SQL注入", "XSS", "SSRF", "命令注入", "路径遍历"
2. 具体技术关键词：如 "字符型注入", "数字型注入", "存储型XSS", "反射型XSS", "DOM XSS", "盲注", "报错注入"
3. 绕过技术关键词：如 "WAF绕过", "编码绕过", "双写绕过", "大小写绕过"
4. Payload关键词：如 "union select", "img onerror", "script alert", "sleep注入"

注意事项：
- 优先使用具体的漏洞类型关键词
- 如果用户提到具体漏洞类型，生成该类型的具体技术关键词
- 如果用户没有明确漏洞类型，生成通用的安全测试关键词

【任务2：生成语义搜索问题】
根据用户需求，生成 2-4 个完整的问题（semantic_questions），用于语义向量搜索相关安全知识：

问题格式要求：
1. 必须是完整的主谓宾句式
2. 禁止使用代词（它、这个、那个等）
3. 每个问题要从不同角度描述需求
4. 问题要具体，涉及具体的漏洞类型和测试技术

问题示例：
✅ Good: "如何检测GET参数中的字符型SQL注入漏洞？"
✅ Good: "img标签的onerror属性如何触发XSS攻击？"
✅ Good: "Cookie参数中的SQL注入有哪些常见payload？"
✅ Good: "如何绕过WAF进行SQL注入测试？"
✅ Good: "JSON请求体中如何检测命令注入漏洞？"
✅ Good: "时间盲注的payload有哪些？"
✅ Good: "反射型XSS和存储型XSS有什么区别？"
❌ Bad: "如何注入？" - 太笼统
❌ Bad: "它怎么绕过？" - 使用代词
❌ Bad: "XSS" - 不完整句式

<|USER_INPUT_{{ .nonce }}|>
{{ .userInput }}
<|USER_INPUT_END_{{ .nonce }}|>
`

		renderedPrompt := utils.MustRenderTemplate(analysisPrompt, map[string]any{
			"nonce":     utils.RandStringBytes(4),
			"userInput": task.GetUserInput(),
		})

		result, err := r.InvokeLiteForge(
			task.GetContext(),
			"analyze-user-requirements",
			renderedPrompt,
			[]aitool.ToolOption{
				aitool.WithStringArrayParam("search_keywords", aitool.WithParam_Description("2-5 search keywords for finding relevant security knowledge. Each keyword should be specific to vulnerability types or attack techniques.")),
				aitool.WithStringArrayParam("semantic_questions", aitool.WithParam_Description("2-4 complete questions for semantic search. Each question must be a complete sentence describing specific security testing needs.")),
				aitool.WithStringParam("analysis_summary", aitool.WithParam_Description("Summary of the user's security testing requirements and recommended approach")),
			},
			aicommon.WithGeneralConfigStreamableFieldWithNodeId("init-analyze-requirements", "analysis_summary"),
		)

		if err != nil {
			log.Warnf("failed to analyze user requirements: %v", err)
			return nil
		}

		searchKeywords := result.GetStringSlice("search_keywords")
		semanticQuestions := result.GetStringSlice("semantic_questions")
		analysisSummary := result.GetString("analysis_summary")

		// Emit analysis results
		if analysisSummary != "" {
			emitter.EmitThoughtStream(task.GetIndex(), "Requirements Analysis:\n"+analysisSummary)
			r.AddToTimeline("requirements_analysis", analysisSummary)
		}

		log.Infof("identified search_keywords: %d, semantic_questions: %d", len(searchKeywords), len(semanticQuestions))

		// Step 2: 执行知识库搜索
		var allSearchResults strings.Builder
		db := config.GetDB()

		// Step 2.1: 执行关键词搜索
		if db != nil && len(searchKeywords) > 0 {
			log.Infof("init task step 2.1: keyword searching security knowledge with %d keywords", len(searchKeywords))
			emitter.EmitThoughtStream(task.GetIndex(), "Searching security knowledge base with keywords...")

			keywordResults := searchByKeywords(db, searchKeywords)
			if keywordResults != "" {
				allSearchResults.WriteString(keywordResults)
			}
		}

		// Step 2.2: 执行语义搜索
		if db != nil && len(semanticQuestions) > 0 {
			log.Infof("init task step 2.2: semantic searching security knowledge with %d questions", len(semanticQuestions))
			emitter.EmitThoughtStream(task.GetIndex(), "Searching security knowledge base with semantic questions...")

			semanticResults := searchBySemantic(db, defaultSecurityKBCollectionName, semanticQuestions)
			if semanticResults != "" {
				allSearchResults.WriteString(semanticResults)
			}
		}

		// Step 3: 存储搜索结果
		if allSearchResults.Len() > 0 {
			searchResultsStr := allSearchResults.String()
			log.Infof("collected %d bytes of security knowledge", len(searchResultsStr))

			// 限制结果大小
			maxSize := 20 * 1024 // 20KB
			if len(searchResultsStr) > maxSize {
				searchResultsStr = searchResultsStr[:maxSize] + "\n\n[... 内容已截断 ...]"
			}

			loop.Set("security_knowledge", searchResultsStr)
			r.AddToTimeline("security_knowledge", fmt.Sprintf("Found security knowledge (%d bytes)", len(searchResultsStr)))
			emitter.EmitThoughtStream(task.GetIndex(), "Found relevant security knowledge:\n"+utils.ShrinkTextBlock(searchResultsStr, 500))
		}

		log.Infof("http_differ loop initialized successfully")
		return nil
	}
}

// searchByKeywords searches knowledge base by keywords
func searchByKeywords(db *gorm.DB, keywords []string) string {
	if db == nil || len(keywords) == 0 {
		return ""
	}

	var results strings.Builder
	results.WriteString("\n=== Keyword Search Results ===\n")

	foundCount := 0
	seenIDs := make(map[uint]bool) // 用于去重

	for _, keyword := range keywords {
		if keyword == "" {
			continue
		}

		// 使用现有的 API 搜索知识库
		filter := &ypb.SearchKnowledgeBaseEntryFilter{
			Keyword: keyword,
		}
		paging := &ypb.Paging{
			Page:  1,
			Limit: 5,
		}

		_, entries, err := yakit.QueryKnowledgeBaseEntryPaging(db, filter, paging)
		if err != nil {
			log.Warnf("keyword search failed for '%s': %v", keyword, err)
			continue
		}

		if len(entries) > 0 {
			results.WriteString(fmt.Sprintf("\n--- Keyword: %s (Found %d) ---\n", keyword, len(entries)))
			for i, entry := range entries {
				// 去重
				if seenIDs[entry.ID] {
					continue
				}
				seenIDs[entry.ID] = true

				results.WriteString(fmt.Sprintf("[%d] %s\n", i+1, entry.KnowledgeTitle))
				content := entry.KnowledgeDetails
				if len(content) > 500 {
					content = content[:500] + "..."
				}
				results.WriteString(content + "\n\n")
				foundCount++
			}
		}
	}

	if foundCount == 0 {
		return ""
	}

	results.WriteString("=== End of Keyword Search Results ===\n")
	return results.String()
}

// searchBySemantic searches knowledge base by semantic questions
func searchBySemantic(db *gorm.DB, collectionName string, questions []string) string {
	if db == nil || len(questions) == 0 {
		return ""
	}

	ragSys, err := rag.GetRagSystem(collectionName, rag.WithDB(db))
	if err != nil {
		log.Warnf("RAG system not available: %v", err)
		return ""
	}

	var results strings.Builder
	results.WriteString("\n=== Semantic Search Results ===\n")

	// 使用 map 去重
	type ResultKey struct {
		DocID string
	}
	allResultsMap := make(map[ResultKey]rag.SearchResult)

	for _, question := range questions {
		if question == "" {
			continue
		}

		log.Infof("semantic searching: %s", question)

		searchResults, err := ragSys.QueryTopN(question, 10, 0.3)
		if err != nil {
			log.Warnf("semantic search failed for '%s': %v", question, err)
			continue
		}

		for _, result := range searchResults {
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

	if len(allResultsMap) == 0 {
		return ""
	}

	results.WriteString(fmt.Sprintf("Found %d unique matches:\n\n", len(allResultsMap)))

	displayCount := 0
	maxDisplay := 10
	for _, result := range allResultsMap {
		if displayCount >= maxDisplay {
			break
		}

		results.WriteString(fmt.Sprintf("--- [%d] Score: %.3f ---\n", displayCount+1, result.Score))

		var content string
		if result.KnowledgeBaseEntry != nil {
			results.WriteString(fmt.Sprintf("Title: %s\n", result.KnowledgeBaseEntry.KnowledgeTitle))
			content = result.KnowledgeBaseEntry.KnowledgeDetails
		} else if result.Document != nil {
			content = result.Document.Content
		}

		if len(content) > 800 {
			content = content[:800] + "\n[... content truncated ...]"
		}

		results.WriteString(content + "\n\n")
		displayCount++
	}

	if len(allResultsMap) > maxDisplay {
		results.WriteString(fmt.Sprintf("\n... (%d more results not shown)\n", len(allResultsMap)-maxDisplay))
	}

	results.WriteString("=== End of Semantic Search Results ===\n")
	return results.String()
}
