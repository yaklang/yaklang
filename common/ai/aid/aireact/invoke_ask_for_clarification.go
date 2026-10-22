package aireact

import (
	"context"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/interaction"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func (r *ReAct) AskForClarification(ctx context.Context, question string, payloads []string) string {
	if utils.IsNil(ctx) {
		ctx = r.config.GetContext()
	}

	// Check context cancellation early
	select {
	case <-ctx.Done():
		log.Errorf("AskForClarification cancelled: %v", ctx.Err())
		return ""
	default:
	}

	taskID := ""
	if task := r.GetCurrentTask(); task != nil {
		taskID = task.GetId()
	}
	// Existing framework callers use the same rich request lifecycle on capable
	// clients. Truly open-ended legacy questions retain their original transport.
	if r.config.UIQuestionsEnabled() && len(payloads) >= 2 && len(payloads) <= 10 {
		options := make([]interaction.Option, 0, len(payloads))
		for i, label := range payloads {
			options = append(options, interaction.Option{ID: fmt.Sprint(i + 1), Label: label})
		}
		result, err := r.config.RequestQuestions(ctx, taskID, []interaction.Question{{ID: "clarification", Title: "Clarification", Prompt: question, AllowFreeText: true, Options: options}}, true, 0)
		if err != nil {
			return "Clarification unavailable: " + err.Error() + ". No user consent was given."
		}
		r.SaveTimeline()
		if reply, ok := result.(map[string]any); ok && reply["status"] == "submitted" {
			return utils.InterfaceToString(reply["answer"])
		}
		return "No answer was received. This is not consent."
	}
	if err := r.config.AcquireClarification(taskID, true); err != nil {
		return "Clarification unavailable: " + err.Error() + ". No user consent was given."
	}
	ep := r.config.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_REQUIRE_USER_INTERACTIVE)
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
	ctx = utils.SetContextKey(ctx, SKIP_AI_REVIEW, true)
	r.config.DoWaitAgreeWithPolicy(ctx, aicommon.AgreePolicyManual, ep) // ask for user clarification must should be manual mode
	params := ep.GetParams()
	r.config.EmitInteractiveRelease(ep.GetId(), params)
	r.config.CallAfterInteractiveEventReleased(ep.GetId(), params)
	r.SaveTimeline()
	suggestion := utils.InterfaceToString(params)
	return suggestion
}
