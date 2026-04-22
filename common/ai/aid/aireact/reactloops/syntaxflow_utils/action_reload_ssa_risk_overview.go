package syntaxflow_utils

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// WithReloadSSARiskOverviewAction registers reload_ssa_risk_overview: re-query SSA risks with structured filter/limit and refresh reactive preface fields.
func WithReloadSSARiskOverviewAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"reload_ssa_risk_overview",
		"Re-run the SSA risk list query using the project database and update ssa_risk_overview_preface / ssa_risk_list_summary / ssa_risk_total_hint. Without parameters, reuses the last effective filter stored on the loop (ssa_overview_filter_json) or falls back to attachments and loop vars like the init task. Use filter_json for a full ypb.SSARisksFilter (protojson); runtime_id accepts comma-separated SSA runtime ids.",
		[]aitool.ToolOption{
			aitool.WithIntegerParam("limit", aitool.WithParam_Description("Max number of risk rows to sample for the preface (default 40).")),
			aitool.WithStringParam("search", aitool.WithParam_Description("Fuzzy search string; sets SSARisksFilter.Search when non-empty.")),
			aitool.WithStringParam("runtime_id", aitool.WithParam_Description("SSA runtime id(s), comma-separated; each non-empty token is merged into SSARisksFilter.RuntimeID.")),
			aitool.WithStringParam("program_name", aitool.WithParam_Description("Program name; merged into SSARisksFilter.ProgramName when non-empty.")),
			aitool.WithStringParam("filter_json", aitool.WithParam_Description("Full SSARisksFilter as JSON (google.protobuf JSON / protojson). When set, used as the base filter before applying search/runtime_id/program_name overrides.")),
		},
		func(_ *reactloops.ReActLoop, _ *aicommon.Action) error {
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			task := operator.GetTask()
			db := r.GetConfig().GetDB()
			filter := MergeReloadSSARiskOverviewFilter(loop, task, action)
			limit := int64(action.GetInt("limit"))
			summary := ApplySSARiskOverviewDB(loop, r, db, task, filter, limit)
			operator.Feedback(fmt.Sprintf("[reload_ssa_risk_overview] updated overview context (%d runes).\n%s", len([]rune(summary)), summary))
			operator.Continue()
		},
	)
}
