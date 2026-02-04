package loop_knowledge_enhance

import (
	"fmt"
	"strconv"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

// makeFinalSummaryAction 创建一个最终总结 action
// 用于让 AI 返回收集到的信息的总结并结束循环
func makeFinalSummaryAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	desc := "输出信息收集的最终总结。当你认为已经收集到足够的信息时，使用此 action 提交最终总结并结束信息收集流程。"

	toolOpts := []aitool.ToolOption{
		aitool.WithStringParam("summary",
			aitool.WithParam_Description("对已收集信息的总结，包括收集到的关键信息点"),
			aitool.WithParam_Required(true)),
	}

	return reactloops.WithRegisterLoopAction(
		"final_summary",
		desc,
		toolOpts,
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			// 验证参数
			loop.LoadingStatus("验证最终总结参数 - validating final summary parameters")
			summary := action.GetString("summary")
			if summary == "" {
				return utils.Error("summary is required")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			loop.LoadingStatus("生成最终总结中 - generating final summary")

			summary := action.GetString("summary")
			userQuery := loop.Get("user_query")
			searchHistory := loop.Get("search_history")
			searchResultsSummary := loop.Get("search_results_summary")
			searchCountStr := loop.Get("search_count")

			searchCount := 0
			if searchCountStr != "" {
				if c, err := strconv.Atoi(searchCountStr); err == nil {
					searchCount = c
				}
			}

			iteration := loop.GetCurrentIterationIndex()
			if iteration <= 0 {
				iteration = 1
			}

			// 构建最终报告
			finalReport := fmt.Sprintf(`# 知识收集最终报告

## 用户问题
%s

## 收集统计
- 搜索轮次: %d
- 总搜索次数: %d
- 完成时间: %s

## 总结
%s

## 搜索历史
%s

## 收集到的所有信息
%s

---
报告生成时间: %s
`,
				userQuery,
				iteration,
				searchCount,
				time.Now().Format("2006-01-02 15:04:05"),
				summary,
				searchHistory,
				searchResultsSummary,
				time.Now().Format("2006-01-02 15:04:05"),
			)

			// 保存到 loop 上下文
			loop.Set("final_summary", finalReport)

			emitter := loop.GetEmitter()
			_ = emitter
			loop.GetInvoker().EmitFileArtifactWithExt(
				fmt.Sprintf("knowledge_final_report_%s_%s", utils.DatetimePretty2(), utils.RandStringBytes(4)),
				".md",
				finalReport,
			)

			result, err := loop.GetInvoker().DirectlyAnswer(
				loop.GetCurrentTask().GetContext(),
				finalReport,
				nil,
			)
			_ = result
			if err != nil {
				op.Continue()
			} else {
				op.Exit()
			}
		},
	)
}

// finalSummaryAction 导出的 action 构造函数
var finalSummaryAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return makeFinalSummaryAction(r)
}
