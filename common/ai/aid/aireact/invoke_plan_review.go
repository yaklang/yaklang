package aireact

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
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
	session, err := r.BeginPlanCoordinatorSession(ctx, input, forceManualReview)
	if err != nil {
		return nil, err
	}
	defer session.Close()

	if err := session.ReviewPlan(ctx); err != nil {
		return nil, err
	}
	return session.ApprovedPlanInput(), nil
}
