package aicommon

import (
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

// mergePostHandlerAndCallbackError combines postHandler validation errors with
// async AI callback errors (SetError). Infrastructure failures from the model
// layer are surfaced as the primary error so timeouts are not masked as parse failures.
func mergePostHandlerAndCallbackError(postHandlerErr, callbackErr error) error {
	if callbackErr == nil {
		return postHandlerErr
	}
	if postHandlerErr == nil {
		return callbackErr
	}
	if isAICallbackInfrastructureError(callbackErr) {
		return utils.Wrapf(callbackErr,
			"ai call failed before response could be parsed (parse error: %v)", postHandlerErr)
	}
	return utils.Errorf("post handler: %v; ai callback: %v", postHandlerErr, callbackErr)
}

func isAICallbackInfrastructureError(err error) bool {
	if err == nil {
		return false
	}
	if IsStreamIdleTimeout(err) {
		return true
	}
	msg := strings.ToLower(err.Error())
	markers := []string{
		"context deadline exceeded",
		"context canceled",
		"i/o timeout",
		"ai stream read failed",
		"unexpected eof",
		"stream idle timeout",
		"request post to",
		"connection refused",
		"connection reset",
		"no such host",
		"tls:",
		"http 5",
		"http 4",
		"eof",
		"broken pipe",
	}
	for _, marker := range markers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}
