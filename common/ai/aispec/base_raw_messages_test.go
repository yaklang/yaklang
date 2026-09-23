package aispec

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

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

// TestChatBase_RawMessagesMergesGatewayImage 验证 RawMessages 非空时，
// gateway 注入的 ImageUrls 仍会合并进最后一条 user 的 content（image_url 部件）。
// 关键词: RawMessages ImageUrls 合并, gateway 多模态
func TestChatBase_RawMessagesMergesGatewayImage(t *testing.T) {
	url, get, closeFn := rawMessagesMockServer(t)
	defer closeFn()

	input := []ChatDetail{
		{Role: "user", Content: "raw msg only"},
	}
	runChatBaseWithRawMessages(t, url, input,
		WithChatBase_ImageRawInstance(&ImageDescription{Url: "https://example.com/merged-from-gateway.png"}),
	)

	raw, parsed := get()
	if parsed == nil {
		t.Fatalf("upstream did not parse a ChatMessage")
	}
	if len(parsed.Messages) != 1 {
		t.Fatalf("messages should be 1, got %d", len(parsed.Messages))
	}
	if !bytes.Contains(raw, []byte("merged-from-gateway.png")) {
		t.Fatalf("expected gateway ImageUrls merged into user content, body=%s", string(raw))
	}
	last := parsed.Messages[0]
	contents, ok := last.Content.([]any)
	if !ok {
		t.Fatalf("user content should become multimodal array, got %T", last.Content)
	}
	if len(contents) < 2 {
		t.Fatalf("expected text + image_url parts, got len=%d", len(contents))
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

func TestLegacyGatewayMediaWire(t *testing.T) {
	for _, test := range []struct {
		name   string
		msg    string
		videos []*VideoDescription
		images []*ImageDescription
		direct bool
		want   string
	}{
		{
			name:   "invalid videos default text",
			videos: []*VideoDescription{nil, {Url: ""}},
			direct: true, // Exercise the legacy branch before option filtering.
			want:   `{"model":"test-model","messages":[{"role":"user","content":[{"type":"text","text":"请描述视频内容"}]}],"stream":false}`,
		},
		{
			name:   "whitespace text",
			msg:    "  ",
			images: []*ImageDescription{{Url: "https://example.com/a.png"}},
			want:   `{"model":"test-model","messages":[{"role":"user","content":[{"type":"text","text":"  "},{"type":"image_url","image_url":{"url":"https://example.com/a.png"}}]}],"stream":false}`,
		},
		{
			name:   "mixed duplicates",
			videos: []*VideoDescription{{Url: "https://example.com/shared"}, {Url: "https://example.com/shared"}, nil},
			images: []*ImageDescription{{Url: "https://example.com/shared"}, {Url: "https://example.com/b.png"}, {Url: "https://example.com/b.png"}},
			want:   `{"model":"test-model","messages":[{"role":"user","content":[{"type":"video_url","video_url":{"url":"https://example.com/shared"}},{"type":"image_url","image_url":{"url":"https://example.com/shared"}},{"type":"image_url","image_url":{"url":"https://example.com/b.png"}},{"type":"text","text":"请描述视频内容"}]}],"stream":false,"modalities":["text"]}`,
		},
		{
			name:   "image only default text",
			images: []*ImageDescription{nil, {Url: ""}, {Url: "https://example.com/a.png"}, {Url: "https://example.com/a.png"}},
			want:   `{"model":"test-model","messages":[{"role":"user","content":[{"type":"text","text":"请描述图片内容"},{"type":"image_url","image_url":{"url":"https://example.com/a.png"}}]}],"stream":false}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			url, get, closeFn := rawMessagesMockServer(t)
			defer closeFn()
			var err error
			if test.direct {
				ctx := NewChatBaseContext(
					WithChatBase_DisableStream(true),
					WithChatBase_PoCOptions(func() ([]poc.PocConfigOption, error) { return nil, nil }),
				)
				ctx.VideoUrls = test.videos
				ctx.ImageUrls = test.images
				_, err = chatBaseChatCompletions(url, "test-model", test.msg, ctx)
			} else {
				_, err = ChatBase(url, "test-model", test.msg,
					WithChatBase_DisableStream(true),
					WithChatBase_PoCOptions(func() ([]poc.PocConfigOption, error) { return nil, nil }),
					WithChatBase_VideoRawInstance(test.videos...),
					WithChatBase_ImageRawInstance(test.images...),
				)
			}
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := get()
			if string(raw) != test.want {
				t.Fatalf("request body changed:\n got: %s\nwant: %s", raw, test.want)
			}
		})
	}
}

// TestChatBase_MirrorReceivesSerializedMessages 验证 RawMessages 模式下，
// 注册的 mirror observer 收到的不再是 prompt 字符串，而是 messages 的稳定
// JSON 序列化结果。aicache 据此计算前缀 LCP 才能与上游 LLM 看到的请求体对齐。
// 关键词: mirror 序列化, RawMessages observer 对齐
func TestChatBase_MirrorReceivesSerializedMessages(t *testing.T) {
	url, _, closeFn := rawMessagesMockServer(t)
	defer closeFn()

	ResetChatBaseMirrorObserversForTest()
	t.Cleanup(ResetChatBaseMirrorObserversForTest)
	var obsMsg string
	RegisterChatBaseMirrorObserver(func(model string, msg string) *ChatBaseMirrorResult {
		if obsMsg == "" { // 取第一条即可
			obsMsg = msg
		}
		return nil
	})

	input := []ChatDetail{
		{Role: "system", Content: "obs system"},
		{Role: "user", Content: "obs user"},
	}
	runChatBaseWithRawMessages(t, url, input)

	got := obsMsg
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
