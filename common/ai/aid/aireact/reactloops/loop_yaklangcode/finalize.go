package loop_yaklangcode

import (
	"context"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	yaklangFinalizeReferenceMaxBytes = 12 * 1024
	yaklangFinalizeCodePreviewBytes  = 6 * 1024
	yaklangFinalizedFlagKey          = "yaklang_finalized"
)

// BuildOnPostIterationHook 在 ReActLoop 退出时投递 Yaklang 代码生成的阶段总结(工作流第 9 步)。
// 优先让 AI 基于最终代码 + 语法状态生成 1-3 句中文口语总结(写了什么脚本/核心库/是否通过语法检查/
// 如何运行), 失败则 lite 兜底, 保证 UI 不会空白; max iterations 退出时 IgnoreError。
// 关键词: post_iteration, finalize, write_yaklang_code, AI summary, lite fallback
func BuildOnPostIterationHook(invoker aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, operator *reactloops.OnPostIterationOperator) {
		if !isDone {
			return
		}
		// 已经 directly_answer 或已总结过的, 不重复投递
		if last := loop.GetLastValidAction(); last != nil && last.ActionType == schema.AI_REACT_LOOP_ACTION_DIRECTLY_ANSWER {
			ignoreYaklangMaxIterationError(operator, reason)
			return
		}
		if loop.Get(yaklangFinalizedFlagKey) == "true" {
			ignoreYaklangMaxIterationError(operator, reason)
			return
		}

		if tryDeliverYaklangFinalizeViaAI(loop, invoker, task, reason) {
			loop.Set(yaklangFinalizedFlagKey, "true")
			ignoreYaklangMaxIterationError(operator, reason)
			return
		}

		lite := generateYaklangFinalizeLiteSummary(loop, reason)
		deliverYaklangFinalizeLiteSummary(loop, invoker, lite)
		loop.Set(yaklangFinalizedFlagKey, "true")
		ignoreYaklangMaxIterationError(operator, reason)
	})
}

func ignoreYaklangMaxIterationError(operator *reactloops.OnPostIterationOperator, reason any) {
	if operator == nil {
		return
	}
	if reasonErr, ok := reason.(error); ok && reasonErr != nil && strings.Contains(reasonErr.Error(), "max iterations") {
		operator.IgnoreError()
	}
}

// tryDeliverYaklangFinalizeViaAI 让 AI 基于参考资料生成对话式总结。DirectlyAnswer 内部已负责
// 把答案流式投递到 UI, 因此这里成功后只补 timeline, 不再重复 emit。返回 true 表示已成功投递。
// 关键词: DirectlyAnswer, reference material, write_yaklang_code finalize
func tryDeliverYaklangFinalizeViaAI(loop *reactloops.ReActLoop, invoker aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, reason any) bool {
	if loop == nil || invoker == nil {
		return false
	}
	reference := buildYaklangFinalizeReference(loop, task, reason)
	if strings.TrimSpace(reference) == "" {
		return false
	}
	ctx := yaklangFinalizeContext(loop, task)
	query := buildYaklangFinalizeQuery(loop, task, reason)

	answer, err := invoker.DirectlyAnswer(
		ctx,
		query,
		nil,
		aicommon.WithDirectlyAnswerReferenceMaterial(reference, 0),
	)
	if err != nil {
		log.Warnf("write_yaklang_code finalize: DirectlyAnswer failed, falling back to lite summary: %v", err)
		return false
	}
	if strings.TrimSpace(answer) == "" {
		log.Warnf("write_yaklang_code finalize: DirectlyAnswer returned empty answer, falling back to lite summary")
		return false
	}
	invoker.AddToTimeline("yaklang_code_finalize", "Delivered AI conversational summary for write_yaklang_code")
	return true
}

// buildYaklangFinalizeReference 聚合最终代码、语法状态、目标文件、用户需求作为 AI 总结的参考资料。
// 关键词: reference_material, final_code, syntax_status, write_yaklang_code finalize
func buildYaklangFinalizeReference(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, reason any) string {
	var out strings.Builder

	if task != nil {
		if userInput := strings.TrimSpace(task.GetUserInput()); userInput != "" {
			out.WriteString("## 用户原始需求\n\n")
			out.WriteString(userInput)
			out.WriteString("\n\n")
		}
	}

	editorFilePath := strings.TrimSpace(loop.Get("editor_file_path"))
	if editorFilePath != "" {
		out.WriteString(fmt.Sprintf("## 目标文件\n\n%s\n\n", editorFilePath))
	} else {
		out.WriteString("## 目标文件\n\n新建脚本(任务结束时由系统生成 gen_code_*.yak)\n\n")
	}

	code := loop.Get("full_code")
	if strings.TrimSpace(code) != "" {
		lineCount := strings.Count(code, "\n") + 1
		errMsg, hasBlockingErrors := checkCodeAndFormatErrors(code)
		if hasBlockingErrors {
			out.WriteString(fmt.Sprintf("## 语法状态【系统编译器权威结论，以此为准】\n\n仍存在语法/编译错误(%d 行代码)\n", lineCount))
			if strings.TrimSpace(errMsg) != "" {
				out.WriteString(utils.ShrinkTextBlock(errMsg, 1024))
				out.WriteString("\n")
			}
			out.WriteString("\n")
		} else {
			out.WriteString(fmt.Sprintf("## 语法状态【系统编译器权威结论，以此为准】\n\n已通过语法检查，零阻塞错误(%d 行代码)。注意：时间线里之前出现过的报错均为中间过程，现已修复，请勿据此判定失败。\n\n", lineCount))
		}
		if runOk := loop.Get(loopVarYakRunOK); runOk != "" {
			if runOk == "true" {
				out.WriteString("## 运行状态\n\nYAK_MAIN 自测已通过。\n\n")
			} else {
				out.WriteString("## 运行状态\n\nYAK_MAIN 自测未通过或未完成。\n\n")
				if fb := strings.TrimSpace(loop.Get(loopVarYakRunLastFeedback)); fb != "" {
					out.WriteString(utils.ShrinkTextBlock(fb, 1024))
					out.WriteString("\n\n")
				}
			}
		}
		out.WriteString("## 最终代码\n\n```yak\n")
		out.WriteString(utils.ShrinkTextBlock(code, yaklangFinalizeCodePreviewBytes))
		out.WriteString("\n```\n\n")
	} else {
		out.WriteString("## 最终代码\n\n(本轮未产出代码)\n\n")
	}

	if reasonErr, ok := reason.(error); ok && reasonErr != nil {
		out.WriteString("## 退出原因\n\n")
		out.WriteString(strings.TrimSpace(reasonErr.Error()))
		out.WriteString("\n")
	}

	return strings.TrimSpace(utils.ShrinkTextBlock(out.String(), yaklangFinalizeReferenceMaxBytes))
}

// buildYaklangFinalizeQuery 构造发给 DirectlyAnswer 的提问: 要求 1-3 句中文口语总结。
// 关键: 把"系统编译器对最终代码的权威语法结论"直接写进 query, 并明确要求模型忽略
// 时间线里出现过的历史/中间报错(它们通常已被修复), 避免总结与真实结果相反(防污染)。
// 关键词: summarization_query, finalize, DirectlyAnswer, authoritative verdict, anti-contamination
func buildYaklangFinalizeQuery(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, reason any) string {
	var out strings.Builder

	// 先给出权威语法结论(基于最终代码实时编译), 强制模型采信, 而非时间线里的旧报错。
	verdict := ""
	if loop != nil {
		if code := loop.Get("full_code"); strings.TrimSpace(code) != "" {
			if _, hasBlockingErrors := checkCodeAndFormatErrors(code); hasBlockingErrors {
				verdict = "【权威结论】最终代码仍有阻塞性语法/编译错误，未通过语法检查。"
			} else {
				verdict = "【权威结论】最终代码已通过语法检查、零阻塞错误。时间线里之前的报错都是中间过程且已修复，绝对不要据此说\"仍有错误/未通过\"。"
			}
			if runOk := loop.Get(loopVarYakRunOK); runOk == "true" {
				verdict += " YAK_MAIN 自测已通过。"
			} else if runOk == "false" {
				verdict += " YAK_MAIN 自测未通过（若脚本含 YAK_MAIN 块）。"
			}
		}
	}
	if verdict != "" {
		out.WriteString(verdict)
		out.WriteString(" ")
	}

	out.WriteString("基于参考资料用 1-3 句中文口语，给出本轮 Yaklang 代码生成的精炼结论：")
	out.WriteString("写了什么脚本、用到的核心标准库、是否通过语法检查、YAK_MAIN 自测是否通过（若适用）、如何运行。")
	out.WriteString("是否通过语法检查必须与上面的【权威结论】一致。")
	out.WriteString("不要标题、列表、编号或任何 markdown，纯短句。")
	if reasonErr, ok := reason.(error); ok && reasonErr != nil && strings.Contains(reasonErr.Error(), "max iterations") {
		out.WriteString(" 本轮因达到最大迭代次数退出，若代码可能不完整请提示用户。")
	}
	if task != nil {
		if userInput := strings.TrimSpace(task.GetUserInput()); userInput != "" {
			out.WriteString(" 用户原始需求：")
			out.WriteString(userInput)
		}
	}
	return out.String()
}

// generateYaklangFinalizeLiteSummary 输出 AI 不可用时的 lite 兜底总结(一行口语短句)。
// 关键词: finalize_summary, lite_summary, fallback, concise
func generateYaklangFinalizeLiteSummary(loop *reactloops.ReActLoop, reason any) string {
	var parts []string

	code := loop.Get("full_code")
	if strings.TrimSpace(code) != "" {
		lineCount := strings.Count(code, "\n") + 1
		_, hasBlockingErrors := checkCodeAndFormatErrors(code)
		if hasBlockingErrors {
			parts = append(parts, fmt.Sprintf("已生成 Yaklang 脚本(%d 行)，但仍有语法错误待修复", lineCount))
		} else {
			runPart := ""
			if runOk := loop.Get(loopVarYakRunOK); runOk == "true" {
				runPart = "，YAK_MAIN 自测已通过"
			} else if runOk == "false" {
				runPart = "，但 YAK_MAIN 自测未通过"
			}
			parts = append(parts, fmt.Sprintf("已生成 Yaklang 脚本(%d 行)并通过语法检查%s", lineCount, runPart))
		}
	} else {
		parts = append(parts, "本轮未产出 Yaklang 代码")
	}

	if editorFilePath := strings.TrimSpace(loop.Get("editor_file_path")); editorFilePath != "" {
		parts = append(parts, fmt.Sprintf("目标文件: %s", editorFilePath))
	}

	summary := strings.TrimSpace(strings.Join(parts, "; "))
	if reasonErr, ok := reason.(error); ok && reasonErr != nil && strings.Contains(reasonErr.Error(), "max iterations") {
		summary = summary + " (因达到最大迭代次数退出)"
	}
	return summary
}

// deliverYaklangFinalizeLiteSummary 通过 emitter 流式投递 lite 兜底总结(AI 未投递时使用)。
// 关键词: finalize, lite_summary, emit markdown, EmitResultAfterStream
func deliverYaklangFinalizeLiteSummary(loop *reactloops.ReActLoop, invoker aicommon.AIInvokeRuntime, finalContent string) {
	finalContent = strings.TrimSpace(finalContent)
	if finalContent == "" {
		return
	}
	taskID := ""
	if task := loop.GetCurrentTask(); task != nil {
		taskID = task.GetId()
	}
	if emitter := loop.GetEmitter(); emitter != nil {
		if _, err := emitter.EmitTextMarkdownStreamEvent("re-act-loop-answer-payload", strings.NewReader(finalContent), taskID, func() {}); err != nil {
			log.Warnf("write_yaklang_code finalize: failed to emit markdown stream: %v", err)
		}
	}
	invoker.EmitResultAfterStream(finalContent)
	invoker.AddToTimeline("yaklang_code_finalize", "Delivered fallback summary for write_yaklang_code")
}

// yaklangFinalizeContext 选择一个可用的 context, 优先当前任务, 其次 loop 配置, 最后 Background。
func yaklangFinalizeContext(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) context.Context {
	if task != nil {
		if ctx := task.GetContext(); ctx != nil {
			return ctx
		}
	}
	if loop != nil {
		if cfg := loop.GetConfig(); cfg != nil {
			if ctx := cfg.GetContext(); ctx != nil {
				return ctx
			}
		}
	}
	return context.Background()
}
