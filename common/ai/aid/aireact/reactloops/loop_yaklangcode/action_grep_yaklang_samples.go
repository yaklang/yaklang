package loop_yaklangcode

import (
	"bytes"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/ziputil"
)

var grepYaklangSamplesAction = func(r aicommon.AIInvokeRuntime, docSearcher *ziputil.ZipGrepSearcher) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"grep_yaklang_samples",
		`Grep Yaklang 代码样例库 - 快速搜索真实代码示例

核心原则：禁止臆造 Yaklang API！必须先 grep 搜索真实样例！

【强制使用场景】：
1. 编写任何代码前，先 grep 相关函数用法
2. 遇到 API 错误（ExternLib don't has）时 - 必须立即 grep
3. 遇到语法错误（SyntaxError）时 - 必须立即 grep
4. 不确定函数参数或返回值时

【参数说明】：
- pattern (必需) - 搜索模式，支持：
  * 关键词：如 "端口扫描", "HTTP请求"
  * 正则：如 "servicescan\\.Scan", "poc\\..*"
  * 函数名：如 "str.Split", "yakit.Info"
  
- case_sensitive (可选) - 是否区分大小写，默认 false

- context_lines (可选) - 上下文行数，默认 15
  * 需要更多上下文：设置 20-30
  * 只看函数调用：设置 5-10
  * 看完整实现：设置 30-50

【使用示例】：
grep_yaklang_samples(pattern="servicescan\\.Scan", context_lines=20)
grep_yaklang_samples(pattern="die\\(err\\)", context_lines=10)
grep_yaklang_samples(pattern="端口扫描|服务扫描", context_lines=25)

记住：Yaklang 是 DSL！每个 API 都可能与 Python/Go 不同！
先 grep 找样例，再写代码，节省 90% 调试时间！`,
		[]aitool.ToolOption{
			aitool.WithStructParam(
				"grep_payload",
				[]aitool.PropertyOption{
					aitool.WithParam_Description("USE THIS FIELD for grep_yaklang_samples action. Provide search parameters to grep Yaklang code samples."),
				},
				aitool.WithStringParam(
					"pattern",
					aitool.WithParam_Required(true),
					aitool.WithParam_Description(`搜索模式（必需）- 支持多种格式：
1. 关键词：如 "端口扫描", "HTTP请求", "错误处理"
2. 精确函数名：如 "servicescan.Scan", "str.Split"
3. 正则表达式：如 "servicescan\\.", "poc\\.HTTP.*", "die\\(err\\)"
4. 组合搜索：如 "servicescan\\.Scan|端口扫描"

注意：正则中的 . 需要转义为 \\.`),
				),
				aitool.WithBoolParam(
					"case_sensitive",
					aitool.WithParam_Description("是否区分大小写（默认 false - 不区分，推荐）"),
				),
				aitool.WithIntegerParam(
					"context_lines",
					aitool.WithParam_Description(`上下文行数（默认 15）- 控制返回结果的上下文范围：
• 5-10: 快速查看函数调用
• 15-20: 理解函数用法（默认，推荐）
• 25-35: 学习完整实现
• 40-50: 研究复杂功能`),
				),
			),
		},
		[]*reactloops.LoopStreamField{},
		// Validator
		func(r *reactloops.ReActLoop, action *aicommon.Action) error {
			payloads := action.GetInvokeParams("grep_payload")

			pattern := payloads.GetString("pattern")
			if pattern == "" {
				return utils.Error("grep_yaklang_samples requires 'pattern' parameter in 'grep_payload'")
			}

			return nil
		},
		// Handler
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			payloads := action.GetInvokeParams("grep_payload")

			pattern := payloads.GetString("pattern")
			caseSensitive := payloads.GetBool("case_sensitive")
			contextLines := payloads.GetInt("context_lines")

			// 设置默认值
			if contextLines == 0 {
				contextLines = 15
			}

			invoker := loop.GetInvoker()

			// 检查重复查询
			lastGrepQuery := loop.Get("last_grep_query")
			currentQuery := fmt.Sprintf("%s|%v|%d", pattern, caseSensitive, contextLines)

			if lastGrepQuery == currentQuery {
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
- 如果之前结果太多，使用更精确的模式
- 考虑从业务需求角度重新思考搜索词

【警告】：继续重复查询将浪费时间且无法获得新信息！`, lastGrepQuery, currentQuery)

				invoker.AddToTimeline("grep_duplicate_query_error", errorMsg)
				log.Warnf("duplicate grep query detected: %s", currentQuery)
				op.Continue()
				return
			}

			// 记录当前查询
			loop.Set("last_grep_query", currentQuery)

			// 显示搜索参数
			searchInfo := fmt.Sprintf("Grep pattern: %s, case_sensitive: %v, context: %d lines",
				pattern, caseSensitive, contextLines)
			loop.GetEmitter().EmitTextPlainTextStreamEvent(
				"grep_yaklang_samples",
				bytes.NewReader([]byte(searchInfo)),
				loop.GetCurrentTask().GetIndex(),
				func() {
					log.Infof("grep yaklang samples: %s", searchInfo)
				},
			)

			invoker.AddToTimeline("start_grep_yaklang_samples", searchInfo)

			// 检查 docSearcher
			if docSearcher == nil {
				errorMsg := `【系统错误】文档搜索器不可用！

【错误原因】：yaklang-aikb 未正确安装或初始化失败

【必须执行】：
1. 检查 yaklang-aikb 安装状态
2. 重新初始化文档搜索系统
3. 确认知识库文件完整性

【后果】：无法搜索代码样例，将导致API使用错误！

【建议】：暂停编码任务，优先解决搜索器问题`
				log.Warn("document searcher not available")
				invoker.AddToTimeline("grep_system_error", errorMsg)
				op.Continue()
				return
			}

			// 执行 grep 搜索
			grepOpts := []ziputil.GrepOption{
				ziputil.WithGrepCaseSensitive(caseSensitive),
				ziputil.WithContext(int(contextLines)),
			}

			var results []*ziputil.GrepResult
			var err error

			// 首先尝试正则搜索
			results, err = docSearcher.GrepRegexp(pattern, grepOpts...)
			if err != nil {
				// 如果正则失败，尝试子字符串搜索
				log.Infof("regexp search failed, trying substring search: %v", err)
				results, err = docSearcher.GrepSubString(pattern, grepOpts...)
			}

			if err != nil {
				errorMsg := fmt.Sprintf(`【搜索执行失败】Grep 操作遇到错误！

【错误详情】：%v

【可能原因】：
1. 搜索模式语法错误（正则表达式格式问题）
2. 知识库文件损坏或不完整
3. 系统资源不足或权限问题

【立即行动】：
1. 检查搜索模式语法 - 确保正则表达式正确
2. 简化搜索词 - 使用基本关键词而非复杂正则
3. 重试搜索 - 使用不同的搜索策略

【建议】：
- 先尝试简单的关键词搜索（如 "scan", "http"）
- 避免复杂的正则表达式，改用基础字符串匹配
- 如果持续失败，考虑重启搜索服务

【警告】：搜索失败将影响代码质量，请务必解决！`, err)
				log.Errorf("grep search failed: %v", err)
				invoker.AddToTimeline("grep_execution_error", errorMsg)
				op.Continue()
				return
			}

			if len(results) == 0 {
				noResultMsg := fmt.Sprintf(`【搜索无结果】未找到匹配模式：%s

【严重警告】：无法找到相关代码样例！

【禁止行为】：
❌ 禁止臆造任何 Yaklang API
❌ 禁止参考其他语言的语法
❌ 禁止假设函数存在或用法

【必须立即执行】：
1. 扩大搜索范围 - 使用更通用关键词（如 "scan" 而不是具体函数名）
2. 尝试正则搜索 - 如 "servicescan\\." 搜索所有相关函数
3. 中英文组合 - 如 "端口扫描|port.*scan"
4. 检查拼写错误 - 确认关键词正确性
5. 功能性搜索 - 从需求角度思考关键词

【搜索策略建议】：
- 业务功能词：如 "扫描", "请求", "解析"
- 技术领域词：如 "http", "tcp", "ssl"
- 错误处理词：如 "error", "err", "die"

【后果警告】：不重新搜索将导致代码错误和调试失败！`, pattern)
				log.Infof("no grep results found for pattern: %s", pattern)
				invoker.AddToTimeline("grep_no_results_warning", noResultMsg)
				op.Continue()
				return
			}

			// 格式化搜索结果 - 纯结果展示，不包含建议
			var resultBuffer bytes.Buffer
			resultBuffer.WriteString(fmt.Sprintf("\n[Grep Results] 找到 %d 个匹配\n\n", len(results)))

			// 限制返回结果数量，避免内容过多
			maxResults := 20
			displayCount := len(results)
			if displayCount > maxResults {
				displayCount = maxResults
			}

			for i := 0; i < displayCount; i++ {
				result := results[i]
				resultBuffer.WriteString(fmt.Sprintf("=== [%d/%d] %s:%d ===\n",
					i+1, len(results), result.FileName, result.LineNumber))

				// 显示上下文（前）
				if len(result.ContextBefore) > 0 {
					for _, line := range result.ContextBefore {
						resultBuffer.WriteString(fmt.Sprintf("  %s\n", line))
					}
				}

				// 高亮匹配行
				resultBuffer.WriteString(fmt.Sprintf(">>> %s\n", result.Line))

				// 显示上下文（后）
				if len(result.ContextAfter) > 0 {
					for _, line := range result.ContextAfter {
						resultBuffer.WriteString(fmt.Sprintf("  %s\n", line))
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
			invoker.AddToTimeline("grep_search_results", fmt.Sprintf("Found %d matches for pattern: %s\n%s", len(results), pattern, resultStr))

			// 根据结果数量生成不同的建议，添加到Timeline
			var suggestionMsg string
			var timelineKey string

			if len(results) < 3 {
				suggestionMsg = fmt.Sprintf(`【搜索结果较少】仅找到 %d 个匹配项

【分析】：样例数量不足，可能影响理解完整性

【强烈建议的后续行动】：
1. 扩大搜索范围 - 使用更通用关键词
   • 当前："%s" → 建议：去掉具体版本或参数
   • 示例：从 "servicescan.ScanWithTimeout" 改为 "servicescan"
2. 尝试正则表达式搜索
   • 使用 "servicescan\\." 搜索所有相关函数
   • 使用 ".*scan.*" 搜索所有包含scan的函数
3. 增加上下文行数 (context_lines=25-35)
4. 中英文组合搜索："端口扫描|port.*scan"

【警告】：当前样例可能不足以完全理解API用法
【决策】：建议继续搜索更多样例，或谨慎使用现有结果`, len(results), pattern)
				timelineKey = "grep_few_results_suggestion"
			} else if len(results) > 15 {
				suggestionMsg = fmt.Sprintf(`【搜索结果丰富】找到 %d 个匹配项

【分析】：样例充足，但需要优化查看效率

【推荐优化策略】：
1. 精确化搜索模式
   • 当前："%s" → 建议：添加更具体的限定词
   • 示例：从 "scan" 改为 "servicescan\\.Scan"
2. 减少上下文行数 (context_lines=5-10) 以查看更多概览
3. 专注学习策略：
   • 优先查看前5个最相关的结果
   • 寻找最常用的调用模式
   • 注意参数和返回值的一致性

【优势】：有足够样例学习最佳实践
【建议】：可以开始编码，但要参考多个样例的共同模式`, len(results), pattern)
				timelineKey = "grep_rich_results_suggestion"
			} else {
				suggestionMsg = fmt.Sprintf(`【搜索结果理想】找到 %d 个匹配项

【分析】：样例数量适中，质量和数量平衡良好

【学习指导】：
1. 系统性学习方法：
   • 仔细阅读每个匹配的完整上下文
   • 识别函数的标准调用模式
   • 理解参数类型、返回值和错误处理
2. 模式识别：
   • 寻找多个样例中的共同用法
   • 注意最佳实践和常见错误处理
   • 观察变量命名和代码风格
3. 实践准备：
   • 确保完全理解API用法后再编码
   • 优先使用最常见的调用方式
   • 保持与样例一致的错误处理

【状态】：可以开始编写代码，有充分的参考依据
【原则】：严格按照样例模式编写，避免自创用法`, len(results))
				timelineKey = "grep_optimal_results_suggestion"
			}

			// 将建议添加到Timeline
			invoker.AddToTimeline(timelineKey, suggestionMsg)

			log.Infof("grep search completed: %d results found for pattern: %s", len(results), pattern)

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
