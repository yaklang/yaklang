package aireact

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// ReviewExecutePlan blocks until the plan review flow completes.
// The wait behavior follows the current AgreePolicy (yolo/auto may auto-approve).
func (r *ReAct) ReviewExecutePlan(ctx context.Context, input *aicommon.ExecutePlanInput) (*aicommon.ExecutePlanInput, error) {
	return r.reviewExecutePlan(ctx, input, false)
}

// ForceReviewExecutePlan always requires manual user confirmation for plan review,
// regardless of the current AgreePolicy.
func (r *ReAct) ForceReviewExecutePlan(ctx context.Context, input *aicommon.ExecutePlanInput) (*aicommon.ExecutePlanInput, error) {
	return r.reviewExecutePlan(ctx, input, true)
}

func (r *ReAct) reviewExecutePlan(ctx context.Context, input *aicommon.ExecutePlanInput, forceManualReview bool) (*aicommon.ExecutePlanInput, error) {
	if input == nil {
		return nil, utils.Error("execute plan input is nil")
	}
	if strings.TrimSpace(input.PlanData) == "" {
		return nil, utils.Error("plan data is empty")
	}

	if ctx == nil {
		ctx = r.config.Ctx
	}
	planCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	planPayload := enhancePlanPayloadWithTaskUserInput(input.PlanPayload, r.GetCurrentTask())
	reviewUID := uuid.New().String()

	inputChannel, unregisterMirror := r.registerPlanExecInputMirror(reviewUID)
	defer unregisterMirror()

	baseOpts := aicommon.ConvertConfigToOptions(r.config)
	baseOpts = append(baseOpts,
		aicommon.WithID(reviewUID),
		aicommon.WithTimeline(r.config.Timeline),
		aicommon.WithInheritTieredAICallback(r.config, false),
		aicommon.WithAllowPlanUserInteract(true),
		aicommon.WithEventInputChanx(inputChannel),
		aicommon.WithContext(planCtx),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			e.CoordinatorId = reviewUID
			r.config.EventHandler(e)
		}),
	)
	if forceManualReview {
		baseOpts = append(baseOpts, aicommon.WithForceManualPlanReview(true))
	}

	cod, err := newCoordinatorContextForPlanExec(planCtx, planPayload, baseOpts...)
	if err != nil {
		return nil, utils.Errorf("failed to create coordinator for plan review: %v", err)
	}

	rootTask, err := cod.BuildRootTaskFromPlanData(input.PlanData, planPayload)
	if err != nil {
		return nil, utils.Errorf("failed to build root task for plan review: %v", err)
	}

	planRsp := &aid.PlanResponse{
		RootTask: rootTask,
		Facts:    input.PlanFacts,
		Document: input.PlanDocument,
	}

	approvedRsp, err := cod.ReviewPlanThroughUser(planCtx, planPayload, planRsp)
	if err != nil {
		return nil, err
	}

	reviewed := executePlanInputFromPlanResponse(planPayload, approvedRsp, input)
	if reviewed == nil || strings.TrimSpace(reviewed.PlanData) == "" {
		return nil, utils.Error("approved plan data is empty after review")
	}
	log.Infof("plan review approved, ready to execute asynchronously")
	return reviewed, nil
}

func executePlanInputFromPlanResponse(planPayload string, rsp *aid.PlanResponse, fallback *aicommon.ExecutePlanInput) *aicommon.ExecutePlanInput {
	if rsp == nil || rsp.RootTask == nil {
		return nil
	}
	facts := rsp.Facts
	document := rsp.Document
	if facts == "" && fallback != nil {
		facts = fallback.PlanFacts
	}
	if document == "" && fallback != nil {
		document = fallback.PlanDocument
	}
	return &aicommon.ExecutePlanInput{
		PlanPayload:  planPayload,
		PlanData:     aid.SerializeRootTaskToPlanData(rsp.RootTask),
		PlanFacts:    facts,
		PlanDocument: document,
	}
}
