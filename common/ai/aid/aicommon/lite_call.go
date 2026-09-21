package aicommon

import "github.com/yaklang/yaklang/common/ai/aispec"

// liteCallCallback is bound once to a single-model Config's Speed slot.
// Raw callbacks stay unchanged for inheritance; each invocation owns its options.
func liteCallCallback(cb AICallbackType) AICallbackType {
	if cb == nil {
		return nil
	}
	return func(caller AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
		copied := *req
		copied.extraSpecOpts = append(append([]aispec.AIConfigOption(nil), req.extraSpecOpts...), aispec.WithThinkingLevel("none"))
		return cb(caller, &copied)
	}
}
