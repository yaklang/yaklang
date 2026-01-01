package loop_report_generating

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// GrepResult represents a single grep match result
type GrepResult struct {
	LineNumber int
	Line       string
	Context    []string
}

// grepReferenceAction creates an action for grep searching in reference files
var grepReferenceAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"grep_reference",
		"Search for patterns in reference files using grep-like functionality. Returns matching lines with context. Use regex patterns for flexible matching.",
		[]aitool.ToolOption{
			aitool.WithStringParam("pattern", aitool.WithParam_Description("The search pattern (regex supported). Examples: 'keyword', 'error.*message', '(?i)case-insensitive'"), aitool.WithParam_Required(true)),
			aitool.WithStringParam("file_path", aitool.WithParam_Description("Path to the file to search in"), aitool.WithParam_Required(true)),
			aitool.WithIntegerParam("context_lines", aitool.WithParam_Description("Number of context lines before and after each match (default: 3)")),
			aitool.WithBoolParam("case_insensitive", aitool.WithParam_Description("Whether to perform case-insensitive matching (default: false)")),
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			pattern := action.GetString("pattern")
			if pattern == "" {
				return utils.Error("pattern is required")
			}

			filePath := action.GetString("file_path")
			if filePath == "" {
				return utils.Error("file_path is required")
			}

			// 验证文件存在
			if !utils.FileExists(filePath) {
				return utils.Errorf("file not found: %s", filePath)
			}

			// 验证正则表达式有效
			if _, err := regexp.Compile(pattern); err != nil {
				return utils.Errorf("invalid regex pattern: %v", err)
			}

			log.Infof("grep_reference: verifying pattern '%s' in file %s", pattern, filePath)
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			pattern := action.GetString("pattern")
			filePath := action.GetString("file_path")
			contextLines := action.GetInt("context_lines")
			caseInsensitive := action.GetBool("case_insensitive")

			if contextLines <= 0 {
				contextLines = 3
			}

			log.Infof("grep_reference: searching pattern '%s' in file %s (context=%d, case_insensitive=%v)",
				pattern, filePath, contextLines, caseInsensitive)

			// 读取文件
			content, err := os.ReadFile(filePath)
			if err != nil {
				log.Errorf("grep_reference: failed to read file: %v", err)
				op.Fail(fmt.Sprintf("failed to read file: %v", err))
				return
			}

			// 编译正则表达式
			regexPattern := pattern
			if caseInsensitive && !strings.HasPrefix(pattern, "(?i)") {
				regexPattern = "(?i)" + pattern
			}

			re, err := regexp.Compile(regexPattern)
			if err != nil {
				log.Errorf("grep_reference: invalid regex: %v", err)
				op.Fail(fmt.Sprintf("invalid regex: %v", err))
				return
			}

			// 执行搜索
			lines := strings.Split(string(content), "\n")
			var results []GrepResult

			for i, line := range lines {
				if re.MatchString(line) {
					// 收集上下文
					contextStart := i - contextLines
					if contextStart < 0 {
						contextStart = 0
					}
					contextEnd := i + contextLines + 1
					if contextEnd > len(lines) {
						contextEnd = len(lines)
					}

					var context []string
					for j := contextStart; j < contextEnd; j++ {
						if j != i {
							context = append(context, fmt.Sprintf("%d: %s", j+1, lines[j]))
						}
					}

					results = append(results, GrepResult{
						LineNumber: i + 1,
						Line:       line,
						Context:    context,
					})
				}
			}

			log.Infof("grep_reference: found %d matches for pattern '%s'", len(results), pattern)

			// 构建结果
			var resultBuilder strings.Builder
			resultBuilder.WriteString(fmt.Sprintf("=== Grep Results for pattern '%s' in %s ===\n", pattern, filePath))
			resultBuilder.WriteString(fmt.Sprintf("Total matches: %d\n\n", len(results)))

			// 限制显示的结果数量
			maxResults := 20
			displayCount := len(results)
			if displayCount > maxResults {
				displayCount = maxResults
			}

			for i := 0; i < displayCount; i++ {
				r := results[i]
				resultBuilder.WriteString(fmt.Sprintf("--- Match #%d at line %d ---\n", i+1, r.LineNumber))

				// 显示上下文（匹配行之前）
				for _, ctx := range r.Context {
					if strings.HasPrefix(ctx, fmt.Sprintf("%d:", r.LineNumber)) {
						continue
					}
					lineNum := 0
					fmt.Sscanf(ctx, "%d:", &lineNum)
					if lineNum < r.LineNumber {
						resultBuilder.WriteString(fmt.Sprintf("  %s\n", ctx))
					}
				}

				// 显示匹配行
				resultBuilder.WriteString(fmt.Sprintf(">>> %d: %s\n", r.LineNumber, r.Line))

				// 显示上下文（匹配行之后）
				for _, ctx := range r.Context {
					lineNum := 0
					fmt.Sscanf(ctx, "%d:", &lineNum)
					if lineNum > r.LineNumber {
						resultBuilder.WriteString(fmt.Sprintf("  %s\n", ctx))
					}
				}

				resultBuilder.WriteString("\n")
			}

			if len(results) > maxResults {
				resultBuilder.WriteString(fmt.Sprintf("\n... and %d more matches not shown\n", len(results)-maxResults))
			}

			resultContent := resultBuilder.String()

			// 将搜索结果添加到已收集的参考资料中
			existingRefs := loop.Get("collected_references")
			loop.Set("collected_references", existingRefs+"\n"+resultContent)

			// 添加到时间线
			invoker := loop.GetInvoker()
			invoker.AddToTimeline("grep_search", fmt.Sprintf("Grep search: pattern='%s', file=%s, matches=%d", pattern, filePath, len(results)))

			// 反馈结果
			op.Feedback(resultContent)

			log.Infof("grep_reference: completed, %d matches added to collected references", len(results))
		},
	)
}
