package aispec

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

// TestMirror_DispatchSync 验证 dispatchChatBaseMirror 是同步的：
// 调用返回时所有 observer 一定已经执行完毕。
// 关键词: aispec, mirror dispatch sync
func TestMirror_DispatchSync(t *testing.T) {
	ResetChatBaseMirrorObserversForTest()
	t.Cleanup(ResetChatBaseMirrorObserversForTest)

	var got atomic.Int64
	RegisterChatBaseMirrorObserver(func(model, msg string) *ChatBaseMirrorResult {
		got.Add(1)
		return nil
	})
	res := dispatchChatBaseMirror("m", "hi")
	assert.Nil(t, res, "no hijack observer registered → result should be nil")
	assert.Equal(t, int64(1), got.Load(), "synchronous dispatch must finish observer before returning")
}

// TestMirror_HijackResultPriority 多个 observer 都跑，最后一个 IsHijacked==true 胜出
// 关键词: aispec, mirror hijack 优先级
func TestMirror_HijackResultPriority(t *testing.T) {
	ResetChatBaseMirrorObserversForTest()
	t.Cleanup(ResetChatBaseMirrorObserversForTest)

	RegisterChatBaseMirrorObserver(func(model, msg string) *ChatBaseMirrorResult {
		return &ChatBaseMirrorResult{
			IsHijacked: true,
			Messages: []ChatDetail{
				{Role: "system", Content: "first hijack"},
				{Role: "user", Content: "u"},
			},
		}
	})
	RegisterChatBaseMirrorObserver(func(model, msg string) *ChatBaseMirrorResult {
		return nil // 第二个 observer 仅观测，不应覆盖第一个的 hijack
	})
	res := dispatchChatBaseMirror("m", "hi")
	require.NotNil(t, res)
	require.True(t, res.IsHijacked)
	require.Len(t, res.Messages, 2)
	assert.Equal(t, "first hijack", res.Messages[0].Content)

	// 现在再注册一个 IsHijacked=true 的 observer，它应当胜出（取最后一个）
	RegisterChatBaseMirrorObserver(func(model, msg string) *ChatBaseMirrorResult {
		return &ChatBaseMirrorResult{
			IsHijacked: true,
			Messages: []ChatDetail{
				{Role: "system", Content: "third hijack"},
				{Role: "user", Content: "u"},
			},
		}
	})
	res2 := dispatchChatBaseMirror("m", "hi")
	require.NotNil(t, res2)
	assert.Equal(t, "third hijack", res2.Messages[0].Content,
		"last IsHijacked=true observer should win")
}

// TestMirror_PanicIsolation observer panic 不影响其他 observer 与主流程
// 关键词: aispec, mirror panic 隔离
func TestMirror_PanicIsolation(t *testing.T) {
	ResetChatBaseMirrorObserversForTest()
	t.Cleanup(ResetChatBaseMirrorObserversForTest)

	var ranAfterPanic atomic.Int64
	RegisterChatBaseMirrorObserver(func(model, msg string) *ChatBaseMirrorResult {
		panic("boom")
	})
	RegisterChatBaseMirrorObserver(func(model, msg string) *ChatBaseMirrorResult {
		ranAfterPanic.Add(1)
		return &ChatBaseMirrorResult{
			IsHijacked: true,
			Messages: []ChatDetail{
				{Role: "system", Content: "S"},
				{Role: "user", Content: "U"},
			},
		}
	})

	res := dispatchChatBaseMirror("m", "hi")
	assert.Equal(t, int64(1), ranAfterPanic.Load(), "panic observer should not block siblings")
	require.NotNil(t, res)
	assert.True(t, res.IsHijacked)
}

// TestMirror_NoObserverReturnsNil 未注册 observer 时返回 nil
// 关键词: aispec, mirror 无注册返回 nil
func TestMirror_NoObserverReturnsNil(t *testing.T) {
	ResetChatBaseMirrorObserversForTest()
	t.Cleanup(ResetChatBaseMirrorObserversForTest)
	assert.Nil(t, dispatchChatBaseMirror("m", "hi"))
}

// TestMirror_NilCallbackIgnored 注册 nil observer 不应 panic 也不应进入 list
// 关键词: aispec, mirror nil 注册忽略
func TestMirror_NilCallbackIgnored(t *testing.T) {
	ResetChatBaseMirrorObserversForTest()
	t.Cleanup(ResetChatBaseMirrorObserversForTest)
	RegisterChatBaseMirrorObserver(nil)
	assert.Nil(t, dispatchChatBaseMirror("m", "hi"))
}

// TestChatBase_HijackReplacesMessages 端到端验证 hijack 路径：
// observer 返回 IsHijacked=true 时，ChatBase 把 Messages 灌入 ctx.RawMessages
// 让上游收到 [system, user] 而不是默认单 user。
// 关键词: aispec, ChatBase hijack 端到端
func TestChatBase_HijackReplacesMessages(t *testing.T) {
	ResetChatBaseMirrorObserversForTest()
	t.Cleanup(ResetChatBaseMirrorObserversForTest)

	// mock 上游记录请求体
	var (
		mu     sync.Mutex
		gotMsg *ChatMessage
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		parsed := new(ChatMessage)
		_ = json.Unmarshal(body, parsed)
		mu.Lock()
		gotMsg = parsed
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()

	RegisterChatBaseMirrorObserver(func(model, msg string) *ChatBaseMirrorResult {
		return &ChatBaseMirrorResult{
			IsHijacked: true,
			Messages: []ChatDetail{
				{Role: "system", Content: "hijacked-system"},
				{Role: "user", Content: "hijacked-user"},
			},
		}
	})

	_, err := ChatBase(srv.URL, "test-model", "original-prompt-string",
		WithChatBase_DisableStream(true),
		WithChatBase_StreamHandler(func(reader io.Reader) {
			_, _ = io.Copy(io.Discard, reader)
		}),
		WithChatBase_PoCOptions(func() ([]poc.PocConfigOption, error) { return nil, nil }),
	)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.NotNil(t, gotMsg)
	require.Len(t, gotMsg.Messages, 2)
	assert.Equal(t, "system", gotMsg.Messages[0].Role)
	assert.Equal(t, "hijacked-system", gotMsg.Messages[0].Content)
	assert.Equal(t, "user", gotMsg.Messages[1].Role)
	assert.Equal(t, "hijacked-user", gotMsg.Messages[1].Content)
}

// TestChatBase_PureObserveDoesNotRewrite IsHijacked=false 等价于纯观测，
// 上游收到默认拼装的单 user 消息。
// 关键词: aispec, ChatBase 纯观测不改写
func TestChatBase_PureObserveDoesNotRewrite(t *testing.T) {
	ResetChatBaseMirrorObserversForTest()
	t.Cleanup(ResetChatBaseMirrorObserversForTest)

	var (
		mu     sync.Mutex
		gotMsg *ChatMessage
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		parsed := new(ChatMessage)
		_ = json.Unmarshal(body, parsed)
		mu.Lock()
		gotMsg = parsed
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()

	var seenMsg atomic.Value
	RegisterChatBaseMirrorObserver(func(model, msg string) *ChatBaseMirrorResult {
		seenMsg.Store(msg)
		return nil // 纯观测
	})

	_, err := ChatBase(srv.URL, "test-model", "original-prompt-string",
		WithChatBase_DisableStream(true),
		WithChatBase_StreamHandler(func(reader io.Reader) { _, _ = io.Copy(io.Discard, reader) }),
		WithChatBase_PoCOptions(func() ([]poc.PocConfigOption, error) { return nil, nil }),
	)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.NotNil(t, gotMsg)
	require.Len(t, gotMsg.Messages, 1)
	assert.Equal(t, "user", gotMsg.Messages[0].Role)
	assert.Equal(t, "original-prompt-string", gotMsg.Messages[0].Content)

	// observer 也确实看到了原 prompt
	assert.Equal(t, "original-prompt-string", seenMsg.Load())
}

// TestChatBase_HijackSkippedWhenRawMessagesPresent caller 显式 RawMessages
// 时 hijack 必须自动跳过，尊重 caller 的 messages。
// 关键词: aispec, ChatBase RawMessages 优先于 hijack
func TestChatBase_HijackSkippedWhenRawMessagesPresent(t *testing.T) {
	ResetChatBaseMirrorObserversForTest()
	t.Cleanup(ResetChatBaseMirrorObserversForTest)

	var (
		mu     sync.Mutex
		gotMsg *ChatMessage
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		parsed := new(ChatMessage)
		_ = json.Unmarshal(body, parsed)
		mu.Lock()
		gotMsg = parsed
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()

	// observer 想 hijack 成另一套 messages
	RegisterChatBaseMirrorObserver(func(model, msg string) *ChatBaseMirrorResult {
		return &ChatBaseMirrorResult{
			IsHijacked: true,
			Messages: []ChatDetail{
				{Role: "system", Content: "should-be-ignored"},
			},
		}
	})

	// caller 已显式给 RawMessages，应该尊重它
	caller := []ChatDetail{
		{Role: "system", Content: "caller-system"},
		{Role: "user", Content: "caller-user"},
	}
	_, err := ChatBase(srv.URL, "test-model", "irrelevant-prompt",
		WithChatBase_DisableStream(true),
		WithChatBase_StreamHandler(func(reader io.Reader) { _, _ = io.Copy(io.Discard, reader) }),
		WithChatBase_PoCOptions(func() ([]poc.PocConfigOption, error) { return nil, nil }),
		WithChatBase_RawMessages(caller),
	)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.NotNil(t, gotMsg)
	require.Len(t, gotMsg.Messages, 2)
	assert.Equal(t, "caller-system", gotMsg.Messages[0].Content,
		"hijack must not overwrite caller-supplied RawMessages")
	assert.Equal(t, "caller-user", gotMsg.Messages[1].Content)
}

// TestConvertChatDetailsToResponsesInput 字符串/数组/未知类型都能正确映射
// 关键词: aispec, convertChatDetailsToResponsesInput
func TestConvertChatDetailsToResponsesInput(t *testing.T) {
	msgs := []ChatDetail{
		{Role: "system", Content: "sys text"},
		{Role: "user", Content: []*ChatContent{
			NewUserChatContentText("text part"),
			NewUserChatContentImageUrl("https://example.com/x.png"),
		}},
		{Role: "assistant", Content: 12345}, // 未知类型 → 走 InterfaceToString
	}
	out := convertChatDetailsToResponsesInput(msgs)
	require.Len(t, out, 3)

	// 第 1 条：string content → input_text 单元素数组
	assert.Equal(t, "system", out[0]["role"])
	c0 := out[0]["content"].([]map[string]any)
	require.Len(t, c0, 1)
	assert.Equal(t, "input_text", c0[0]["type"])
	assert.Equal(t, "sys text", c0[0]["text"])

	// 第 2 条：[]*ChatContent → input_text + input_image
	assert.Equal(t, "user", out[1]["role"])
	c1 := out[1]["content"].([]map[string]any)
	require.Len(t, c1, 2)
	assert.Equal(t, "input_text", c1[0]["type"])
	assert.Equal(t, "input_image", c1[1]["type"])

	// 第 3 条：未知类型 → InterfaceToString 兜底
	assert.Equal(t, "assistant", out[2]["role"])
	c2 := out[2]["content"].([]map[string]any)
	require.Len(t, c2, 1)
	assert.Equal(t, "input_text", c2[0]["type"])
	assert.Equal(t, "12345", c2[0]["text"])
}
