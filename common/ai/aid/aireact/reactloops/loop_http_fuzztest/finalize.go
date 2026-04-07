package loop_http_fuzztest

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func BuildOnPostIterationHook(invoker aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, operator *reactloops.OnPostIterationOperator) {
		if !isDone {
			return
		}
		persistLoopHTTPFuzzSessionContext(loop, "post_iteration")
		if hasLoopHTTPFuzzFinalAnswerDelivered(loop) || hasLoopHTTPFuzzDirectlyAnswered(loop) || getLoopHTTPFuzzLastAction(loop) == "directly_answer" {
			return
		}
		finalContent := generateLoopHTTPFuzzFinalizeSummary(loop, reason)
		deliverLoopHTTPFuzzFinalizeSummary(loop, invoker, finalContent)
		if reasonErr, ok := reason.(error); ok && strings.Contains(reasonErr.Error(), "max iterations") {
			operator.IgnoreError()
		}
	})
}

func generateLoopHTTPFuzzFinalizeSummary(loop *reactloops.ReActLoop, reason any) string {
	var out strings.Builder
	out.WriteString("# HTTP Fuzz Test 阶段总结\n\n")

	if task := loop.GetCurrentTask(); task != nil {
		userInput := strings.TrimSpace(task.GetUserInput())
		if userInput != "" {
			out.WriteString("## 用户目标\n\n")
			out.WriteString(userInput)
			out.WriteString("\n\n")
		}
	}

	if currentSummary := strings.TrimSpace(getCurrentRequestSummary(loop)); currentSummary != "" {
		out.WriteString("## 当前有效请求\n\n")
		out.WriteString(currentSummary)
		out.WriteString("\n\n")
	}

	if actionsSummary := strings.TrimSpace(buildLoopHTTPFuzzRecentActionsPrompt(loop)); actionsSummary != "" {
		out.WriteString("## 已执行动作\n\n")
		out.WriteString(actionsSummary)
		out.WriteString("\n\n")
	}

	if payloadSummary := strings.TrimSpace(buildLoopHTTPFuzzTestedPayloadPrompt(loop)); payloadSummary != "" {
		out.WriteString("## 已测试 Payload\n\n")
		out.WriteString(payloadSummary)
		out.WriteString("\n\n")
	}

	if diffResult := strings.TrimSpace(loop.Get("diff_result_compressed")); diffResult == "" {
		if diffResult = strings.TrimSpace(loop.Get("diff_result")); diffResult != "" {
			out.WriteString("## 当前发现\n\n")
			out.WriteString(utils.ShrinkTextBlock(diffResult, 2000))
			out.WriteString("\n\n")
		}
	} else {
		out.WriteString("## 当前发现\n\n")
		out.WriteString(utils.ShrinkTextBlock(diffResult, 2000))
		out.WriteString("\n\n")
	}

	if verification := strings.TrimSpace(loop.Get("verification_result")); verification != "" {
		out.WriteString("## 验证结论\n\n")
		out.WriteString(verification)
		out.WriteString("\n\n")
	}

	if hiddenIndex := strings.TrimSpace(loop.Get("representative_httpflow_hidden_index")); hiddenIndex != "" {
		out.WriteString("## 代表性样本\n\n")
		out.WriteString(fmt.Sprintf("HTTPFlow: %s\n\n", hiddenIndex))
	}

	out.WriteString("## 退出原因\n\n")
	if reasonErr, ok := reason.(error); ok && reasonErr != nil {
		out.WriteString(reasonErr.Error())
	} else if strings.TrimSpace(utils.InterfaceToString(reason)) != "" {
		out.WriteString(strings.TrimSpace(utils.InterfaceToString(reason)))
	} else {
		out.WriteString("当前阶段已结束，系统按已有状态生成了这份总结。")
	}
	return strings.TrimSpace(out.String())
}

func deliverLoopHTTPFuzzFinalizeSummary(loop *reactloops.ReActLoop, invoker aicommon.AIInvokeRuntime, finalContent string) {
	finalContent = strings.TrimSpace(finalContent)
	if finalContent == "" || hasLoopHTTPFuzzFinalAnswerDelivered(loop) || hasLoopHTTPFuzzDirectlyAnswered(loop) || getLoopHTTPFuzzLastAction(loop) == "directly_answer" {
		return
	}
	taskID := ""
	if task := loop.GetCurrentTask(); task != nil {
		taskID = task.GetId()
	}
	if emitter := loop.GetEmitter(); emitter != nil {
		if _, err := emitter.EmitTextMarkdownStreamEvent("re-act-loop-answer-payload", strings.NewReader(finalContent), taskID, func() {}); err != nil {
			log.Warnf("http_fuzztest finalize: failed to emit markdown stream: %v", err)
		}
	}
	invoker.EmitResultAfterStream(finalContent)
	markLoopHTTPFuzzFinalAnswerDelivered(loop)
	recordLoopHTTPFuzzMetaAction(loop, "finalize_summary", "专注模式退出时自动补充阶段总结", utils.ShrinkTextBlock(finalContent, 240))
	persistLoopHTTPFuzzSessionContext(loop, "finalize_summary")
	invoker.AddToTimeline("http_fuzztest_finalize", "Delivered fallback summary for loop_http_fuzztest")
}
