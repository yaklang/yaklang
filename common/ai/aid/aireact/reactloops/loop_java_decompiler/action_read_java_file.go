package loop_java_decompiler

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

var readJavaFileAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"read_java_file",
		"Read and analyze a decompiled Java file. This action will show you the file content with line numbers and identify common decompilation issues like obfuscated variable names, unclear logic, or syntax errors. Use this before deciding whether the file needs to be rewritten.",
		[]aitool.ToolOption{
			aitool.WithStringParam("file_path", aitool.WithParam_Description("Path to the Java file to read (can be relative to working directory or absolute)"), aitool.WithParam_Required(true)),
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			filePath := action.GetString("file_path")
			if filePath == "" {
				r.AddToTimeline("read_file_no_path", `【缺少文件路径】未指定要读取的Java文件路径

【立即行动】：
1. 使用 list_files 查看可用的Java文件
2. 从文件列表中选择需要分析的文件
3. 使用完整的文件路径重新调用 read_java_file

【下一步】：调用 list_files 获取文件列表`)
				return utils.Error("file_path parameter is required")
			}

			// If not absolute, try relative to working directory
			if !filepath.IsAbs(filePath) {
				workingDir := l.Get("working_directory")
				if workingDir != "" {
					filePath = filepath.Join(workingDir, filePath)
				}
			}

			// Check if file exists using filesys
			fs := filesys.NewLocalFs()
			exists, err := fs.Exists(filePath)
			if err != nil {
				r.AddToTimeline("read_file_check_error", fmt.Sprintf(`【文件检查失败】无法检查文件是否存在：%s

【错误信息】：%v

【可能原因】：
1. 文件路径格式错误
2. 文件权限不足
3. 文件系统错误

【立即行动】：
1. 检查文件路径是否正确
2. 确认有读取权限
3. 使用 list_files 验证文件存在

【下一步】：修正文件路径后重试`, filePath, err))
				return utils.Errorf("failed to check file: %v", err)
			}

			if !exists {
				r.AddToTimeline("read_file_not_found", fmt.Sprintf(`【文件不存在】指定的Java文件未找到：%s

【可能原因】：
1. 文件路径拼写错误
2. 使用了相对路径但工作目录不对
3. 文件已被删除或移动
4. 大小写敏感问题

【立即行动】：
1. 使用 list_files 列出目录中的所有Java文件
2. 检查文件路径的拼写
3. 确认当前工作目录: %s
4. 尝试使用绝对路径

【下一步】：使用 list_files 查看实际存在的文件`, filePath, l.Get("working_directory")))
				return utils.Errorf("file not found: %s", filePath)
			}

			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			filePath := action.GetString("file_path")

			// If not absolute, make it relative to working directory
			if !filepath.IsAbs(filePath) {
				workingDir := loop.Get("working_directory")
				if workingDir != "" {
					filePath = filepath.Join(workingDir, filePath)
				}
			}

			invoker := loop.GetInvoker()
			invoker.AddToTimeline("read_file", fmt.Sprintf("Reading Java file: %s", filePath))

			// Read file content using filesys
			fs := filesys.NewLocalFs()
			content, err := fs.ReadFile(filePath)
			if err != nil {
				r.AddToTimeline("read_file_failed", fmt.Sprintf(`【读取文件失败】无法读取文件内容：%s

【错误信息】：%v

【可能原因】：
1. 文件权限不足
2. 文件被其他程序占用
3. 文件系统错误
4. 文件太大无法读取

【立即行动】：
1. 检查文件权限
2. 确认文件没有被锁定
3. 尝试使用系统工具查看文件

【下一步】：解决权限问题后重试，或选择其他文件`, filePath, err))
				op.Fail("failed to read file: " + err.Error())
				return
			}

			contentStr := string(content)

			// Store in loop context
			loop.Set("current_file", filePath)
			loop.Set("current_file_content", contentStr)

			// Analyze decompilation quality
			analysis := analyzeDecompiledCode(contentStr)

			// Add line numbers for better reference
			contentWithLines := utils.PrefixLinesWithLineNumbers(contentStr)

			// Prepare feedback message
			lines := strings.Split(contentStr, "\n")

			var msg string
			if len(lines) > 50 {
				// Show first 40 and last 10 lines for large files
				firstPart := strings.Join(lines[:40], "\n")
				lastPart := strings.Join(lines[len(lines)-10:], "\n")
				msg = fmt.Sprintf(`Successfully read file: %s

【文件统计】：
- 总行数：%d 行
- 文件大小：%d 字节
- 显示：前 40 行 + 后 10 行（中间省略 %d 行）

【代码质量分析】：
%s

【文件内容（带行号）】：
%s

... (省略 %d 行) ...

%s

【建议操作】：
%s`,
					filePath,
					len(lines),
					len(contentStr),
					len(lines)-50,
					formatAnalysis(analysis),
					utils.PrefixLinesWithLineNumbers(firstPart),
					len(lines)-50,
					utils.PrefixLinesWithLineNumbers(lastPart),
					getSuggestion(analysis))
			} else {
				msg = fmt.Sprintf(`Successfully read file: %s

【文件统计】：
- 总行数：%d 行
- 文件大小：%d 字节

【代码质量分析】：
%s

【文件内容（带行号）】：
%s

【建议操作】：
%s`,
					filePath,
					len(lines),
					len(contentStr),
					formatAnalysis(analysis),
					contentWithLines,
					getSuggestion(analysis))
			}

			// Timeline message
			timelineMsg := fmt.Sprintf(`【文件读取成功】%s

【统计信息】：
- 文件行数：%d
- 问题变量：%d 个
- 语法问题：%d 个
- 代码质量：%s

【下一步建议】：%s`,
				filePath,
				len(lines),
				analysis.ObfuscatedVars,
				analysis.SyntaxIssues,
				getQualityLevel(analysis),
				getSuggestion(analysis))

			r.AddToTimeline("read_file_success", timelineMsg)
			op.Feedback(msg)
			op.Continue()
		},
	)
}

// DecompiledCodeAnalysis contains the analysis result of decompiled code
type DecompiledCodeAnalysis struct {
	ObfuscatedVars   int      // Number of obfuscated variable names
	SyntaxIssues     int      // Number of syntax issues found
	HasDecompError   bool     // Contains decompilation error markers
	UnbalancedBraces bool     // Has unbalanced braces
	ComplexityScore  int      // Code complexity (0-100, higher = more complex/unclear)
	Issues           []string // Specific issues found
}

// analyzeDecompiledCode analyzes the quality of decompiled Java code
func analyzeDecompiledCode(content string) DecompiledCodeAnalysis {
	analysis := DecompiledCodeAnalysis{
		Issues: []string{},
	}

	lines := strings.Split(content, "\n")

	// Check for obfuscated variable names (var1, var2, a, b, etc.)
	obfuscatedPatterns := []string{
		"var[0-9]+", // var1, var2, var123
		" [a-z] =",  // single letter variables
		" [a-z] ;",
		"([a-z], [a-z])", // single letter in parameters
	}
	for _, line := range lines {
		for _, pattern := range obfuscatedPatterns {
			if strings.Contains(line, pattern) ||
				strings.Contains(line, " var") && len(strings.Fields(line)) > 0 {
				analysis.ObfuscatedVars++
			}
		}
	}

	// Check for decompilation error markers
	if strings.Contains(content, "/* Error decompiling") ||
		strings.Contains(content, "// $FF:") ||
		strings.Contains(content, "/* synthetic */") {
		analysis.HasDecompError = true
		analysis.Issues = append(analysis.Issues, "包含反编译错误标记")
	}

	// Check for unbalanced braces
	openBraces := strings.Count(content, "{")
	closeBraces := strings.Count(content, "}")
	if openBraces != closeBraces {
		analysis.UnbalancedBraces = true
		analysis.SyntaxIssues++
		analysis.Issues = append(analysis.Issues, fmt.Sprintf("括号不匹配（{=%d, }=%d）", openBraces, closeBraces))
	}

	// Check for unbalanced parentheses
	openParens := strings.Count(content, "(")
	closeParens := strings.Count(content, ")")
	if openParens != closeParens {
		analysis.SyntaxIssues++
		analysis.Issues = append(analysis.Issues, fmt.Sprintf("圆括号不匹配（(=%d, )=%d）", openParens, closeParens))
	}

	// Check for unclear string concatenations or messy code
	messyPatterns := []string{
		"\"\" + ", // Empty string concatenation
		"+ \"\" +",
		"(String)null",           // Null casts
		".toString().toString()", // Redundant calls
	}
	messyCount := 0
	for _, pattern := range messyPatterns {
		messyCount += strings.Count(content, pattern)
	}
	if messyCount > 5 {
		analysis.Issues = append(analysis.Issues, fmt.Sprintf("发现 %d 处冗余代码", messyCount))
		analysis.ComplexityScore += messyCount
	}

	// Calculate complexity score
	analysis.ComplexityScore = analysis.ObfuscatedVars*2 + analysis.SyntaxIssues*10
	if analysis.HasDecompError {
		analysis.ComplexityScore += 30
	}
	if analysis.UnbalancedBraces {
		analysis.ComplexityScore += 50
	}

	// Cap at 100
	if analysis.ComplexityScore > 100 {
		analysis.ComplexityScore = 100
	}

	return analysis
}

// formatAnalysis formats the analysis result for display
func formatAnalysis(analysis DecompiledCodeAnalysis) string {
	var parts []string

	if analysis.ObfuscatedVars > 0 {
		parts = append(parts, fmt.Sprintf("- 晦涩变量名：约 %d 处（如 var1, a, b 等）", analysis.ObfuscatedVars))
	}

	if analysis.SyntaxIssues > 0 {
		parts = append(parts, fmt.Sprintf("- 语法问题：%d 个", analysis.SyntaxIssues))
	}

	if analysis.HasDecompError {
		parts = append(parts, "- 包含反编译错误标记")
	}

	if len(analysis.Issues) > 0 {
		parts = append(parts, "- 具体问题："+strings.Join(analysis.Issues, "、"))
	}

	if len(parts) == 0 {
		return "代码质量良好，无明显问题"
	}

	return strings.Join(parts, "\n")
}

// getQualityLevel returns a quality assessment based on analysis
func getQualityLevel(analysis DecompiledCodeAnalysis) string {
	if analysis.SyntaxIssues > 0 || analysis.UnbalancedBraces {
		return "差（有语法错误）"
	}
	if analysis.ComplexityScore > 50 {
		return "较差（代码晦涩）"
	}
	if analysis.ComplexityScore > 20 {
		return "中等（需要优化）"
	}
	if analysis.ObfuscatedVars > 10 {
		return "尚可（变量名需改进）"
	}
	return "良好"
}

// getSuggestion returns action suggestions based on analysis
func getSuggestion(analysis DecompiledCodeAnalysis) string {
	if analysis.SyntaxIssues > 0 || analysis.UnbalancedBraces {
		return `1. 使用 rewrite_java_file 修复语法错误（指定行范围进行局部重写）
2. 如果问题广泛分布，使用完整重写模式（省略行范围参数）
3. 修复后使用 check_syntax 验证
4. 使用 compare_with_backup 查看修改差异`
	}

	if analysis.ComplexityScore > 30 || analysis.ObfuscatedVars > 15 {
		return `1. 使用 rewrite_java_file 完整重写模式改进代码质量（省略行范围参数）
2. 重点改进：变量命名、代码结构、可读性
3. 重写后使用 check_syntax 验证
4. 使用 compare_with_backup 确认改进效果`
	}

	if analysis.ObfuscatedVars > 5 {
		return `1. 使用 rewrite_java_file 改进变量命名（根据需要选择局部或完整重写）
2. 局部问题：指定行范围进行局部重写
3. 重写后使用 check_syntax 验证结果
4. 使用 compare_with_backup 查看变化`
	}

	return `1. 代码质量良好，可以直接使用
2. 如需进一步优化代码风格，可使用 rewrite_java_file
3. 满意后使用 finish 结束处理`
}
