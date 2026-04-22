package syntaxflow_utils

import (
	"fmt"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/encoding/protojson"
)

const defaultOverviewPageLimit int64 = 40

// PersistEffectiveOverviewFilter stores the filter used for overview queries on the loop (protojson).
// It mirrors the same JSON into LoopVarSSARisksFilterJSON so SSARisksFilterForOverview stays aligned.
func PersistEffectiveOverviewFilter(loop *reactloops.ReActLoop, filter *ypb.SSARisksFilter) {
	if loop == nil || filter == nil {
		return
	}
	b, err := protojson.Marshal(filter)
	if err != nil {
		return
	}
	s := string(b)
	loop.Set(LoopVarSSAOverviewFilterJSON, s)
	loop.Set(LoopVarSSARisksFilterJSON, s)
}

func appendUniqueStr(slice []string, v string) []string {
	for _, x := range slice {
		if x == v {
			return slice
		}
	}
	return append(slice, v)
}

// MergeReloadSSARiskOverviewFilter builds the filter for reload_ssa_risk_overview from action params,
// stored LoopVarSSAOverviewFilterJSON, or SSARisksFilterForOverview.
func MergeReloadSSARiskOverviewFilter(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, action *aicommon.Action) *ypb.SSARisksFilter {
	rawFj := strings.TrimSpace(action.GetString("filter_json"))
	var base *ypb.SSARisksFilter
	if rawFj != "" {
		base = &ypb.SSARisksFilter{}
		if err := protojson.Unmarshal([]byte(rawFj), base); err != nil {
			base = SSARisksFilterForOverview(task, loop, "")
		}
	} else if loop != nil {
		if s := strings.TrimSpace(loop.Get(LoopVarSSAOverviewFilterJSON)); s != "" {
			base = &ypb.SSARisksFilter{}
			if err := protojson.Unmarshal([]byte(s), base); err != nil {
				base = nil
			}
		}
	}
	if base == nil {
		base = SSARisksFilterForOverview(task, loop, "")
	}
	if s := strings.TrimSpace(action.GetString("search")); s != "" {
		base.Search = s
	}
	for _, part := range strings.Split(action.GetString("runtime_id"), ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			base.RuntimeID = appendUniqueStr(base.RuntimeID, part)
		}
	}
	if p := strings.TrimSpace(action.GetString("program_name")); p != "" {
		base.ProgramName = appendUniqueStr(base.ProgramName, p)
	}
	return base
}

// ApplySSARiskOverviewDB loads counts and a page of risks into loop vars (same fields as loop_ssa_risk_overview init_task).
func ApplySSARiskOverviewDB(loop *reactloops.ReActLoop, invoker aicommon.AIInvokeRuntime, db *gorm.DB, task aicommon.AIStatefulTask, filter *ypb.SSARisksFilter, listLimit int64) string {
	if loop == nil || invoker == nil {
		return ""
	}
	if db == nil {
		invoker.AddToTimeline("ssa_risk_overview", "当前环境无数据库连接，无法列出 SSA Risk。请在 Yakit/IRify 连接项目数据库后重试。")
		loop.Set("ssa_risk_overview_preface", "无 DB：仅可根据用户文字做一般性建议，勿编造 risk_id。")
		loop.Set("ssa_risk_total_hint", "")
		return loop.Get("ssa_risk_overview_preface")
	}
	if listLimit <= 0 {
		listLimit = defaultOverviewPageLimit
	}
	PersistEffectiveOverviewFilter(loop, filter)

	count, err := yakit.QuerySSARiskCount(db, filter)
	if err != nil {
		log.Warnf("[ssa_risk_overview] QuerySSARiskCount: %v", err)
		msg := fmt.Sprintf("统计 SSA Risk 失败: %v", err)
		invoker.AddToTimeline("ssa_risk_overview", msg)
		loop.Set("ssa_risk_overview_preface", "无法完成数据库统计。\n\n"+msg)
		loop.Set("ssa_risk_total_hint", "")
		return loop.Get("ssa_risk_overview_preface")
	}

	paging := &ypb.Paging{Page: 1, Limit: listLimit, OrderBy: "id", Order: "desc"}
	_, risks, err := yakit.QuerySSARisk(db, filter, paging)
	if err != nil {
		log.Warnf("[ssa_risk_overview] QuerySSARisk: %v", err)
		msg := fmt.Sprintf("查询 SSA Risk 失败: %v", err)
		invoker.AddToTimeline("ssa_risk_overview", msg)
		loop.Set("ssa_risk_overview_preface", "无法拉取风险列表。\n\n"+msg)
		loop.Set("ssa_risk_total_hint", fmt.Sprintf("%d", count))
		return loop.Get("ssa_risk_overview_preface")
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
	invoker.AddToTimeline("ssa_risk_overview", utils.ShrinkTextBlock(summary, 4000))

	preface := "下列摘要来自数据库查询，仅可在此基础上归纳、聚类、搜索建议；不得编造未列出的 risk_id。\n\n" + summary
	loop.Set("ssa_risk_overview_preface", preface)
	return preface
}
