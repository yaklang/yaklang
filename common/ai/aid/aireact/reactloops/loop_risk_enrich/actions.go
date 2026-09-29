package loop_risk_enrich

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func projectDB(invoker aicommon.AIInvokeRuntime) *gorm.DB {
	if invoker == nil || invoker.GetConfig() == nil {
		return nil
	}
	return invoker.GetConfig().GetDB()
}

func loadRisk(db *gorm.DB, id int) (*schema.Risk, error) {
	if db == nil {
		return nil, fmt.Errorf("project database unavailable")
	}
	if id <= 0 {
		return nil, fmt.Errorf("risk_id must be positive")
	}
	return yakit.GetRisk(db, int64(id))
}

func riskContext(r *schema.Risk) string {
	var b strings.Builder
	s := scopeForRisk(r)
	fmt.Fprintf(&b, "Risk #%d: %s / %s\nType: %s | Severity: %s\nTarget: %s (host=%s, ip=%s, port=%d, path=%s)\nCreated: %s | Updated: %s\nRuntime: %s | AI session: %s\nParameter: %s\nPayload: %s\nDescription: %s\nDetails: %s\nExisting request: %s\nExisting response: %s\nExisting packet pairs: %d\n",
		r.ID, r.Title, r.TitleVerbose, r.RiskType, r.Severity, r.Url, s.host, r.IP, s.port, s.path, r.CreatedAt.Format("2006-01-02T15:04:05Z07:00"), r.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		r.RuntimeId, r.AISessionID, r.Parameter, utils.ShrinkString(r.Payload, 1200),
		utils.ShrinkString(r.Description, 3000), utils.ShrinkString(unquoteRiskField(r.Details), 2000),
		utils.ShrinkString(unquoteRiskField(r.QuotedRequest), 1200),
		utils.ShrinkString(unquoteRiskField(r.QuotedResponse), 1200), len(r.PacketPairs))
	return b.String()
}

func unquoteRiskField(s string) string {
	if unquoted, err := strconv.Unquote(s); err == nil {
		return unquoted
	}
	return s
}

func inspectFlowAction(invoker aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction("inspect_risk_flow",
		"Inspect a flow inside the attached risk's fixed runtime boundary. Compare request/response with the risk's claim.",
		[]aitool.ToolOption{aitool.WithIntegerParam("flow_id", aitool.WithParam_Required(true))}, nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			env, r, err := loadBoundRisk(loop, invoker)
			if err != nil {
				op.Fail(err.Error())
				return
			}
			f, err := getScopedFlow(projectDB(invoker), int64(action.GetInt("flow_id")), env.RuntimeIDs)
			if err != nil {
				op.Fail(fmt.Sprintf("load flow: %v", err))
				return
			}
			matched, exact := flowMatchesTarget(scopeForRisk(r), f)
			loop.Set(inspectedKey(env.RiskID, int64(f.ID)), "true")
			op.Feedback(fmt.Sprintf("flow_id=%d URL=%s target_match=%t exact_path=%t runtime=%s\nCross-target flows are read-only clues; they cannot be attached to this risk.\nRequest:\n%s\nResponse:\n%s",
				f.ID, f.Url, matched, exact, f.RuntimeId, utils.ShrinkString(f.GetRequest(), 4000), utils.ShrinkString(f.GetResponse(), 4000)))
		})
}

func attachFlowAction(invoker aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction("attach_risk_flow",
		"Persist one inspected, assessed supporting HTTP flow to the attached risk, only when the user requested persistence.",
		[]aitool.ToolOption{aitool.WithIntegerParam("flow_id", aitool.WithParam_Required(true))}, nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			env, err := environment(loop)
			if err != nil {
				op.Fail(err.Error())
				return
			}
			if loop.GetCurrentTask() == nil || !userRequestedWrite(loop.GetCurrentTask().GetUserInput()) {
				op.Fail("user did not request persisting evidence; use read-only lookup instead")
				return
			}
			flowID := int64(action.GetInt("flow_id"))
			if loop.Get(inspectedKey(env.RiskID, flowID)) != "true" || getAssessment(loop, flowID).Verdict != "supports" {
				op.Fail("inspect and mark this flow as supporting the risk before attaching evidence")
				return
			}
			if err := attachFlow(projectDB(invoker), env, int(flowID)); err != nil {
				op.Fail(err.Error())
				return
			}
			if _, updated, err := loadBoundRisk(loop, invoker); err == nil {
				loop.Set("risk_enrich_context", riskContext(updated))
			}
			op.Feedback(fmt.Sprintf("Attached flow #%d to risk #%d (preserved existing evidence).", flowID, env.RiskID))
		})
}
