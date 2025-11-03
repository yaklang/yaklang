package loop_java_decompiler

import (
	"fmt"
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

var listFilesAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"list_files",
		"List all Java files in a directory. Use this to discover what files exist and need to be checked or fixed.",
		[]aitool.ToolOption{
			aitool.WithStringParam("directory_path", aitool.WithParam_Description("Directory path to list Java files from. If not specified, uses the working directory.")),
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			dirPath := action.GetString("directory_path")
			if dirPath == "" {
				dirPath = l.Get("working_directory")
			}
			if dirPath == "" {
				invoker := l.GetInvoker()
				errorMsg := `【目录路径未提供】无法列出文件，需要指定目录路径

【可能原因】：
1. 未提供 directory_path 参数
2. 未设置工作目录 (working_directory)
3. 尚未反编译 JAR 文件

【立即行动】：
1. 如果需要列出特定目录，提供 directory_path 参数
2. 如果要列出反编译后的文件，先使用 decompile_jar
3. 确认当前是否有可用的工作目录

【建议】：
- 先执行 decompile_jar 创建工作目录
- 或者明确指定要列出的目录路径

【下一步】：使用 decompile_jar 反编译 JAR 文件，或提供明确的目录路径`
				invoker.AddToTimeline("list_files_no_directory", errorMsg)
				return utils.Error("directory_path is required or working_directory must be set")
			}

			// Create filesystem instance
			fs := filesys.NewLocalFs()

			// Check if directory exists
			exists, err := fs.Exists(dirPath)
			if err != nil {
				invoker := l.GetInvoker()
				errorMsg := fmt.Sprintf(`【目录检查失败】无法检查目录是否存在：%s

【错误详情】：%v

【可能原因】：
1. 路径格式不正确
2. 权限问题
3. 文件系统错误
4. 路径过长

【立即行动】：
1. 确认路径格式是否正确
2. 检查目录访问权限
3. 尝试使用绝对路径
4. 验证路径是否包含特殊字符

【建议】：
- 使用简单的ASCII字符路径
- 确保有读取权限
- 可以尝试列出父目录

【下一步】：修正路径格式或检查权限设置`, dirPath, err)
				invoker.AddToTimeline("list_files_check_error", errorMsg)
				return utils.Errorf("failed to check directory existence: %v", err)
			}

			if !exists {
				invoker := l.GetInvoker()
				errorMsg := fmt.Sprintf(`【目录不存在】指定的目录不存在：%s

【可能原因】：
1. 目录路径拼写错误
2. 目录尚未创建
3. 使用了错误的相对路径
4. 目录已被删除或移动

【立即行动】：
1. 使用 list_files 检查父目录内容
2. 确认目录路径的拼写
3. 检查是否需要先创建目录
4. 验证工作目录设置

【建议】：
- 如果是反编译目录，先执行 decompile_jar
- 使用绝对路径避免相对路径问题
- 检查路径中的目录分隔符是否正确

【下一步】：
- 如果是反编译目录，使用 decompile_jar 创建
- 如果是其他目录，确认路径是否正确`, dirPath)
				invoker.AddToTimeline("list_files_not_found", errorMsg)
				return utils.Errorf("directory not found: %s", dirPath)
			}

			// Check if it's actually a directory
			info, err := fs.Stat(dirPath)
			if err != nil {
				invoker := l.GetInvoker()
				errorMsg := fmt.Sprintf(`【目录状态获取失败】无法获取目录信息：%s

【错误详情】：%v

【可能原因】：
1. 权限不足
2. 路径是符号链接但目标不存在
3. 文件系统错误
4. 路径被占用

【立即行动】：
1. 检查目录访问权限
2. 确认不是损坏的符号链接
3. 尝试使用其他目录

【建议】：
- 确保有读取权限
- 避免使用符号链接
- 选择有权限访问的目录`, dirPath, err)
				invoker.AddToTimeline("list_files_stat_error", errorMsg)
				return utils.Errorf("failed to stat directory: %v", err)
			}

			if !info.IsDir() {
				invoker := l.GetInvoker()
				errorMsg := fmt.Sprintf(`【路径不是目录】指定的路径是文件而非目录：%s

【问题】：提供的路径指向一个文件，而非目录

【可能原因】：
1. 路径拼写导致指向了文件
2. 误将文件路径当作目录
3. 同名文件覆盖了目录

【立即行动】：
1. 检查路径是否正确
2. 如果要读取文件，使用 read_java_file
3. 如果要列出文件所在目录，使用父目录路径

【建议】：
- 对于文件，使用 read_java_file 而非 list_files
- 对于目录，确保路径指向目录
- 可以移除路径末尾的文件名部分

【下一步】：
- 如果要读取该文件：使用 read_java_file
- 如果要列出目录：使用正确的目录路径`, dirPath)
				invoker.AddToTimeline("list_files_not_directory", errorMsg)
				return utils.Errorf("path is not a directory: %s", dirPath)
			}

			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			dirPath := action.GetString("directory_path")
			if dirPath == "" {
				dirPath = loop.Get("working_directory")
			}

			invoker := loop.GetInvoker()
			invoker.AddToTimeline("list_files_start", fmt.Sprintf("开始列出目录中的 Java 文件：%s", dirPath))

			// Create filesystem instance
			fs := filesys.NewLocalFs()

			var javaFiles []string
			var walkErr error

			// Use filesys.Recursive to walk through the directory
			err := filesys.Recursive(dirPath,
				filesys.WithFileSystem(fs),
				filesys.WithFileStat(func(path string, info os.FileInfo) error {
					if strings.HasSuffix(path, ".java") {
						// Make path relative to dirPath for cleaner display
						relPath, err := fs.Rel(dirPath, path)
						if err == nil {
							javaFiles = append(javaFiles, relPath)
						} else {
							javaFiles = append(javaFiles, path)
						}
					}
					return nil
				}),
			)

			if err != nil {
				walkErr = err
				errorMsg := fmt.Sprintf(`【目录遍历失败】无法遍历目录获取文件列表：%s

【错误详情】：%v

【可能原因】：
1. 权限不足无法访问部分子目录
2. 目录结构过于复杂
3. 存在损坏的符号链接
4. 文件系统错误
5. 递归深度过大

【立即行动】：
1. 检查目录及子目录的访问权限
2. 查看是否有损坏的文件或链接
3. 尝试列出更具体的子目录
4. 检查磁盘健康状态

【建议】：
- 如果权限不足，尝试使用有权限的目录
- 对于大型目录，直接指定需要的子目录
- 修复或删除损坏的符号链接
- 确保目录结构正常

【警告】：部分文件可能已列出，但遍历未完成！`, dirPath, err)
				invoker.AddToTimeline("list_files_walk_error", errorMsg)

				// If we have some files, continue with partial results
				if len(javaFiles) == 0 {
					op.Fail("failed to list files: " + err.Error())
					return
				}
				// Otherwise, show warning but continue with partial results
			}

			if len(javaFiles) == 0 {
				msg := fmt.Sprintf("No Java files found in directory: %s", dirPath)

				timelineMsg := fmt.Sprintf(`【未找到 Java 文件】目录中没有 Java 文件：%s

【结果】：目录遍历完成，但未发现任何 .java 文件

【可能原因】：
1. 该目录确实不包含 Java 文件
2. 文件扩展名不是 .java
3. Java 文件在子目录中但遍历未到达
4. 文件被过滤或隐藏

【立即行动】：
1. 确认这是正确的目录
2. 检查是否有子目录
3. 确认文件扩展名是否为 .java
4. 尝试列出具体的子目录

【建议】：
- 如果是反编译目录，检查反编译是否成功
- 确认目录路径是否正确
- 可能需要指定更具体的子目录路径

【下一步】：
- 检查目录结构是否正确
- 如果是错误的目录，使用正确的路径
- 如果需要，先执行 decompile_jar`, dirPath)
				invoker.AddToTimeline("list_files_empty", timelineMsg)

				op.Feedback(msg)
				op.Continue()
				return
			}

			// Create a formatted list
			fileList := strings.Join(javaFiles, "\n  ")
			msg := fmt.Sprintf("Found %d Java files:\n  %s", len(javaFiles), fileList)

			// Limit message size if too many files
			if len(msg) > 2000 {
				msg = fmt.Sprintf("Found %d Java files. First 20:\n  %s\n  ... and %d more files",
					len(javaFiles),
					strings.Join(javaFiles[:20], "\n  "),
					len(javaFiles)-20)
			}

			if walkErr != nil {
				msg += fmt.Sprintf("\n\nWarning: Directory traversal had errors, results may be incomplete: %v", walkErr)
			}

			timelineMsg := fmt.Sprintf(`【文件列表成功】在目录中找到 %d 个 Java 文件

【结果】：
- 目录：%s
- Java 文件数：%d
- 文件列表已展示

【下一步建议】：
1. 使用 read_java_file 读取具体文件内容
2. 使用 check_syntax 检查文件语法
3. 使用 rewrite_java_file 重写文件内容（可进行局部重写或完整重写）
4. 根据文件名识别可能有问题的文件

【注意事项】：
- 文件路径已相对化，便于引用
- 如果文件很多，仅显示前20个
- 可以直接使用相对路径操作文件

【提示】：
- 关注文件名中的关键字（如 Main, Config, Utils等）
- 优先处理核心业务文件
- 如有编译错误列表，对照处理`, len(javaFiles), dirPath, len(javaFiles))
			invoker.AddToTimeline("list_files_success", timelineMsg)

			op.Feedback(msg)
			op.Continue()
		},
	)
}
