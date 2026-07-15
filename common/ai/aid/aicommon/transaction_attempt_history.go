package aicommon

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

// transactionAttemptRecord captures a single attempt inside callAITransaction so
// that, when the whole transaction eventually fails, the caller can inspect the
// full retry history (every AI response / error that happened) instead of only
// the last one.
//
// PlainOutput / PlainReason hold the *pure* AI output / thinking text captured
// via the AIResponse output-capture hooks after the stream has been consumed by
// the postHandler. This makes structural problems in the AI response easy to
// spot, unlike the raw HTTP dump which is hard to read (especially for streaming
// responses). When the stream was not consumed (e.g. callAi returned an error
// before postHandler ran) these fields are empty and RawHTTPResponseDump is used
// as a fallback.
type transactionAttemptRecord struct {
	// Attempt is the 1-based attempt index.
	Attempt int64

	// PromptSummary is a shrunk copy of the prompt sent to the AI for this
	// attempt (after RetryPromptBuilder).
	PromptSummary string

	// CallAiErr is the error returned by the callAi callback (nil on
	// success).
	CallAiErr error

	// PostHandlerErr is the error returned by postHandler (or merged with
	// the async callback error) for this attempt. Nil when postHandler
	// succeeded.
	PostHandlerErr error

	// HTTPStatus is the HTTP status code extracted from the raw response
	// header (0 when no header / response was received).
	HTTPStatus int

	// ProviderName / ModelName capture which AI model served this attempt.
	ProviderName string
	ModelName    string

	// PlainOutput is the pure AI output text captured after the stream was
	// consumed. Empty when the stream could not be captured.
	PlainOutput string

	// PlainReason is the pure AI thinking / reason text captured after the
	// stream was consumed. Empty when not captured.
	PlainReason string

	// RawHTTPResponseDump is the raw HTTP response (header + body preview).
	// Used as a fallback diagnostic when PlainOutput is unavailable.
	RawHTTPResponseDump string

	// AsyncCallbackErr is the error stored on the response via SetError by
	// the async AI callback goroutine (e.g. timeout while streaming).
	AsyncCallbackErr error
}

// buildAttemptRecord collects a snapshot of the current attempt's diagnostic
// data. It only reads non-streaming fields so it is safe to call at any time.
func buildAttemptRecord(attempt int64, prompt string, callAiErr error, rsp *AIResponse) transactionAttemptRecord {
	rec := transactionAttemptRecord{
		Attempt:       attempt,
		PromptSummary: utils.ShrinkString(prompt, 512),
		CallAiErr:     callAiErr,
	}
	if rsp != nil {
		rec.HTTPStatus = rsp.GetHTTPStatusCode()
		rec.ProviderName = rsp.GetProviderName()
		rec.ModelName = rsp.GetModelName()
		rec.RawHTTPResponseDump = rsp.GetRawHTTPResponseDump()
		rec.AsyncCallbackErr = rsp.GetError()
		rec.PlainOutput = rsp.GetPlainOutput()
		rec.PlainReason = rsp.GetPlainReason()
	}
	return rec
}

// ToMap converts the attempt record into a JSON-friendly map suitable for
// inclusion in the structured AI call failure event payload.
func (r transactionAttemptRecord) ToMap() map[string]any {
	m := map[string]any{
		"attempt":      r.Attempt,
		"prompt":       r.PromptSummary,
		"http_status":  r.HTTPStatus,
		"provider":     r.ProviderName,
		"model":        r.ModelName,
		"output":       utils.ShrinkString(r.PlainOutput, 4096),
		"reason":       utils.ShrinkString(r.PlainReason, 4096),
		"raw_response": utils.ShrinkString(r.RawHTTPResponseDump, 4096),
	}
	if r.CallAiErr != nil {
		m["call_ai_error"] = r.CallAiErr.Error()
	}
	if r.PostHandlerErr != nil {
		m["post_handler_error"] = r.PostHandlerErr.Error()
	}
	if r.AsyncCallbackErr != nil {
		m["async_callback_error"] = r.AsyncCallbackErr.Error()
	}
	return m
}

// FailedAIOutput returns a shrunk copy of the AI output text that caused this
// attempt to fail. It prefers the plain output, then the plain reason. The
// raw HTTP response dump is intentionally excluded — showing raw HTTP to the
// AI is meaningless for retry correction.
func (r transactionAttemptRecord) FailedAIOutput() string {
	if r.PlainOutput != "" {
		return utils.ShrinkString(r.PlainOutput, 2048)
	}
	if r.PlainReason != "" {
		return utils.ShrinkString(r.PlainReason, 2048)
	}
	return ""
}

// String renders a human-readable summary of a single attempt for inclusion in
// the transaction error message. The plain AI output / reason text is shown
// prominently (shrunk to keep the error message readable) so structural issues
// in the AI response are easy to spot.
func (r transactionAttemptRecord) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "  [Attempt %d]", r.Attempt)
	if r.HTTPStatus != 0 {
		fmt.Fprintf(&b, " HTTP %d", r.HTTPStatus)
	}
	if r.ProviderName != "" || r.ModelName != "" {
		fmt.Fprintf(&b, " (model: %s:%s)", r.ProviderName, r.ModelName)
	}
	b.WriteString("\n")

	if r.CallAiErr != nil {
		fmt.Fprintf(&b, "    call-ai error: %v\n", r.CallAiErr)
	}
	if r.AsyncCallbackErr != nil {
		fmt.Fprintf(&b, "    async callback error: %v\n", r.AsyncCallbackErr)
	}
	if r.PostHandlerErr != nil {
		fmt.Fprintf(&b, "    post-handler error: %v\n", r.PostHandlerErr)
	}
	if r.PromptSummary != "" {
		fmt.Fprintf(&b, "    prompt: %s\n", r.PromptSummary)
	}
	if r.PlainReason != "" {
		fmt.Fprintf(&b, "    reason: %s\n", utils.ShrinkString(r.PlainReason, 1024))
	}
	if r.PlainOutput != "" {
		fmt.Fprintf(&b, "    output: %s\n", utils.ShrinkString(r.PlainOutput, 2048))
	}
	// Only show the raw HTTP dump when no plain output was captured, to avoid
	// drowning the readable text in a hard-to-parse HTTP blob.
	if r.PlainOutput == "" && r.RawHTTPResponseDump != "" {
		fmt.Fprintf(&b, "    raw response: %s\n", utils.ShrinkString(r.RawHTTPResponseDump, 2048))
	}
	return b.String()
}

// formatAttemptHistory renders all attempt records into a single readable
// block used by the transaction failure message.
func formatAttemptHistory(records []transactionAttemptRecord) string {
	if len(records) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n--- Attempt History ---\n")
	for _, r := range records {
		b.WriteString(r.String())
	}
	return b.String()
}
