package aicommon

import (
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// Fixed AiOutputEvent.NodeId / error_code values for clients to detect AI failures.

const (
	NodeAICallFailure = "ai_call_failure"
	// ErrorCodeAICallFailed is returned when any AI call fails after all retries.
	ErrorCodeAICallFailed = "AI_CALL_FAILED"
)

// EmitAICallFailureIfApplicable emits a system structured event when any AI call fails.
// It includes model tier, provider name, model name and the error cause.
// Extra context fields (e.g. liteforge_action, react_loop_name) can be passed via extra.
func EmitAICallFailureIfApplicable(c AICallerConfigIf, tier consts.ModelTier, rsp *AIResponse, err error, extra map[string]any) {
	if err == nil || c == nil {
		return
	}
	em := c.GetEmitter()
	if utils.IsNil(em) {
		return
	}

	payload := map[string]any{
		"error_code":    ErrorCodeAICallFailed,
		"model_tier":    string(tier),
		"provider_name": "",
		"model_name":    "",
		"cause":         err.Error(),
	}
	if rsp != nil && !utils.IsNil(rsp) {
		payload["provider_name"] = rsp.GetProviderName()
		payload["model_name"] = rsp.GetModelName()
	}
	for k, v := range extra {
		payload[k] = v
	}

	_, emitErr := em.EmitAPIRequestFailed(NodeAICallFailure, payload)
	if emitErr != nil {
		log.Errorf("emit ai call failure event: %v", emitErr)
	}
}
