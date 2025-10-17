package aid

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
)

func (pr *planRequest) handlePlanWithUserInteract(interactAction *aicommon.Action) (*PlanResponse, error) {
	q := interactAction.GetString("question")
	opt := interactAction.GetInvokeParamsArray("options")
	var opts []*RequireInteractiveRequestOption
	idx := -1
	for _, o := range opt {
		idx++
		opts = append(opts, &RequireInteractiveRequestOption{
			Index:       idx,
			PromptTitle: o.GetString("option_name"),
			Prompt:      o.GetString("option_value"),
		})
	}
	haveOpt := len(opt) > 0
	_ = haveOpt
	params, ep, err := pr.config.RequireUserPromptWithEndpointResult(q, opts...)
	if err != nil {
		return nil, utils.Errorf("plan: require user interact failed: %v", err)
	}
	_ = params

	pr.config.memory.timeline.PushUserInteraction(
		aicommon.UserInteractionStage_BeforePlan,
		ep.GetSeq(),
		q,
		string(utils.Jsonify(params)),
	)

	pr.deltaInteractCount(1)
	if pr.GetInteractCount() >= pr.config.planUserInteractMaxCount {
		pr.disableInteract = true
	}
	return pr.Invoke()
}
