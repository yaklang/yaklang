package aicommon

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http/httputil"
	"strings"
)

// aiHTTPResponseBody accepts both raw chunk framing and already-decoded JSON
// from provider callbacks. It never waits for data from a completed callback.
func aiHTTPResponseBody(rsp *AIResponse) []byte {
	if rsp == nil {
		return nil
	}
	body := rsp.GetHTTPResponseBody()
	if json.Valid(body) || !strings.Contains(strings.ToLower(rsp.GetHTTPHeader("Transfer-Encoding")), "chunked") {
		return body
	}
	decoded, err := io.ReadAll(httputil.NewChunkedReader(bytes.NewReader(body)))
	if err == nil {
		return decoded
	}
	return body
}

// aiHTTPResponseError describes a provider failure independently of retry policy.
func aiHTTPResponseError(rsp *AIResponse) error {
	if overflowErr := ClassifyPromptTooLong(rsp, nil); overflowErr != nil {
		return overflowErr
	}
	body := aiHTTPResponseBody(rsp)
	message := strings.TrimSpace(string(body))
	var envelope struct {
		Error   json.RawMessage `json:"error"`
		Message string          `json:"message"`
	}
	if json.Unmarshal(body, &envelope) == nil {
		var detail struct {
			Message string `json:"message"`
		}
		var text string
		switch {
		case json.Unmarshal(envelope.Error, &detail) == nil && strings.TrimSpace(detail.Message) != "":
			message = strings.TrimSpace(detail.Message)
		case json.Unmarshal(envelope.Error, &text) == nil && strings.TrimSpace(text) != "":
			message = strings.TrimSpace(text)
		case strings.TrimSpace(envelope.Message) != "":
			message = strings.TrimSpace(envelope.Message)
		}
	}
	description := fmt.Sprintf("AI provider returned HTTP %d", rsp.GetHTTPStatusCode())
	if message != "" {
		description += ": " + message
	}
	if cause := rsp.GetError(); cause != nil {
		return fmt.Errorf("%s: %w", description, cause)
	}
	return errors.New(description)
}

// normalizeAIRateLimitError bridges response-only rate limits to the existing
// retry path without replacing callback errors. Unlike is429Response, it must
// not wait for headers: completed non-HTTP callbacks may never provide them.
func normalizeAIRateLimitError(rsp *AIResponse, err error) error {
	if err != nil || rsp.GetHTTPStatusCode() != 429 {
		return err
	}
	return aiHTTPResponseError(rsp)
}
