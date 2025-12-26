package loop_explore_filesystem

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
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
	FilePath      string
	LineNumber    int
	Line          string
	ContextBefore []string
	ContextAfter  []string
}

// grepFilesystemAction creates the grep action for filesystem exploration
var grepFilesystemAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"grep_filesystem",
		`Grep Filesystem - 在指定目录下搜索代码模式

【核心功能】
使用 grep 在文件系统中搜索匹配的代码模式，用于：
1. 查找函数/类/接口的定义和实现
2. 追踪代码调用关系
3. 理解代码结构和依赖
4. 发现相关代码片段

【参数说明】
- path (必需): 要搜索的目录或文件路径
- pattern (必需): 搜索模式，支持正则表达式
- context_lines (可选): 上下文行数，默认 10
- file_pattern (可选): 文件名过滤模式，如 "*.go", "*.py"
- case_sensitive (可选): 是否区分大小写，默认 false
- max_results (可选): 最大结果数，默认 20

【使用示例】
grep_filesystem(path="/path/to/project", pattern="func.*HandleRequest", file_pattern="*.go")
grep_filesystem(path="./src", pattern="import.*react", context_lines=5)`,
		[]aitool.ToolOption{
			aitool.WithStringParam("path",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("The directory or file path to search in")),
			aitool.WithStringParam("pattern",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("Search pattern (supports regex)")),
			aitool.WithIntegerParam("context_lines",
				aitool.WithParam_Description("Number of context lines before and after match (default: 10)")),
			aitool.WithStringParam("file_pattern",
				aitool.WithParam_Description("File name pattern filter, e.g. '*.go', '*.py', '*.js'")),
			aitool.WithBoolParam("case_sensitive",
				aitool.WithParam_Description("Case sensitive search (default: false)")),
			aitool.WithIntegerParam("max_results",
				aitool.WithParam_Description("Maximum number of results to return (default: 20)")),
		},
		[]*reactloops.LoopStreamField{},
		// Validator
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			path := action.GetString("path")
			pattern := action.GetString("pattern")

			if path == "" {
				return utils.Error("grep_filesystem requires 'path' parameter")
			}
			if pattern == "" {
				return utils.Error("grep_filesystem requires 'pattern' parameter")
			}

			// Validate path exists
			if _, err := os.Stat(path); os.IsNotExist(err) {
				return utils.Errorf("path does not exist: %s", path)
			}

			return nil
		},
		// Handler
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			path := action.GetString("path")
			pattern := action.GetString("pattern")
			contextLines := action.GetInt("context_lines")
			filePattern := action.GetString("file_pattern")
			caseSensitive := action.GetBool("case_sensitive")
			maxResults := action.GetInt("max_results")

			// Set defaults
			if contextLines == 0 {
				contextLines = 10
			}
			if maxResults == 0 {
				maxResults = 20
			}

			invoker := loop.GetInvoker()
			emitter := loop.GetEmitter()

			// Check for duplicate queries
			lastQuery := loop.Get("last_grep_query")
			currentQuery := fmt.Sprintf("%s|%s|%s|%v", path, pattern, filePattern, caseSensitive)
			if lastQuery == currentQuery {
				errMsg := fmt.Sprintf(`【警告】检测到重复查询！
上次查询: %s
本次查询: %s

请调整搜索策略:
1. 使用不同的关键词或模式
2. 搜索不同的目录
3. 调整文件过滤条件`, lastQuery, currentQuery)
				invoker.AddToTimeline("grep_duplicate_query", errMsg)
				log.Warnf("duplicate grep query detected: %s", currentQuery)
				op.Continue()
				return
			}
			loop.Set("last_grep_query", currentQuery)

			// Log search info
			searchInfo := fmt.Sprintf("Grep: path=%s, pattern=%s, file_pattern=%s, context=%d",
				path, pattern, filePattern, contextLines)
			emitter.EmitThoughtStream(op.GetTask().GetId(), searchInfo)
			invoker.AddToTimeline("start_grep_filesystem", searchInfo)

			// Execute grep search
			results, err := executeGrepSearch(path, pattern, filePattern, caseSensitive, int(contextLines), int(maxResults))
			if err != nil {
				errMsg := fmt.Sprintf("Grep search failed: %v", err)
				log.Errorf(errMsg)
				invoker.AddToTimeline("grep_error", errMsg)
				op.Continue()
				return
			}

			if len(results) == 0 {
				noResultMsg := fmt.Sprintf(`【搜索无结果】未找到匹配模式: %s

【建议】
1. 扩大搜索范围 - 使用更通用的关键词
2. 调整正则表达式
3. 检查文件过滤条件
4. 尝试不区分大小写搜索`, pattern)
				log.Infof("no grep results for pattern: %s in path: %s", pattern, path)
				invoker.AddToTimeline("grep_no_results", noResultMsg)
				op.Continue()
				return
			}

			// Format results
			var resultBuffer bytes.Buffer
			resultBuffer.WriteString(fmt.Sprintf("\n[Grep Results] 找到 %d 个匹配\n\n", len(results)))

			for i, result := range results {
				resultBuffer.WriteString(fmt.Sprintf("=== [%d/%d] %s:%d ===\n",
					i+1, len(results), result.FilePath, result.LineNumber))

				// Show context before
				for _, line := range result.ContextBefore {
					resultBuffer.WriteString(fmt.Sprintf("  %s\n", line))
				}

				// Highlight matching line
				resultBuffer.WriteString(fmt.Sprintf(">>> %s\n", result.Line))

				// Show context after
				for _, line := range result.ContextAfter {
					resultBuffer.WriteString(fmt.Sprintf("  %s\n", line))
				}

				resultBuffer.WriteString("\n")
			}

			resultStr := resultBuffer.String()

			// Update exploration findings
			currentFindings := loop.Get("exploration_findings")
			newFindings := currentFindings + "\n\n---\n" + resultStr
			// Limit findings size to prevent memory issues
			if len(newFindings) > 50000 {
				// Keep only the most recent findings
				newFindings = newFindings[len(newFindings)-40000:]
			}
			loop.Set("exploration_findings", newFindings)

			// Emit results
			emitter.EmitThoughtStream("grep_result", "Search Result:\n"+resultStr)
			invoker.AddToTimeline("grep_results", fmt.Sprintf("Found %d matches for pattern: %s\n%s",
				len(results), pattern, resultStr))

			log.Infof("grep search completed: %d results for pattern: %s", len(results), pattern)

			op.Continue()
		},
	)
}

// executeGrepSearch performs the actual grep search in the filesystem
func executeGrepSearch(basePath, pattern, filePattern string, caseSensitive bool, contextLines, maxResults int) ([]*GrepResult, error) {
	var results []*GrepResult

	// Compile regex pattern
	flags := ""
	if !caseSensitive {
		flags = "(?i)"
	}
	re, err := regexp.Compile(flags + pattern)
	if err != nil {
		return nil, utils.Errorf("invalid regex pattern: %v", err)
	}

	// Walk the filesystem
	err = filepath.Walk(basePath, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files with errors
		}

		// Skip directories
		if info.IsDir() {
			// Skip common non-code directories
			dirName := info.Name()
			if dirName == ".git" || dirName == "node_modules" || dirName == "vendor" ||
				dirName == "__pycache__" || dirName == ".idea" || dirName == ".vscode" {
				return filepath.SkipDir
			}
			return nil
		}

		// Check file size limit (skip files > 5MB)
		if info.Size() > 5*1024*1024 {
			return nil
		}

		// Apply file pattern filter if specified
		if filePattern != "" {
			matched, _ := filepath.Match(filePattern, info.Name())
			if !matched {
				return nil
			}
		}

		// Skip binary files (simple heuristic)
		ext := strings.ToLower(filepath.Ext(filePath))
		binaryExts := map[string]bool{
			".exe": true, ".dll": true, ".so": true, ".dylib": true,
			".bin": true, ".dat": true, ".db": true, ".sqlite": true,
			".jpg": true, ".jpeg": true, ".png": true, ".gif": true,
			".pdf": true, ".zip": true, ".tar": true, ".gz": true,
		}
		if binaryExts[ext] {
			return nil
		}

		// Read and search file
		content, err := os.ReadFile(filePath)
		if err != nil {
			return nil
		}

		lines := strings.Split(string(content), "\n")
		for lineNum, line := range lines {
			if len(results) >= maxResults {
				return filepath.SkipAll // Stop if we have enough results
			}

			if re.MatchString(line) {
				result := &GrepResult{
					FilePath:   filePath,
					LineNumber: lineNum + 1,
					Line:       line,
				}

				// Add context before
				startLine := lineNum - contextLines
				if startLine < 0 {
					startLine = 0
				}
				for i := startLine; i < lineNum; i++ {
					result.ContextBefore = append(result.ContextBefore, lines[i])
				}

				// Add context after
				endLine := lineNum + contextLines + 1
				if endLine > len(lines) {
					endLine = len(lines)
				}
				for i := lineNum + 1; i < endLine; i++ {
					result.ContextAfter = append(result.ContextAfter, lines[i])
				}

				results = append(results, result)
			}
		}

		return nil
	})

	if err != nil && err != filepath.SkipAll {
		return nil, err
	}

	return results, nil
}
