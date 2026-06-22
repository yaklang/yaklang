package loop_java_decompiler

import (
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
	return reactloops.WithRegisterLoopAction(
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
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			filePath := action.GetString("file_path")
			if filePath == "" {
				log.Warnf("[rewrite_java_file] file_path is required")
				r.AddToTimeline("rewrite_no_path", `【缺少文件路径】未指定要重写的Java文件路径

【立即行动】：
1. 确认要重写的文件
2. 使用 read_java_file 查看文件内容和问题分析
3. 提供文件路径重新调用 rewrite_java_file

【下一步】：先使用 read_java_file 读取文件`)
				return utils.Error("file_path parameter is required")
			}

			if !filepath.IsAbs(filePath) {
				workingDir := l.Get("working_directory")
				if workingDir != "" {
					filePath = filepath.Join(workingDir, filePath)
				}
			}

			fs := filesys.NewLocalFs()
			exists, err := fs.Exists(filePath)
			if err != nil {
				log.Errorf("[rewrite_java_file] failed to check file %s: %v", filePath, err)
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
				log.Warnf("[rewrite_java_file] file not found: %s", filePath)
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
			if (start > 0 || end > 0) && (start <= 0 || end <= 0 || end < start) {
				log.Warnf("[rewrite_java_file] invalid line range: %d-%d", start, end)
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

			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			const nodeID = "java-rewrite-file"

			filePath := action.GetString("file_path")
			if !filepath.IsAbs(filePath) {
				workingDir := loop.Get("working_directory")
				if workingDir != "" {
					filePath = filepath.Join(workingDir, filePath)
				}
			}

			rewriteStartLine := action.GetInt("rewrite_start_line")
			rewriteEndLine := action.GetInt("rewrite_end_line")

			var mode string
			if rewriteStartLine > 0 && rewriteEndLine > 0 {
				mode = fmt.Sprintf("lines %d-%d", rewriteStartLine, rewriteEndLine)
			} else {
				mode = "entire file"
			}

			invoker := loop.GetInvoker()
			startLine := fmt.Sprintf("重写 Java 文件: %s (%s)", filepath.Base(filePath), mode)
			reactloops.EmitActionLog(loop, nodeID, startLine)
			reactloops.EmitStatus(loop, "重写文件中 / Rewriting File...")

			fs := filesys.NewLocalFs()
			content, err := fs.ReadFile(filePath)
			if err != nil {
				log.Errorf("[rewrite_java_file] failed to read %s: %v", filePath, err)
				reactloops.EmitStatus(loop, "读取失败 / Read Failed")
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

			backupPath := filePath + ".bak"
			backupExists, _ := fs.Exists(backupPath)
			if !backupExists {
				if writeErr := fs.WriteFile(backupPath, content, 0644); writeErr != nil {
					log.Warnf("[rewrite_java_file] failed to create backup %s: %v", backupPath, writeErr)
					r.AddToTimeline("backup_warning", "备份创建失败："+writeErr.Error())
				} else {
					log.Infof("[rewrite_java_file] created backup: %s", backupPath)
					r.AddToTimeline("backup_created", "已创建备份："+backupPath)
				}
			} else {
				r.AddToTimeline("backup_exists", "备份已存在："+backupPath)
			}

			newCode := loop.Get("java_code")
			if newCode == "" {
				newCode = action.GetString("new_code")
			}
			if newCode == "" {
				log.Warnf("[rewrite_java_file] no rewritten code provided for %s", filePath)
				reactloops.EmitStatus(loop, "缺少重写代码 / No Rewritten Code")
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
			if rewriteStartLine > 0 && rewriteEndLine > 0 {
				lines := strings.Split(string(content), "\n")
				if rewriteStartLine > len(lines) || rewriteEndLine > len(lines) {
					log.Warnf("[rewrite_java_file] line range %d-%d exceeds file length %d", rewriteStartLine, rewriteEndLine, len(lines))
					reactloops.EmitStatus(loop, "行号超出范围 / Line Range Out of Bounds")
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
				log.Infof("[rewrite_java_file] partial rewrite lines %d-%d in %s", rewriteStartLine, rewriteEndLine, filePath)
				r.AddToTimeline("rewrite_partial", fmt.Sprintf("部分重写：第 %d-%d 行（共 %d 行）", rewriteStartLine, rewriteEndLine, len(lines)))
			} else {
				finalContent = newCode
				log.Infof("[rewrite_java_file] full rewrite of %s", filePath)
				r.AddToTimeline("rewrite_full", "完全重写整个文件")
			}

			err = fs.WriteFile(filePath, []byte(finalContent), 0644)
			if err != nil {
				log.Errorf("[rewrite_java_file] failed to write %s: %v", filePath, err)
				reactloops.EmitStatus(loop, "写入失败 / Write Failed")
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

			loop.Set("current_file", filePath)
			loop.Set("current_file_content", finalContent)

			rewrittenFiles := loop.GetInt("rewritten_files")
			loop.Set("rewritten_files", rewrittenFiles+1)

			ctx := op.GetContext()
			if ctx == nil {
				ctx = invoker.GetConfig().GetContext()
			}
			syntaxIssues := checkJavaFileSyntax(ctx, finalContent, filePath)
			hasSyntaxErrors := len(syntaxIssues) > 0
			if hasSyntaxErrors {
				op.DisallowNextLoopExit()
				log.Infof("[rewrite_java_file] syntax errors remain in %s", filePath)
			}

			preview := utils.ShrinkTextBlock(newCode, 500)
			var syntaxStatus string
			if hasSyntaxErrors {
				syntaxStatus = fmt.Sprintf("语法问题 %d 个", len(syntaxIssues))
			} else {
				syntaxStatus = "语法检查通过"
			}

			finishLine := fmt.Sprintf("完成: %s (%s, %d 字节, %s)",
				filepath.Base(filePath), mode, len(finalContent), syntaxStatus)
			if hasSyntaxErrors {
				reactloops.EmitStatus(loop, "重写完成，仍有语法问题 / Rewrite Complete, Syntax Issues Remain")
			} else {
				reactloops.EmitStatus(loop, "重写完成 / Rewrite Complete")
			}

			reference := fmt.Sprintf("备份: %s\n新代码预览:\n%s", backupPath, preview)
			if hasSyntaxErrors {
				issueText := strings.Join(syntaxIssues, "\n")
				issueSummary, _ := spillOrPreview(loop, "rewrite_syntax_issues", issueText)
				reference += "\n\n语法问题:\n" + issueSummary
			}
			reactloops.EmitActionLog(loop, nodeID, finishLine, reference)

			feedbackMsg := fmt.Sprintf(
				"Rewrote %s (%s): %d bytes, backup=%s, syntax_ok=%v",
				filePath, mode, len(finalContent), backupPath, !hasSyntaxErrors,
			)
			if hasSyntaxErrors {
				feedbackMsg += "; issues: " + strings.Join(syntaxIssues[:min(len(syntaxIssues), 5)], "; ")
				if len(syntaxIssues) > 5 {
					feedbackMsg += fmt.Sprintf(" (and %d more)", len(syntaxIssues)-5)
				}
			}

			var nextAction string
			if hasSyntaxErrors {
				nextAction = "必须修复语法错误：分析语法问题，继续使用 rewrite_java_file 修复，或使用 read_java_file 重新查看文件内容"
			} else {
				nextAction = "可以使用 compare_with_backup 查看具体改动，继续处理其他文件，或使用 check_syntax 对全部文件做最终验证"
			}
			timelineMsg := fmt.Sprintf(`【重写完成】%s

【模式】：%s
【文件】：已重写 %d 个文件
【代码】：%d 字节
【语法】：%s

【下一步】：%s`,
				filePath,
				mode,
				rewrittenFiles+1,
				len(finalContent),
				syntaxStatus,
				nextAction)
			if hasSyntaxErrors {
				r.AddToTimeline("rewrite_with_errors", timelineMsg)
			} else {
				r.AddToTimeline("rewrite_success", timelineMsg)
			}
			log.Infof("[rewrite_java_file] completed %s: mode=%s bytes=%d syntax_ok=%v",
				filePath, mode, len(finalContent), !hasSyntaxErrors)

			op.Feedback(feedbackMsg)
			op.Continue()
		},
	)
}
