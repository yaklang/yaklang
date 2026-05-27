package loop_ssa_risk_overview

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	sfu "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

func buildQuerySSARiskOverviewParamSummary(action *aicommon.Action, filterDesc string) string {
	if action == nil {
		return filterDesc
	}
	var parts []string
	if v := strings.TrimSpace(action.GetString("search")); v != "" {
		parts = append(parts, fmt.Sprintf("search=%q", v))
	}
	if v := strings.TrimSpace(action.GetString("runtime_id")); v != "" {
		parts = append(parts, "runtime_id="+v)
	}
	if v := strings.TrimSpace(action.GetString("program_name")); v != "" {
		parts = append(parts, "program_name="+v)
	}
	if v := strings.TrimSpace(action.GetString("severity")); v != "" {
		parts = append(parts, "severity="+v)
	}
	if v := strings.TrimSpace(action.GetString("risk_type")); v != "" {
		parts = append(parts, "risk_type="+v)
	}
	if lim := action.GetInt("limit"); lim > 0 {
		parts = append(parts, fmt.Sprintf("limit=%d", lim))
	}
	if len(parts) == 0 {
		return "effective: " + filterDesc
	}
	return strings.Join(parts, ", ") + " → effective: " + filterDesc
}

// WithQuerySSARiskOverviewAction registers query_ssa_risk_overview: query SSA risks with scalar filters (no raw filter JSON).
func WithQuerySSARiskOverviewAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"query_ssa_risk_overview",
		"Query the SSA risk list from the SSA project database and refresh ssa_risk_overview_preface / ssa_risk_list_summary / ssa_risk_total_hint. "+
			"Use scalar params only (search, runtime_id, program_name, severity, risk_type, limit). "+
			"Without params, reuses the last filter on the loop (ssa_overview_filter_json) or Irify attachment fields synced at init — same pattern as filter_and_match_http_flows.",
		[]aitool.ToolOption{
			aitool.WithIntegerParam("limit", aitool.WithParam_Description("Max risk rows in the preface sample (default 40).")),
			aitool.WithStringParam("search", aitool.WithParam_Description("Fuzzy search across risk title, program, rule, tags, etc.")),
			aitool.WithStringParam("runtime_id", aitool.WithParam_Description("SSA scan runtime id(s), comma-separated.")),
			aitool.WithStringParam("program_name", aitool.WithParam_Description("Program name(s), comma-separated.")),
			aitool.WithStringParam("severity", aitool.WithParam_Description("Severity filter, comma-separated (e.g. high,critical).")),
			aitool.WithStringParam("risk_type", aitool.WithParam_Description("Risk type filter, comma-separated.")),
		},
		func(_ *reactloops.ReActLoop, _ *aicommon.Action) error {
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			task := operator.GetTask()
			db := sfu.GetSSADB()
			filter := sfu.MergeQuerySSARiskOverviewFilter(loop, task, action)
			filterDesc := sfu.FormatSSARisksFilterHuman(filter)
			loop.Set("ssa_overview_last_filter_summary", filterDesc)
			if dup := duplicateQueryFeedback(loop, overviewFilterCacheKey(string(utils.Jsonify(filter)))); dup != "" {
				operator.Feedback(dup)
				operator.Continue()
				return
			}
			limit := int64(action.GetInt("limit"))
			paramSummary := buildQuerySSARiskOverviewParamSummary(action, filterDesc)
			if emitter := loop.GetEmitter(); emitter != nil && task != nil {
				emitter.EmitThoughtStream(task.GetId(), "[query_ssa_risk_overview] %s", paramSummary)
			}
			summary := sfu.ApplySSARiskOverviewDB(loop, r, db, task, filter, limit)
			recordAction(loop, "query_ssa_risk_overview", paramSummary,
				fmt.Sprintf("count=%s", strings.TrimSpace(loop.Get("ssa_risk_total_hint"))))
			r.AddToTimeline("query_ssa_risk_overview",
				fmt.Sprintf("%s\n\n%s", paramSummary, utils.ShrinkTextBlock(summary, 3000)))
			operator.Feedback(fmt.Sprintf("[query_ssa_risk_overview] updated overview context (%d runes).\n%s", len([]rune(summary)), summary))
			operator.Continue()
		},
	)
}
