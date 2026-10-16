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

// decodeAIHTTPErrorBody decodes chunk framing when present, leaving already-decoded
// JSON intact. Empty or malformed input is returned unchanged for diagnostics.
// It reads only the currently available body and never waits for more data.
func decodeAIHTTPErrorBody(rsp *AIResponse) []byte {
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

// aiHTTPResponseError constructs an error for a response already identified as
// an HTTP failure. It preserves prompt-overflow classification and response
// causes; it does not decide whether the caller should retry.
func aiHTTPResponseError(rsp *AIResponse) error {
	if overflowErr := ClassifyPromptTooLong(rsp, nil); overflowErr != nil {
		return overflowErr
	}
	body := decodeAIHTTPErrorBody(rsp)
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

// ensure429CallbackError fills a missing callback error for a known HTTP 429.
// Existing errors and all other statuses are returned unchanged. It never waits
// for headers: completed non-HTTP callbacks may never provide them.
func ensure429CallbackError(rsp *AIResponse, err error) error {
	if err != nil || rsp.GetHTTPStatusCode() != 429 {
		return err
	}
	return aiHTTPResponseError(rsp)
}
