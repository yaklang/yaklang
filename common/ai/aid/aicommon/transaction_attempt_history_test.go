package aicommon

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/schema"
)

// streamTestConfig is a transactionTestConfig whose CallAI emits a streaming
// AI response (reason + output) so we can verify the plain text capture.
type streamTestConfig struct {
	*transactionTestConfig
	callAI func(AICallerConfigIf, *AIRequest) (*AIResponse, error)
}

func (s *streamTestConfig) CallAI(req *AIRequest) (*AIResponse, error) {
	return s.callAI(s.transactionTestConfig, req)
}
func (s *streamTestConfig) CallSpeedPriorityAI(req *AIRequest) (*AIResponse, error) {
	return s.CallAI(req)
}
func (s *streamTestConfig) CallQualityPriorityAI(req *AIRequest) (*AIResponse, error) {
	return s.CallAI(req)
}

// newStreamingCallAI returns a callAi that emits a streaming response with the
// given reason + output text, then succeeds (no callAi error). postHandler is
// expected to fail so the attempt is recorded.
func newStreamingCallAI(reason, output string) func(AICallerConfigIf, *AIRequest) (*AIResponse, error) {
	return func(c AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
		rsp := NewAIResponse(c)
		go func() {
			defer rsp.Close()
			rsp.EmitReasonStream(strings.NewReader(reason))
			rsp.EmitOutputStream(strings.NewReader(output))
		}()
		return rsp, nil
	}
}

// TestCallAITransaction_AttemptHistoryPlainOutput verifies that the attempt
// history records the *pure* AI output / reason text (not the raw HTTP blob),
// so structural problems in the AI response are easy to spot.
func TestCallAITransaction_AttemptHistoryPlainOutput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cfg := &streamTestConfig{
		transactionTestConfig: newTransactionTestConfig(ctx),
	}
	cfg.retryMax = 1

	const (
		reasonMarker = "REASON_THINKING_BODY"
		// Malformed JSON-ish output that postHandler will reject.
		outputMarker = `{"action":"<unterminated"`
	)
	cfg.callAI = newStreamingCallAI(reasonMarker, outputMarker)

	postHandler := func(rsp *AIResponse) error {
		// Drain the stream so the capture hooks fire.
		r := rsp.GetOutputStreamReader("test", true, cfg.GetEmitter())
		ioCopyDiscard(r)
		return fmt.Errorf("action type is empty (available_actions=[finish])")
	}

	err := CallAITransaction(cfg, "plain-output-prompt", cfg.CallAI, postHandler)
	require.Error(t, err)
	errMsg := err.Error()
	t.Logf("error message:\n%s", errMsg)

	assert.Contains(t, errMsg, "Attempt History", "error should contain attempt history section")
	assert.Contains(t, errMsg, outputMarker, "error should contain the plain AI output text")
	assert.Contains(t, errMsg, reasonMarker, "error should contain the plain AI reason text")
	assert.Contains(t, errMsg, "action type is empty", "error should contain postHandler error")
}

// TestCallAITransaction_AttemptHistoryInErrorMessage verifies that the final
// transaction error message contains the history of every retry attempt
// (errors + AI responses), not only the last one.
func TestCallAITransaction_AttemptHistoryInErrorMessage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cfg := newTransactionTestConfig(ctx)
	cfg.retryMax = 3

	var callCount int64
	callAi := func(req *AIRequest) (*AIResponse, error) {
		n := atomic.AddInt64(&callCount, 1)
		rsp := NewUnboundAIResponse()
		rsp.SetRawHTTPResponseData(
			[]byte("HTTP/1.1 500 Internal Server Error\r\nContent-Type: application/json\r\n\r\n"),
			[]byte(fmt.Sprintf(`{"error":"attempt-%d-marker"}`, n)),
		)
		return rsp, fmt.Errorf("HTTP 500: attempt-%d-failure", n)
	}
	postHandler := func(rsp *AIResponse) error { return nil }

	err := CallAITransaction(cfg, "history-prompt", callAi, postHandler)
	require.Error(t, err)
	errMsg := err.Error()
	t.Logf("error message:\n%s", errMsg)

	for n := int64(1); n <= 3; n++ {
		assert.Contains(t, errMsg, fmt.Sprintf("attempt-%d-failure", n),
			"error should contain call-ai error for attempt %d", n)
		assert.Contains(t, errMsg, fmt.Sprintf("attempt-%d-marker", n),
			"error should contain raw response marker for attempt %d", n)
	}
	assert.Contains(t, errMsg, "Attempt History")
}

// TestCallAITransaction_AttemptHistoryStructuredPayload verifies the structured
// failure event payload includes an "attempts" array with one entry per retry.
func TestCallAITransaction_AttemptHistoryStructuredPayload(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var mu sync.Mutex
	var captured []*schema.AiOutputEvent
	base := newTransactionTestConfig(ctx)
	base.retryMax = 2
	base.emitter = NewEmitter("attempt-history-struct-test", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		mu.Lock()
		captured = append(captured, e)
		mu.Unlock()
		return e, nil
	})

	var callCount int64
	callAi := func(req *AIRequest) (*AIResponse, error) {
		n := atomic.AddInt64(&callCount, 1)
		rsp := NewUnboundAIResponse()
		rsp.SetRawHTTPResponseData(
			[]byte("HTTP/1.1 500 Internal Server Error\r\nContent-Type: application/json\r\n\r\n"),
			[]byte(fmt.Sprintf(`{"error":"struct-attempt-%d"}`, n)),
		)
		return rsp, fmt.Errorf("HTTP 500: struct-attempt-%d-err", n)
	}
	postHandler := func(rsp *AIResponse) error { return nil }

	err := CallAITransaction(base, "struct-prompt", callAi, postHandler)
	require.Error(t, err)

	failures := collectAICallFailureEvents(captured)
	require.Len(t, failures, 1, "expected one structured failure event")
	payload := parseFailurePayload(t, failures[0])

	attemptsRaw, ok := payload["attempts"]
	require.True(t, ok, "payload should contain attempts array, got keys: %v", payloadKeys(payload))
	var attempts []map[string]any
	switch v := attemptsRaw.(type) {
	case []any:
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				attempts = append(attempts, m)
			}
		}
	default:
		t.Fatalf("attempts has unexpected type: %T", attemptsRaw)
	}
	require.Len(t, attempts, 2, "expected 2 attempt records, got %d", len(attempts))

	for idx, a := range attempts {
		cause := fmt.Sprint(a["call_ai_error"])
		assert.Contains(t, cause, fmt.Sprintf("struct-attempt-%d-err", idx+1),
			"attempt %d call_ai_error mismatch: %v", idx+1, a)
		raw := fmt.Sprint(a["raw_response"])
		assert.Contains(t, raw, fmt.Sprintf("struct-attempt-%d", idx+1),
			"attempt %d raw_response mismatch: %v", idx+1, a)
	}
}

// TestCallAITransaction_AttemptHistoryStructuredPlainOutput verifies the
// structured payload carries the plain output text when postHandler fails on a
// streaming response.
func TestCallAITransaction_AttemptHistoryStructuredPlainOutput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var mu sync.Mutex
	var captured []*schema.AiOutputEvent
	base := newTransactionTestConfig(ctx)
	base.retryMax = 1
	base.emitter = NewEmitter("plain-struct-test", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		mu.Lock()
		captured = append(captured, e)
		mu.Unlock()
		return e, nil
	})

	const outputMarker = `{"action":"<unterminated"`
	cfg := &streamTestConfig{transactionTestConfig: base}
	cfg.callAI = newStreamingCallAI("REASON_BODY", outputMarker)

	postHandler := func(rsp *AIResponse) error {
		r := rsp.GetOutputStreamReader("test", true, cfg.GetEmitter())
		ioCopyDiscard(r)
		return fmt.Errorf("action type is empty (available_actions=[finish])")
	}

	err := CallAITransaction(cfg, "plain-struct-prompt", cfg.CallAI, postHandler)
	require.Error(t, err)

	failures := collectAICallFailureEvents(captured)
	require.Len(t, failures, 1)
	payload := parseFailurePayload(t, failures[0])
	attemptsRaw, ok := payload["attempts"]
	require.True(t, ok)
	var attempts []map[string]any
	if v, ok := attemptsRaw.([]any); ok {
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				attempts = append(attempts, m)
			}
		}
	}
	require.Len(t, attempts, 1)
	out := fmt.Sprint(attempts[0]["output"])
	assert.Contains(t, out, outputMarker, "structured attempt output should contain plain AI text")
	reason := fmt.Sprint(attempts[0]["reason"])
	assert.Contains(t, reason, "REASON_BODY", "structured attempt reason should contain plain AI reason")
}

func payloadKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func ioCopyDiscard(r interface{ Read([]byte) (int, error) }) {
	buf := make([]byte, 4096)
	for {
		if _, err := r.Read(buf); err != nil {
			return
		}
	}
}
