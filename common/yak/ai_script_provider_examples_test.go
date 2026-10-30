package yak

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/yakscript"
)

// Execute the shipped source, so edits to the copyable template are covered by
// the same HTTP/VM/output boundary as a saved model configuration.
func TestYakScriptProviderChatCompletionsExample(t *testing.T) {
	source := yakscript.ExampleScript
	const prompt = "完整动作协议和工具结果\n用户问题：6 × 7"
	const answer = `{"@action":"directly_answer","answer_payload":"42，中文"}`
	encode := func(v any) string {
		b, err := json.Marshal(v)
		require.NoError(t, err)
		return string(b)
	}
	jsonAnswer := encode(map[string]any{"choices": []any{map[string]any{
		"message": map[string]any{"content": answer, "reasoning_content": "private reasoning"}, "finish_reason": "stop",
	}}})
	chunk := func(delta any, finish any) string {
		return "data: " + encode(map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": delta, "finish_reason": finish,
		}}}) + "\n\n"
	}
	reason := chunk(map[string]any{"reasoning_content": "private reasoning"}, nil)
	content := chunk(map[string]any{"content": answer[:20]}, nil) + chunk(map[string]any{"content": answer[20:]}, nil)
	stop := chunk(map[string]any{}, "stop")
	done := "data: [DONE]\n\n"
	fullSSE := ": heartbeat\n\n" + reason + content + stop + "data: {\"choices\":[],\"usage\":{\"total_tokens\":10}}\n\n" + done

	for _, tc := range []struct {
		name, mode, contentType, body string
		status                        int
		valid, endpoint               bool
	}{
		{name: "JSON default", contentType: "application/json", body: jsonAnswer, valid: true},
		{name: "JSON explicit endpoint", mode: "json", contentType: "application/json; charset=utf-8", body: jsonAnswer, valid: true, endpoint: true},
		{name: "SSE fragmented UTF8", mode: "sse", contentType: "text/event-stream; charset=utf-8", body: fullSSE, valid: true},
		{name: "SSE multiline data", mode: "sse", contentType: "text/event-stream", body: strings.Replace(fullSSE, `{"choices":[`, "{\"choices\":\ndata: [", 1), valid: true},
		{name: "SSE missing done", mode: "sse", contentType: "text/event-stream", body: reason + content + stop},
		{name: "SSE missing stop", mode: "sse", contentType: "text/event-stream", body: content + done},
		{name: "SSE incomplete last event", mode: "sse", contentType: "text/event-stream", body: content + stop + strings.TrimRight(done, "\n")},
		{name: "SSE reasoning only", mode: "sse", contentType: "text/event-stream", body: reason + stop + done},
		{name: "SSE length cutoff", mode: "sse", contentType: "text/event-stream", body: content + chunk(map[string]any{}, "length") + done},
		{name: "SSE refusal", mode: "sse", contentType: "text/event-stream", body: chunk(map[string]any{"refusal": "private diagnostics"}, "stop") + done},
		{name: "SSE native tool calls", mode: "sse", contentType: "text/event-stream", body: chunk(map[string]any{"tool_calls": []any{map[string]any{"index": 0}}}, nil) + stop + done},
		{name: "SSE nontext content", mode: "sse", contentType: "text/event-stream", body: chunk(map[string]any{"content": 42}, "stop") + done},
		{name: "SSE malformed JSON", mode: "sse", contentType: "text/event-stream", body: content + "data: {broken}\n\n" + done},
		{name: "SSE API error", mode: "sse", contentType: "text/event-stream", body: content + "data: {\"error\":{\"message\":\"private diagnostics\"}}\n\n" + done},
		{name: "SSE unknown event", mode: "sse", contentType: "text/event-stream", body: "event: custom\ndata: {}\n\n" + fullSSE},
		{name: "SSE choice after stop", mode: "sse", contentType: "text/event-stream", body: content + stop + content + done},
		{name: "SSE wrong choice", mode: "sse", contentType: "text/event-stream", body: strings.Replace(fullSSE, `"index":0`, `"index":1`, 1)},
		{name: "JSON length cutoff", mode: "json", contentType: "application/json", body: strings.Replace(jsonAnswer, `"stop"`, `"length"`, 1)},
		{name: "JSON malformed", mode: "json", contentType: "application/json", body: "{broken}"},
		{name: "JSON API error", mode: "json", contentType: "application/json", body: `{"error":{"message":"private diagnostics"}}`},
		{name: "wrong content type", mode: "sse", contentType: "text/html", body: "private diagnostics"},
		{name: "HTTP 429", mode: "sse", status: 429, contentType: "application/json", body: `{"message":"private diagnostics"}`},
		{name: "HTTP 401", mode: "json", status: 401, contentType: "application/json", body: `{"message":"private diagnostics"}`},
		{name: "HTTP redirect", mode: "json", status: 307, contentType: "application/json", body: jsonAnswer},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if tc.name == "JSON default" {
					time.Sleep(20 * time.Millisecond)
				}
				wantPath := "/v1/chat/completions"
				if tc.endpoint {
					wantPath = "/custom-model"
				}
				if r.URL.Path != wantPath || r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer fixture-token" || r.Header.Get("X-Adapter-Test") != "contract" {
					t.Error("incorrect request path, method or headers")
				}
				var body struct {
					Model    string                           `json:"model"`
					Stream   bool                             `json:"stream"`
					Messages []struct{ Role, Content string } `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Model != "upstream-model-id" || body.Stream != (tc.mode == "sse") || len(body.Messages) != 1 || body.Messages[0].Role != "user" || body.Messages[0].Content != prompt {
					t.Error("incorrect model, stream mode or complete prompt mapping")
				}
				w.Header().Set("Content-Type", tc.contentType)
				if tc.status != 0 {
					w.Header().Set("Retry-After", "1")
					w.Header().Set("Location", "/must-not-follow")
					w.WriteHeader(tc.status)
				}
				// Deliberately split JSON and UTF-8 across network writes.
				for i := 0; i < len(tc.body); i += 3 {
					_, _ = io.WriteString(w, tc.body[i:min(i+3, len(tc.body))])
					w.(http.Flusher).Flush()
				}
			}))
			defer server.Close()
			mode := tc.mode
			if mode == "" {
				mode = "json"
			}
			script := strings.Replace(source, `mode = "json"`, fmt.Sprintf("mode = %q", mode), 1)
			endpoint := server.URL + "/v1/chat/completions"
			if tc.endpoint {
				endpoint = server.URL + "/custom-model"
			}
			script = strings.NewReplacer(
				`https://model.example/v1/chat/completions`, endpoint,
				`model = "your-model-id"`, `model = "upstream-model-id"`,
				`apiKey = ""`, `apiKey = "fixture-token"`,
				`headers = {}`, `headers = {"X-Adapter-Test": "contract"}`,
			).Replace(script)
			chat, err := ai.LoadChater(yakscript.ProviderType, aispec.WithScript(script),
				aispec.WithModel("display-label"), aispec.WithBaseURL("https://ignored.invalid"),
				aispec.WithAPIKey("ignored-key"), aispec.WithTimeout(.001))
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var output, thinking strings.Builder
			text, err := chat(prompt, aispec.WithContext(ctx),
				aispec.WithStreamHandler(func(r io.Reader) { _, _ = io.Copy(&output, r) }),
				aispec.WithReasonStreamHandler(func(r io.Reader) { _, _ = io.Copy(&thinking, r) }))
			require.EqualValues(t, 1, requests.Load(), "one network attempt; retry policy is disabled in this fixture")
			require.Empty(t, thinking.String(), "the current script ABI has no reason emitter")
			if tc.valid {
				require.NoError(t, err)
				require.Equal(t, answer, text)
				require.Equal(t, answer, output.String())
			} else {
				require.Error(t, err)
				require.Empty(t, text)
				require.Empty(t, output.String(), "unvalidated or partial responses must not be emitted")
				require.NotContains(t, err.Error(), "private diagnostics")
			}
		})
	}
}
