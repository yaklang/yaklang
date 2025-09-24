package aireact

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func (r *ReAct) invokeAskForClarification(question string, payloads []string) string {
	r.currentUserInteractiveCount++
	r.addToTimeline("question-for-clarification", question)
	ep := r.config.epm.CreateEndpointWithEventType(schema.EVENT_TYPE_REQUIRE_USER_INTERACTIVE)
	ep.SetDefaultSuggestionContinue()
	var opts []map[string]any
	for i, payload := range payloads {
		opts = append(opts, map[string]any{
			"index":        i + 1,
			"prompt_title": payload,
		})
	}
	result := map[string]any{
		"id":      ep.GetId(),
		"prompt":  question,
		"options": opts,
	}
	ep.SetReviewMaterials(result)
	err := r.config.SubmitCheckpointRequest(ep.GetCheckpoint(), result)
	if err != nil {
		log.Errorf("Failed to submit checkpoint request: %v", err)
	}
	r.config.EmitInteractiveJSON(
		ep.GetId(),
		schema.EVENT_TYPE_REQUIRE_USER_INTERACTIVE,
		"require-user-interact",
		result,
	)
	ctx := r.config.GetContext()
	ctx = utils.SetContextKey(ctx, SKIP_AI_REVIEW, true)
	r.config.DoWaitAgree(ctx, ep)
	params := ep.GetParams()
	r.config.EmitInteractiveRelease(ep.GetId(), params)
	r.config.CallAfterInteractiveEventReleased(ep.GetId(), params)
	suggestion := params.GetAnyToString("suggestion")
	r.addToTimeline(
		"user-clarification",
		fmt.Sprintf("User clarification requested: %s result: %v",
			question, suggestion),
	)
	return suggestion
}
