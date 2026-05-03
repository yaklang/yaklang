package aibalance

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aispec"
)

// passthroughMockServer 起一个本地 mock 上游 LLM，把每次收到的请求体捕获下来
// 供断言 messages 数组结构是否被原样透传。
// 关键词: aibalance messages 透传单测, mock 上游
func passthroughMockServer(t *testing.T) (string, func() ([]byte, *aispec.ChatMessage), func()) {
	t.Helper()
	var (
		mu      sync.Mutex
		gotRaw  []byte
		gotChat *aispec.ChatMessage
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body failed: %v", err)
			return
		}
		mu.Lock()
		gotRaw = append([]byte(nil), body...)
		parsed := new(aispec.ChatMessage)
		if e := json.Unmarshal(body, parsed); e == nil {
			gotChat = parsed
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	get := func() ([]byte, *aispec.ChatMessage) {
		mu.Lock()
		defer mu.Unlock()
		return append([]byte(nil), gotRaw...), gotChat
	}
	return srv.URL, get, srv.Close
}

// makePassthroughProvider 构造一个指向 mock 上游的 openai 类型 Provider。
// 关键词: aibalance 透传单测 provider 构造
func makePassthroughProvider(url string) *Provider {
	return &Provider{
		ModelName:   "test-model",
		TypeName:    "openai",
		DomainOrURL: url,
		APIKey:      "test-api-key",
		NoHTTPS:     true,
	}
}

// invokeChatViaRawMessages 调 GetAIClientWithRawMessages 并触发一次 Chat("")，
// 等待 mock 上游捕获请求体后返回。
// 关键词: aibalance 透传单测调用入口
func invokeChatViaRawMessages(t *testing.T, p *Provider, msgs []aispec.ChatDetail, tools []aispec.Tool) {
	t.Helper()
	var streamWg sync.WaitGroup
	streamWg.Add(1)
	streamSeen := false
	client, err := p.GetAIClientWithRawMessages(
		msgs,
		tools,
		nil,
		false,
		func(reader io.Reader) {
			defer streamWg.Done()
			_, _ = io.Copy(io.Discard, reader)
			streamSeen = true
		},
		func(reader io.Reader) {
			_, _ = io.Copy(io.Discard, reader)
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("GetAIClientWithRawMessages failed: %v", err)
	}
	if _, err := client.Chat(""); err != nil {
		t.Fatalf("client.Chat failed: %v", err)
	}
	streamWg.Wait()
	_ = streamSeen
}

// TestServeChatCompletions_PreservesMessagesIntegrity 验证经 aibalance 客户端
// 构造路径透传后，mock 上游收到的 messages 数组与客户端发送的 RawMessages 在
// 顺序、role、content 上完全一致；尤其是 image_url content 数组保持完整。
// 关键词: aibalance messages 完整性, image_url 保持
func TestServeChatCompletions_PreservesMessagesIntegrity(t *testing.T) {
	url, get, closeFn := passthroughMockServer(t)
	defer closeFn()
	p := makePassthroughProvider(url)

	input := []aispec.ChatDetail{
		{Role: "system", Content: "you are an assistant"},
		{Role: "user", Content: "hello multi-role"},
		{Role: "assistant", Content: "hi back"},
		{
			Role: "user",
			Content: []*aispec.ChatContent{
				aispec.NewUserChatContentText("describe this picture"),
				aispec.NewUserChatContentImageUrl("https://example.com/photo.jpg"),
			},
		},
	}
	invokeChatViaRawMessages(t, p, input, nil)

	raw, parsed := get()
	if parsed == nil {
		t.Fatalf("upstream did not parse a ChatMessage, raw=%s", string(raw))
	}
	if got := len(parsed.Messages); got != len(input) {
		t.Fatalf("messages length mismatch: got %d want %d (raw=%s)", got, len(input), string(raw))
	}
	for i, want := range input {
		if parsed.Messages[i].Role != want.Role {
			t.Fatalf("messages[%d].role mismatch: got %q want %q", i, parsed.Messages[i].Role, want.Role)
		}
	}
	// image_url content 数组应当保持完整（2 项: text + image_url）
	last := parsed.Messages[3]
	contents, ok := last.Content.([]any)
	if !ok {
		t.Fatalf("messages[3].content should remain array, got %T", last.Content)
	}
	if len(contents) != 2 {
		t.Fatalf("messages[3].content len: got %d want 2 (raw=%s)", len(contents), string(raw))
	}
	// 关键断言：上游 body 中能看到 image_url
	if !bytes.Contains(raw, []byte("https://example.com/photo.jpg")) {
		t.Fatalf("image_url should be present in upstream body, raw=%s", string(raw))
	}
}

// TestServeChatCompletions_NoFlattening 显式断言 mock 上游收到的 messages
// 长度等于客户端发送数组长度，绝不再被拍平为 1。
// 关键词: aibalance 不拍平断言
func TestServeChatCompletions_NoFlattening(t *testing.T) {
	url, get, closeFn := passthroughMockServer(t)
	defer closeFn()
	p := makePassthroughProvider(url)

	input := []aispec.ChatDetail{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "u1"},
		{Role: "assistant", Content: "a1"},
		{Role: "user", Content: "u2"},
		{Role: "assistant", Content: "a2"},
		{Role: "user", Content: "u3"},
	}
	invokeChatViaRawMessages(t, p, input, nil)

	_, parsed := get()
	if parsed == nil {
		t.Fatalf("upstream did not parse a ChatMessage")
	}
	if got := len(parsed.Messages); got != len(input) {
		t.Fatalf("messages must NOT be flattened: got %d want %d", got, len(input))
	}
	if got := len(parsed.Messages); got == 1 {
		t.Fatalf("messages was flattened to single user (regression)")
	}
}

// TestServeChatCompletions_PreservesToolCalls 验证 assistant 的 tool_calls
// 与 tool 的 tool_call_id 字段都能逐字透传。
// 关键词: aibalance tool_calls 透传, tool_call_id 透传
func TestServeChatCompletions_PreservesToolCalls(t *testing.T) {
	url, get, closeFn := passthroughMockServer(t)
	defer closeFn()
	p := makePassthroughProvider(url)

	toolCallID := "call_123"
	input := []aispec.ChatDetail{
		{Role: "user", Content: "what's the weather?"},
		{
			Role:    "assistant",
			Content: "",
			ToolCalls: []*aispec.ToolCall{
				{
					ID:   toolCallID,
					Type: "function",
					Function: aispec.FuncReturn{
						Name:      "get_weather",
						Arguments: `{"city":"Beijing"}`,
					},
				},
			},
		},
		{
			Role:       "tool",
			Name:       "get_weather",
			ToolCallID: toolCallID,
			Content:    `{"temp":22,"unit":"C"}`,
		},
	}
	invokeChatViaRawMessages(t, p, input, nil)

	raw, parsed := get()
	if parsed == nil {
		t.Fatalf("upstream did not parse a ChatMessage, raw=%s", string(raw))
	}
	if len(parsed.Messages) != 3 {
		t.Fatalf("messages len mismatch: got %d want 3", len(parsed.Messages))
	}
	if !bytes.Contains(raw, []byte(`"tool_calls"`)) {
		t.Fatalf("upstream body must carry tool_calls, raw=%s", string(raw))
	}
	if !bytes.Contains(raw, []byte(`"tool_call_id":"`+toolCallID+`"`)) {
		t.Fatalf("upstream body must carry tool_call_id %q, raw=%s", toolCallID, string(raw))
	}
	if !bytes.Contains(raw, []byte(`"name":"get_weather"`)) {
		t.Fatalf("upstream body must carry tool function name, raw=%s", string(raw))
	}
}

// TestServeChatCompletions_AffinityKeyStable 验证相同 messages 输入产生相同
// affinity key（保证亲和性路由稳定到同一上游 provider）。
// 关键词: aibalance affinity key 稳定
func TestServeChatCompletions_AffinityKeyStable(t *testing.T) {
	msgs := []aispec.ChatDetail{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "stable input"},
	}
	first := BuildMessagesAffinityKey(msgs, "key-A", "qwen-max", 2048)
	for i := 0; i < 50; i++ {
		again := BuildMessagesAffinityKey(msgs, "key-A", "qwen-max", 2048)
		if again != first {
			t.Fatalf("affinity key not stable at iteration %d: %q vs %q", i, again, first)
		}
	}

	// 不同 messages -> 不同 key
	other := BuildMessagesAffinityKey([]aispec.ChatDetail{
		{Role: "user", Content: "different content"},
	}, "key-A", "qwen-max", 2048)
	if other == first {
		t.Fatalf("different messages must produce different affinity key, got both %q", first)
	}
}

// TestSerializeMessagesForAffinity_Stable 验证 messages 序列化字节稳定，
// 是 BuildMessagesAffinityKey 稳定的基础。
// 关键词: aibalance serializeMessagesForAffinity 稳定性
func TestSerializeMessagesForAffinity_Stable(t *testing.T) {
	msgs := []aispec.ChatDetail{
		{Role: "system", Content: "stable system"},
		{Role: "user", Content: "stable user"},
		{Role: "assistant", Content: "stable assistant"},
	}
	first := serializeMessagesForAffinity(msgs)
	if first == "" {
		t.Fatalf("serialize result should not be empty for non-empty msgs")
	}
	for i := 0; i < 50; i++ {
		again := serializeMessagesForAffinity(msgs)
		if again != first {
			t.Fatalf("serialize not stable at iteration %d: %q vs %q", i, again, first)
		}
	}
	// 空数组 -> 空串
	if got := serializeMessagesForAffinity(nil); got != "" {
		t.Fatalf("empty msgs should serialize to empty string, got %q", got)
	}
}
