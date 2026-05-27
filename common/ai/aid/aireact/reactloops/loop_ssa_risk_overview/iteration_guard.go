package loop_ssa_risk_overview

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/utils"
)

func isReActOverviewQueryAction(name string) bool {
	switch strings.TrimSpace(name) {
	case "query_ssa_risk_overview":
		return true
	default:
		return false
	}
}

func buildMidIterationGuardHook(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, op *reactloops.OnPostIterationOperator) {
		if isDone {
			return
		}
		if hasFinalAnswerDelivered(loop) {
			op.EndIteration("final answer already delivered")
			return
		}

		records := getRecentActions(loop)
		if len(records) >= 2 {
			prev := records[len(records)-2]
			last := records[len(records)-1]
			// Init load + first query is normal (e.g. user asks "only grav project?").
			if isReActOverviewQueryAction(prev.ActionName) && isReActOverviewQueryAction(last.ActionName) {
				msg := "检测到连续两次 query_ssa_risk_overview（无 init 间隔）。preface 已足够；下一轮请 output_overview_findings + 一次 directly_answer，勿再 query。"
				r.AddToTimeline("ssa_risk_overview_spin", msg)
				if !hasFinalAnswerDelivered(loop) && strings.TrimSpace(loop.Get("ssa_risk_overview_preface")) != "" {
					ctx := collectFinalizeContext(loop, task, msg)
					if ctx != "" {
						deliverOverviewFinalize(loop, r, ctx, iteration)
					}
				}
				op.EndIteration(msg)
			}
		}
	})
}

func duplicateQueryFeedback(loop *reactloops.ReActLoop, filterKey string) string {
	last := strings.TrimSpace(loop.Get("ssa_overview_last_query_filter_key"))
	if last != "" && last == filterKey {
		return "重复 query：过滤条件未变，preface 已是最新。请 output_overview_findings 记录要点，然后 directly_answer **一次**；不要再次输出完整 severity 表格。"
	}
	loop.Set("ssa_overview_last_query_filter_key", filterKey)
	return ""
}

func overviewFilterCacheKey(filterJSON string) string {
	return strings.TrimSpace(filterJSON)
}

func collectIterationFindingsFromAction(loop *reactloops.ReActLoop) {
	la := loop.GetLastAction()
	if la == nil || la.ActionType != "output_overview_findings" {
		return
	}
	if la.ActionParams == nil {
		return
	}
	incoming := normalizeFindings(fmt.Sprint(la.ActionParams[overviewFindingsFieldName]))
	if incoming == "" {
		return
	}
	if _, changed := appendOverviewFindings(loop, incoming); changed {
		recordMetaAction(loop, "findings_hook", "merged post-iteration findings", utils.ShrinkTextBlock(incoming, 120))
	}
}
