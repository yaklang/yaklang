package loop_ssa_risk_review

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	sfu "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// WithReloadSSARiskAction registers reload_ssa_risk — loads one SSA Risk row (+ disposals) from the SSA DB.
func WithReloadSSARiskAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"reload_ssa_risk",
		"Load one SSA Risk by risk_id from the SSA project database including code_fragment, detail, runtime_id, program, rule; includes recent disposition history (with inheritance). Prefer this over guessing when reviewing.",
		[]aitool.ToolOption{
			aitool.WithIntegerParam("risk_id", aitool.WithParam_Description("SSA Risk primary key (positive integer)."), aitool.WithParam_Required(true)),
			aitool.WithIntegerParam("get_full_code", aitool.WithParam_Description("Set to 1 for longer code_fragment in feedback (default 0).")),
		},
		func(_ *reactloops.ReActLoop, action *aicommon.Action) error {
			if action.GetInt("risk_id") <= 0 {
				return utils.Error("risk_id must be a positive integer")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			id := int64(action.GetInt("risk_id"))
			db := sfu.GetSSADB()
			if db == nil {
				r.AddToTimeline("ssa_risk_review", "reload_ssa_risk: no SSA DB")
				operator.Feedback("reload_ssa_risk failed: SSA database not available")
				operator.SetReflectionLevel(reactloops.ReflectionLevel_Critical)
				operator.Continue()
				return
			}
			risk, err := sfu.LoadRisk(db, id)
			if err != nil || risk == nil {
				msg := fmt.Sprintf("reload_ssa_risk: cannot load risk_id=%d: %v", id, err)
				r.AddToTimeline("ssa_risk_review", msg)
				operator.Feedback(msg)
				operator.SetReflectionLevel(reactloops.ReflectionLevel_Critical)
				operator.Continue()
				return
			}
			disposals, _ := sfu.ListRiskDisposals(db, id, true)
			codeLim := 4000
			if action.GetInt("get_full_code") != 0 {
				codeLim = 12000
			}
			text := sfu.RiskReloadText(risk, disposals, codeLim)
			loop.Set(sfu.LoopVarSSARiskID, fmt.Sprintf("%d", id))
			loop.Set("ssa_risk_reload_digest", utils.ShrinkTextBlock(text, 16000))
			operator.Feedback("[reload_ssa_risk]\n" + utils.ShrinkTextBlock(text, 12000))
			operator.Continue()
		},
	)
}

// WithMarkSSARiskDisposalAction registers mark_ssa_risk_disposal — writes SSA risk disposal rows via yakit.
func WithMarkSSARiskDisposalAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"mark_ssa_risk_disposal",
		"Create disposal record(s) for one or more SSA risk ids with status not_issue/suspicious/is_issue/not_set and optional comment.",
		[]aitool.ToolOption{
			aitool.WithStringParam("risk_ids", aitool.WithParam_Description("Comma-separated SSA risk ids, e.g. \"12,34\"."), aitool.WithParam_Required(true)),
			aitool.WithStringParam("status", aitool.WithParam_Description("Disposal status: not_issue | suspicious | is_issue | not_set (aliases mapped)."), aitool.WithParam_Required(true)),
			aitool.WithStringParam("comment", aitool.WithParam_Description("Short rationale / note stored on disposal rows.")),
		},
		func(_ *reactloops.ReActLoop, action *aicommon.Action) error {
			if strings.TrimSpace(action.GetString("risk_ids")) == "" {
				return utils.Error("risk_ids is required")
			}
			if strings.TrimSpace(action.GetString("status")) == "" {
				return utils.Error("status is required")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			db := sfu.GetSSADB()
			if db == nil {
				operator.Feedback("mark_ssa_risk_disposal failed: SSA database not available")
				operator.SetReflectionLevel(reactloops.ReflectionLevel_Critical)
				operator.Continue()
				return
			}
			var ids []int64
			for _, p := range strings.Split(action.GetString("risk_ids"), ",") {
				p = strings.TrimSpace(p)
				if p == "" {
					continue
				}
				v, err := strconv.ParseInt(p, 10, 64)
				if err != nil || v <= 0 {
					operator.Feedback(fmt.Sprintf("invalid risk id token %q", p))
					operator.SetReflectionLevel(reactloops.ReflectionLevel_Critical)
					operator.Continue()
					return
				}
				ids = append(ids, v)
			}
			if len(ids) == 0 {
				operator.Feedback("mark_ssa_risk_disposal: no valid risk_ids")
				operator.Continue()
				return
			}
			st := sfu.NormalizeDisposalStatus(action.GetString("status"))
			req := &ypb.CreateSSARiskDisposalsRequest{
				RiskIds: ids,
				Status:  st,
				Comment: strings.TrimSpace(action.GetString("comment")),
			}
			created, err := yakit.CreateSSARiskDisposals(db, req)
			if err != nil {
				operator.Feedback(fmt.Sprintf("mark_ssa_risk_disposal failed: %v", err))
				operator.SetReflectionLevel(reactloops.ReflectionLevel_Critical)
				operator.Continue()
				return
			}
			operator.Feedback(fmt.Sprintf("[mark_ssa_risk_disposal] wrote %d disposal row(s) status=%s comment=%q", len(created), st, req.GetComment()))
			if loop != nil {
				loop.Set("ssa_risk_last_disposal_status", st)
			}
			r.AddToTimeline("ssa_risk_review", fmt.Sprintf("disposal status=%s for %d risk(s)", st, len(ids)))
			operator.Continue()
		},
	)
}

// WithDeriveRuleSeedFromRiskAction registers derive_rule_seed_from_risk — JSON seed for rule writer.
func WithDeriveRuleSeedFromRiskAction(_ aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"derive_rule_seed_from_risk",
		"Build a structured JSON seed from the given risk_id for write_syntaxflow_rule.",
		[]aitool.ToolOption{
			aitool.WithIntegerParam("risk_id", aitool.WithParam_Description("SSA Risk primary key."), aitool.WithParam_Required(true)),
		},
		func(_ *reactloops.ReActLoop, action *aicommon.Action) error {
			if action.GetInt("risk_id") <= 0 {
				return utils.Error("risk_id must be a positive integer")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			id := int64(action.GetInt("risk_id"))
			db := sfu.GetSSADB()
			if db == nil {
				operator.Feedback("derive_rule_seed_from_risk: SSA database not available")
				operator.Continue()
				return
			}
			risk, err := sfu.LoadRisk(db, id)
			if err != nil || risk == nil {
				operator.Feedback(fmt.Sprintf("derive_rule_seed_from_risk: load failed: %v", err))
				operator.Continue()
				return
			}
			seed := sfu.RiskToRuleSeedJSON(risk)
			if loop != nil {
				loop.Set("sf_rule_seed_from_risk_json", seed)
			}
			operator.Feedback("[derive_rule_seed_from_risk]\n" + seed)
			operator.Continue()
		},
	)
}

// WithSetSSARiskReviewTargetAction registers set_ssa_risk_review_target: switch the focused SSA risk id mid-session without new attachments.
func WithSetSSARiskReviewTargetAction(_ aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"set_ssa_risk_review_target",
		"Set the active SSA risk primary key for this ssa_risk_review loop (loop var ssa_risk_id). After changing, use reload_ssa_risk or the ssa-risk tool with the new risk_id before drawing conclusions.",
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
			msg := fmt.Sprintf("目标 SSA Risk ID 已切换为 %d。请先使用 reload_ssa_risk（或 ssa-risk 工具）拉取该条（risk_id=%d, get_full_code 视需要设为 true）。", id, id)
			invoker.AddToTimeline("ssa_risk_review", msg)
			operator.Feedback(msg)
			operator.Continue()
		},
	)
}
