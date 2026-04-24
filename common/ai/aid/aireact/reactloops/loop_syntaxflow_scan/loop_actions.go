package loop_syntaxflow_scan

import (
	"fmt"

	sfu "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// WithReloadSSARiskOverviewAction registers reload_ssa_risk_overview: re-query SSA risks with structured filter/limit and refresh reactive preface fields.
func WithReloadSSARiskOverviewAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"reload_ssa_risk_overview",
		"Re-run the SSA risk list query using the SSA project database and update ssa_risk_overview_preface / ssa_risk_list_summary / ssa_risk_total_hint. Without parameters, reuses the last effective filter stored on the loop (ssa_overview_filter_json) or falls back to attachments and loop vars like the init task. Use filter_json for a full ypb.SSARisksFilter (protojson); runtime_id accepts comma-separated SSA runtime ids.",
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
			db := sfu.GetSSADB()
			filter := MergeReloadSSARiskOverviewFilter(loop, task, action)
			limit := int64(action.GetInt("limit"))
			summary := ApplySSARiskOverviewDB(loop, r, db, task, filter, limit)
			operator.Feedback(fmt.Sprintf("[reload_ssa_risk_overview] updated overview context (%d runes).\n%s", len([]rune(summary)), summary))
			operator.Continue()
		},
	)
}

// WithReloadSyntaxFlowScanSessionAction registers reload_syntaxflow_scan_session: reload scan task + SSA risk sample for a task_id from DB.
func WithReloadSyntaxFlowScanSessionAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"reload_syntaxflow_scan_session",
		"Load SyntaxFlowScanTask and a sample of SSA risks for the given task_id (SSA runtime id) from the database, then refresh sf_scan_review_preface, sf_scan_task_id, and sf_scan_session_mode=attach. Equivalent to a successful attach path in the syntaxflow_scan init task.",
		[]aitool.ToolOption{
			aitool.WithStringParam("task_id", aitool.WithParam_Description("SyntaxFlow scan task id (UUID), same as SSA Risk runtime_id for that scan."), aitool.WithParam_Required(true)),
		},
		func(_ *reactloops.ReActLoop, action *aicommon.Action) error {
			if action.GetString("task_id") == "" {
				return utils.Error("task_id is required")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			taskID := action.GetString("task_id")
			db := sfu.GetSSADB()
			if db == nil {
				r.AddToTimeline("syntaxflow_scan", "reload_syntaxflow_scan_session: 无 SSA 数据库连接")
				operator.Feedback("reload_syntaxflow_scan_session failed: SSA database not available")
				operator.Continue()
				return
			}
			res, err := LoadScanSessionResult(db, taskID, DefaultRiskSampleLimit)
			if err != nil {
				log.Warnf("[syntaxflow_scan] reload LoadScanSessionResult: %v", err)
				r.AddToTimeline("syntaxflow_scan", fmt.Sprintf("reload failed task_id=%s: %v", taskID, err))
				operator.Feedback(fmt.Sprintf("reload_syntaxflow_scan_session failed: %v", err))
				operator.Continue()
				return
			}
			loop.Set("sf_scan_task_id", taskID)
			loop.Set("sf_scan_session_mode", "attach")
			preface := "下列信息来自数据库（扫描任务 + 该 runtime 下 SSA Risk 列表），仅可在此基础上解读；不得编造未列出的 risk id。\n\n" + res.Preface
			loop.Set("sf_scan_review_preface", preface)
			AppendSfScanInterpretLog(loop, r, taskID, "reload_syntaxflow_scan_session: 已刷新任务与 risk 样本")
			r.AddToTimeline("syntaxflow_scan", utils.ShrinkTextBlock(preface, 4000))
			operator.Feedback(preface)
			operator.Continue()
		},
	)
}

// WithSetSSARiskReviewTargetAction registers set_ssa_risk_review_target: switch the focused SSA risk id mid-session without new attachments.
func WithSetSSARiskReviewTargetAction(_ aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"set_ssa_risk_review_target",
		"Set the active SSA risk primary key for this ssa_risk_review loop (loop var ssa_risk_id). After changing, use the ssa-risk tool with the new risk_id before drawing conclusions.",
		[]aitool.ToolOption{
			aitool.WithIntegerParam("risk_id", aitool.WithParam_Description("SSA Risk database id (positive integer)."), aitool.WithParam_Required(true)),
		},
		func(_ *reactloops.ReActLoop, action *aicommon.Action) error {
			if action.GetInt("risk_id") <= 0 {
				return utils.Error("risk_id must be a positive integer")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			id := int64(action.GetInt("risk_id"))
			loop.Set(sfu.LoopVarSSARiskID, fmt.Sprintf("%d", id))
			invoker := loop.GetInvoker()
			msg := fmt.Sprintf("目标 SSA Risk ID 已切换为 %d。请先使用 ssa-risk 工具拉取该条（risk_id=%d, get_full_code 视需要设为 true）。", id, id)
			invoker.AddToTimeline("ssa_risk_review", msg)
			operator.Feedback(msg)
			operator.Continue()
		},
	)
}
