package aispec

import (
	"errors"
	"strings"
)

var ErrRequestSizeLimit = errors.New("AI request exceeds byte limit")

// Retrying an identical oversized or explicitly unsupported input cannot repair
// it. Return control to the agent's projection/compression/routing layer even
// when network recovery is configured to retry forever.
func IsUnrecoverableInput(err error) bool {
	if errors.Is(err, ErrRequestSizeLimit) || errors.Is(err, ErrPromptOverflow) {
		return true
	}
	return IsUnsupportedImageInput(err)
}

func IsUnsupportedImageInput(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	if !strings.Contains(message, "http 400") && !strings.Contains(message, "http 422") {
		return false
	}
	for _, needle := range []string{"does not support image", "image inputs are not supported", "image input is not supported", "image_url is not supported", "image_url is only supported", "unsupported image input", "only text is supported"} {
		if strings.Contains(message, needle) {
			return true
		}
	}
	return false
}
