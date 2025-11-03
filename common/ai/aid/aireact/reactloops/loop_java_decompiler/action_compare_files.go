package loop_java_decompiler

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/yakgit/yakdiff"
)

var compareFilesAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"compare_with_backup",
		"Compare a modified Java file with its backup (.bak) to see what changes were made. This helps verify that modifications are correct and nothing was lost.",
		[]aitool.ToolOption{
			aitool.WithStringParam("file_path", aitool.WithParam_Description("Path to the Java file to compare with its backup"), aitool.WithParam_Required(true)),
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			filePath := action.GetString("file_path")
			if filePath == "" {
				return utils.Error("file_path parameter is required")
			}

			// If not absolute, make it relative to working directory
			if !filepath.IsAbs(filePath) {
				workingDir := l.Get("working_directory")
				if workingDir != "" {
					filePath = filepath.Join(workingDir, filePath)
				}
			}

			// Check if file exists
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				invoker := l.GetInvoker()
				errorMsg := fmt.Sprintf(`【文件未找到】无法找到要比较的Java文件：%s

【可能原因】：
1. 文件路径拼写错误
2. 文件已被删除或移动
3. 使用了错误的相对路径
4. 工作目录设置不正确

【立即行动】：
1. 使用 list_files 查看当前目录的文件
2. 确认文件路径是否正确
3. 检查工作目录设置
4. 使用 read_java_file 确认文件是否存在

【建议】：
- 使用相对于工作目录的路径
- 确保文件扩展名为 .java
- 检查路径分隔符是否正确

【下一步】：使用 list_files 查看可用的文件列表`, filePath)
				invoker.AddToTimeline("compare_file_not_found", errorMsg)
				return utils.Errorf("file not found: %s", filePath)
			}

			// Check if backup exists
			backupPath := filePath + ".bak"
			if _, err := os.Stat(backupPath); os.IsNotExist(err) {
				invoker := l.GetInvoker()
				errorMsg := fmt.Sprintf(`【备份文件未找到】无法找到原始备份文件：%s

【可能原因】：
1. 该文件从未被反编译（没有创建备份）
2. 备份文件已被删除
3. 这是手动创建的文件（不是反编译生成的）
4. 反编译时备份创建失败

【重要信息】：
- 备份文件在反编译时自动创建
- 备份文件名格式：原文件名.bak
- 只有反编译生成的文件才有备份

【立即行动】：
1. 确认文件是否来自反编译
2. 检查是否存在 .bak 文件
3. 如果是新创建的文件，无需比较
4. 如果备份被删除，无法恢复

【建议】：
- 只对反编译生成的文件进行比较
- 如果备份丢失，可以重新反编译
- 新创建的文件不需要使用此操作

【下一步】：
- 如果需要重新反编译，使用 decompile_jar
- 如果是新文件，直接使用 check_syntax 验证语法`, backupPath)
				invoker.AddToTimeline("compare_backup_not_found", errorMsg)
				return utils.Errorf("backup file not found: %s", backupPath)
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

			backupPath := filePath + ".bak"

			invoker := loop.GetInvoker()
			invoker.AddToTimeline("compare_files", fmt.Sprintf("Comparing file %s with backup", filePath))

			// Read both files
			originalContent, err := os.ReadFile(backupPath)
			if err != nil {
				errorMsg := fmt.Sprintf(`【备份文件读取失败】无法读取备份文件内容

【错误详情】：%v

【可能原因】：
1. 文件权限问题
2. 文件已损坏
3. 磁盘IO错误
4. 文件正被其他进程占用

【立即行动】：
1. 检查文件权限设置
2. 验证文件是否损坏
3. 尝试直接打开文件查看
4. 检查磁盘健康状态

【建议】：
- 使用系统工具检查文件权限
- 如果文件损坏，考虑重新反编译
- 确保没有其他程序占用该文件`, err)
				invoker.AddToTimeline("compare_read_backup_error", errorMsg)
				op.Fail("failed to read backup file: " + err.Error())
				return
			}

			modifiedContent, err := os.ReadFile(filePath)
			if err != nil {
				errorMsg := fmt.Sprintf(`【文件读取失败】无法读取当前文件内容

【错误详情】：%v

【可能原因】：
1. 文件权限问题
2. 文件已损坏
3. 磁盘IO错误
4. 文件正被其他进程占用

【立即行动】：
1. 检查文件权限设置
2. 验证文件是否损坏
3. 尝试使用 read_java_file 操作
4. 检查磁盘健康状态

【建议】：
- 确保有读取权限
- 如果文件损坏，从备份恢复
- 检查是否有其他程序在编辑该文件`, err)
				invoker.AddToTimeline("compare_read_file_error", errorMsg)
				op.Fail("failed to read modified file: " + err.Error())
				return
			}

			// Use yakdiff to generate professional unified diff
			diffResult, err := yakdiff.Diff(originalContent, modifiedContent)
			if err != nil {
				errorMsg := fmt.Sprintf(`【差异生成失败】无法生成文件差异对比

【错误详情】：%v

【可能原因】：
1. 文件内容格式异常
2. 内存不足
3. diff引擎内部错误
4. 文件过大

【立即行动】：
1. 检查系统可用内存
2. 尝试手动查看两个文件
3. 如果文件很大，分段比较

【建议】：
- 对于大文件，使用外部diff工具
- 检查文件内容是否为有效文本
- 确保有足够的系统资源`, err)
				invoker.AddToTimeline("compare_diff_error", errorMsg)
				op.Fail("failed to generate diff: " + err.Error())
				return
			}

			// Prepare feedback message
			msg := fmt.Sprintf("Comparison of %s with backup:\n\n", filepath.Base(filePath))
			msg += fmt.Sprintf("Original file (backup): %d bytes\n", len(originalContent))
			msg += fmt.Sprintf("Modified file: %d bytes\n\n", len(modifiedContent))

			if strings.TrimSpace(diffResult) == "" {
				msg += "No differences found - files are identical."
				timelineMsg := fmt.Sprintf(`【文件对比完成】文件与备份完全相同：%s

【结果】：未发现任何差异

【说明】：
- 当前文件与原始反编译版本一致
- 没有进行任何修改
- 文件内容完全相同

【可能情况】：
1. 文件确实没有被修改
2. 修改后又撤销了
3. 使用了备份文件覆盖

【下一步建议】：
- 如果预期有修改但未发现差异，检查是否编辑了正确的文件
- 如果确认无需修改，可以继续处理其他文件
- 如果需要修改，使用 rewrite_java_file 进行编辑（局部或完整重写）`, filepath.Base(filePath))
				invoker.AddToTimeline("compare_no_changes", timelineMsg)
			} else {
				// Count diff lines (lines starting with + or -)
				diffLines := strings.Split(diffResult, "\n")
				addedLines := 0
				removedLines := 0
				for _, line := range diffLines {
					if len(line) > 0 {
						if line[0] == '+' && !strings.HasPrefix(line, "+++") {
							addedLines++
						} else if line[0] == '-' && !strings.HasPrefix(line, "---") {
							removedLines++
						}
					}
				}

				msg += fmt.Sprintf("Changes: +%d lines added, -%d lines removed\n\n", addedLines, removedLines)
				msg += "Unified Diff:\n"
				msg += "```diff\n"
				msg += diffResult

				// Limit total message size
				if len(msg) > 8000 {
					msg = msg[:8000] + "\n... (diff truncated)"
				}

				msg += "\n```"

				timelineMsg := fmt.Sprintf(`【文件对比完成】发现文件修改：%s

【修改统计】：
- 新增行数：+%d
- 删除行数：-%d
- 总变更量：%d 行

【对比结果已展示】：
- Unified Diff 格式已生成
- 绿色(+)：新增内容
- 红色(-)：删除内容

【下一步建议】：
1. 仔细审查变更内容是否符合预期
2. 确认修改没有引入新的语法错误
3. 如果发现问题，可以继续修改
4. 修改正确后，使用 check_syntax 验证
5. 如果需要撤销，可以从 .bak 恢复

【注意事项】：
- 大的修改建议分批验证
- 关注是否误删了重要代码
- 确保修改保持代码的逻辑完整性`, filepath.Base(filePath), addedLines, removedLines, addedLines+removedLines)
				invoker.AddToTimeline("compare_changes_found", timelineMsg)
			}

			op.Feedback(msg)
			op.Continue()
		},
	)
}
