package loop_yaklangcode

import (
	"bytes"
	"fmt"
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

var searchYaklangSamplesAction = func(r aicommon.AIInvokeRuntime, ragSystem *rag.RAGSystem) reactloops.ReActLoopOption {
	if ragSystem == nil {
		log.Warnf("search_yaklang_samples: ragSystem is nil")
		return func(r *reactloops.ReActLoop) {
			r.GetInvoker().AddToTimeline("search_yaklang_samples_error", "Yaklang AIKB RAG 系统未正确初始化或加载失败")
		}
	}
	return reactloops.WithRegisterLoopActionWithStreamField(
		"search_yaklang_samples",
		`RAG搜索 Yaklang 代码样例库 - 基于语义向量搜索真实代码示例

核心原则：禁止臆造 Yaklang API！必须先通过向量搜索找到真实样例！

【强制使用场景】：
1. 编写任何代码前，先向量搜索相关函数用法
2. 遇到 API 错误（ExternLib don't has）时 - 必须立即向量搜索
3. 遇到语法错误（SyntaxError）时 - 必须立即向量搜索
4. 不确定函数参数或返回值时

【参数说明】：
- query (必需) - 搜索查询，支持自然语言描述：
  * 功能描述：如 "端口扫描功能", "HTTP请求处理"
  * 关键词组合：如 "文件上传", "数据库查询"
  * 代码意图：如 "如何处理错误", "循环遍历数组"

- top_n (可选) - 返回结果数量，默认 10
  * 需要更多结果：设置 15-20
  * 快速预览：设置 5-8
  * 深入研究：设置 20-30

- score_threshold (可选) - 相似度阈值，默认 0.1
  * 严格匹配：设置 0.3-0.5
  * 宽松匹配：设置 0.05-0.1（默认）
  * 非常宽松：设置 0.01-0.05

【使用示例】：
search_yaklang_samples(query="端口扫描和漏洞检测", top_n=15)
search_yaklang_samples(query="HTTP请求和响应处理", score_threshold=0.2)
search_yaklang_samples(query="错误处理和异常捕获", top_n=8)

记住：Yaklang 是 DSL！每个 API 都可能与 Python/Go 不同！
先向量搜索找样例，再写代码，节省 90% 调试时间！`,
		[]aitool.ToolOption{
			aitool.WithStringParam(
				"query",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description(`搜索查询（必需）- 支持自然语言描述：
1. 功能描述：如 "端口扫描功能", "HTTP请求处理", "文件操作"
2. 业务场景：如 "漏洞检测", "数据处理", "网络通信"
3. 技术需求：如 "错误处理", "并发编程", "数据验证"
4. 代码意图：如 "循环遍历", "条件判断", "函数调用"

注意：向量搜索基于语义相似度，描述越准确，结果越相关`),
			),
			aitool.WithIntegerParam(
				"top_n",
				aitool.WithParam_Description(`返回结果数量（默认 10）- 控制返回的代码片段数量：
• 5-8: 快速预览核心用法
• 10-15: 标准搜索（推荐，默认）
• 20-30: 深入研究完整实现`),
			),
			aitool.WithNumberParam(
				"score_threshold",
				aitool.WithParam_Description(`相似度阈值（默认 0.1）- 过滤低质量结果：
• 0.3-0.5: 严格匹配，高质量结果
• 0.1-0.3: 平衡质量和数量（默认）
• 0.01-0.1: 宽松匹配，更多结果但可能包含不相关内容`),
			),
		},
		[]*reactloops.LoopStreamField{},
		// Validator
		func(r *reactloops.ReActLoop, action *aicommon.Action) error {
			query := action.GetString("query")
			if query == "" {
				return utils.Error("search_yaklang_samples requires 'query' parameter")
			}

			return nil
		},
		// Handler
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			query := action.GetString("query")
			topN := action.GetInt("top_n")
			scoreThreshold := action.GetFloat("score_threshold")

			// 设置默认值
			if topN == 0 {
				topN = 10
			}
			if scoreThreshold == 0 {
				scoreThreshold = 0.1
			}

			invoker := loop.GetInvoker()

			// 检查重复查询
			lastSearchQuery := loop.Get("last_search_query")
			currentQuery := fmt.Sprintf("%s|%d|%f", query, topN, scoreThreshold)

			if lastSearchQuery == currentQuery {
				errorMsg := fmt.Sprintf(`【严重错误】检测到重复查询！

上次查询：%s
本次查询：%s

【拒绝执行】：禁止重复相同的搜索模式！

【必须调整】：
1. 修改搜索关键词 - 使用同义词或相关词汇
2. 调整搜索策略 - 扩大或缩小搜索范围
3. 改变搜索方向 - 从功能角度而非API角度搜索
4. 检查拼写错误 - 确认关键词正确性

【建议行动】：
- 如果之前搜索无结果，尝试更通用的词汇
- 如果之前结果太多，使用更精确的描述
- 考虑从业务需求角度重新思考搜索词

【警告】：继续重复查询将浪费时间且无法获得新信息！`, lastSearchQuery, currentQuery)

				invoker.AddToTimeline("search_duplicate_query_error", errorMsg)
				log.Warnf("duplicate search query detected: %s", currentQuery)
				op.Continue()
				return
			}

			// 记录当前查询
			loop.Set("last_search_query", currentQuery)

			emitter := loop.GetEmitter()

			// 显示搜索参数
			searchInfo := fmt.Sprintf("RAG search query: %s, top_n: %d, score_threshold: %.2f",
				query, topN, scoreThreshold)
			emitter.EmitThoughtStream(op.GetTask().GetId(), searchInfo)
			loop.GetEmitter().EmitDefaultStreamEvent(
				"search_yaklang_samples",
				bytes.NewReader([]byte(searchInfo)),
				loop.GetCurrentTask().GetIndex(),
				func() {
					log.Infof("search yaklang samples: %s", searchInfo)
				},
			)

			invoker.AddToTimeline("start_search_yaklang_samples", searchInfo)

			// 检查 RAG 系统
			if ragSystem == nil {
				errorMsg := `【系统错误】RAG搜索器不可用！

【错误原因】：RAG系统未正确初始化或加载失败

【必须执行】：
1. 检查RAG知识库是否正确加载
2. 重新初始化向量搜索系统
3. 确认知识库文件完整性

【后果】：无法进行语义搜索，将导致API使用错误！

【建议】：暂停编码任务，优先解决RAG系统问题`
				log.Warn("RAG system not available")
				invoker.AddToTimeline("search_system_error", errorMsg)
				op.Continue()
				return
			}

			// 执行向量搜索
			results, err := ragSystem.QueryTopN(query, int(topN), scoreThreshold)

			if err != nil {
				errorMsg := fmt.Sprintf(`【搜索执行失败】RAG向量搜索遇到错误！

【错误详情】：%v

【可能原因】：
1. 查询语句过长或格式错误
2. 向量数据库连接问题
3. 知识库索引损坏

【立即行动】：
1. 检查查询语句长度和格式
2. 简化搜索查询
3. 重试搜索操作

【建议】：
- 使用简洁的关键词组合
- 避免特殊字符和过长描述
- 如果持续失败，考虑重启搜索服务

【警告】：搜索失败将影响代码质量，请务必解决！`, err)
				log.Errorf("RAG search failed: %v", err)
				invoker.AddToTimeline("search_execution_error", errorMsg)
				op.Continue()
				return
			}

			if len(results) == 0 {
				noResultMsg := fmt.Sprintf(`【搜索无结果】未找到相关的代码片段：%s

【严重警告】：无法找到相关代码样例！

【禁止行为】：
❌ 禁止臆造任何 Yaklang API
❌ 禁止参考其他语言的语法
❌ 禁止假设函数存在或用法

【必须立即执行】：
1. 扩大搜索范围 - 使用更通用关键词
2. 尝试功能性描述 - 如 "网络连接" 而不是具体函数名
3. 降低相似度阈值 - 设置 score_threshold=0.05
4. 中英文组合 - 如 "端口扫描|port scan"
5. 功能性搜索 - 从需求角度思考搜索词

【搜索策略建议】：
- 业务功能词：如 "扫描", "请求", "解析"
- 技术领域词：如 "http", "tcp", "file"
- 错误处理词：如 "error", "exception", "handle"

【后果警告】：不重新搜索将导致代码错误和调试失败！`, query)
				log.Infof("no RAG search results found for query: %s", query)
				invoker.AddToTimeline("search_no_results_warning", noResultMsg)
				op.Continue()
				return
			}

			// 格式化搜索结果
			var resultBuffer bytes.Buffer
			resultBuffer.WriteString(fmt.Sprintf("\n[RAG Search Results] 找到 %d 个相关代码片段\n\n", len(results)))

			// 限制返回结果数量，避免内容过多
			maxResults := 20
			displayCount := len(results)
			if displayCount > maxResults {
				displayCount = maxResults
			}

			for i := 0; i < displayCount; i++ {
				result := results[i]
				resultBuffer.WriteString(fmt.Sprintf("=== [%d/%d] 相似度: %.3f ===\n",
					i+1, len(results), result.Score))

				// 显示文档内容
				content := result.Document.Content
				if len(content) > 1000 {
					content = content[:1000] + "\n[... 内容已截断 ...]"
				}
				resultBuffer.WriteString(fmt.Sprintf("内容：\n%s\n", content))

				// 显示元数据信息
				if len(result.Document.Metadata) > 0 {
					resultBuffer.WriteString("元数据：\n")
					for key, value := range result.Document.Metadata {
						resultBuffer.WriteString(fmt.Sprintf("  %s: %v\n", key, value))
					}
				}

				resultBuffer.WriteString("\n")
			}

			if len(results) > maxResults {
				resultBuffer.WriteString(fmt.Sprintf("... 还有 %d 个结果未显示（总共 %d 个）\n\n",
					len(results)-maxResults, len(results)))
			}

			// 将搜索结果添加到Timeline
			resultStr := resultBuffer.String()

			// 尝试压缩和优化搜索结果
			if len(results) > 5 {
				log.Infof("search_yaklang_samples: attempting to compress %d results", len(results))
				compressedResult := compressRAGResults(resultStr, query, invoker, op)
				if len(compressedResult) < len(resultStr) {
					resultStr = compressedResult
					log.Infof("search_yaklang_samples: successfully compressed results")
				}
			}

			emitter.EmitThoughtStream("search_samples_result", "Search Result:\n"+resultStr)
			invoker.AddToTimeline("search_results", fmt.Sprintf("Found %d relevant code snippets for query: %s\n%s", len(results), query, resultStr))

			// 根据结果数量生成不同的建议，添加到Timeline
			var suggestionMsg string
			var timelineKey string

			if len(results) < 3 {
				suggestionMsg = fmt.Sprintf(`【搜索结果较少】仅找到 %d 个相关片段

【分析】：样例数量不足，可能影响理解完整性

【强烈建议的后续行动】：
1. 扩大搜索范围 - 使用更通用关键词
   • 当前："%s" → 建议：去掉具体技术细节
   • 示例：从 "TCP端口扫描超时处理" 改为 "端口扫描"
2. 降低相似度阈值 (score_threshold=0.05)
3. 尝试功能性搜索："网络连接|连接处理"
4. 增加返回数量 (top_n=15-20)

【警告】：当前样例可能不足以完全理解用法
【决策】：建议继续搜索更多样例，或谨慎使用现有结果`, len(results), query)
				timelineKey = "search_few_results_suggestion"
			} else if len(results) > 15 {
				suggestionMsg = fmt.Sprintf(`【搜索结果丰富】找到 %d 个相关片段

【分析】：样例充足，但需要优化查看效率

【推荐优化策略】：
1. 精确化搜索描述
   • 当前："%s" → 建议：添加更具体的限定词
   • 示例：从 "扫描" 改为 "端口扫描和漏洞检测"
2. 提高相似度阈值 (score_threshold=0.2)
3. 减少返回数量 (top_n=8-12) 以查看精华
4. 专注学习策略：
   • 优先查看相似度最高的3-5个结果
   • 寻找多个样例中的共同用法模式
   • 注意参数类型和返回值的一致性

【优势】：有足够样例学习最佳实践
【建议】：可以开始编码，但要参考多个样例的共同模式`, len(results), query)
				timelineKey = "search_rich_results_suggestion"
			} else {
				suggestionMsg = fmt.Sprintf(`【搜索结果理想】找到 %d 个相关片段

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
				timelineKey = "search_optimal_results_suggestion"
			}

			// 将建议添加到Timeline
			invoker.AddToTimeline(timelineKey, suggestionMsg)

			log.Infof("RAG search completed: %d results found for query: %s", len(results), query)

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
