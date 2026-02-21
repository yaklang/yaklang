package loop_internet_research

import (
	"fmt"
	"strconv"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func makeFinalSummaryAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	desc := "提交互联网调研的最终报告。当你认为已经收集到足够的信息时，使用此 action 提交最终调研报告并结束流程。"

	toolOpts := []aitool.ToolOption{
		aitool.WithStringParam("summary",
			aitool.WithParam_Description("调研报告内容，包括收集到的关键信息和来源"),
			aitool.WithParam_Required(true)),
	}

	return reactloops.WithRegisterLoopAction(
		"final_summary",
		desc,
		toolOpts,
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			loop.LoadingStatus("validating final summary parameters")
			summary := action.GetString("summary")
			if summary == "" {
				return utils.Error("summary is required")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			loop.LoadingStatus("generating final research report")

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

			finalReport := fmt.Sprintf(`# Internet Research Final Report

## User Query
%s

## Research Statistics
- Search Rounds: %d
- Total Searches: %d
- Completion Time: %s

## Summary
%s

## Search History
%s

## Collected Information
%s

---
Report Generated: %s
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

			loop.Set("final_summary", finalReport)

			loop.GetInvoker().EmitFileArtifactWithExt(
				fmt.Sprintf("internet_research_final_report_%s_%s", utils.DatetimePretty2(), utils.RandStringBytes(4)),
				".md",
				finalReport,
			)

			result, err := loop.GetInvoker().DirectlyAnswer(
				loop.GetCurrentTask().GetContext(),
				finalReport,
				nil,
			)
			_ = result
			log.Infof("final summary direct answer result: %s", utils.ShrinkTextBlock(result, 2048))
			if err != nil {
				op.Continue()
			} else {
				op.Exit()
			}
		},
	)
}

var finalSummaryAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return makeFinalSummaryAction(r)
}
