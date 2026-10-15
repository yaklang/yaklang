package aicommon

import (
	"errors"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils"
)

// ErrPromptTooLong is shared with aispec, so callers need not know which layer
// classified the provider failure.
var ErrPromptTooLong = aispec.ErrPromptOverflow

func IsPromptTooLong(err error) bool { return errors.Is(err, ErrPromptTooLong) }

// PromptOverflowEvidenceFrom returns a copy safe for retry-policy consumers.
func PromptOverflowEvidenceFrom(err error) (aispec.PromptOverflowEvidence, bool) {
	var overflow *aispec.PromptOverflowError
	if !errors.As(err, &overflow) || overflow == nil {
		return aispec.PromptOverflowEvidence{}, false
	}
	return overflow.PromptOverflowEvidence, true
}

// ClassifyPromptTooLong supplements transport classification for custom
// callbacks. It never waits for streams, mutates responses, or examines model
// output. Callers must invoke it again after callback completion when needed.
// A nil result means the failure was not confirmed as prompt overflow.
func ClassifyPromptTooLong(rsp *AIResponse, err error) error {
	cause := err
	if callbackErr := rsp.GetError(); callbackErr != nil {
		if cause == nil {
			cause = callbackErr
		} else if !IsPromptTooLong(cause) {
			cause = utils.JoinErrors(cause, callbackErr)
		}
	}
	evidence, classified := PromptOverflowEvidenceFrom(cause)
	if !classified {
		errText := ""
		if cause != nil {
			errText = cause.Error()
		}
		ev := aispec.DetectPromptOverflow(rsp.GetHTTPStatusCode(), rsp.GetHTTPResponseBody(), errText)
		if ev == nil {
			return nil
		}
		evidence = *ev
	}
	if evidence.Provider == "" {
		evidence.Provider = strings.TrimSpace(rsp.GetProviderName())
	}
	if evidence.Model == "" {
		evidence.Model = strings.TrimSpace(rsp.GetModelName())
	}
	if evidence.HTTPStatus == 0 {
		evidence.HTTPStatus = rsp.GetHTTPStatusCode()
	}
	if cause == nil {
		cause = utils.Errorf("AI provider returned HTTP %d: %s", evidence.HTTPStatus, rsp.GetHTTPResponseBody())
	}
	return &aispec.PromptOverflowError{PromptOverflowEvidence: evidence, Cause: cause}
}
