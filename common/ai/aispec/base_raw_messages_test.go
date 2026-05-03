package aispec

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

// rawMessagesMockServer 起一个本地 mock 上游，把最后一次收到的 body 与 ChatMessage
// 解析结果暴露出来，供 RawMessages 系列单测断言使用。
// 关键词: RawMessages 单测 mock, ChatBase 透传断言
func rawMessagesMockServer(t *testing.T) (string, func() ([]byte, *ChatMessage), func()) {
	t.Helper()
	var (
		mu      sync.Mutex
		gotRaw  []byte
		gotChat *ChatMessage
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body failed: %v", err)
			return
		}
		mu.Lock()
		gotRaw = append([]byte(nil), body...)
		parsed := new(ChatMessage)
		if e := json.Unmarshal(body, parsed); e == nil {
			gotChat = parsed
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	get := func() ([]byte, *ChatMessage) {
		mu.Lock()
		defer mu.Unlock()
		return append([]byte(nil), gotRaw...), gotChat
	}
	return srv.URL, get, srv.Close
}

// runChatBaseWithRawMessages 是测试用的 ChatBase 包装：
// 自动叠加 PoCOptions 与 DisableStream，避免每个测试都重复样板。
func runChatBaseWithRawMessages(t *testing.T, url string, msgs []ChatDetail, extra ...ChatBaseOption) {
	t.Helper()
	opts := []ChatBaseOption{
		WithChatBase_DisableStream(true),
		WithChatBase_StreamHandler(func(reader io.Reader) {
			_, _ = io.Copy(io.Discard, reader)
		}),
		WithChatBase_PoCOptions(func() ([]poc.PocConfigOption, error) {
			return nil, nil
		}),
		WithChatBase_RawMessages(msgs),
	}
	opts = append(opts, extra...)
	if _, err := ChatBase(url, "test-model", "ignored-prompt-string", opts...); err != nil {
		t.Fatalf("ChatBase failed: %v", err)
	}
}

// TestChatBase_RawMessagesPriority 验证 RawMessages 非空时，
// 最终发出的 ChatMessage.Messages 与输入完全一致（包括 role / name /
// tool_calls / tool_call_id / content 数组），并且不会被单 user 包装吞掉。
// 关键词: RawMessages 优先, messages 完整透传
func TestChatBase_RawMessagesPriority(t *testing.T) {
	url, get, closeFn := rawMessagesMockServer(t)
	defer closeFn()

	input := []ChatDetail{
		{Role: "system", Content: "you are a helpful assistant"},
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
		{
			Role: "user",
			Content: []*ChatContent{
				NewUserChatContentText("describe this image"),
				NewUserChatContentImageUrl("https://example.com/x.png"),
			},
		},
	}
	runChatBaseWithRawMessages(t, url, input)

	_, parsed := get()
	if parsed == nil {
		t.Fatalf("upstream did not parse a ChatMessage")
	}
	if got := len(parsed.Messages); got != len(input) {
		t.Fatalf("messages length mismatch: got %d want %d", got, len(input))
	}
	for i, want := range input {
		got := parsed.Messages[i]
		if got.Role != want.Role {
			t.Fatalf("messages[%d].role: got %q want %q", i, got.Role, want.Role)
		}
	}
	// 第 4 条 content 必须保持数组结构（含 text + image_url 两项）
	last := parsed.Messages[3]
	contents, ok := last.Content.([]any)
	if !ok {
		t.Fatalf("messages[3].content should remain array, got %T", last.Content)
	}
	if len(contents) != 2 {
		t.Fatalf("messages[3].content len: got %d want 2", len(contents))
	}
}

// TestChatBase_RawMessagesByteStability 验证 serializeRawMessagesForMirror
// 对相同输入产生相同 JSON 字节序列。aicache 等观测者基于字符串 LCP 计算缓存命中
// 率，必须保证字节稳定，否则会出现"逻辑相同实际不命中"的统计噪音。
// 关键词: RawMessages 字节稳定, mirror 序列化稳定性
func TestChatBase_RawMessagesByteStability(t *testing.T) {
	input := []ChatDetail{
		{Role: "system", Content: "stable system prompt"},
		{Role: "user", Content: "first user msg"},
		{Role: "assistant", Content: "first assistant reply"},
		{Role: "user", Content: "second user msg"},
	}
	first := serializeRawMessagesForMirror(input)
	for i := 0; i < 50; i++ {
		again := serializeRawMessagesForMirror(input)
		if again != first {
			t.Fatalf("serialization not stable at iteration %d: %q vs %q", i, again, first)
		}
	}
	if first == "" {
		t.Fatalf("serialization should not be empty for non-empty input")
	}
}

// TestChatBase_RawMessagesIgnoresLegacyImage 验证 RawMessages 模式下，
// 历史 ImageUrls/VideoUrls 选项会被忽略，不会污染最终请求体的 messages。
// 关键词: RawMessages 优先, 旧 image url 忽略
func TestChatBase_RawMessagesIgnoresLegacyImage(t *testing.T) {
	url, get, closeFn := rawMessagesMockServer(t)
	defer closeFn()

	input := []ChatDetail{
		{Role: "user", Content: "raw msg only"},
	}
	runChatBaseWithRawMessages(t, url, input,
		WithChatBase_ImageRawInstance(&ImageDescription{Url: "https://example.com/should-be-ignored.png"}),
	)

	raw, parsed := get()
	if parsed == nil {
		t.Fatalf("upstream did not parse a ChatMessage")
	}
	if len(parsed.Messages) != 1 {
		t.Fatalf("messages should be 1 (RawMessages only), got %d", len(parsed.Messages))
	}
	// 关键断言：上游收到的 body 中不应出现旧 ImageUrls 注入的 url
	if bytes.Contains(raw, []byte("should-be-ignored.png")) {
		t.Fatalf("legacy ImageUrls should be ignored under RawMessages mode, but body contains it: %s", string(raw))
	}
}

// TestChatBase_BackwardCompat 验证 RawMessages 为空时，旧路径不变：
// msg 字符串会被包装为单条 user 消息送上游。
// 关键词: RawMessages 向后兼容, 旧 prompt 路径
func TestChatBase_BackwardCompat(t *testing.T) {
	url, get, closeFn := rawMessagesMockServer(t)
	defer closeFn()

	if _, err := ChatBase(url, "test-model", "legacy prompt", []ChatBaseOption{
		WithChatBase_DisableStream(true),
		WithChatBase_StreamHandler(func(reader io.Reader) { _, _ = io.Copy(io.Discard, reader) }),
		WithChatBase_PoCOptions(func() ([]poc.PocConfigOption, error) { return nil, nil }),
	}...); err != nil {
		t.Fatalf("ChatBase failed: %v", err)
	}

	_, parsed := get()
	if parsed == nil {
		t.Fatalf("upstream did not parse a ChatMessage")
	}
	if len(parsed.Messages) != 1 {
		t.Fatalf("legacy path should produce single user message, got %d", len(parsed.Messages))
	}
	if parsed.Messages[0].Role != "user" {
		t.Fatalf("legacy path role should be user, got %q", parsed.Messages[0].Role)
	}
	gotContent, _ := parsed.Messages[0].Content.(string)
	if gotContent != "legacy prompt" {
		t.Fatalf("legacy path content mismatch: got %q want %q", gotContent, "legacy prompt")
	}
}

// TestChatBase_MirrorReceivesSerializedMessages 验证 RawMessages 模式下，
// 注册的 mirror observer 收到的不再是 prompt 字符串，而是 messages 的稳定
// JSON 序列化结果。aicache 据此计算前缀 LCP 才能与上游 LLM 看到的请求体对齐。
// 关键词: mirror 序列化, RawMessages observer 对齐
func TestChatBase_MirrorReceivesSerializedMessages(t *testing.T) {
	url, _, closeFn := rawMessagesMockServer(t)
	defer closeFn()

	// 注册 observer，捕获 model + msg
	var (
		obsMu  sync.Mutex
		obsMsg string
	)
	RegisterChatBaseMirrorObserver(func(model string, msg string) {
		obsMu.Lock()
		defer obsMu.Unlock()
		if obsMsg == "" { // 取第一条即可
			obsMsg = msg
		}
	})

	input := []ChatDetail{
		{Role: "system", Content: "obs system"},
		{Role: "user", Content: "obs user"},
	}
	runChatBaseWithRawMessages(t, url, input)

	// observer 是 goroutine 异步触发，最多等 1s
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		obsMu.Lock()
		if obsMsg != "" {
			obsMu.Unlock()
			break
		}
		obsMu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}

	obsMu.Lock()
	got := obsMsg
	obsMu.Unlock()
	if got == "" {
		t.Fatalf("mirror observer did not receive any msg")
	}
	expected := serializeRawMessagesForMirror(input)
	if got != expected {
		t.Fatalf("mirror msg mismatch:\n got: %s\nwant: %s", got, expected)
	}
	if got == "ignored-prompt-string" {
		t.Fatalf("mirror should NOT receive the legacy prompt string under RawMessages mode")
	}
}
