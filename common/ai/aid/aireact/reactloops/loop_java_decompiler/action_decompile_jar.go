package loop_java_decompiler

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/javaclassparser/jarwar"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

var decompileJarAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"decompile_jar",
		"Decompile a JAR/WAR file to Java source files. This should be the first action when working with JAR files. The output directory will contain all decompiled Java source files.",
		[]aitool.ToolOption{
			aitool.WithStringParam("jar_path", aitool.WithParam_Description("Path to the input JAR/WAR file to decompile"), aitool.WithParam_Required(true)),
			aitool.WithStringParam("output_dir", aitool.WithParam_Description("Output directory path where decompiled Java files will be stored")),
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			jarPath := action.GetString("jar_path")
			if jarPath == "" {
				return utils.Error("jar_path parameter is required")
			}

			// Check if JAR file exists
			if utils.GetFirstExistedFile(jarPath) == "" {
				invoker := l.GetInvoker()
				errorMsg := fmt.Sprintf(`【JAR文件未找到】无法找到指定的JAR文件：%s

【可能原因】：
1. 文件路径拼写错误或格式不正确
2. 文件不存在于指定位置
3. 使用了相对路径但当前工作目录不正确
4. 文件权限问题导致无法访问

【立即行动】：
1. 使用 list_files 检查目标目录内容
2. 确认JAR文件的完整路径
3. 检查文件名是否包含特殊字符
4. 验证文件扩展名是否为 .jar 或 .war

【建议】：
- 优先使用绝对路径而非相对路径
- 确保路径中没有拼写错误
- 如果是用户提供的路径，请再次确认

【警告】：必须提供有效的JAR文件路径才能继续反编译操作！`, jarPath)
				invoker.AddToTimeline("decompile_jar_not_found", errorMsg)
				return utils.Errorf("JAR file not found: %s", jarPath)
			}

			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			jarPath := action.GetString("jar_path")
			outputDir := action.GetString("output_dir")

			// If no output directory specified, create one based on JAR name
			if outputDir == "" {
				jarName := filepath.Base(jarPath)
				jarName = jarName[:len(jarName)-len(filepath.Ext(jarName))]
				outputDir = filepath.Join(filepath.Dir(jarPath), jarName+"_decompiled")
			}

			// Make output directory absolute
			outputDir, err := filepath.Abs(outputDir)
			if err != nil {
				invoker := loop.GetInvoker()
				errorMsg := fmt.Sprintf(`【路径解析失败】无法获取输出目录的绝对路径

【错误详情】：%v

【可能原因】：
1. 输出路径包含无效字符
2. 路径格式不符合操作系统要求
3. 路径长度超过系统限制
4. 磁盘或文件系统错误

【立即行动】：
1. 检查输出路径是否包含特殊字符
2. 尝试使用更简短的路径名
3. 确认路径格式符合当前操作系统

【建议】：
- 使用简单的ASCII字符命名目录
- 避免过长的路径嵌套
- 可以不指定output_dir参数，让系统自动生成

【警告】：路径解析失败将无法创建输出目录！`, err)
				invoker.AddToTimeline("decompile_path_error", errorMsg)
				op.Fail("failed to get absolute path: " + err.Error())
				return
			}

			invoker := loop.GetInvoker()
			invoker.AddToTimeline("decompile_start", fmt.Sprintf("Starting decompilation of JAR file: %s to %s", jarPath, outputDir))

			// Create output directory
			err = os.MkdirAll(outputDir, 0755)
			if err != nil {
				errorMsg := fmt.Sprintf(`【目录创建失败】无法创建输出目录：%s

【错误详情】：%v

【可能原因】：
1. 磁盘空间不足
2. 没有写入权限
3. 父目录不存在或无法访问
4. 目录名包含非法字符
5. 路径已存在且是一个文件（而非目录）

【立即行动】：
1. 检查磁盘剩余空间 (df -h)
2. 确认当前用户对父目录有写权限
3. 尝试使用不同的输出目录
4. 检查目标位置是否已存在同名文件

【建议】：
- 选择有足够空间和权限的目录
- 使用用户主目录下的路径
- 检查并清理已存在的同名文件或目录

【警告】：无法创建输出目录将导致反编译失败！`, outputDir, err)
				invoker.AddToTimeline("decompile_mkdir_error", errorMsg)
				op.Fail("failed to create output directory: " + err.Error())
				return
			}

			// Decompile JAR file
			log.Infof("decompiling JAR file: %s to %s", jarPath, outputDir)
			decompileStartTime := time.Now()
			err = jarwar.AutoDecompile(jarPath, outputDir)
			decompileDuration := time.Since(decompileStartTime)
			if err != nil {
				errorMsg := fmt.Sprintf(`【反编译失败】JAR文件反编译过程中遇到错误

【错误详情】：%v

【可能原因】：
1. JAR文件损坏或格式不正确
2. 不是有效的JAR/WAR文件
3. 文件被加密或使用了特殊保护
4. 反编译器不支持该JAR的版本
5. 文件正在被其他程序占用
6. 内存不足或系统资源限制

【立即行动】：
1. 验证文件确实是有效的JAR/WAR文件 (file命令或ZIP工具)
2. 检查文件是否完整，没有下载中断
3. 尝试用ZIP工具打开检查文件结构
4. 确认文件大小是否异常（太小可能损坏）
5. 检查系统可用内存

【建议】：
- 如果是网络下载的文件，重新下载确保完整性
- 尝试使用MD5/SHA256校验文件完整性
- 检查是否有其他进程占用该文件
- 对于超大JAR文件，确保有足够的内存和磁盘空间

【警告】：反编译失败意味着无法提取Java源代码！`, err)
				invoker.AddToTimeline("decompile_execution_error", errorMsg)
				op.Fail(fmt.Sprintf("failed to decompile JAR file: %v", err))
				return
			}

			// Create backup copies of all decompiled files (*.java -> *.java.bak)
			log.Infof("creating backup copies of decompiled files")
			backupCount := 0
			filepath.Walk(outputDir, func(path string, info os.FileInfo, err error) error {
				if err == nil && !info.IsDir() && filepath.Ext(path) == ".java" {
					backupPath := path + ".bak"
					content, err := os.ReadFile(path)
					if err == nil {
						if err := os.WriteFile(backupPath, content, 0644); err == nil {
							backupCount++
						}
					}
				}
				return nil
			})

			// Check for compilation errors and count files
			log.Infof("checking for compilation errors in decompiled files")
			checkStartTime := time.Now()
			totalFiles := 0
			filesWithIssues := 0
			var compilationErrors []string
			var filesList []string

			filepath.Walk(outputDir, func(path string, info os.FileInfo, err error) error {
				if err == nil && !info.IsDir() && filepath.Ext(path) == ".java" {
					totalFiles++
					relPath, _ := filepath.Rel(outputDir, path)
					filesList = append(filesList, relPath)

					// Quick syntax check on decompiled files
					content, readErr := os.ReadFile(path)
					if readErr == nil {
						issues := checkBasicJavaSyntax(string(content))
						if len(issues) > 0 {
							filesWithIssues++
							for _, issue := range issues {
								compilationErrors = append(compilationErrors, fmt.Sprintf("%s: %s", relPath, issue))
							}
						}
					}
				}
				return nil
			})
			checkDuration := time.Since(checkStartTime)

			// Generate README.md with decompilation report
			readmePath := filepath.Join(outputDir, "README.md")
			readme := generateDecompilationReport(
				jarPath,
				outputDir,
				totalFiles,
				backupCount,
				filesWithIssues,
				decompileDuration,
				checkDuration,
				compilationErrors,
				filesList,
			)

			if err := os.WriteFile(readmePath, []byte(readme), 0644); err != nil {
				log.Warnf("failed to write README.md: %v", err)
			} else {
				log.Infof("generated decompilation report: %s", readmePath)
			}

			// Save compilation errors to a separate file if any exist
			if len(compilationErrors) > 0 {
				errorsPath := filepath.Join(outputDir, "COMPILATION_ERRORS.txt")
				errorsContent := strings.Join(compilationErrors, "\n")
				if err := os.WriteFile(errorsPath, []byte(errorsContent), 0644); err != nil {
					log.Warnf("failed to write COMPILATION_ERRORS.txt: %v", err)
				} else {
					log.Infof("saved compilation errors to: %s", errorsPath)
				}
			}

			// Store context
			loop.Set("working_directory", outputDir)
			loop.Set("jar_path", jarPath)
			loop.Set("total_files", totalFiles)
			loop.Set("files_with_issues", filesWithIssues)
			loop.Set("compilation_errors_count", len(compilationErrors))

			// Prepare feedback message
			msg := fmt.Sprintf("Successfully decompiled JAR file.\n\n"+
				"Summary:\n"+
				"- Total Java files: %d\n"+
				"- Backup files created: %d\n"+
				"- Files with potential issues: %d\n"+
				"- Output directory: %s\n\n"+
				"Reports generated:\n"+
				"- README.md: Full decompilation report\n",
				totalFiles, backupCount, filesWithIssues, outputDir)

			if len(compilationErrors) > 0 {
				msg += "- COMPILATION_ERRORS.txt: List of detected issues\n\n"
				msg += fmt.Sprintf("Found %d potential compilation issues. Review COMPILATION_ERRORS.txt for details.\n", len(compilationErrors))

				// Add detailed suggestions to Timeline for handling compilation errors
				timelineMsg := fmt.Sprintf(`【反编译完成但存在编译问题】成功反编译但检测到 %d 个潜在编译问题

【当前状态】：
- 总文件数：%d
- 存在问题的文件：%d
- 问题详情已保存到：COMPILATION_ERRORS.txt

【常见问题类型】：
1. 反编译器局限性导致的语法错误
2. 泛型类型推断不完整
3. Lambda表达式反编译不准确
4. 内部类和匿名类的引用问题
5. 混淆代码的变量名冲突

【推荐处理流程】：
1. 首先查看 README.md 了解整体情况
2. 查看 COMPILATION_ERRORS.txt 了解具体错误
3. 使用 read_java_file 读取有问题的文件
4. 使用 check_syntax 验证具体语法问题
5. 使用 rewrite_java_file 修复语法错误（局部重写或完整重写）
6. 使用 compare_with_backup 对比修改前后

【优先修复策略】：
- 先修复高频出现的错误模式
- 关注核心业务逻辑文件
- 工具类和配置文件可以后续处理
- 测试类的错误优先级较低

【注意事项】：
- 所有原始文件都有 .bak 备份
- 可以随时使用 compare_with_backup 查看改动
- 建议批量处理相同类型的错误

【下一步行动】：使用 read_java_file 或 list_files 开始检查问题文件`, len(compilationErrors), totalFiles, filesWithIssues)
				invoker.AddToTimeline("decompile_success_with_issues", timelineMsg)
			} else {
				msg += "\nNo obvious compilation issues detected.\n"
				invoker.AddToTimeline("decompile_success", fmt.Sprintf("成功反编译 %d 个文件，未检测到明显编译问题。工作目录：%s", totalFiles, outputDir))
			}

			msg += "\nNote: Original decompiled files have been backed up with .bak extension (e.g., MyClass.java.bak). You can use compare_with_backup action to see changes."
			op.Feedback(msg)
			op.Continue()
		},
	)
}

// generateDecompilationReport creates a detailed README.md for the decompiled output
func generateDecompilationReport(
	jarPath string,
	outputDir string,
	totalFiles int,
	backupCount int,
	filesWithIssues int,
	decompileDuration time.Duration,
	checkDuration time.Duration,
	compilationErrors []string,
	filesList []string,
) string {
	var sb strings.Builder

	sb.WriteString("# JAR Decompilation Report\n\n")
	sb.WriteString(fmt.Sprintf("**Generated**: %s\n\n", time.Now().Format("2006-01-02 15:04:05")))

	sb.WriteString("## Source Information\n\n")
	sb.WriteString(fmt.Sprintf("- **JAR File**: `%s`\n", filepath.Base(jarPath)))
	sb.WriteString(fmt.Sprintf("- **Full Path**: `%s`\n", jarPath))
	sb.WriteString(fmt.Sprintf("- **Output Directory**: `%s`\n\n", outputDir))

	sb.WriteString("## Decompilation Statistics\n\n")
	sb.WriteString(fmt.Sprintf("- **Total Java Files**: %d\n", totalFiles))
	sb.WriteString(fmt.Sprintf("- **Backup Files Created**: %d\n", backupCount))
	sb.WriteString(fmt.Sprintf("- **Files with Potential Issues**: %d\n", filesWithIssues))
	sb.WriteString(fmt.Sprintf("- **Decompilation Time**: %v\n", decompileDuration.Round(time.Millisecond)))
	sb.WriteString(fmt.Sprintf("- **Syntax Check Time**: %v\n\n", checkDuration.Round(time.Millisecond)))

	if filesWithIssues > 0 {
		sb.WriteString("## ⚠️ Compilation Issues Detected\n\n")
		sb.WriteString(fmt.Sprintf("Found %d potential issues in %d files. Common causes:\n\n", len(compilationErrors), filesWithIssues))
		sb.WriteString("1. Decompiler limitations (may not perfectly reconstruct source)\n")
		sb.WriteString("2. Use of obfuscation in original JAR\n")
		sb.WriteString("3. Advanced Java features that are hard to decompile\n")
		sb.WriteString("4. Missing dependencies or libraries\n\n")

		sb.WriteString("### Issue Summary\n\n")
		if len(compilationErrors) > 20 {
			sb.WriteString("```\n")
			for i, err := range compilationErrors[:20] {
				sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, err))
			}
			sb.WriteString(fmt.Sprintf("... and %d more issues\n", len(compilationErrors)-20))
			sb.WriteString("```\n\n")
			sb.WriteString(fmt.Sprintf("See `COMPILATION_ERRORS.txt` for the complete list of all %d issues.\n\n", len(compilationErrors)))
		} else {
			sb.WriteString("```\n")
			for i, err := range compilationErrors {
				sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, err))
			}
			sb.WriteString("```\n\n")
		}
	} else {
		sb.WriteString("## Compilation Status\n\n")
		sb.WriteString("No obvious compilation issues detected. The decompiled code appears to be syntactically correct.\n\n")
		sb.WriteString("**Note**: This is a basic syntax check. Some issues may only be discovered during actual compilation with javac.\n\n")
	}

	sb.WriteString("## File Structure\n\n")
	if totalFiles <= 50 {
		sb.WriteString("### All Decompiled Files\n\n")
		sb.WriteString("```\n")
		for _, file := range filesList {
			sb.WriteString(file + "\n")
		}
		sb.WriteString("```\n\n")
	} else {
		sb.WriteString(fmt.Sprintf("This JAR contains %d files. First 50 files:\n\n", totalFiles))
		sb.WriteString("```\n")
		for i := 0; i < 50 && i < len(filesList); i++ {
			sb.WriteString(filesList[i] + "\n")
		}
		sb.WriteString(fmt.Sprintf("... and %d more files\n", totalFiles-50))
		sb.WriteString("```\n\n")
	}

	sb.WriteString("## Backup Information\n\n")
	sb.WriteString("All decompiled `.java` files have been backed up with a `.bak` extension:\n\n")
	sb.WriteString("- Original decompiled file: `MyClass.java.bak`\n")
	sb.WriteString("- Working file (can be modified): `MyClass.java`\n\n")
	sb.WriteString("Use the `compare_with_backup` action to see differences between modified and original files.\n\n")

	sb.WriteString("## Next Steps\n\n")
	sb.WriteString("1. **Review Files**: Use `list_files` action to explore the directory structure\n")
	sb.WriteString("2. **Read Code**: Use `read_java_file` action to examine specific files\n")
	sb.WriteString("3. **Fix Issues**: Use `rewrite_java_file` action to fix compilation errors (partial or full rewrite)\n")
	sb.WriteString("4. **Check Syntax**: Use `check_syntax` action to verify fixes\n")
	sb.WriteString("5. **Compare**: Use `compare_with_backup` action to review changes\n\n")

	sb.WriteString("## Important Notes\n\n")
	sb.WriteString("- **Decompilation Limitations**: Decompilers cannot perfectly reconstruct the original source code\n")
	sb.WriteString("- **Obfuscation**: If the JAR was obfuscated, code may have unclear names and structure\n")
	sb.WriteString("- **Dependencies**: External libraries and dependencies are not included\n")
	sb.WriteString("- **Bytecode Features**: Some bytecode-level features may not have Java source equivalents\n")

	return sb.String()
}
