package aiprojection

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

func TestProjectAndObserveMessagesReachChatBaseRequest(t *testing.T) {
	aispec.ResetChatBaseHijackHooksForTest()
	aispec.RegisterChatBaseHijackHook(ProjectAndObserve)
	t.Cleanup(func() {
		aispec.ResetChatBaseHijackHooksForTest()
		aispec.RegisterChatBaseHijackHook(ProjectAndObserve)
	})
	ResetForTest()

	var mu sync.Mutex
	var got *aispec.ChatMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		request := new(aispec.ChatMessage)
		_ = json.Unmarshal(body, request)
		mu.Lock()
		got = request
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()

	prompt := "<|AI_CACHE_SYSTEM_high-static|>stable<|AI_CACHE_SYSTEM_END_high-static|>" +
		"<|PROMPT_SECTION_dynamic_n1|>question<|PROMPT_SECTION_dynamic_END_n1|>"
	_, err := aispec.ChatBase(srv.URL, "test-model", CreateTemplate(prompt),
		aispec.WithChatBase_DisableStream(true),
		aispec.WithChatBase_PoCOptions(func() ([]poc.PocConfigOption, error) { return nil, nil }),
	)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.NotNil(t, got)
	require.Len(t, got.Messages, 2)
	assert.Equal(t, "system", got.Messages[0].Role)
	assert.Contains(t, got.Messages[0].Content, "stable")
	assert.Equal(t, "user", got.Messages[1].Role)
	assert.Contains(t, got.Messages[1].Content, "question")
	assert.Equal(t, int64(1), gCache.totalRequests)
}

func TestProjectAndObserveMessagesReachResponsesRequest(t *testing.T) {
	ResetForTest()
	var mu sync.Mutex
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request map[string]any
		_ = json.Unmarshal(body, &request)
		mu.Lock()
		got = request
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}]}`))
	}))
	defer srv.Close()

	prompt := "<|AI_CACHE_SYSTEM_high-static|>stable<|AI_CACHE_SYSTEM_END_high-static|>" +
		"<|PROMPT_SECTION_dynamic_n1|>question<|PROMPT_SECTION_dynamic_END_n1|>"
	selected := []aispec.Tool{{Type: "function", Function: aispec.ToolFunction{Name: "approved"}}}
	_, err := aispec.ChatBase(srv.URL+"/responses", "test-model", CreateTemplate(prompt),
		aispec.WithChatBase_DisableStream(true),
		aispec.WithChatBase_Tools(selected),
		aispec.WithChatBase_PoCOptions(func() ([]poc.PocConfigOption, error) { return nil, nil }),
	)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.NotNil(t, got)
	input, ok := got["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 2)
	require.Equal(t, "system", input[0].(map[string]any)["role"])
	require.Equal(t, "user", input[1].(map[string]any)["role"])
	tools, ok := got["tools"].([]any)
	require.True(t, ok)
	require.Len(t, tools, 1)
	require.Equal(t, "approved", tools[0].(map[string]any)["name"])
	require.Equal(t, int64(1), gCache.totalRequests)
}

// 关键词: aicache, Observe, 入口冒烟
func TestProjectAndObserve_SmokeWithFourSections(t *testing.T) {
	ResetForTest()

	prompt := buildFourSectionPrompt("nz", "qz", "tools", "static", "tl", "mem")
	ProjectAndObserve("smoke-model", CreateTemplate(prompt))

	// Observe 内部直接同步调 Record，可以立即查
	rep := gCache.Record(Split(prompt), "smoke-model")
	assert.Greater(t, rep.GlobalUniqueChunks, 0)
	assert.GreaterOrEqual(t, rep.TotalRequests, int64(2))
}

// 关键词: aicache, Observe, 空 prompt 静默
func TestProjectAndObserve_EmptyMsgNoop(t *testing.T) {
	ResetForTest()
	ProjectAndObserve("m", "")
	assert.Equal(t, int64(0), gCache.totalRequests, "empty msg should not be recorded")
}

// 关键词: aicache, dispatchChatBaseHijackHooks, 注册联通性
func TestProjectAndObserve_RegisteredOnAispecHijackHook(t *testing.T) {
	// 验证 aiprojection.init() 把自己挂到了 aispec hook 上：
	// 注册一个额外 observer，看到调用即认为联通；同时验证我们的 Observer 也跑了。
	var got atomic.Int64
	var lastModel atomic.Value
	var lastMsg atomic.Value
	aispec.RegisterChatBaseHijackHook(func(model string, msg string) *aispec.ChatBaseHijackResult {
		got.Add(1)
		lastModel.Store(model)
		lastMsg.Store(msg)
		return nil
	})

	ResetForTest()

	prompt := buildFourSectionPrompt("nm", "qm", "tools", "static", "tl", "mem")
	// 模拟 ChatBase 入口分发
	dispatchChatBaseHijackHooksForTest("verify-model", prompt)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if got.Load() > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	assert.GreaterOrEqual(t, got.Load(), int64(1))
	require.Equal(t, int64(1), gCache.totalRequests, "projection observer should be registered once")
	assert.Equal(t, "verify-model", lastModel.Load())
	assert.Equal(t, prompt, lastMsg.Load())
}

// dispatchChatBaseHijackHooksForTest 通过真实 aispec.ChatBase 入口触发 hijack hook
// 这里走 ChatBase 会发起 HTTP，所以转用直接调用 RegisterChatBaseHijackHook 注册的副作用：
// aispec 包内 dispatchChatBaseHijackHooks 是私有函数，等价方式是直接调 ChatBase（指向不可达 URL，模拟立即失败但 dispatch 已经发生）。
// 关键词: aicache, test helper, hijack hook dispatch
func dispatchChatBaseHijackHooksForTest(model, msg string) {
	// ChatBase 第一行就调 dispatchChatBaseHijackHooks(model, msg)，
	// 后续 HTTP 一定会因为 url 空/无效而错误返回，但 hijack hook 已经触发
	_, _ = aispec.ChatBase("http://127.0.0.1:1/__aiprojection_test__", model, msg)
}

// TestProjectAndObserve_HijackPathStillRecords 验证 hijack 通路被触发时（msg 中含
// high-static 段），缓存分析依然完整：
//  1. 全局缓存表的 totalRequests 仍按调用次数递增
//  2. Split 仍记录到完整 4 个 chunk 的 hash
//  3. Observe 同时返回了 hijack 决策 (IsHijacked=true)
//
// 关键词: aicache, Observe, hijack 路径不影响缓存记录
func TestProjectAndObserve_HijackPathStillRecords(t *testing.T) {
	ResetForTest()
	prompt := buildFourSectionPrompt("hpath", "u", "tools", "static-body", "tl", "mem")

	res := ProjectAndObserve("hp-model", CreateTemplate(prompt))
	require.NotNil(t, res, "Observe should return hijack result for prompt with high-static")
	assert.True(t, res.IsHijacked)
	assert.Len(t, res.Messages, 2)

	// 缓存分析路径不受 hijack 影响：4 个 chunk 全部进表
	assert.Equal(t, int64(1), gCache.totalRequests)
	assert.Equal(t, 4, len(gCache.chunks), "all 4 chunks should be recorded in global cache table")

	// 再来一发同样的 prompt：totalRequests==2，chunks 不增（hash 复用）
	res2 := ProjectAndObserve("hp-model", CreateTemplate(prompt))
	require.NotNil(t, res2)
	assert.True(t, res2.IsHijacked)
	assert.Equal(t, int64(2), gCache.totalRequests)
	assert.Equal(t, 4, len(gCache.chunks), "second call should reuse hashes; chunk count unchanged")
}

// TestProjectAndObserve_NoHighStaticStripsNonceAndReturnsCorrelation 没 high-static 时
// 返回仅带 CorrelationID 的 result, 让 ChatBase 仍能
// 把 SeqId 透传到 SSE 末帧 ChatUsage.MirrorCorrelationID, 与 dump 文件名精确 join.
// IsHijacked 必为 false, Messages 必为空, 不影响默认拼装路径.
func TestProjectAndObserve_NoHighStaticStripsNonceAndReturnsCorrelation(t *testing.T) {
	ResetForTest()
	prompt := "<|PROMPT_SECTION_semi-dynamic|>\nsd\n<|PROMPT_SECTION_END_semi-dynamic|>\n\n" +
		"<|PROMPT_SECTION_dynamic_xx|>\nuq\n<|PROMPT_SECTION_dynamic_END_xx|>"

	res := ProjectAndObserve("nh-model", CreateTemplate(prompt))
	require.NotNil(t, res, "hook should still return a correlation ID")
	assert.True(t, res.IsHijacked, "authenticated envelopes must lose their nonce even without cache splitting")
	require.Len(t, res.Messages, 1)
	assert.Equal(t, prompt, res.Messages[0].Content)
	assert.Equal(t, int64(1), gCache.totalRequests, "cache analysis should still record the request")
	assert.NotEmpty(t, res.CorrelationID, "CorrelationID must be set")
	// ID 必须等于本次 Record 的 SeqId 字符串, 让 dump (000XXX.txt 名为 SeqId)
	// 与 cachebench 抓到的 ChatUsage.MirrorCorrelationID 直接对齐.
	assert.Equal(t, strconv.FormatInt(int64(gCache.totalRequests), 10), res.CorrelationID,
		"CorrelationID must equal current seqId")
}

// TestProjectAndObserve_HijackResultCarriesCorrelationID 验证 hijack 路径返回的 result
// 也带 CorrelationID = 当前 seqId 字符串, 让 dump 文件与 ChatUsage 上的
// ID 一一对齐. 同一 prompt 第二次调用时 ID 必须递增, 反映新一次 Record.
func TestProjectAndObserve_HijackResultCarriesCorrelationID(t *testing.T) {
	ResetForTest()
	prompt := buildFourSectionPrompt("seq", "uu", "tools", "static-body", "tl", "mem")

	res1 := ProjectAndObserve("seq-model", CreateTemplate(prompt))
	require.NotNil(t, res1)
	require.True(t, res1.IsHijacked)
	require.NotEmpty(t, res1.CorrelationID)
	id1, err := strconv.ParseInt(res1.CorrelationID, 10, 64)
	require.NoError(t, err, "CorrelationID must be a numeric seqId string")
	assert.Greater(t, id1, int64(0))

	// 同 prompt 再来一发, 缓存 chunks 应复用, 但 SeqId 必递增, 即 ID 不同.
	res2 := ProjectAndObserve("seq-model", CreateTemplate(prompt))
	require.NotNil(t, res2)
	require.NotEmpty(t, res2.CorrelationID)
	id2, err := strconv.ParseInt(res2.CorrelationID, 10, 64)
	require.NoError(t, err)
	assert.Greater(t, id2, id1, "second call must produce strictly larger seqId")
}
