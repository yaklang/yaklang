package loop_ssa_risk_overview

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	sfu "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func buildInitTask(r aicommon.AIInvokeRuntime) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
		userInput := task.GetUserInput()
		cfg := r.GetConfig()
		db := cfg.GetDB()
		if db == nil {
			r.AddToTimeline("ssa_risk_overview", "当前环境无数据库连接，无法列出 SSA Risk。请在 Yakit/IRify 连接项目数据库后重试。")
			loop.Set("ssa_risk_overview_preface", "无 DB：仅可根据用户文字做一般性建议，勿编造 risk_id。")
			loop.Set("ssa_risk_total_hint", "")
			op.Continue()
			return
		}

		filter := sfu.SSARisksFilterForOverview(task, loop, userInput)
		sfu.PersistEffectiveOverviewFilter(loop, filter)

		count, err := yakit.QuerySSARiskCount(db, filter)
		if err != nil {
			log.Warnf("[ssa_risk_overview] QuerySSARiskCount: %v", err)
			msg := fmt.Sprintf("统计 SSA Risk 失败: %v", err)
			r.AddToTimeline("ssa_risk_overview", msg)
			loop.Set("ssa_risk_overview_preface", "无法完成数据库统计。\n\n"+msg)
			loop.Set("ssa_risk_total_hint", "")
			op.Continue()
			return
		}

		paging := &ypb.Paging{Page: 1, Limit: 40, OrderBy: "id", Order: "desc"}
		_, risks, err := yakit.QuerySSARisk(db, filter, paging)
		if err != nil {
			log.Warnf("[ssa_risk_overview] QuerySSARisk: %v", err)
			msg := fmt.Sprintf("查询 SSA Risk 失败: %v", err)
			r.AddToTimeline("ssa_risk_overview", msg)
			loop.Set("ssa_risk_overview_preface", "无法拉取风险列表。\n\n"+msg)
			loop.Set("ssa_risk_total_hint", fmt.Sprintf("%d", count))
			op.Continue()
			return
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("匹配条件 approximate count: %d；本页抽样 %d 条。\n\n", count, len(risks)))
		for i, rk := range risks {
			sb.WriteString(fmt.Sprintf("%d. id=%d | sev=%s | program=%s | rule=%s | title=%s\n",
				i+1, rk.ID, rk.Severity, utils.ShrinkTextBlock(rk.ProgramName, 80),
				utils.ShrinkTextBlock(rk.FromRule, 60), utils.ShrinkTextBlock(rk.Title, 120)))
		}
		summary := sb.String()
		loop.Set("ssa_risk_list_summary", summary)
		loop.Set("ssa_risk_total_hint", fmt.Sprintf("%d", count))
		r.AddToTimeline("ssa_risk_overview", utils.ShrinkTextBlock(summary, 4000))

		preface := "下列摘要来自数据库查询，仅可在此基础上归纳、聚类、搜索建议；不得编造未列出的 risk_id。\n\n" + summary
		loop.Set("ssa_risk_overview_preface", preface)
		op.Continue()
	}
}
