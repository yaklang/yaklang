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
		`🔍 Grep Yaklang 代码样例库 - 快速搜索真实代码示例

⚠️ 核心原则：禁止臆造 Yaklang API！必须先 grep 搜索真实样例！

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

			invoker := loop.GetInvoker()
			invoker.AddToTimeline("start_grep_yaklang_samples", searchInfo)

			// 检查 docSearcher
			if docSearcher == nil {
				errorMsg := "Document searcher not available, cannot grep. Please ensure yaklang-aikb is properly installed."
				log.Warn(errorMsg)
				invoker.AddToTimeline("grep_failed", errorMsg)
				op.Feedback("⚠️ " + errorMsg)
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
				errorMsg := fmt.Sprintf("Grep search failed: %v", err)
				log.Error(errorMsg)
				invoker.AddToTimeline("grep_failed", errorMsg)
				op.Feedback("❌ " + errorMsg)
				op.Continue()
				return
			}

			if len(results) == 0 {
				noResultMsg := fmt.Sprintf(`No matches found for pattern: %s

💡 建议：
- 尝试更通用的搜索词（如 "scan" 而不是 "servicescan.ScanWithTimeout"）
- 使用正则表达式扩大搜索范围（如 "servicescan\\." 搜索所有 servicescan 函数）
- 检查拼写是否正确
- 尝试中英文组合搜索（如 "端口扫描|port.*scan"）`, pattern)
				log.Info(noResultMsg)
				invoker.AddToTimeline("grep_no_results", noResultMsg)
				op.Feedback("ℹ️ " + noResultMsg)
				op.Continue()
				return
			}

			// 格式化结果 - 直接返回 grep 结果，不经过 summarizer
			var resultBuffer bytes.Buffer
			resultBuffer.WriteString(fmt.Sprintf("\n🔍 Grep Results: 找到 %d 个匹配\n\n", len(results)))

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
				resultBuffer.WriteString(fmt.Sprintf("▶ %s\n", result.Line))

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
				resultBuffer.WriteString("💡 提示：如果需要查看更多结果，可以：\n")
				resultBuffer.WriteString("  - 使用更精确的 pattern 缩小搜索范围\n")
				resultBuffer.WriteString("  - 减少 context_lines 以查看更多匹配项\n")
			}

			resultStr := resultBuffer.String()
			log.Infof("grep results:\n%s", resultStr)
			invoker.AddToTimeline("grep_success", fmt.Sprintf("Found %d matches, displayed %d", len(results), displayCount))

			// 检查是否有 lint 错误
			var msg string
			fullcode := loop.Get("full_code")
			if fullcode != "" {
				errMsg, blocking := checkCodeAndFormatErrors(fullcode)
				if blocking {
					op.DisallowNextLoopExit()
				}
				if errMsg != "" {
					msg += "LINT ERR:\n" + errMsg + "\n\n"
				}
			}

			// 组合返回消息
			if msg != "" {
				resultStr = msg + resultStr
			}

			// 直接返回 grep 结果，不需要 summarizer
			op.Feedback(resultStr)
			op.Continue()
		},
	)
}
