package loop_risk_enrich

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

type riskAssessment struct {
	Verdict string
	Reason  string
}

func inspectedKey(riskID, flowID int64) string {
	return fmt.Sprintf("risk-enrich-inspected-%d-%d", riskID, flowID)
}

func getAssessment(loop *reactloops.ReActLoop, flowID int64) riskAssessment {
	if loop == nil {
		return riskAssessment{}
	}
	if items, ok := loop.GetVariable("risk_enrich_assessments").(map[int64]riskAssessment); ok {
		return items[flowID]
	}
	return riskAssessment{}
}

func assessmentPrompt(loop *reactloops.ReActLoop) string {
	items, _ := loop.GetVariable("risk_enrich_assessments").(map[int64]riskAssessment)
	ids := make([]int64, 0, len(items))
	for id := range items {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var b strings.Builder
	for _, id := range ids[:min(len(ids), 18)] {
		item := items[id]
		fmt.Fprintf(&b, "- flow_id=%d verdict=%s: %s\n", id, item.Verdict, utils.ShrinkString(item.Reason, 300))
	}
	return b.String()
}

func recordAssessmentAction() reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction("assess_risk_flow",
		"After inspecting a flow, record whether its request/response supports the attached risk, is uncertain, or is irrelevant. Prevents repeated inspection and unsafe writeback.",
		[]aitool.ToolOption{
			aitool.WithIntegerParam("flow_id", aitool.WithParam_Required(true)),
			aitool.WithStringParam("verdict", aitool.WithParam_Required(true), aitool.WithParam_Description("supports, uncertain, or irrelevant")),
			aitool.WithStringParam("reason", aitool.WithParam_Required(true), aitool.WithParam_Description("Observed request/payload/response evidence or specific missing proof")),
		}, nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			env, err := environment(loop)
			if err != nil {
				op.Fail(err.Error())
				return
			}
			id := int64(action.GetInt("flow_id"))
			if id <= 0 || loop.Get(inspectedKey(env.RiskID, id)) != "true" {
				op.Fail("inspect_risk_flow must be called for this attached risk and flow first")
				return
			}
			verdict, reason := strings.ToLower(strings.TrimSpace(action.GetString("verdict"))), strings.TrimSpace(action.GetString("reason"))
			if verdict != "supports" && verdict != "uncertain" && verdict != "irrelevant" {
				op.Fail("verdict must be supports, uncertain, or irrelevant")
				return
			}
			if reason == "" {
				op.Fail("reason must cite the observation or evidence gap")
				return
			}
			items, _ := loop.GetVariable("risk_enrich_assessments").(map[int64]riskAssessment)
			copyItems := make(map[int64]riskAssessment, len(items)+1)
			for k, v := range items {
				copyItems[k] = v
			}
			copyItems[id] = riskAssessment{Verdict: verdict, Reason: utils.ShrinkString(reason, 800)}
			loop.Set("risk_enrich_assessments", copyItems)
			op.Feedback(fmt.Sprintf("Recorded flow #%d assessment: %s (%s)", id, verdict, utils.ShrinkString(reason, 300)))
		})
}
