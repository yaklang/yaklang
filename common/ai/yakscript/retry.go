package yakscript

import (
	"errors"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

// ModelError reports a provider failure without turning it into model content.
// Use return ai.ModelError(req, "http", 429, "", "busy", "60"). For malformed
// SSE/JSON use kind "protocol"; for application error codes use "business".
// The request-local record survives Yak's ~ / panic wrapping. Nothing is global.
func ModelError(req *Request, kind string, status int, code, message, retryAfter string) error {
	failure := &aispec.ModelCallFailure{Kind: kind, StatusCode: status, Code: code, Cause: errors.New(message), RetryAfter: retryAfter}
	switch kind {
	case "network", "http", "protocol", "business", "configuration", "script":
	default:
		failure = &aispec.ModelCallFailure{Kind: "script", Cause: errors.New("invalid model failure kind: " + kind)}
	}
	req.SetFailure(failure)
	return failure
}

func (r *Request) SetFailure(f *aispec.ModelCallFailure) {
	r.failureMu.Lock()
	defer r.failureMu.Unlock()
	r.failure = f
}
func (r *Request) Failure() *aispec.ModelCallFailure {
	r.failureMu.Lock()
	defer r.failureMu.Unlock()
	if r.failure == nil {
		return nil
	}
	clone := *r.failure
	return &clone
}
