package aibalance

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aispec"
)

// TestExplicitCache_PassthroughToUpstream 验证经 RewriteMessagesForExplicitCache
// 改写后的 messages, 通过 GetAIClientWithRawMessages -> aispec.ChatBase -> mock
// 上游链路, 上游收到的 HTTP body 里:
//   1. cache_control:{"type":"ephemeral"} 字段实际出现在最末 system 消息的
//      content 数组的最末一项里;
//   2. messages 长度未被拍平;
//   3. 其它消息不含 cache_control 字段(避免误注入到 user/assistant 上).
//
// 注意: 这里 mock 上游用 makePassthroughProvider (type=openai) 起的, 但因为
// RawMessages 是直接逐字 JSON 序列化, 与上游协议无关, 仅用于验证「带
// cache_control 字段的 RawMessages 能被 aispec ChatBase 完整透传到上游 body」
// 这一行为. 真实部署时由 server.go 在 type==tongyi + model 命中白名单时调用
// RewriteMessagesForExplicitCache, 改写后的 messages 通过 dashscope 的
// /compatible-mode/v1/chat/completions 端点被上游识别。
//
// 关键词: dashscope 显式缓存端到端透传, cache_control passthrough,
//        aispec ChatBase RawMessages 序列化保真
func TestExplicitCache_PassthroughToUpstream(t *testing.T) {
	url, get, closeFn := passthroughMockServer(t)
	defer closeFn()
	p := makePassthroughProvider(url)

	original := []aispec.ChatDetail{
		{Role: "system", Content: "you are a strict security research assistant. " +
			strings.Repeat("repeat for prefix stability. ", 32)},
		{Role: "user", Content: "what is SSRF?"},
	}
	// 模拟 server.go 的实际链路: 先 rewrite, 再透传给 GetAIClientWithRawMessages.
	rewritten := RewriteMessagesForExplicitCache(original, "tongyi", "qwen3.6-plus")
	if len(rewritten) != len(original) {
		t.Fatalf("rewrite must keep message count, got %d want %d",
			len(rewritten), len(original))
	}
	if _, ok := rewritten[0].Content.([]*aispec.ChatContent); !ok {
		t.Fatalf("rewrite must convert system content to []*ChatContent, got %T",
			rewritten[0].Content)
	}

	invokeChatViaRawMessages(t, p, rewritten, nil)

	raw, parsed := get()
	if parsed == nil {
		t.Fatalf("upstream did not parse a ChatMessage, raw=%s", string(raw))
	}
	if got := len(parsed.Messages); got != len(rewritten) {
		t.Fatalf("upstream messages len: got %d want %d (raw=%s)",
			got, len(rewritten), string(raw))
	}
	// 最关键断言: 上游 HTTP body 里要能看到 cache_control:{"type":"ephemeral"}
	if !bytes.Contains(raw, []byte(`"cache_control":{"type":"ephemeral"}`)) {
		t.Fatalf("upstream body MUST carry cache_control field, raw=%s", string(raw))
	}
	// user 消息上不能挂 cache_control (避免误注入)
	if cnt := bytes.Count(raw, []byte(`"cache_control"`)); cnt != 1 {
		t.Fatalf("expect exactly 1 cache_control in upstream body, got %d, raw=%s",
			cnt, string(raw))
	}
	// 序列化后 system 的 type=text 也必须出现
	if !bytes.Contains(raw, []byte(`"type":"text"`)) {
		t.Fatalf("upstream body should contain type=text from converted system content, raw=%s",
			string(raw))
	}
}

// TestExplicitCache_PassthroughNotInjected 验证非白名单 model 走同一条
// GetAIClientWithRawMessages 链路时, 上游 HTTP body 完全不含 cache_control 字段.
// 这是保护性回归: 防止未来误把 rewriter 接成默认全注入.
// 关键词: 显式缓存改写 默认不注入回归保护
func TestExplicitCache_PassthroughNotInjected(t *testing.T) {
	url, get, closeFn := passthroughMockServer(t)
	defer closeFn()
	p := makePassthroughProvider(url)

	original := []aispec.ChatDetail{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "u"},
	}
	// 非白名单 (qwen-turbo 走隐式) 不应被改写
	rewritten := RewriteMessagesForExplicitCache(original, "tongyi", "qwen-turbo")
	invokeChatViaRawMessages(t, p, rewritten, nil)

	raw, _ := get()
	if bytes.Contains(raw, []byte("cache_control")) {
		t.Fatalf("non-whitelist model upstream body must NOT contain cache_control, raw=%s",
			string(raw))
	}
}
