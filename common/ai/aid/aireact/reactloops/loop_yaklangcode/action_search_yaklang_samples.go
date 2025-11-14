package loop_yaklangcode

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

// Universal compress function for search results
func compressRAGSearchResults(resultStr string, searchQuery string, invoker aicommon.AIInvokeRuntime, op *reactloops.LoopActionHandlerOperator, maxRanges int, minLines int, maxLines int, title string) string {
	if len(resultStr) == 0 {
		return resultStr
	}

	resultEditor := memedit.NewMemEditor(resultStr)
	dNonce := utils.RandStringBytes(4)

	promptTemplate := `
<|RAG_RESULT_{{ .nonce }}|>
{{ .samples }}
<|RAG_RESULT_END_{{ .nonce }}|>

<|INSTRUCT_{{ .nonce }}|>
【智能代码片段提取与排序】

请从上述向量搜索结果中提取最有价值的代码片段，按重要性排序：

【提取要求】
1. 最多提取 %d 个代码片段
2. 每个片段 %d-%d 行，确保上下文完整
3. 按重要性从高到低排序（rank: 1最重要，数字越大越不重要）

【重要性评判标准】（按优先级排序）
🔥 最高优先级 (rank 1-3)：
- 完整的函数调用示例 + 错误处理
- 包含关键参数配置的典型用法
- 展示核心API调用模式的代码

⭐ 高优先级 (rank 4-6)：
- 包含重要配置或选项的示例
- 展示常见使用场景的代码
- 有详细注释说明的关键代码

📝 中等优先级 (rank 7-10)：
- 辅助功能或工具函数调用
- 简单的变量赋值或初始化
- 补充性的代码片段

【输出格式】
返回JSON数组，每个元素包含：
{
  "range": "start-end",
  "rank": 数字(1-10),
  "reason": "选择理由"
}

【严格要求】
- 总行数控制在80行以内
- 避免重复或相似的代码片段
- 优先选择能独立理解的完整代码块
- 确保每个片段都有实际参考价值

请按重要性排序输出ranges数组。
<|INSTRUCT_END_{{ .nonce }}|>
`

	materials, err := utils.RenderTemplate(fmt.Sprintf(promptTemplate, maxRanges, minLines, maxLines), map[string]any{
		"nonce":       dNonce,
		"samples":     utils.PrefixLinesWithLineNumbers(resultStr),
		"searchQuery": searchQuery,
	})

	if err != nil {
		log.Errorf("compressRAGSearchResults: template render failed: %v", err)
		return resultStr
	}

	var context = invoker.GetConfig().GetContext()
	if op != nil {
		context = op.GetTask().GetContext()
	}

	forgeResult, err := invoker.InvokeLiteForge(
		context,
		"extract-ranked-lines",
		materials,
		[]aitool.ToolOption{
			aitool.WithStructArrayParam(
				"ranges",
				[]aitool.PropertyOption{
					aitool.WithParam_Description("按重要性排序的代码片段范围数组"),
				},
				nil,
				aitool.WithStringParam("range", aitool.WithParam_Description("行范围，格式: start-end")),
				aitool.WithIntegerParam("rank", aitool.WithParam_Description("重要性排序，1最重要，数字越大越不重要")),
				aitool.WithStringParam("reason", aitool.WithParam_Description("选择此片段的理由")),
			),
		},
		aicommon.WithGeneralConfigStreamableField("reason"),
	)

	if err != nil {
		log.Errorf("compressRAGSearchResults: forge failed: %v", err)
		return resultStr
	}

	if forgeResult == nil {
		log.Warnf("compressRAGSearchResults: forge result is nil")
		return resultStr
	}

	rangeItems := forgeResult.GetInvokeParamsArray("ranges")

	if len(rangeItems) == 0 {
		log.Warnf("compressRAGSearchResults: no ranges extracted")
		return resultStr
	}

	// 提取并排序代码片段
	type RankedRange struct {
		Range  string
		Rank   int
		Reason string
		Text   string
	}

	var rankedRanges []RankedRange
	totalLines := 0

	for _, item := range rangeItems {
		rangeStr := item.GetString("range")
		rank := item.GetInt("rank")
		reason := item.GetString("reason")

		if rangeStr == "" {
			continue
		}

		// 解析行范围
		parts := strings.Split(rangeStr, "-")
		if len(parts) != 2 {
			log.Warnf("compressRAGSearchResults: invalid range format: %s", rangeStr)
			continue
		}

		startLine, err1 := strconv.Atoi(parts[0])
		endLine, err2 := strconv.Atoi(parts[1])

		if err1 != nil || err2 != nil {
			log.Errorf("compressRAGSearchResults: parse range failed: %s, errors: %v, %v", rangeStr, err1, err2)
			continue
		}

		if startLine <= 0 || endLine < startLine {
			log.Warnf("compressRAGSearchResults: invalid range values: %s (start=%d, end=%d)", rangeStr, startLine, endLine)
			continue
		}

		// 提取文本
		text := resultEditor.GetTextFromPositionInt(startLine, 1, endLine, 1)
		if text == "" {
			log.Warnf("compressRAGSearchResults: empty text for range: %s", rangeStr)
			continue
		}

		lineCount := strings.Count(text, "\n") + 1
		if totalLines+lineCount > 100 {
			log.Warnf("compressRAGSearchResults: would exceed 100 lines limit, stopping at range: %s", rangeStr)
			break
		}

		rankedRanges = append(rankedRanges, RankedRange{
			Range:  rangeStr,
			Rank:   int(rank),
			Reason: reason,
			Text:   text,
		})

		totalLines += lineCount
	}

	if len(rankedRanges) == 0 {
		log.Warnf("compressRAGSearchResults: no valid ranges extracted")
		return resultStr
	}

	// 构建优化后的结果
	var result strings.Builder
	result.WriteString(title + "\n\n")

	for i, item := range rankedRanges {
		result.WriteString(fmt.Sprintf("=== [%d] 重要性排序: %d | 范围: %s ===\n", i+1, item.Rank, item.Range))
		if item.Reason != "" {
			result.WriteString(fmt.Sprintf("选择理由: %s\n", item.Reason))
		}
		result.WriteString(item.Text)
		result.WriteString("\n\n")
	}

	finalResult := result.String()

	// 手动截断超过100行的内容
	lines := strings.Split(finalResult, "\n")
	if len(lines) > 100 {
		log.Warnf("compressRAGSearchResults: result has %d lines, truncating to 100", len(lines))
		finalResult = strings.Join(lines[:100], "\n") + "\n\n[... 内容已截断，共提取了前100行最重要的代码片段 ...]"
	}

	log.Infof("compressRAGSearchResults: compressed from %d chars to %d chars, %d ranges",
		len(resultStr), len(finalResult), len(rankedRanges))

	return finalResult
}

// compressRAGResults is now a wrapper for compressRAGSearchResults with specific parameters for RAG search
func compressRAGResults(resultStr string, query string, invoker aicommon.AIInvokeRuntime, op *reactloops.LoopActionHandlerOperator) string {
	return compressRAGSearchResults(resultStr, query, invoker, op, 10, 3, 15, "【AI智能提取】按重要性排序的代码片段：")
}

var semanticSearchYaklangSamplesAction = func(r aicommon.AIInvokeRuntime, ragSystem *rag.RAGSystem) reactloops.ReActLoopOption {
	if ragSystem == nil {
		log.Warnf("semantic_search_yaklang_samples: ragSystem is nil")
		return func(r *reactloops.ReActLoop) {
			r.GetInvoker().AddToTimeline("semantic_search_yaklang_samples_error", "Yaklang AIKB RAG 系统未正确初始化或加载失败")
		}
	}
	return reactloops.WithRegisterLoopActionWithStreamField(
		"semantic_search_yaklang_samples",
		`语义搜索 Yaklang 代码样例库 - 基于向量语义理解搜索真实代码示例

核心原则：禁止臆造 Yaklang API！必须先通过语义向量搜索找到真实样例！

【强制使用场景】：
1. 编写任何代码前，先语义搜索相关函数用法
2. 遇到 API 错误（ExternLib don't has）时 - 必须立即语义搜索
3. 遇到语法错误（SyntaxError）时 - 必须立即语义搜索
4. 不确定函数参数或返回值时

【参数说明】：
- questions (必需) - 问题数组，支持多个具体问题同时搜索：
  * 每个问题必须是完整的主谓宾句式
  * 禁止使用代词（它、这个、那个等）
  * 问题风格示例：
    ✅ Good: "Yaklang中如何获取数组长度？"
    ✅ Good: "Yaklang中append函数如何使用？"
    ✅ Good: "Yaklang中如何配置默认的嵌入处理函数？"
    ❌ Bad: "如何获取长度？"（缺少主语）
    ❌ Bad: "它如何使用？"（使用代词）
    ❌ Bad: "配置嵌入"（不完整句式）
  * 复杂问题可拆解为多个问题：
    例如："Yaklang数组操作" 拆解为：
    - "Yaklang中如何创建数组？"
    - "Yaklang数组如何访问元素？"
    - "Yaklang中如何获取数组长度？"

- top_n (可选) - 每个问题返回结果数量，默认 30
  * 标准搜索：20-30（推荐，默认）
  * 深入研究：40-50
  * 快速预览：10-15

- score_threshold (可选) - 相似度阈值，默认 0.3
  * 余弦相似度范围：-1.0 到 1.0
  * 0.6-1.0：高置信度匹配（强烈推荐）
  * 0.4-0.6：中等置信度匹配（可接受）
  * 0.3-0.4：低置信度匹配（需谨慎）
  * 0.0-0.3：弱匹配（不推荐）

【使用示例】：
semantic_search_yaklang_samples(questions=["Yaklang中如何进行端口扫描？", "Yaklang中如何检测漏洞？"])
semantic_search_yaklang_samples(questions=["Yaklang中如何发送HTTP请求？"], top_n=40)
semantic_search_yaklang_samples(questions=["Yaklang中如何处理错误？", "Yaklang中如何捕获异常？"], score_threshold=0.5)

记住：Yaklang 是 DSL！每个 API 都可能与 Python/Go 不同！
先语义搜索找样例，再写代码，节省 90% 调试时间！`,
		[]aitool.ToolOption{
			aitool.WithStructArrayParam(
				"questions",
				[]aitool.PropertyOption{
					aitool.WithParam_Required(true),
					aitool.WithParam_Description(`问题数组（必需）- 支持多个具体问题同时搜索。

【问题格式要求】：
1. 必须是完整的主谓宾句式
2. 禁止使用代词（它、这个、那个等）
3. 明确指明 Yaklang 语言

【Good Cases - 正确示例】：
✅ "Yaklang中如何获取数组长度？"
✅ "Yaklang中append函数如何使用？"
✅ "Yaklang中如何配置默认的嵌入处理函数？"
✅ "Yaklang中生产环境嵌入请求如何添加TOTP验证头？"
✅ "Yaklang中如何递归读取ZIP文件内容？"
✅ "Yaklang中如何处理嵌入响应的JSON解析错误？"

【Bad Cases - 错误示例】：
❌ "如何获取长度？" - 缺少主语（Yaklang）
❌ "它如何使用？" - 使用代词
❌ "配置嵌入" - 不完整句式
❌ "数组操作" - 过于宽泛，应拆解为多个具体问题

【拆解复杂问题】：
复杂需求应拆解为多个具体问题：
例如："Yaklang数组综合复杂操作" 应拆解为：
- "Yaklang中如何创建数组？"
- "Yaklang数组如何访问元素？"
- "Yaklang中如何获取数组长度？"`),
				},
				nil,
				aitool.WithStringParam("question", aitool.WithParam_Description("具体的问题，必须是完整主谓宾句式")),
			),
			aitool.WithIntegerParam(
				"top_n",
				aitool.WithParam_Description(`每个问题返回结果数量（默认 30）- 控制每个问题返回的代码片段数量：
• 10-15: 快速预览核心用法
• 20-30: 标准搜索（推荐，默认）
• 40-50: 深入研究完整实现

注意：多个问题的结果会合并去重后返回`),
			),
			aitool.WithNumberParam(
				"score_threshold",
				aitool.WithParam_Description(`相似度阈值（默认 0.3）- 基于余弦相似度过滤结果：

【评分范围】：-1.0 到 1.0（余弦相似度）
• 0.6-1.0: 高置信度匹配 - 强烈推荐使用
• 0.4-0.6: 中等置信度匹配 - 可接受
• 0.3-0.4: 低置信度匹配 - 需谨慎验证
• 0.0-0.3: 弱匹配 - 不推荐使用

【建议】：
- 默认 0.3 适合大多数场景
- 如需高质量结果，设置 0.5-0.6
- 如搜索无结果，可降低到 0.2`),
			),
		},
		[]*reactloops.LoopStreamField{},
		// Validator
		func(r *reactloops.ReActLoop, action *aicommon.Action) error {
			questions := action.GetInvokeParamsArray("questions")
			if len(questions) == 0 {
				return utils.Error("semantic_search_yaklang_samples requires 'questions' parameter with at least one question")
			}

			// 验证每个问题格式
			for i, q := range questions {
				question := q.GetString("question")
				if question == "" {
					return utils.Errorf("question at index %d is empty", i)
				}
				// 检查是否包含 Yaklang 关键词
				if !strings.Contains(question, "Yaklang") && !strings.Contains(question, "yaklang") && !strings.Contains(question, "yak") {
					log.Warnf("question at index %d does not contain 'Yaklang' keyword: %s", i, question)
				}
			}

			return nil
		},
		// Handler
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			questions := action.GetInvokeParamsArray("questions")
			topN := action.GetInt("top_n")
			scoreThreshold := action.GetFloat("score_threshold")

			// 设置默认值
			if topN == 0 {
				topN = 30
			}
			if scoreThreshold == 0 {
				scoreThreshold = -1
			}

			invoker := loop.GetInvoker()

			// 构建查询字符串用于重复检测
			var questionTexts []string
			for _, q := range questions {
				questionTexts = append(questionTexts, q.GetString("question"))
			}
			questionsStr := strings.Join(questionTexts, "|")

			// 检查重复查询
			lastSearchQuery := loop.Get("last_semantic_search_query")
			currentQuery := fmt.Sprintf("%s|%d|%f", questionsStr, topN, scoreThreshold)

			if lastSearchQuery == currentQuery {
				errorMsg := fmt.Sprintf(`【严重错误】检测到重复语义搜索！

上次查询：%s
本次查询：%s

【拒绝执行】：禁止重复相同的搜索模式！

【必须调整】：
1. 修改问题表述 - 使用不同的问法或角度
2. 拆解或合并问题 - 调整问题粒度
3. 调整搜索参数 - 修改 top_n 或 score_threshold
4. 检查问题质量 - 确保问题完整且明确

【建议行动】：
- 如果之前搜索无结果，尝试更通用的问题
- 如果之前结果太多，使用更精确的问题
- 考虑从不同角度提问

【警告】：继续重复查询将浪费时间且无法获得新信息！`, lastSearchQuery, currentQuery)

				invoker.AddToTimeline("semantic_search_duplicate_query_error", errorMsg)
				log.Warnf("duplicate semantic search query detected: %s", currentQuery)
				op.Continue()
				return
			}

			// 记录当前查询
			loop.Set("last_semantic_search_query", currentQuery)

			emitter := loop.GetEmitter()

			// 显示搜索参数
			searchInfo := fmt.Sprintf("Semantic RAG search - Questions: %d, top_n per question: %d, score_threshold: %.2f\nQuestions:\n%s",
				len(questions), topN, scoreThreshold, questionsStr)
			emitter.EmitThoughtStream(op.GetTask().GetId(), searchInfo)
			loop.GetEmitter().EmitDefaultStreamEvent(
				"semantic_search_yaklang_samples",
				bytes.NewReader([]byte(searchInfo)),
				loop.GetCurrentTask().GetIndex(),
				func() {
					log.Infof("semantic search yaklang samples: %s", searchInfo)
				},
			)

			invoker.AddToTimeline("start_semantic_search_yaklang_samples", searchInfo)

			// 检查 RAG 系统
			if ragSystem == nil {
				errorMsg := `【系统错误】语义搜索系统不可用！

【错误原因】：RAG系统未正确初始化或加载失败

【必须执行】：
1. 检查RAG知识库是否正确加载
2. 重新初始化向量搜索系统
3. 确认知识库文件完整性

【后果】：无法进行语义搜索，将导致API使用错误！

【建议】：暂停编码任务，优先解决RAG系统问题`
				log.Warn("semantic search: RAG system not available")
				invoker.AddToTimeline("semantic_search_system_error", errorMsg)
				op.Continue()
				return
			}

			// 执行多问题向量搜索并合并结果
			type ResultKey struct {
				DocID string
			}
			allResultsMap := make(map[ResultKey]rag.SearchResult)
			var totalSearchCount int

			for idx, q := range questions {
				question := q.GetString("question")
				if question == "" {
					continue
				}

				log.Infof("semantic search question %d/%d: %s", idx+1, len(questions), question)

				// 执行单个问题的搜索
				results, err := ragSystem.QueryTopN(question, int(topN), scoreThreshold)
				if err != nil {
					log.Errorf("semantic search failed for question '%s': %v", question, err)
					continue
				}

				totalSearchCount += len(results)

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

			// 将 map 转换为切片并按分数排序
			var results []rag.SearchResult
			for _, result := range allResultsMap {
				results = append(results, result)
			}

			// 按分数降序排序
			sort.Slice(results, func(i, j int) bool {
				return results[i].Score > results[j].Score
			})

			log.Infof("semantic search: %d questions searched, %d total results, %d unique results after deduplication",
				len(questions), totalSearchCount, len(results))

			if len(results) == 0 {
				noResultMsg := fmt.Sprintf(`【语义搜索无结果】未找到相关的代码片段

【搜索的问题】：
%s

【严重警告】：无法找到相关代码样例！

【禁止行为】：
❌ 禁止臆造任何 Yaklang API
❌ 禁止参考其他语言的语法
❌ 禁止假设函数存在或用法

【必须立即执行】：
1. 重新表述问题 - 使用更通用或更具体的描述
2. 拆解复杂问题 - 将一个问题分解为多个简单问题
3. 降低相似度阈值 - 设置 score_threshold=0.2 或更低
4. 尝试不同角度 - 从功能、用途、场景等不同角度提问
5. 使用 grep_yaklang_samples - 如果知道关键词，使用精确搜索

【问题质量检查】：
- 是否包含 "Yaklang" 关键词？
- 是否使用完整主谓宾句式？
- 是否避免使用代词？
- 是否足够具体明确？

【后果警告】：不重新搜索将导致代码错误和调试失败！`, questionsStr)
				log.Infof("no semantic search results found for questions: %s", questionsStr)
				invoker.AddToTimeline("semantic_search_no_results_warning", noResultMsg)
				op.Continue()
				return
			}

			// 格式化搜索结果
			var resultBuffer bytes.Buffer
			resultBuffer.WriteString(fmt.Sprintf("\n[Semantic Search Results] 找到 %d 个相关代码片段（来自 %d 个问题，去重后）\n\n", len(results), len(questions)))

			// 限制返回结果数量，避免内容过多
			maxResults := 30
			displayCount := len(results)
			if displayCount > maxResults {
				displayCount = maxResults
			}

			for i := 0; i < displayCount; i++ {
				result := results[i]
				resultBuffer.WriteString(fmt.Sprintf("=== [%d/%d] 相似度: %.3f ===\n",
					i+1, len(results), result.Score))

				// 显示文档内容
				var content string
				if result.KnowledgeBaseEntry != nil {
					content = result.KnowledgeBaseEntry.KnowledgeDetails
				} else {
					content = result.Document.Content
				}
				if len(content) > 1000 {
					content = content[:1000] + "\n[... 内容已截断 ...]"
				}
				resultBuffer.WriteString(fmt.Sprintf("内容：\n%s\n", content))
				resultBuffer.WriteString("\n")
			}

			if len(results) > maxResults {
				resultBuffer.WriteString(fmt.Sprintf("... 还有 %d 个结果未显示（总共 %d 个）\n\n",
					len(results)-maxResults, len(results)))
			}

			// 将搜索结果添加到Timeline
			resultStr := resultBuffer.String()

			// 尝试压缩和优化搜索结果 - 使用与 grep 相同的压缩策略
			if len(results) > 5 {
				log.Infof("semantic_search_yaklang_samples: attempting to compress %d results", len(results))
				compressedResult := compressRAGResults(resultStr, questionsStr, invoker, op)
				if len(compressedResult) < len(resultStr) {
					resultStr = compressedResult
					log.Infof("semantic_search_yaklang_samples: successfully compressed results")
				}
			}

			emitter.EmitThoughtStream("semantic_search_samples_result", "Semantic Search Result:\n"+resultStr)
			invoker.AddToTimeline("semantic_search_results", fmt.Sprintf("Found %d relevant code snippets for %d questions (deduplicated)\nQuestions: %s\n%s", len(results), len(questions), questionsStr, resultStr))

			// 根据结果数量生成不同的建议，添加到Timeline
			var suggestionMsg string
			var timelineKey string

			if len(results) < 5 {
				suggestionMsg = fmt.Sprintf(`【语义搜索结果较少】仅找到 %d 个相关片段

【分析】：样例数量不足，可能影响理解完整性

【强烈建议的后续行动】：
1. 重新表述问题 - 使用更通用或不同角度的问法
   • 检查问题是否过于具体或过于宽泛
   • 尝试从功能、用途、场景等不同角度提问
2. 降低相似度阈值 (score_threshold=0.2 或更低)
3. 增加问题数量 - 将一个问题拆解为多个相关问题
4. 增加每个问题返回数量 (top_n=40-50)
5. 使用 grep_yaklang_samples - 如果知道关键词，使用精确搜索

【警告】：当前样例可能不足以完全理解用法
【决策】：建议继续搜索更多样例，或谨慎使用现有结果`, len(results))
				timelineKey = "semantic_search_few_results_suggestion"
			} else if len(results) > 20 {
				suggestionMsg = fmt.Sprintf(`【语义搜索结果丰富】找到 %d 个相关片段

【分析】：样例充足，但需要优化查看效率

【推荐优化策略】：
1. 精确化问题描述
   • 使用更具体的限定词和场景描述
   • 避免过于宽泛的问题
2. 提高相似度阈值 (score_threshold=0.5-0.6) 以获取高质量结果
3. 减少每个问题返回数量 (top_n=15-20) 以查看精华
4. 专注学习策略：
   • 优先查看相似度最高的前5-10个结果
   • 寻找多个样例中的共同用法模式
   • 注意参数类型和返回值的一致性

【优势】：有足够样例学习最佳实践
【建议】：可以开始编码，但要参考多个样例的共同模式`, len(results))
				timelineKey = "semantic_search_rich_results_suggestion"
			} else {
				suggestionMsg = fmt.Sprintf(`【语义搜索结果理想】找到 %d 个相关片段

【分析】：样例数量适中，质量和数量平衡良好

【学习指导】：
1. 系统性学习方法：
   • 仔细阅读每个匹配的完整代码片段
   • 识别函数的标准调用模式和参数配置
   • 理解错误处理和边界情况
2. 模式识别：
   • 寻找多个样例中的共同用法
   • 注意最佳实践和常见错误处理
   • 观察代码风格和命名规范
3. 实践准备：
   • 确保完全理解API用法后再编码
   • 优先使用相似度最高的调用方式
   • 保持与样例一致的代码风格

【状态】：有充分的参考依据，可以开始编写代码
【原则】：严格按照样例模式编写，避免自创用法`, len(results))
				timelineKey = "semantic_search_optimal_results_suggestion"
			}

			// 将建议添加到Timeline
			invoker.AddToTimeline(timelineKey, suggestionMsg)

			log.Infof("semantic search completed: %d results found for %d questions", len(results), len(questions))

			// 检查是否有语法错误 - 参考 action_modify_code.go 的实现
			fullcode := loop.Get("full_code")
			if fullcode != "" {
				errMsg, hasBlockingErrors := checkCodeAndFormatErrors(fullcode)
				if hasBlockingErrors {
					op.DisallowNextLoopExit()
				}
				if errMsg != "" {
					// 语法错误使用 Feedback 返回，参考 action_modify_code.go 第104行
					op.Feedback(errMsg)

					// 同时在Timeline中记录语法错误的存在（但不包含具体错误内容）
					invoker.AddToTimeline("syntax_error_detected", "语法错误已检测到并通过Feedback返回，需要修复后继续")
				}
			}

			// 继续执行
			op.Continue()
		},
	)
}
