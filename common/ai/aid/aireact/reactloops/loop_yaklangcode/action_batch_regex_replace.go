package loop_yaklangcode

import (
	"bytes"
	"fmt"
	"os"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

var batchRegexReplace = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"batch_regex_replace",
		`批量正则替换代码 - 对整个文件进行正则表达式替换操作

【功能说明】：
使用正则表达式对代码文件进行批量替换，只能修改单行内容，不支持跨行匹配。
这是一种高效的批量修改方式，适用于重命名变量、函数名、修改配置等场景。

【使用场景】：
1. 批量重命名变量或函数名
2. 修改配置参数或常量值
3. 统一代码风格或格式
4. 批量修改特定模式的代码内容

【参数说明】：
- regexp_pattern (必需) - 正则表达式模式，用于匹配要替换的内容
- group (可选) - 指定要替换的捕获组编号（从1开始），如果不指定则替换整个匹配
- replaced_string (必需) - 替换后的字符串，支持 $1, $2 等捕获组引用

【使用示例】：
1. 重命名函数：pattern="func\\s+(\\w+)\\(", group=1, replaced_string="new_$1"
2. 修改变量值：pattern="var\\s+port\\s*=\\s*(\\d+)", replaced_string="var port = 8080"
3. 添加前缀：pattern="(\\w+)\\(\\)", group=1, replaced_string="prefix_$1"

【重要提醒】：
- 只能匹配单行内容，不支持跨行正则
- 替换会应用到所有匹配的行
- 如需删除整行，请使用 delete_lines action 而不是设置 replaced_string 为空
- 使用前请确保正则表达式正确，避免误替换`,
		[]aitool.ToolOption{
			aitool.WithStringParam(
				"regexp_pattern",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description(`正则表达式模式（必需）- 用于匹配要替换的内容
支持标准的 Go 正则表达式语法，包括：
- 字符类：\\d, \\w, \\s 等
- 量词：+, *, ?, {n,m} 等  
- 分组：(pattern) 用于捕获
- 边界：^, $, \\b 等

注意：只能匹配单行内容，不支持跨行模式`),
			),
			aitool.WithIntegerParam(
				"group",
				aitool.WithParam_Description(`指定要替换的捕获组编号（可选）
- 不指定或为0：替换整个匹配内容
- 1, 2, 3...：替换对应的捕获组内容
- 使用捕获组可以实现更精确的替换`),
			),
			aitool.WithStringParam(
				"replaced_string",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description(`替换后的字符串（必需）
支持捕获组引用：
- $1, $2, $3... 引用对应的捕获组
- $0 引用整个匹配内容
- 普通字符串直接替换

示例：
- "new_$1" - 在第一个捕获组前添加 "new_"
- "$1_suffix" - 在第一个捕获组后添加 "_suffix"
- "fixed_value" - 直接替换为固定值`),
			),
			aitool.WithStringParam(
				"replace_reason",
				aitool.WithParam_Description(`替换原因说明（可选）
解释为什么进行这次批量替换，包括：
- 替换的目的和预期效果
- 涉及的代码模式和范围
- 替换策略和注意事项`),
			),
		},
		[]*reactloops.LoopStreamField{
			{
				FieldName: "replace_reason",
				AINodeId:  "re-act-loop-thought",
			},
		},
		// Validator
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			pattern := action.GetString("regexp_pattern")
			if pattern == "" {
				return utils.Error("batch_regex_replace requires 'regexp_pattern' parameter")
			}

			replacedString := action.GetString("replaced_string")
			if replacedString == "" {
				return utils.Error("batch_regex_replace requires 'replaced_string' parameter")
			}

			group := action.GetInt("group")
			if group < 0 {
				return utils.Error("group parameter must be >= 0")
			}

			// 验证正则表达式语法
			opts := &BatchRegexReplaceOptions{
				Pattern:     pattern,
				Replacement: replacedString,
				Group:       group,
			}
			err := ValidateBatchRegexReplaceOptions(opts)
			if err != nil {
				return err
			}

			l.GetEmitter().EmitTextPlainTextStreamEvent(
				"thought",
				bytes.NewReader([]byte(fmt.Sprintf("Preparing batch regex replace: pattern=%s, group=%d", pattern, group))),
				l.GetCurrentTask().GetIndex())
			return nil
		},
		// Handler
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			// 获取必要的上下文信息
			filename := loop.Get("filename")
			if filename == "" {
				op.Fail("no filename found in loop context for batch_regex_replace action")
				return
			}

			fullCode := loop.Get("full_code")
			if fullCode == "" {
				op.Fail("no code found in loop context for batch_regex_replace action")
				return
			}

			invoker := loop.GetInvoker()

			// 提取参数
			regexpPattern := action.GetString("regexp_pattern")
			group := action.GetInt("group")
			replacedString := action.GetString("replaced_string")
			reason := action.GetString("replace_reason")

			// 记录操作开始
			msg := fmt.Sprintf("decided to batch regex replace with pattern: %s, group: %d, replacement: %s",
				regexpPattern, group, replacedString)
			invoker.AddToTimeline("batch_regex_replace", msg)

			if reason != "" {
				r.AddToTimeline("replace_reason", reason)
			}

			// 构建替换选项
			opts := &BatchRegexReplaceOptions{
				Pattern:     regexpPattern,
				Replacement: replacedString,
				Group:       group,
				VerboseLog:  true,
			}

			// 执行核心替换逻辑
			result, err := BatchRegexReplace(fullCode, opts)
			if err != nil {
				errorMsg := fmt.Sprintf("Batch regex replace failed: %v", err)
				r.AddToTimeline("batch_replace_failed", errorMsg)
				op.Fail(errorMsg)
				return
			}

			// 处理无匹配的情况
			if !result.HasModifications {
				warningMsg := fmt.Sprintf("No matches found for pattern '%s' in the code", regexpPattern)
				log.Warnf("batch_regex_replace: %s", warningMsg)
				invoker.AddToTimeline("no_matches_warning", warningMsg)
				op.Continue()
				return
			}

			// 更新代码状态
			loop.Set("full_code", result.ModifiedCode)

			// 写入文件
			os.RemoveAll(filename)
			err = os.WriteFile(filename, []byte(result.ModifiedCode), 0644)
			if err != nil {
				errorMsg := fmt.Sprintf("Failed to write modified code to file: %v", err)
				r.AddToTimeline("write_file_failed", errorMsg)
				op.Fail(errorMsg)
				return
			}

			// 语法检查和错误处理
			errMsg, hasBlockingErrors := checkCodeAndFormatErrors(result.ModifiedCode)
			if hasBlockingErrors {
				op.DisallowNextLoopExit()
			}

			// 构建结果报告
			resultMsg := buildBatchReplaceResultMessage(result, regexpPattern, replacedString, errMsg)

			if errMsg != "" {
				op.Feedback(errMsg)
			}

			// 记录完成状态
			r.AddToTimeline("batch_replace_completed", resultMsg)
			log.Infof("batch_regex_replace done: %d replacements, hasBlockingErrors=%v",
				result.ReplacementCount, hasBlockingErrors)

			// 发送编辑器事件
			loop.GetEmitter().EmitJSON(schema.EVENT_TYPE_YAKLANG_CODE_EDITOR, "batch_regex_replace", result.ModifiedCode)

			// 提供后续建议
			if errMsg != "" {
				invoker.AddToTimeline("advice", "use 'grep_yaklang_samples' to find more syntax examples ")
			}

			op.Continue()
		},
	)
}

// buildBatchReplaceResultMessage 构建批量替换结果消息
func buildBatchReplaceResultMessage(result *BatchRegexReplaceResult, pattern, replacement, errMsg string) string {
	resultMsg := fmt.Sprintf("Batch regex replace completed: %d lines modified\nPattern: %s\nReplacement: %s",
		result.ReplacementCount, pattern, replacement)

	// 添加修改详情
	if len(result.ModifiedLines) > 0 {
		resultMsg += "\n\nModified lines:"
		for i, modLine := range result.ModifiedLines {
			if i >= 5 { // 最多显示5行详情
				resultMsg += fmt.Sprintf("\n  ... and %d more lines", len(result.ModifiedLines)-5)
				break
			}
			resultMsg += fmt.Sprintf("\n  Line %d: %s -> %s",
				modLine.LineNumber,
				utils.ShrinkTextBlock(modLine.OriginalLine, 30),
				utils.ShrinkTextBlock(modLine.ModifiedLine, 30))
		}
	}

	// 添加语法检查结果
	if errMsg != "" {
		resultMsg += "\n\n--[linter]--\nCode Linter Check:\n" + utils.PrefixLines(utils.ShrinkTextBlock(errMsg, 2048), "  ")
	} else {
		resultMsg += "\n\n--[linter]--\nNo issues found in the modified code."
	}

	return resultMsg
}
