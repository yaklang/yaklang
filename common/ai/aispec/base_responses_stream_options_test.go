package aispec

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

// TestChatBase_Responses_NoStreamOptionsInjection verifies that the responses
// path does NOT auto-inject stream_options.include_usage into the outbound
// request body even when streaming with a usage callback registered.
//
// Some responses-compatible gateways (e.g. packyapi codex group) reject
// stream_options as an unknown parameter and return 400
// "Unknown parameter: 'stream_options.include_usage'", which breaks streaming
// entirely and yields empty output on the client.
// 关键词: chatBaseResponses stream_options 不注入, packyapi codex 流式 400 修复
func TestChatBase_Responses_NoStreamOptionsInjection(t *testing.T) {
	var gotRequest []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotRequest = append([]byte(nil), body...)
		// SSE streaming responses body: emit output_text.delta then completed
		// carrying usage (responses protocol carries usage in response.completed,
		// NOT via stream_options.include_usage).
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintln(w, "event: response.created")
		fmt.Fprintln(w, `data: {"type":"response.created","response":{"id":"r1","status":"in_progress"}}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, "event: response.output_text.delta")
		fmt.Fprintln(w, `data: {"type":"response.output_text.delta","delta":"hi","item_id":"m1","output_index":0}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, "event: response.completed")
		fmt.Fprintln(w, `data: {"type":"response.completed","response":{"id":"r1","status":"completed","output_text":"hi","usage":{"input_tokens":3,"output_tokens":1,"total_tokens":4}}}`)
		fmt.Fprintln(w)
	}))
	defer server.Close()

	var gotUsage *ChatUsage
	_, err := ChatBase(
		server.URL,
		"resp-model",
		"ping",
		WithChatBase_InterfaceType(ChatBaseInterfaceTypeResponses),
		// Registering a usage callback used to trigger stream_options injection
		// on the responses path (breaking packyapi streaming). It must no longer.
		WithChatBase_UsageCallback(func(u *ChatUsage) { gotUsage = u }),
		WithChatBase_StreamHandler(func(reader io.Reader) {
			_, _ = io.Copy(io.Discard, reader)
		}),
		WithChatBase_PoCOptions(func() ([]poc.PocConfigOption, error) {
			return nil, nil
		}),
	)
	if err != nil {
		t.Fatalf("chat base failed: %v", err)
	}

	if !bytes.Contains(gotRequest, []byte(`"input"`)) {
		t.Fatalf("expected responses-shaped request (with input), got: %s", string(gotRequest))
	}
	if !bytes.Contains(gotRequest, []byte(`"stream":true`)) {
		t.Fatalf("expected streaming request, got: %s", string(gotRequest))
	}
	if bytes.Contains(gotRequest, []byte(`stream_options`)) {
		t.Fatalf("responses path must NOT inject stream_options, got: %s", string(gotRequest))
	}
	// usage still flows via response.completed (extractLastChatUsageFromPayload),
	// proving stream_options removal does not lose usage on the responses path.
	if gotUsage == nil || gotUsage.TotalTokens == 0 {
		t.Fatalf("expected usage to be captured from response.completed even without stream_options, got: %v", gotUsage)
	}
	if gotUsage.PromptTokens != 3 || gotUsage.CompletionTokens != 1 {
		t.Fatalf("expected responses input_tokens/output_tokens to map to prompt/completion, got: %+v", gotUsage)
	}
}

// TestExtractLastChatUsageFromPayload_ResponsesAliases verifies usage extraction
// handles responses-protocol usage nested under response.usage with
// input_tokens/output_tokens aliases (cached_tokens preserved).
// 关键词: extractLastChatUsageFromPayload, responses usage 别名归一化单测
func TestExtractLastChatUsageFromPayload_ResponsesAliases(t *testing.T) {
	raw := []byte(`{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":4388,"output_tokens":6,"total_tokens":4394,"input_tokens_details":{"cached_tokens":3840}}}}`)
	u := extractLastChatUsageFromPayload(raw)
	if u == nil {
		t.Fatalf("expected usage, got nil")
	}
	if u.PromptTokens != 4388 || u.CompletionTokens != 6 || u.TotalTokens != 4394 {
		t.Fatalf("alias mapping wrong: %+v", u)
	}
	if u.PromptTokensDetails == nil || u.PromptTokensDetails.CachedTokens != 3840 {
		t.Fatalf("cached_tokens not preserved: %+v", u.PromptTokensDetails)
	}
}
