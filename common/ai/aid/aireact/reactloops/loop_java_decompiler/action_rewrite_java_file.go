package loop_java_decompiler

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

var rewriteJavaFileAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"rewrite_java_file",
		`Rewrite decompiled Java code to improve readability and fix issues. This is the PRIMARY action for improving decompiled code.

【使用场景】：
1. 修复反编译产生的语法错误（括号不匹配、类型错误等）
2. 重命名晦涩的变量名（var1, var2, a, b 等）→ 有意义的名称
3. 整理混乱的字符串拼接和冗余代码
4. 改进代码结构，提高可读性
5. 完全重写整个文件或仅重写部分行

【操作模式】：
- 可以重写整个文件（不指定行号）
- 可以重写指定行范围（指定 rewrite_start_line 和 rewrite_end_line）
- AI 生成的新代码通过 <JAVA_CODE> 标签提供`,
		[]aitool.ToolOption{
			aitool.WithStringParam("file_path", aitool.WithParam_Description("Path to the Java file to rewrite (can be relative to working directory or absolute)"), aitool.WithParam_Required(true)),
			aitool.WithIntegerParam("rewrite_start_line", aitool.WithParam_Description("Starting line number to rewrite (1-based, inclusive). Leave empty to rewrite entire file.")),
			aitool.WithIntegerParam("rewrite_end_line", aitool.WithParam_Description("Ending line number to rewrite (1-based, inclusive). Leave empty to rewrite entire file.")),
			aitool.WithStringParam("rewrite_reason", aitool.WithParam_Description("Detailed explanation of what needs to be improved and why (e.g., 'Fix syntax errors', 'Rename obfuscated variables', 'Improve readability')")),
		},
		[]*reactloops.LoopStreamField{
			{
				FieldName: "rewrite_reason",
				AINodeId:  "re-act-loop-thought",
			},
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			filePath := action.GetString("file_path")
			if filePath == "" {
				r.AddToTimeline("rewrite_no_path", `【缺少文件路径】未指定要重写的Java文件路径

【立即行动】：
1. 确认要重写的文件
2. 使用 read_java_file 查看文件内容和问题分析
3. 提供文件路径重新调用 rewrite_java_file

【下一步】：先使用 read_java_file 读取文件`)
				return utils.Error("file_path parameter is required")
			}

			// If not absolute, make it relative to working directory
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
				r.AddToTimeline("rewrite_check_error", fmt.Sprintf(`【文件检查失败】无法检查文件：%s

【错误信息】：%v

【可能原因】：
1. 文件路径格式错误
2. 文件权限问题
3. 文件系统错误

【立即行动】：
1. 使用 list_files 确认文件存在
2. 检查文件路径拼写
3. 确认文件权限

【下一步】：修正路径后重试`, filePath, err))
				return utils.Errorf("failed to check file: %v", err)
			}

			if !exists {
				r.AddToTimeline("rewrite_file_not_found", fmt.Sprintf(`【文件不存在】指定的文件未找到：%s

【可能原因】：
1. 文件路径错误
2. 文件已被删除
3. 工作目录不正确：%s

【立即行动】：
1. 使用 list_files 查看可用文件
2. 确认文件路径
3. 检查工作目录设置

【下一步】：使用正确的文件路径重试`, filePath, l.Get("working_directory")))
				return utils.Errorf("file not found: %s", filePath)
			}

			start := action.GetInt("rewrite_start_line")
			end := action.GetInt("rewrite_end_line")

			// Validate line range if provided
			if (start > 0 || end > 0) && (start <= 0 || end <= 0 || end < start) {
				r.AddToTimeline("rewrite_invalid_range", fmt.Sprintf(`【行号范围无效】指定的行号范围不正确：%d-%d

【规则】：
1. 行号从 1 开始（第一行是 1）
2. 起始行号必须 ≤ 结束行号
3. 两个行号都要指定，或都不指定（重写整个文件）

【立即行动】：
1. 使用 read_java_file 查看文件内容和行号
2. 确定需要重写的具体行范围
3. 使用正确的行号重新调用

【建议】：
- 小范围修改：指定具体行号
- 大范围改进：重写整个文件（不指定行号）

【下一步】：重新指定正确的行号范围`, start, end))
				return utils.Error("rewrite_java_file requires valid line range (start <= end, both > 0) or no line numbers for full file rewrite")
			}

			l.GetEmitter().EmitTextPlainTextStreamEvent(
				"thought",
				bytes.NewReader([]byte(fmt.Sprintf("Preparing to rewrite Java file %s", filePath))),
				l.GetCurrentTask().GetIndex())

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

			rewriteStartLine := action.GetInt("rewrite_start_line")
			rewriteEndLine := action.GetInt("rewrite_end_line")
			rewriteReason := action.GetString("rewrite_reason")

			invoker := loop.GetInvoker()

			var mode string
			if rewriteStartLine > 0 && rewriteEndLine > 0 {
				mode = fmt.Sprintf("lines %d-%d", rewriteStartLine, rewriteEndLine)
			} else {
				mode = "entire file"
			}

			msg := fmt.Sprintf("Rewriting Java file %s (%s)", filePath, mode)
			invoker.AddToTimeline("rewrite_file", msg)

			if rewriteReason != "" {
				r.AddToTimeline("rewrite_reason", "重写原因："+rewriteReason)
			}

			// Read current file content using filesys
			fs := filesys.NewLocalFs()
			content, err := fs.ReadFile(filePath)
			if err != nil {
				r.AddToTimeline("rewrite_read_failed", fmt.Sprintf(`【读取文件失败】无法读取文件内容：%s

【错误信息】：%v

【可能原因】：
1. 文件权限不足
2. 文件被占用
3. 文件系统错误

【立即行动】：
1. 检查文件权限
2. 确认文件可读
3. 选择其他文件处理

【下一步】：解决权限问题或选择其他文件`, filePath, err))
				op.Fail("failed to read file: " + err.Error())
				return
			}

			// Create backup if it doesn't exist yet
			backupPath := filePath + ".bak"
			backupExists, _ := fs.Exists(backupPath)
			if !backupExists {
				err = fs.WriteFile(backupPath, content, 0644)
				if err != nil {
					log.Warnf("failed to create backup file: %v", err)
					r.AddToTimeline("backup_warning", "备份创建失败："+err.Error())
				} else {
					r.AddToTimeline("backup_created", "已创建备份："+backupPath)
				}
			} else {
				r.AddToTimeline("backup_exists", "备份已存在："+backupPath)
			}

			// Get rewritten content from AI-generated code
			// Try to get from loop context (from <JAVA_CODE> tag) first
			newCode := loop.Get("java_code")
			
			// Fall back to action parameter (for testing scenarios)
			if newCode == "" {
				newCode = action.GetString("new_code")
			}
			
			if newCode == "" {
				r.AddToTimeline("rewrite_no_code", `【缺少重写代码】未提供新的Java代码

【原因】：
AI 没有生成 <JAVA_CODE> 标签内容

【立即行动】：
1. 在 <JAVA_CODE> 标签中提供完整的重写代码
2. 确保代码格式正确
3. 包含必要的导入语句和包声明

【示例】：
<JAVA_CODE>
package com.example;

public class Example {
    private String meaningfulName;  // 改进的变量名
    
    public void processData() {
        // 重写后的清晰逻辑
    }
}
</JAVA_CODE>

【下一步】：重新生成包含代码的响应`)
				op.Fail("no rewritten code provided in <JAVA_CODE> tag")
				return
			}

			var finalContent string

			// Determine if rewriting entire file or specific lines
			if rewriteStartLine > 0 && rewriteEndLine > 0 {
				// Partial rewrite: replace specific line range
				lines := strings.Split(string(content), "\n")

				if rewriteStartLine > len(lines) || rewriteEndLine > len(lines) {
					r.AddToTimeline("rewrite_line_out_of_range", fmt.Sprintf(`【行号超出范围】指定的行号超出文件范围

【文件信息】：
- 文件总行数：%d
- 指定范围：%d-%d

【立即行动】：
1. 使用 read_java_file 确认文件行数
2. 调整行号范围
3. 重新调用 rewrite_java_file

【下一步】：使用正确的行号范围`, len(lines), rewriteStartLine, rewriteEndLine))
					op.Fail(fmt.Sprintf("line range %d-%d exceeds file length (%d lines)", rewriteStartLine, rewriteEndLine, len(lines)))
					return
				}

				// Build new content: before + new code + after
				before := lines[:rewriteStartLine-1]
				after := lines[rewriteEndLine:]

				finalContent = strings.Join(before, "\n")
				if len(before) > 0 {
					finalContent += "\n"
				}
				finalContent += newCode
				if len(after) > 0 {
					finalContent += "\n" + strings.Join(after, "\n")
				}

				log.Infof("rewrote Java file lines %d-%d in %s", rewriteStartLine, rewriteEndLine, filePath)
				r.AddToTimeline("rewrite_partial", fmt.Sprintf("部分重写：第 %d-%d 行（共 %d 行）", rewriteStartLine, rewriteEndLine, len(lines)))
			} else {
				// Full file rewrite
				finalContent = newCode
				log.Infof("rewrote entire Java file %s", filePath)
				r.AddToTimeline("rewrite_full", "完全重写整个文件")
			}

			// Write back to file
			err = fs.WriteFile(filePath, []byte(finalContent), 0644)
			if err != nil {
				r.AddToTimeline("rewrite_write_failed", fmt.Sprintf(`【写入文件失败】无法保存重写后的代码：%s

【错误信息】：%v

【可能原因】：
1. 文件权限不足（只读）
2. 磁盘空间不足
3. 文件被其他程序锁定

【数据安全】：
- 原始内容未被修改
- 备份文件安全（如果已创建）

【立即行动】：
1. 检查文件权限（需要写权限）
2. 确认磁盘空间充足
3. 关闭可能占用文件的程序

【下一步】：解决权限问题后重试`, filePath, err))
				op.Fail("failed to write file: " + err.Error())
				return
			}

			// Update context
			loop.Set("current_file", filePath)
			loop.Set("current_file_content", finalContent)

			// Increment rewritten files counter
			rewrittenFiles := loop.GetInt("rewritten_files")
			loop.Set("rewritten_files", rewrittenFiles+1)

			// Prepare feedback
			preview := utils.ShrinkTextBlock(newCode, 800)
			feedback := fmt.Sprintf(`成功重写 Java 文件：%s

【重写模式】：%s

【文件信息】：
- 文件路径：%s
- 备份位置：%s

【重写后代码预览】：
%s

【统计】：
- 本次重写：第 %d 个文件
- 新代码长度：%d 字节

【下一步建议】：
1. 使用 check_syntax 验证语法正确性
2. 使用 compare_with_backup 查看具体改动
3. 继续处理其他文件或使用 finish 结束`,
				filePath,
				mode,
				filePath,
				backupPath,
				preview,
				rewrittenFiles+1,
				len(finalContent))

			if rewriteReason != "" {
				feedback += "\n\n【重写原因】：" + rewriteReason
			}

			timelineMsg := fmt.Sprintf(`【重写成功】%s

【模式】：%s
【文件】：已重写 %d 个文件
【代码】：%d 字节

【下一步】：验证语法或继续处理其他文件`,
				filePath,
				mode,
				rewrittenFiles+1,
				len(finalContent))

			r.AddToTimeline("rewrite_success", timelineMsg)
			op.Feedback(feedback)
			op.Continue()
		},
	)
}
