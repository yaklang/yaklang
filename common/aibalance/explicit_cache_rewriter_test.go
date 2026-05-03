package aibalance

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aispec"
)

// TestIsTongyiExplicitCacheModel_Whitelist 验证白名单严格限定到 (tongyi, model in 白名单).
// 关键词: dashscope explicit cache 白名单边界
func TestIsTongyiExplicitCacheModel_Whitelist(t *testing.T) {
	cases := []struct {
		name         string
		providerType string
		model        string
		want         bool
	}{
		{"tongyi+qwen3.6-plus", "tongyi", "qwen3.6-plus", true},
		{"tongyi+qwen3.5-flash", "tongyi", "qwen3.5-flash", true},
		{"tongyi+qwen3-vl-flash", "tongyi", "qwen3-vl-flash", true},
		{"tongyi+qwen3-coder-plus", "tongyi", "qwen3-coder-plus", true},
		{"tongyi+kimi-k2.5", "tongyi", "kimi-k2.5", true},
		{"tongyi+deepseek-v3.2", "tongyi", "deepseek-v3.2", true},
		{"tongyi+glm-5.1", "tongyi", "glm-5.1", true},
		{"tongyi+qwen3.5-plus-2026-04-20", "tongyi", "qwen3.5-plus-2026-04-20", true},
		// 大小写 / 前后空格鲁棒
		{"TONGYI+qwen3.6-plus", "TONGYI", "qwen3.6-plus", true},
		{"tongyi+QWEN3-VL-FLASH", "tongyi", "QWEN3-VL-FLASH", true},
		{"tongyi+ qwen-plus ", "tongyi", " qwen-plus ", true},
		// 不在白名单 (隐式缓存模型 / 老型号)
		{"tongyi+qwen-turbo", "tongyi", "qwen-turbo", false},
		{"tongyi+qwen-max", "tongyi", "qwen-max", false},
		{"tongyi+qwen-long", "tongyi", "qwen-long", false},
		{"tongyi+qwen-vl-plus", "tongyi", "qwen-vl-plus", false},
		// 非 tongyi 类型一律 false
		{"openai+qwen3.6-plus", "openai", "qwen3.6-plus", false},
		{"deepseek+kimi-k2.5", "deepseek", "kimi-k2.5", false},
		{"kimi+kimi-k2.5", "kimi", "kimi-k2.5", false},
		// 空值
		{"empty+qwen3.6-plus", "", "qwen3.6-plus", false},
		{"tongyi+empty", "tongyi", "", false},
		{"empty+empty", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsTongyiExplicitCacheModel(c.providerType, c.model); got != c.want {
				t.Fatalf("IsTongyiExplicitCacheModel(%q, %q) = %v, want %v",
					c.providerType, c.model, got, c.want)
			}
		})
	}
}

// TestRewriteMessages_StringContent 验证 system content 是字符串时被改写为
// []*ChatContent 数组形式并附带 cache_control:{"type":"ephemeral"}.
// 关键词: 显式缓存改写 string content
func TestRewriteMessages_StringContent(t *testing.T) {
	in := []aispec.ChatDetail{
		{Role: "system", Content: "you are a strict assistant"},
		{Role: "user", Content: "hello"},
	}
	out := RewriteMessagesForExplicitCache(in, "tongyi", "qwen3.6-plus")
	if len(out) != len(in) {
		t.Fatalf("len mismatch: got %d want %d", len(out), len(in))
	}
	if out[1].Content != in[1].Content {
		t.Fatalf("non-system message must not be touched")
	}
	contents, ok := out[0].Content.([]*aispec.ChatContent)
	if !ok {
		t.Fatalf("system content should become []*ChatContent, got %T", out[0].Content)
	}
	if len(contents) != 1 {
		t.Fatalf("system content len: got %d want 1", len(contents))
	}
	c := contents[0]
	if c.Type != "text" || c.Text != "you are a strict assistant" {
		t.Fatalf("system text mismatch: %+v", c)
	}
	cc, ok := c.CacheControl.(map[string]any)
	if !ok {
		t.Fatalf("CacheControl should be map[string]any, got %T", c.CacheControl)
	}
	if cc["type"] != "ephemeral" {
		t.Fatalf("CacheControl.type should be ephemeral, got %v", cc["type"])
	}
}

// TestRewriteMessages_ChatContentArray 验证 system content 是 []*ChatContent 数组时,
// 浅复制后在最末元素加 cache_control, 原元素不被污染.
// 关键词: 显式缓存改写 []*ChatContent, 不污染原数组
func TestRewriteMessages_ChatContentArray(t *testing.T) {
	c1 := &aispec.ChatContent{Type: "text", Text: "first part"}
	c2 := &aispec.ChatContent{Type: "text", Text: "second part"}
	in := []aispec.ChatDetail{
		{Role: "system", Content: []*aispec.ChatContent{c1, c2}},
		{Role: "user", Content: "hi"},
	}
	out := RewriteMessagesForExplicitCache(in, "tongyi", "qwen3.5-flash")

	contents, ok := out[0].Content.([]*aispec.ChatContent)
	if !ok {
		t.Fatalf("system content should remain []*ChatContent, got %T", out[0].Content)
	}
	if len(contents) != 2 {
		t.Fatalf("contents len: got %d want 2", len(contents))
	}
	if contents[0].CacheControl != nil {
		t.Fatalf("non-last element must not get CacheControl")
	}
	if cc, ok := contents[1].CacheControl.(map[string]any); !ok || cc["type"] != "ephemeral" {
		t.Fatalf("last content CacheControl mismatch: %+v", contents[1].CacheControl)
	}
	// 原元素不被污染
	if c1.CacheControl != nil || c2.CacheControl != nil {
		t.Fatalf("original ChatContent must not be mutated, c1=%v c2=%v",
			c1.CacheControl, c2.CacheControl)
	}
}

// TestRewriteMessages_MapArrayContent 验证 system content 是 []map[string]any 数组时,
// 浅复制后在最末元素加 cache_control 字段, 原 map 不被污染.
// 关键词: 显式缓存改写 []map[string]any, OpenAI 兼容 raw 形态
func TestRewriteMessages_MapArrayContent(t *testing.T) {
	rawMap := map[string]any{"type": "text", "text": "system prompt body"}
	in := []aispec.ChatDetail{
		{Role: "system", Content: []map[string]any{rawMap}},
		{Role: "user", Content: "hi"},
	}
	out := RewriteMessagesForExplicitCache(in, "tongyi", "qwen3-vl-flash")

	arr, ok := out[0].Content.([]map[string]any)
	if !ok {
		t.Fatalf("system content should remain []map[string]any, got %T", out[0].Content)
	}
	if len(arr) != 1 {
		t.Fatalf("len: got %d want 1", len(arr))
	}
	if arr[0]["type"] != "text" || arr[0]["text"] != "system prompt body" {
		t.Fatalf("inner map mismatch: %+v", arr[0])
	}
	cc, ok := arr[0]["cache_control"].(map[string]any)
	if !ok || cc["type"] != "ephemeral" {
		t.Fatalf("cache_control field mismatch: %+v", arr[0]["cache_control"])
	}
	// 原 map 不被污染
	if _, exists := rawMap["cache_control"]; exists {
		t.Fatalf("original raw map must not be mutated")
	}
}

// TestRewriteMessages_AnyArrayContent 验证 system content 是 []any (末项是 map) 时,
// 浅复制后在最末项加 cache_control.
// 关键词: 显式缓存改写 []any 形态
func TestRewriteMessages_AnyArrayContent(t *testing.T) {
	in := []aispec.ChatDetail{
		{Role: "system", Content: []any{
			map[string]any{"type": "text", "text": "p1"},
			map[string]any{"type": "text", "text": "p2"},
		}},
	}
	out := RewriteMessagesForExplicitCache(in, "tongyi", "qwen3.6-plus")
	arr, ok := out[0].Content.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", out[0].Content)
	}
	last, ok := arr[len(arr)-1].(map[string]any)
	if !ok {
		t.Fatalf("last element should be map, got %T", arr[len(arr)-1])
	}
	cc, ok := last["cache_control"].(map[string]any)
	if !ok || cc["type"] != "ephemeral" {
		t.Fatalf("cache_control mismatch: %+v", last["cache_control"])
	}
}

// TestRewriteMessages_NoSystem 验证没有 system 消息时, 原数组完全不被改动.
// 关键词: 显式缓存改写 无 system pass-through
func TestRewriteMessages_NoSystem(t *testing.T) {
	in := []aispec.ChatDetail{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
		{Role: "user", Content: "again"},
	}
	out := RewriteMessagesForExplicitCache(in, "tongyi", "qwen3.6-plus")
	if &in[0] != &out[0] && jsonEq(t, in, out) == false {
		t.Fatalf("messages should be untouched when no system role present, in=%v out=%v", in, out)
	}
}

// TestRewriteMessages_NotInWhitelist 验证非白名单 model (例如 qwen-turbo / qwen-max,
// 它们走隐式缓存) 不会被改写.
// 关键词: 显式缓存改写 非白名单 pass-through
func TestRewriteMessages_NotInWhitelist(t *testing.T) {
	in := []aispec.ChatDetail{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "u"},
	}
	for _, model := range []string{"qwen-turbo", "qwen-max", "qwen-long", "qwen-vl-plus"} {
		out := RewriteMessagesForExplicitCache(in, "tongyi", model)
		if !jsonEq(t, in, out) {
			t.Fatalf("model=%s should NOT be rewritten, in=%v out=%v", model, in, out)
		}
		if s, ok := out[0].Content.(string); !ok || s != "sys" {
			t.Fatalf("model=%s system content was unexpectedly changed: %T %v", model, out[0].Content, out[0].Content)
		}
	}
}

// TestRewriteMessages_NotTongyi 验证 provider type 非 tongyi 时, 即便 model 在白名单
// 内也不改写 (避免给 openai / kimi 等 provider 误注入 dashscope 专属字段).
// 关键词: 显式缓存改写 type 限定
func TestRewriteMessages_NotTongyi(t *testing.T) {
	in := []aispec.ChatDetail{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "u"},
	}
	for _, ptype := range []string{"openai", "deepseek", "kimi", "moonshot", "claude", "ollama"} {
		out := RewriteMessagesForExplicitCache(in, ptype, "qwen3.6-plus")
		if !jsonEq(t, in, out) {
			t.Fatalf("type=%s should NOT trigger rewrite, in=%v out=%v", ptype, in, out)
		}
	}
}

// TestRewriteMessages_PicksLastSystem 验证当存在多条 system 消息时,
// 仅最末一条 system 被改写, 前面的 system 保持原样.
// 关键词: 显式缓存改写 最末 system
func TestRewriteMessages_PicksLastSystem(t *testing.T) {
	in := []aispec.ChatDetail{
		{Role: "system", Content: "sys-A"},
		{Role: "user", Content: "u1"},
		{Role: "system", Content: "sys-B"},
		{Role: "user", Content: "u2"},
	}
	out := RewriteMessagesForExplicitCache(in, "tongyi", "qwen3.6-plus")
	// 第 0 条 system 不动 (仍是 string)
	if s, ok := out[0].Content.(string); !ok || s != "sys-A" {
		t.Fatalf("first system must remain string, got %T %v", out[0].Content, out[0].Content)
	}
	// 第 2 条 system 被改写
	contents, ok := out[2].Content.([]*aispec.ChatContent)
	if !ok {
		t.Fatalf("last system should be rewritten, got %T", out[2].Content)
	}
	if contents[0].Text != "sys-B" {
		t.Fatalf("rewritten content text mismatch: %s", contents[0].Text)
	}
	if cc, ok := contents[0].CacheControl.(map[string]any); !ok || cc["type"] != "ephemeral" {
		t.Fatalf("CacheControl mismatch: %+v", contents[0].CacheControl)
	}
}

// TestRewriteMessages_DoesNotMutateInput 验证改写永不修改入参 messages 切片.
// 关键词: 显式缓存改写 零副作用, 不污染原切片
func TestRewriteMessages_DoesNotMutateInput(t *testing.T) {
	in := []aispec.ChatDetail{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "u"},
	}
	snapshot, _ := json.Marshal(in)

	_ = RewriteMessagesForExplicitCache(in, "tongyi", "qwen3.6-plus")

	after, _ := json.Marshal(in)
	if string(snapshot) != string(after) {
		t.Fatalf("input messages mutated.\nbefore=%s\nafter =%s", snapshot, after)
	}
	// 入参 system content 仍是 string
	if _, ok := in[0].Content.(string); !ok {
		t.Fatalf("input system content type mutated: %T", in[0].Content)
	}
}

// TestRewriteMessages_EmptyOrNil 验证空/nil 入参不 panic.
// 关键词: 显式缓存改写 空数组容错
func TestRewriteMessages_EmptyOrNil(t *testing.T) {
	if got := RewriteMessagesForExplicitCache(nil, "tongyi", "qwen3.6-plus"); got != nil {
		t.Fatalf("nil input should return nil, got %v", got)
	}
	if got := RewriteMessagesForExplicitCache([]aispec.ChatDetail{}, "tongyi", "qwen3.6-plus"); len(got) != 0 {
		t.Fatalf("empty input should stay empty, got %v", got)
	}
}

// TestRewriteMessages_SerializeContainsCacheControl 验证最终序列化的 JSON
// 字节里能看到 cache_control 字段, 这是上游识别的关键. 同时再次确认
// 非白名单 model 的序列化里绝不含 cache_control.
// 关键词: 显式缓存改写 JSON 序列化字节断言
func TestRewriteMessages_SerializeContainsCacheControl(t *testing.T) {
	in := []aispec.ChatDetail{
		{Role: "system", Content: "you are an assistant"},
		{Role: "user", Content: "hi"},
	}

	hit := RewriteMessagesForExplicitCache(in, "tongyi", "qwen3.6-plus")
	hitJSON, err := json.Marshal(hit)
	if err != nil {
		t.Fatalf("marshal hit failed: %v", err)
	}
	if !strings.Contains(string(hitJSON), `"cache_control":{"type":"ephemeral"}`) {
		t.Fatalf("expected cache_control field in JSON, got: %s", hitJSON)
	}

	miss := RewriteMessagesForExplicitCache(in, "tongyi", "qwen-turbo")
	missJSON, err := json.Marshal(miss)
	if err != nil {
		t.Fatalf("marshal miss failed: %v", err)
	}
	if strings.Contains(string(missJSON), "cache_control") {
		t.Fatalf("non-whitelist model JSON must NOT carry cache_control, got: %s", missJSON)
	}
}

// jsonEq 通过 JSON 序列化比较两个值是否结构等价.
func jsonEq(t *testing.T, a, b any) bool {
	t.Helper()
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}
