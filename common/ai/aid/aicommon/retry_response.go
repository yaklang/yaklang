package aicommon

import (
	"io"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/ai/aispec"
)

const retryResponseLimit = 16 * 1024
const retryToolCallLimit = 8

// Keep diagnostics separate from admission: observing a response must never
// repair its arguments, invoke an action, or change the selected protocol.
type retryToolCall struct {
	Index              int    `json:"index"`
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Arguments          string `json:"arguments"`
	ArgumentsTruncated bool   `json:"arguments_truncated,omitempty"`
}

type retryResponseTrace struct {
	mu                      sync.Mutex
	native                  bool
	finish, content, reason string
	contentTruncated        bool
	reasonTruncated         bool
	calls                   []retryToolCall
	callsTruncated          bool
}

// clipRetryText preserves line breaks and JSON punctuation. ShrinkString is
// intended for display and escapes backslashes, so it cannot capture arguments.
func clipRetryText(s string, limit int) (string, bool) {
	if limit < 0 {
		limit = 0
	}
	if len(s) <= limit {
		return s, false
	}
	for limit > 0 && !utf8.RuneStart(s[limit]) {
		limit--
	}
	return s[:limit], true
}

type retryTraceWriter struct {
	trace  *retryResponseTrace
	reason bool
}

func (w retryTraceWriter) Write(p []byte) (int, error) {
	w.trace.mu.Lock()
	defer w.trace.mu.Unlock()
	text, truncated := &w.trace.content, &w.trace.contentTruncated
	if w.reason {
		text, truncated = &w.trace.reason, &w.trace.reasonTruncated
	}
	// At most one chunk plus the bounded prefix is allocated per write.
	part, clipped := clipRetryText(string(p), retryResponseLimit-len(*text))
	*text += part
	*truncated = *truncated || clipped
	return len(p), nil
}

func (t *retryResponseTrace) observe(calls []*aispec.ToolCall) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, call := range calls {
		if call == nil {
			continue
		}
		var target *retryToolCall
		for i := range t.calls {
			candidate := &t.calls[i]
			if candidate.Index == call.Index && (candidate.ID == "" || call.ID == "" || candidate.ID == call.ID) {
				target = candidate
				break
			}
		}
		if target == nil {
			if len(t.calls) >= retryToolCallLimit {
				t.callsTruncated = true
				continue
			}
			t.calls = append(t.calls, retryToolCall{Index: call.Index})
			target = &t.calls[len(t.calls)-1]
		}
		if call.ID != "" {
			target.ID, _ = clipRetryText(call.ID, 256)
		}
		if part := call.Function.Name; part != "" && part != target.Name {
			target.Name, _ = clipRetryText(target.Name+part, 256)
		}
		part, clipped := clipRetryText(call.Function.Arguments, retryResponseLimit-len(target.Arguments))
		target.Arguments += part
		target.ArgumentsTruncated = target.ArgumentsTruncated || clipped
	}
}

// Applied last by GetExtraSpecOpts, after each caller has installed its own
// callbacks. Dedicated arguments streaming remains untouched: the delta copy
// is diagnostic only and is never fed into the field parser a second time.
func (t *retryResponseTrace) option() aispec.AIConfigOption {
	return func(cfg *aispec.AIConfig) {
		t.mu.Lock()
		t.native = cfg.ToolCallCallback != nil
		t.resetLocked()
		t.mu.Unlock()
		if previous := cfg.ToolCallCallback; previous != nil {
			cfg.ToolCallCallback = func(calls []*aispec.ToolCall) {
				t.observe(calls)
				previous(calls)
			}
		}
		previousHeader := cfg.RawHTTPResponseHeaderCallback
		cfg.RawHTTPResponseHeaderCallback = func(header []byte) {
			t.mu.Lock()
			t.resetLocked()
			t.mu.Unlock()
			if previousHeader != nil {
				previousHeader(header)
			}
		}
		previousFinish := cfg.FinishReasonCallback
		cfg.FinishReasonCallback = func(reason string, raw []byte) {
			t.mu.Lock()
			t.finish, _ = clipRetryText(reason, 256)
			t.mu.Unlock()
			if previousFinish != nil {
				previousFinish(reason, raw)
			}
		}
		if previous := cfg.StreamHandler; previous != nil {
			cfg.StreamHandler = func(reader io.Reader) {
				previous(io.TeeReader(reader, retryTraceWriter{trace: t}))
			}
		}
		if previous := cfg.ReasonStreamHandler; previous != nil {
			cfg.ReasonStreamHandler = func(reader io.Reader) {
				previous(io.TeeReader(reader, retryTraceWriter{trace: t, reason: true}))
			}
		}
	}
}

func (t *retryResponseTrace) resetLocked() {
	t.finish, t.content, t.reason = "", "", ""
	t.calls = nil
	t.contentTruncated, t.reasonTruncated, t.callsTruncated = false, false, false
}

func (t *retryResponseTrace) fillRecord(rec *transactionAttemptRecord) {
	t.mu.Lock()
	defer t.mu.Unlock()
	rec.Protocol = "text_stream"
	if t.native {
		rec.Protocol = "function_call"
	}
	rec.FinishReason = t.finish
	rec.ToolCalls = append([]retryToolCall(nil), t.calls...)
	rec.ToolCallsTruncated = t.callsTruncated
	if t.content != "" {
		rec.PlainOutput = t.content
	}
	if t.reason != "" {
		rec.PlainReason = t.reason
	}
	rec.PlainOutput, rec.OutputTruncated = clipRetryText(rec.PlainOutput, retryResponseLimit)
	rec.PlainReason, rec.ReasonTruncated = clipRetryText(rec.PlainReason, retryResponseLimit)
	rec.OutputTruncated = rec.OutputTruncated || t.contentTruncated
	rec.ReasonTruncated = rec.ReasonTruncated || t.reasonTruncated
}

func (r transactionAttemptRecord) failedArguments() string {
	var b strings.Builder
	for _, call := range r.ToolCalls {
		b.WriteString(call.Name + ": " + call.Arguments + "\n")
	}
	return b.String()
}
