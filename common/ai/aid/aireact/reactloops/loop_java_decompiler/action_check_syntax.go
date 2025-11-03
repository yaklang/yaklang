package loop_java_decompiler

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

var checkJavaSyntaxAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"check_syntax",
		"Check if Java files have syntax errors or compilation issues. Can check a single file or all files in a directory. Uses SSA in-memory compilation (preferred, safe) or javac as fallback (compilation only, no execution). All operations are safe and respect task context for cancellation.",
		[]aitool.ToolOption{
			aitool.WithStringParam("file_path", aitool.WithParam_Description("Path to a single Java file to check")),
			aitool.WithStringParam("directory_path", aitool.WithParam_Description("Path to a directory to check all Java files within it")),
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			filePath := action.GetString("file_path")
			dirPath := action.GetString("directory_path")

			if filePath == "" && dirPath == "" {
				// Use working directory if neither specified
				dirPath = l.Get("working_directory")
				if dirPath == "" {
					return utils.Error("either file_path or directory_path must be specified")
				}
			}

			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			filePath := action.GetString("file_path")
			dirPath := action.GetString("directory_path")

			if filePath == "" && dirPath == "" {
				dirPath = loop.Get("working_directory")
			}

			invoker := loop.GetInvoker()

			var filesToCheck []string

			if filePath != "" {
				// Check single file
				if !filepath.IsAbs(filePath) {
					workingDir := loop.Get("working_directory")
					if workingDir != "" {
						filePath = filepath.Join(workingDir, filePath)
					}
				}
				filesToCheck = append(filesToCheck, filePath)
				invoker.AddToTimeline("check_syntax", fmt.Sprintf("Checking syntax for file: %s", filePath))
			} else {
				// Check all Java files in directory
				if !filepath.IsAbs(dirPath) {
					workingDir := loop.Get("working_directory")
					if workingDir != "" {
						dirPath = filepath.Join(workingDir, dirPath)
					}
				}

				filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
					if err == nil && !info.IsDir() && strings.HasSuffix(path, ".java") {
						filesToCheck = append(filesToCheck, path)
					}
					return nil
				})
				invoker.AddToTimeline("check_syntax", fmt.Sprintf("Checking syntax for %d Java files in: %s", len(filesToCheck), dirPath))
			}

			if len(filesToCheck) == 0 {
				errorMsg := `【未找到Java文件】指定的位置没有找到任何Java文件

【可能原因】：
1. 指定的目录路径不正确
2. 目录中确实没有 .java 文件
3. 文件扩展名不是 .java
4. 文件权限问题导致无法读取

【立即行动】：
1. 使用 list_files 检查目录内容
2. 确认是否在正确的工作目录
3. 检查文件扩展名是否正确
4. 验证目录路径是否拼写正确

【建议】：
- 先反编译JAR文件获取Java源码
- 确认当前工作目录是反编译的输出目录
- 使用绝对路径指定目录

【下一步】：使用 decompile_jar 反编译JAR文件，或使用 list_files 查看当前目录内容`
				invoker.AddToTimeline("check_syntax_no_files", errorMsg)
				op.Feedback("No Java files found to check")
				op.Continue()
				return
			}

			// Get context from task operator (respects task cancellation)
			ctx := op.GetContext()
			if ctx == nil {
				ctx = context.Background()
			}

			// Check syntax using SSA or javac
			filesWithIssues := 0
			var issueReports []string

			for _, file := range filesToCheck {
				content, err := os.ReadFile(file)
				if err != nil {
					filesWithIssues++
					issueReports = append(issueReports, fmt.Sprintf("- %s: failed to read file: %v", file, err))
					continue
				}

				// Try to check syntax
				issues := checkJavaFileSyntax(ctx, string(content), file)
				if len(issues) > 0 {
					filesWithIssues++
					relPath, _ := filepath.Rel(loop.Get("working_directory"), file)
					if relPath == "" {
						relPath = file
					}
					issueReports = append(issueReports, fmt.Sprintf("- %s:\n  %s", relPath, strings.Join(issues, "\n  ")))
				}
			}

			// Update statistics
			loop.Set("files_with_issues", filesWithIssues)

			// Prepare feedback
			var msg string
			if filesWithIssues == 0 {
				msg = fmt.Sprintf("All %d Java files passed basic syntax checks. No issues found.", len(filesToCheck))
				invoker.AddToTimeline("check_syntax_success", fmt.Sprintf("所有 %d 个Java文件语法检查通过，可以继续下一步操作", len(filesToCheck)))
			} else {
				msg = fmt.Sprintf("Found issues in %d out of %d Java files:\n\n%s",
					filesWithIssues,
					len(filesToCheck),
					strings.Join(issueReports, "\n"))

				// Add detailed suggestions for fixing syntax issues
				timelineMsg := fmt.Sprintf(`【发现语法错误】在 %d/%d 个文件中发现语法问题

【问题分布】：
- 检查文件总数：%d
- 存在问题的文件：%d
- 通过检查的文件：%d

【常见语法错误类型】：
1. 括号不匹配（{} [] ()）
2. 分号缺失
3. 泛型类型声明错误
4. 导入语句缺失或错误
5. 变量类型推断失败
6. Lambda表达式语法错误

【推荐修复流程】：
1. 使用 read_java_file 查看具体文件内容
2. 分析错误信息，定位问题行
3. 使用 rewrite_java_file 修复语法错误（指定行范围进行局部重写，或省略行范围进行完整重写）
4. 再次使用 check_syntax 验证修复
5. 使用 compare_with_backup 查看修改差异

【修复优先级】：
- 高优先级：核心业务逻辑类
- 中优先级：工具类和辅助类
- 低优先级：测试类和示例代码

【注意事项】：
- 每次修复后及时验证
- 保持与原有代码风格一致
- 可以参考 .bak 备份文件
- 批量修复相同类型的错误更高效

【下一步行动】：选择一个有问题的文件，使用 read_java_file 查看详细内容`,
					filesWithIssues, len(filesToCheck), len(filesToCheck), filesWithIssues, len(filesToCheck)-filesWithIssues)
				invoker.AddToTimeline("check_syntax_issues_found", timelineMsg)
			}

			op.Feedback(msg)
			op.Continue()
		},
	)
}

// checkJavaFileSyntax checks Java file syntax using SSA or javac
// Priority: 1. SSA in-memory compilation (safe), 2. javac (only compilation, no execution)
func checkJavaFileSyntax(ctx context.Context, content string, filePath string) []string {
	var issues []string

	// Basic checks first
	issues = append(issues, checkBasicJavaSyntax(content)...)

	// If basic checks fail critically, no need to try compilation
	if len(issues) > 3 {
		return issues
	}

	// Try SSA compilation first (preferred, in-memory, safe)
	ssaIssues := trySSACompilation(ctx, content, filePath)
	if ssaIssues != nil {
		issues = append(issues, ssaIssues...)
		return issues
	}

	// If SSA succeeded, no syntax errors
	if len(issues) == 0 {
		return nil
	}

	// If only basic issues found and SSA succeeded, still report basic issues
	return issues
}

// checkBasicJavaSyntax performs basic Java syntax checks
func checkBasicJavaSyntax(content string) []string {
	var issues []string

	// Check for common decompilation issues
	if strings.Contains(content, "/* Error decompiling") {
		issues = append(issues, "Contains decompilation error markers")
	}

	// Check for unbalanced braces
	openBraces := strings.Count(content, "{")
	closeBraces := strings.Count(content, "}")
	if openBraces != closeBraces {
		issues = append(issues, fmt.Sprintf("Unbalanced braces: %d opening, %d closing", openBraces, closeBraces))
	}

	// Check for unbalanced parentheses
	openParens := strings.Count(content, "(")
	closeParens := strings.Count(content, ")")
	if openParens != closeParens {
		issues = append(issues, fmt.Sprintf("Unbalanced parentheses: %d opening, %d closing", openParens, closeParens))
	}

	// Check if file is empty or too small
	trimmed := strings.TrimSpace(content)
	if len(trimmed) < 10 {
		issues = append(issues, "File is empty or too small")
	}

	return issues
}

// trySSACompilation attempts to compile Java code using yaklang SSA in memory
// This is the preferred method as it's safe (no file execution) and doesn't require external tools
// Returns nil if compilation succeeded, error messages if failed
func trySSACompilation(ctx context.Context, content string, filePath string) []string {
	// Create a context with timeout to prevent hanging
	compileCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Try to compile using SSA in memory mode
	// This is SAFE - it only parses and compiles to SSA, never executes Java code
	_, err := ssaapi.Parse(
		content,
		ssaapi.WithLanguage(ssaconfig.JAVA),
		ssaapi.WithMemory(1*time.Hour), // In-memory only, cache for 1 hour
		ssaapi.WithContext(compileCtx),
	)

	if err != nil {
		// SSA compilation failed, extract error messages
		errMsg := err.Error()
		var issues []string

		// Simplify error messages
		lines := strings.Split(errMsg, "\n")
		for i, line := range lines {
			if i >= 5 { // Limit to first 5 error lines
				issues = append(issues, "... (more compilation errors)")
				break
			}
			if strings.TrimSpace(line) != "" {
				issues = append(issues, fmt.Sprintf("SSA compilation error: %s", strings.TrimSpace(line)))
			}
		}

		// If SSA failed, try javac as fallback (still safe - only compilation)
		javacIssues := tryJavacCompilation(compileCtx, content)
		if javacIssues != nil {
			issues = append(issues, javacIssues...)
		}

		return issues
	}

	// SSA compilation succeeded
	log.Debugf("SSA compilation succeeded for: %s", filePath)
	return nil
}

// tryJavacCompilation attempts to use javac for compilation checking if available
// SECURITY: Only uses javac for compilation (-encoding UTF-8), NEVER executes the compiled code
// This respects the task context for cancellation
func tryJavacCompilation(ctx context.Context, content string) []string {
	// Check if javac is available
	javacPath, err := exec.LookPath("javac")
	if err != nil {
		// javac not available, skip silently
		log.Debugf("javac not available for syntax checking: %v", err)
		return nil
	}

	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "syntax_check_*.java")
	if err != nil {
		log.Debugf("Failed to create temp file for javac: %v", err)
		return nil
	}
	tmpFilePath := tmpFile.Name()
	defer os.Remove(tmpFilePath)
	defer func() {
		// Also remove .class file if generated
		classFile := strings.TrimSuffix(tmpFilePath, ".java") + ".class"
		os.Remove(classFile)
	}()

	_, err = tmpFile.WriteString(content)
	if err != nil {
		tmpFile.Close()
		return nil
	}
	tmpFile.Close()

	// SECURITY: Only compile with javac, do NOT execute
	// Use fixed, safe parameters - no user-controlled parameters
	cmd := exec.CommandContext(ctx, javacPath, "-encoding", "UTF-8", tmpFilePath)
	output, err := cmd.CombinedOutput()

	if err != nil {
		// Compilation failed, extract error messages
		errMsg := string(output)
		if errMsg != "" {
			var issues []string
			lines := strings.Split(errMsg, "\n")
			for i, line := range lines {
				if i >= 5 { // Limit to first 5 error lines
					issues = append(issues, "... (more javac errors)")
					break
				}
				trimmed := strings.TrimSpace(line)
				if trimmed != "" && !strings.HasPrefix(trimmed, tmpFilePath) {
					issues = append(issues, fmt.Sprintf("javac: %s", trimmed))
				}
			}
			return issues
		}
	}

	// Compilation succeeded
	log.Debugf("javac compilation succeeded")
	return nil
}
