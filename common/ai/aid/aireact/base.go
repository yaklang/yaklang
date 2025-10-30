package aireact

import (
	"fmt"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

type InteractOption struct {
	Value  string
	Prompt string
}

func (r *ReAct) _requireUserInteract(question string, options []map[string]any) (string, string, error) {
	if !r.config.AllowRequireForUserInteract {
		r.AddToTimeline("note", "Require user interact but not enabled, skip it.")
		return "", "", utils.Errorf("require user interact but not enabled")
	}

	ep := r.config.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_REQUIRE_USER_INTERACTIVE)
	ep.SetDefaultSuggestionContinue()

	result := map[string]any{
		"id":      ep.GetId(),
		"prompt":  question,
		"options": options,
	}
	ep.SetReviewMaterials(result)
	err := r.config.SubmitCheckpointRequest(ep.GetCheckpoint(), result)
	if err != nil {
		log.Errorf(err.Error())
	}
	r.config.EmitInteractiveJSON(
		ep.GetId(),
		schema.EVENT_TYPE_REQUIRE_USER_INTERACTIVE,
		"require-user-interact",
		result,
	)
	ctx := r.GetCurrentTask().GetContext()
	ctx = utils.SetContextKey(ctx, SKIP_AI_REVIEW, true)
	r.config.DoWaitAgree(ctx, ep)
	params := ep.GetParams()
	r.config.EmitInteractiveRelease(ep.GetId(), params)
	r.config.CallAfterInteractiveEventReleased(ep.GetId(), params)
	suggestion := params.GetAnyToString("suggestion")
	extra := params.GetAnyToString("extra_info")
	r.AddToTimeline(
		"user-clarification",
		fmt.Sprintf("User clarification requested: %s result: %v",
			question, suggestion),
	)
	return suggestion, extra, nil
}

func (r *ReAct) RequireUserInteractEx(question string, options []*InteractOption) (string, string, error) {
	opts := []map[string]any{}
	for i, payload := range options {
		opts = append(opts, map[string]any{
			"index":        i + 1,
			"prompt_title": payload.Value,
			"prompt_value": payload.Prompt,
		})
	}
	return r._requireUserInteract(question, opts)
}

func (r *ReAct) RequireUserInteract(question string, options []string) (string, string, error) {
	opts := []map[string]any{}
	for i, payload := range options {
		opts = append(opts, map[string]any{
			"index":        i + 1,
			"prompt_title": payload,
		})
	}
	return r._requireUserInteract(question, opts)
}
