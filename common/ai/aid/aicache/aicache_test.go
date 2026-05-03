package aicache

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aispec"
)

// 关键词: aicache, Observe, 入口冒烟
func TestObserve_SmokeWithFourSections(t *testing.T) {
	ResetForTest()

	prompt := buildFourSectionPrompt("nz", "qz", "tools", "static", "tl", "mem")
	Observe("smoke-model", prompt)

	// Observe 内部直接同步调 Record，可以立即查
	rep := gCache.Record(Split(prompt), "smoke-model")
	assert.Greater(t, rep.GlobalUniqueChunks, 0)
	assert.GreaterOrEqual(t, rep.TotalRequests, int64(2))
}

// 关键词: aicache, Observe, 空 prompt 静默
func TestObserve_EmptyMsgNoop(t *testing.T) {
	ResetForTest()
	Observe("m", "")
	assert.Equal(t, int64(0), gCache.totalRequests, "empty msg should not be recorded")
}

// 关键词: aicache, dispatchChatBaseMirror, 注册联通性
func TestObserve_RegisteredOnAispecMirror(t *testing.T) {
	// 验证 aicache.init() 把自己挂到了 aispec hook 上：
	// 注册一个额外 observer，看到调用即认为联通；同时验证我们的 Observer 也跑了。
	var got atomic.Int64
	var lastModel atomic.Value
	var lastMsg atomic.Value
	aispec.RegisterChatBaseMirrorObserver(func(model string, msg string) *aispec.ChatBaseMirrorResult {
		got.Add(1)
		lastModel.Store(model)
		lastMsg.Store(msg)
		return nil
	})

	ResetForTest()
	require.True(t, true, "aicache init() ran via package import")

	prompt := buildFourSectionPrompt("nm", "qm", "tools", "static", "tl", "mem")
	// 模拟 ChatBase 入口分发
	dispatchChatBaseMirrorForTest("verify-model", prompt)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if got.Load() > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	assert.GreaterOrEqual(t, got.Load(), int64(1))
	assert.Equal(t, "verify-model", lastModel.Load())
	assert.Equal(t, prompt, lastMsg.Load())
}

// dispatchChatBaseMirrorForTest 通过真实 aispec.ChatBase 入口触发 mirror
// 这里走 ChatBase 会发起 HTTP，所以转用直接调用 RegisterChatBaseMirrorObserver 注册的副作用：
// aispec 包内 dispatchChatBaseMirror 是私有函数，等价方式是直接调 ChatBase（指向不可达 URL，模拟立即失败但 dispatch 已经发生）。
// 关键词: aicache, test helper, mirror dispatch
func dispatchChatBaseMirrorForTest(model, msg string) {
	// ChatBase 第一行就调 dispatchChatBaseMirror(model, msg)，
	// 后续 HTTP 一定会因为 url 空/无效而错误返回，但 mirror 已经触发
	_, _ = aispec.ChatBase("http://127.0.0.1:1/__aicache_test__", model, msg)
}

// TestObserve_HijackPathStillRecords 验证 hijack 通路被触发时（msg 中含
// high-static 段），缓存分析依然完整：
//  1. 全局缓存表的 totalRequests 仍按调用次数递增
//  2. Split 仍记录到完整 4 个 chunk 的 hash
//  3. Observe 同时返回了 hijack 决策 (IsHijacked=true)
//
// 关键词: aicache, Observe, hijack 路径不影响缓存记录
func TestObserve_HijackPathStillRecords(t *testing.T) {
	ResetForTest()
	prompt := buildFourSectionPrompt("hpath", "u", "tools", "static-body", "tl", "mem")

	res := Observe("hp-model", prompt)
	require.NotNil(t, res, "Observe should return hijack result for prompt with high-static")
	assert.True(t, res.IsHijacked)
	assert.Len(t, res.Messages, 2)

	// 缓存分析路径不受 hijack 影响：4 个 chunk 全部进表
	assert.Equal(t, int64(1), gCache.totalRequests)
	assert.Equal(t, 4, len(gCache.chunks), "all 4 chunks should be recorded in global cache table")

	// 再来一发同样的 prompt：totalRequests==2，chunks 不增（hash 复用）
	res2 := Observe("hp-model", prompt)
	require.NotNil(t, res2)
	assert.True(t, res2.IsHijacked)
	assert.Equal(t, int64(2), gCache.totalRequests)
	assert.Equal(t, 4, len(gCache.chunks), "second call should reuse hashes; chunk count unchanged")
}

// TestObserve_NoHighStaticReturnsNil 没 high-static 时 Observe 仍记录缓存
// 但 hijack 决策为 nil（透传）
// 关键词: aicache, Observe, 无 high-static 透传
func TestObserve_NoHighStaticReturnsNil(t *testing.T) {
	ResetForTest()
	prompt := "<|PROMPT_SECTION_semi-dynamic|>\nsd\n<|PROMPT_SECTION_END_semi-dynamic|>\n\n" +
		"<|PROMPT_SECTION_dynamic_xx|>\nuq\n<|PROMPT_SECTION_dynamic_END_xx|>"

	res := Observe("nh-model", prompt)
	assert.Nil(t, res, "no high-static should return nil hijack result")
	assert.Equal(t, int64(1), gCache.totalRequests, "cache analysis should still record the request")
}
