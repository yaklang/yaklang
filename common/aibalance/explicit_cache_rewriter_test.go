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

// ---------------------------------------------------------------------------
// 客户端自带 cc 退让测试 (§7.7.7 职责重排: aibalance 默认只给最末 system 单 cc;
// 客户端任何位置自带 cc 时整体 pass-through, 由客户端自管缓存策略)
// ---------------------------------------------------------------------------

// TestRewriteMessages_RespectClientCC_OnSystem 客户端 system 已自带 cc,
// aibalance 必须完全退让, 一字不动 (返回原切片)。
// 关键词: 客户端自带 cc, 整体 pass-through, system 自带
func TestRewriteMessages_RespectClientCC_OnSystem(t *testing.T) {
	in := []aispec.ChatDetail{
		{Role: "system", Content: []*aispec.ChatContent{
			{Type: "text", Text: "client-cc-system", CacheControl: map[string]any{"type": "ephemeral"}},
		}},
		{Role: "user", Content: "U1"},
	}
	out := RewriteMessagesForExplicitCache(in, "tongyi", "qwen3.6-plus")
	if !jsonEq(t, in, out) {
		t.Fatalf("client-cc on system: aibalance must pass-through, in=%v out=%v", in, out)
	}
	// 同切片返回 (零浅复制)
	if len(out) != len(in) || (len(in) > 0 && &out[0] != &in[0]) {
		t.Fatalf("client-cc pass-through must return same slice header (zero-alloc)")
	}
}

// TestRewriteMessages_RespectClientCC_OnUser 客户端 user 自带 cc (例如
// aicache hijacker 3 段路径下的 user1), aibalance 不再给 system 注入,
// 整体 pass-through 让客户端自管。
// 关键词: 客户端自带 cc, user 自带, system 不再覆盖注入
func TestRewriteMessages_RespectClientCC_OnUser(t *testing.T) {
	in := []aispec.ChatDetail{
		{Role: "system", Content: "S-no-cc"},
		{Role: "user", Content: []*aispec.ChatContent{
			{Type: "text", Text: "client-cc-user", CacheControl: map[string]any{"type": "ephemeral"}},
		}},
		{Role: "user", Content: "U-open"},
	}
	out := RewriteMessagesForExplicitCache(in, "tongyi", "qwen3.6-plus")
	if !jsonEq(t, in, out) {
		t.Fatalf("client-cc on user: aibalance must pass-through, in=%v out=%v", in, out)
	}
	// system 必须仍是原 string (没被注入 cc)
	if s, ok := out[0].Content.(string); !ok || s != "S-no-cc" {
		t.Fatalf("system must remain untouched string when client cc on user, got %T %v",
			out[0].Content, out[0].Content)
	}
}

// TestRewriteMessages_RespectClientCC_DeepInArray cc 在 []*ChatContent 中
// 间元素上 (不是末元素), 也要被识别为"客户端自带 cc"。
// 关键词: 客户端自带 cc, 中间元素 cc 识别
func TestRewriteMessages_RespectClientCC_DeepInArray(t *testing.T) {
	in := []aispec.ChatDetail{
		{Role: "system", Content: []*aispec.ChatContent{
			{Type: "text", Text: "first-no-cc"},
			{Type: "text", Text: "middle-with-cc", CacheControl: map[string]any{"type": "ephemeral"}},
			{Type: "text", Text: "last-no-cc"},
		}},
		{Role: "user", Content: "U"},
	}
	out := RewriteMessagesForExplicitCache(in, "tongyi", "qwen3.6-plus")
	if !jsonEq(t, in, out) {
		t.Fatalf("client-cc deep in array: aibalance must pass-through, in=%v out=%v", in, out)
	}
}

// TestRewriteMessages_RespectClientCC_InMapForm cc 以 map[string]any 形态
// 出现 (例如 OpenAI 兼容 raw 直传), 也要被识别。
// 关键词: 客户端自带 cc, map 形态识别
func TestRewriteMessages_RespectClientCC_InMapForm(t *testing.T) {
	in := []aispec.ChatDetail{
		{Role: "system", Content: []map[string]any{
			{"type": "text", "text": "raw-system", "cache_control": map[string]any{"type": "ephemeral"}},
		}},
		{Role: "user", Content: "U"},
	}
	out := RewriteMessagesForExplicitCache(in, "tongyi", "qwen3.6-plus")
	if !jsonEq(t, in, out) {
		t.Fatalf("client-cc in map form: aibalance must pass-through, in=%v out=%v", in, out)
	}
}

// TestRewriteMessages_RespectClientCC_ReturnsSameSlice 客户端自带 cc 时
// 应当直接返回**原切片头**, 不做任何浅复制 (零分配)。这是性能契约。
// 关键词: 客户端自带 cc, 零浅复制契约, 返回同切片
func TestRewriteMessages_RespectClientCC_ReturnsSameSlice(t *testing.T) {
	in := []aispec.ChatDetail{
		{Role: "system", Content: []*aispec.ChatContent{
			{Type: "text", Text: "S", CacheControl: map[string]any{"type": "ephemeral"}},
		}},
		{Role: "user", Content: "U"},
	}
	out := RewriteMessagesForExplicitCache(in, "tongyi", "qwen3.6-plus")
	// 切片底层数组与长度都必须一致 (即同一切片头)
	if len(out) != len(in) {
		t.Fatalf("len mismatch: got %d want %d", len(out), len(in))
	}
	for i := range in {
		if &out[i] != &in[i] {
			t.Fatalf("client-cc pass-through must return same backing array, idx %d differs", i)
		}
	}
}

// ---------------------------------------------------------------------------
// provider-aware cc 处理测试 (RewriteMessagesForProvider 主入口分发)
// 行为契约:
//   - tongyi          -> 走 RewriteMessagesForExplicitCache (保留 cc + 单 cc 兜底)
//   - 其他所有 provider -> 走 StripCacheControlFromMessages (强制移除所有 cc)
// ---------------------------------------------------------------------------

// TestIsCacheControlAwareProvider 验证 provider 白名单严格只放 tongyi.
// 关键词: provider-aware, IsCacheControlAwareProvider 白名单
func TestIsCacheControlAwareProvider(t *testing.T) {
	cases := []struct {
		name string
		typ  string
		want bool
	}{
		{"tongyi", "tongyi", true},
		{"TONGYI uppercase", "TONGYI", true},
		{"tongyi with space", "  tongyi  ", true},
		{"openai", "openai", false},
		{"siliconflow", "siliconflow", false},
		{"openrouter", "openrouter", false},
		{"anthropic", "anthropic", false},
		{"deepseek", "deepseek", false},
		{"kimi", "kimi", false},
		{"azure", "azure", false},
		{"empty", "", false},
		{"random", "some-provider", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsCacheControlAwareProvider(c.typ); got != c.want {
				t.Fatalf("IsCacheControlAwareProvider(%q) = %v, want %v", c.typ, got, c.want)
			}
		})
	}
}

// TestRewriteForProvider_TongyiKeepsClientCC tongyi provider 路径下,
// 客户端自带 cc 应当被完整保留 (走 explicit cache rewriter pass-through 分支)。
// 关键词: provider-aware, tongyi 保留 cc
func TestRewriteForProvider_TongyiKeepsClientCC(t *testing.T) {
	in := []aispec.ChatDetail{
		{Role: "system", Content: []*aispec.ChatContent{
			{Type: "text", Text: "S", CacheControl: map[string]any{"type": "ephemeral"}},
		}},
		{Role: "user", Content: []*aispec.ChatContent{
			{Type: "text", Text: "U1", CacheControl: map[string]any{"type": "ephemeral"}},
		}},
		{Role: "user", Content: "U2"},
	}
	out := RewriteMessagesForProvider(in, "tongyi", "qwen3.6-plus")
	if !jsonEq(t, in, out) {
		t.Fatalf("tongyi provider should keep client cc verbatim, in=%v out=%v", in, out)
	}
}

// TestRewriteForProvider_TongyiInjectsBaselineCC tongyi provider + 显式缓存
// 白名单 model + 客户端无 cc 时, 应当在最末 system 注入 baseline 单 cc.
// 关键词: provider-aware, tongyi baseline 兜底
func TestRewriteForProvider_TongyiInjectsBaselineCC(t *testing.T) {
	in := []aispec.ChatDetail{
		{Role: "system", Content: "S-no-cc"},
		{Role: "user", Content: "U"},
	}
	out := RewriteMessagesForProvider(in, "tongyi", "qwen3.6-plus")
	contents, ok := out[0].Content.([]*aispec.ChatContent)
	if !ok {
		t.Fatalf("tongyi baseline: system content should be []*ChatContent, got %T", out[0].Content)
	}
	if cc, _ := contents[0].CacheControl.(map[string]any); cc == nil || cc["type"] != "ephemeral" {
		t.Fatalf("tongyi baseline: system should carry ephemeral cc, got %+v", contents[0].CacheControl)
	}
}

// TestRewriteForProvider_SiliconflowStripsAllCC siliconflow provider 路径下,
// 客户端自带的所有位置 cc 必须被强制 strip (兼容性硬约束)。
// 关键词: provider-aware, siliconflow strip cc, 跨 provider 安全
func TestRewriteForProvider_SiliconflowStripsAllCC(t *testing.T) {
	in := []aispec.ChatDetail{
		{Role: "system", Content: []*aispec.ChatContent{
			{Type: "text", Text: "S", CacheControl: map[string]any{"type": "ephemeral"}},
		}},
		{Role: "user", Content: []*aispec.ChatContent{
			{Type: "text", Text: "U1", CacheControl: map[string]any{"type": "ephemeral"}},
		}},
		{Role: "user", Content: "U2"},
	}
	out := RewriteMessagesForProvider(in, "siliconflow", "any-model")

	if cc := out[0].Content.([]*aispec.ChatContent)[0].CacheControl; cc != nil {
		t.Fatalf("siliconflow: system cc must be stripped, got %v", cc)
	}
	if cc := out[1].Content.([]*aispec.ChatContent)[0].CacheControl; cc != nil {
		t.Fatalf("siliconflow: user1 cc must be stripped, got %v", cc)
	}
	if s, ok := out[2].Content.(string); !ok || s != "U2" {
		t.Fatalf("siliconflow: user2 (string) must be unchanged, got %T %v", out[2].Content, out[2].Content)
	}

	// 入参 messages 必须未被原地修改
	if cc := in[0].Content.([]*aispec.ChatContent)[0].CacheControl; cc == nil {
		t.Fatalf("input must not be mutated: original system cc disappeared")
	}
}

// TestRewriteForProvider_OpenAIStripsCC openai provider 路径下也必须 strip
// 所有 cc 字段 (避免被 OpenAI 兼容层因未知字段 400)。
// 关键词: provider-aware, openai strip cc
func TestRewriteForProvider_OpenAIStripsCC(t *testing.T) {
	in := []aispec.ChatDetail{
		{Role: "system", Content: []*aispec.ChatContent{
			{Type: "text", Text: "S", CacheControl: map[string]any{"type": "ephemeral"}},
		}},
		{Role: "user", Content: "U"},
	}
	out := RewriteMessagesForProvider(in, "openai", "gpt-4o")
	if cc := out[0].Content.([]*aispec.ChatContent)[0].CacheControl; cc != nil {
		t.Fatalf("openai: cc must be stripped, got %v", cc)
	}
}

// TestRewriteForProvider_AnthropicStripsMapCC anthropic provider 路径下,
// map 形态的 cc 也必须被剥离 (避免和 anthropic 自家不同语义的 cc 混淆)。
// 关键词: provider-aware, anthropic strip cc, map 形态
func TestRewriteForProvider_AnthropicStripsMapCC(t *testing.T) {
	in := []aispec.ChatDetail{
		{Role: "system", Content: []map[string]any{
			{"type": "text", "text": "S", "cache_control": map[string]any{"type": "ephemeral"}},
		}},
		{Role: "user", Content: "U"},
	}
	out := RewriteMessagesForProvider(in, "anthropic", "claude-3-5-sonnet")
	maps, ok := out[0].Content.([]map[string]any)
	if !ok {
		t.Fatalf("anthropic strip: system content should remain []map[string]any, got %T", out[0].Content)
	}
	if _, hasCC := maps[0]["cache_control"]; hasCC {
		t.Fatalf("anthropic: cache_control key must be removed from map, got %v", maps[0])
	}
	if maps[0]["text"] != "S" {
		t.Fatalf("anthropic: other map fields must be preserved, got %v", maps[0])
	}
}

// TestRewriteForProvider_NoCC_ReturnsSameSlice 任何 provider 在 messages
// 不含 cc 时都应返回同一切片头 (零分配性能契约)。
// 关键词: provider-aware, 零分配契约
func TestRewriteForProvider_NoCC_ReturnsSameSlice(t *testing.T) {
	in := []aispec.ChatDetail{
		{Role: "user", Content: "no system, no cc"},
		{Role: "user", Content: "another user"},
	}
	for _, providerType := range []string{"openai", "siliconflow", "anthropic"} {
		t.Run(providerType, func(t *testing.T) {
			out := RewriteMessagesForProvider(in, providerType, "any")
			if len(out) != len(in) {
				t.Fatalf("len mismatch: got %d want %d", len(out), len(in))
			}
			for i := range in {
				if &out[i] != &in[i] {
					t.Fatalf("no-cc + non-tongyi: must return same slice header, idx %d differs", i)
				}
			}
		})
	}
}

// TestStripCacheControl_DoesNotMutateInput strip 路径必须零副作用 (浅复制).
// 关键词: provider-aware, strip 零副作用
func TestStripCacheControl_DoesNotMutateInput(t *testing.T) {
	in := []aispec.ChatDetail{
		{Role: "system", Content: []*aispec.ChatContent{
			{Type: "text", Text: "S", CacheControl: map[string]any{"type": "ephemeral"}},
		}},
		{Role: "user", Content: "U"},
	}
	snapshot, _ := json.Marshal(in)
	_ = StripCacheControlFromMessages(in)
	after, _ := json.Marshal(in)
	if string(snapshot) != string(after) {
		t.Fatalf("strip mutated input\nbefore=%s\nafter =%s", snapshot, after)
	}
}

// TestStripCacheControl_HijackerOutputBecomesPlain 验证 aicache hijacker
// 双 cc 输出 (system+user1 都带 cc, user2 string) 经 strip 后:
//   - cc 字段全部清空
//   - text 内容完整保留
//   - 切片结构与 message 顺序不变
// 这是"hijacker 一律打 cc + aibalance 跨 provider strip"分工的端到端验证。
// 关键词: provider-aware, hijacker dual cc + non-tongyi strip 端到端
func TestStripCacheControl_HijackerOutputBecomesPlain(t *testing.T) {
	in := []aispec.ChatDetail{
		{Role: "system", Content: []*aispec.ChatContent{
			{Type: "text", Text: "system-text", CacheControl: map[string]any{"type": "ephemeral"}},
		}},
		{Role: "user", Content: []*aispec.ChatContent{
			{Type: "text", Text: "user1-frozen-prefix", CacheControl: map[string]any{"type": "ephemeral"}},
		}},
		{Role: "user", Content: "user2-open-tail"},
	}
	out := RewriteMessagesForProvider(in, "siliconflow", "any-model")

	if len(out) != 3 {
		t.Fatalf("len mismatch: got %d want 3", len(out))
	}
	if cc := out[0].Content.([]*aispec.ChatContent)[0].CacheControl; cc != nil {
		t.Fatalf("siliconflow: system cc must be nil after strip, got %v", cc)
	}
	if txt := out[0].Content.([]*aispec.ChatContent)[0].Text; txt != "system-text" {
		t.Fatalf("system text must be preserved, got %q", txt)
	}
	if cc := out[1].Content.([]*aispec.ChatContent)[0].CacheControl; cc != nil {
		t.Fatalf("siliconflow: user1 cc must be nil after strip, got %v", cc)
	}
	if txt := out[1].Content.([]*aispec.ChatContent)[0].Text; txt != "user1-frozen-prefix" {
		t.Fatalf("user1 text must be preserved, got %q", txt)
	}
	if s, _ := out[2].Content.(string); s != "user2-open-tail" {
		t.Fatalf("user2 string must be unchanged, got %v", out[2].Content)
	}
}

// TestStripCacheControl_NoCCReturnsSameSlice messages 不含 cc 时直接返回原切片头.
// 关键词: provider-aware, strip 零分配
func TestStripCacheControl_NoCCReturnsSameSlice(t *testing.T) {
	in := []aispec.ChatDetail{
		{Role: "system", Content: "S-no-cc"},
		{Role: "user", Content: "U"},
	}
	out := StripCacheControlFromMessages(in)
	if len(out) != len(in) {
		t.Fatalf("len mismatch: got %d want %d", len(out), len(in))
	}
	for i := range in {
		if &out[i] != &in[i] {
			t.Fatalf("no-cc strip must return same slice header, idx %d differs", i)
		}
	}
}

// TestRewriteMessages_FourSegmentClientCCPassThrough 验证 aicache hijacker
// 4 段输出形态 (system+cc / user1+cc / user2+cc / user3 string) 在 tongyi
// 显式缓存路径下能整体 pass-through 不被 cap 在 2 个 cc 或被某条 cc 抹掉.
//
// 这是 P1 双 cache 边界 (frozen + semi) 的端到端契约: 三条带 cc 消息都必须
// 原样到达 dashscope, 由其上游决定命中前 N 个 cache 锚点.
//
// 关键词: 4 段 hijacker 输出, P1 双 cache 边界, 客户端自带 3 cc pass-through,
//        AI_CACHE_FROZEN + AI_CACHE_SEMI
func TestRewriteMessages_FourSegmentClientCCPassThrough(t *testing.T) {
	in := []aispec.ChatDetail{
		{Role: "system", Content: []*aispec.ChatContent{
			{Type: "text", Text: "system-text", CacheControl: map[string]any{"type": "ephemeral"}},
		}},
		{Role: "user", Content: []*aispec.ChatContent{
			{Type: "text", Text: "user1-frozen-prefix", CacheControl: map[string]any{"type": "ephemeral"}},
		}},
		{Role: "user", Content: []*aispec.ChatContent{
			{Type: "text", Text: "user2-semi-prefix", CacheControl: map[string]any{"type": "ephemeral"}},
		}},
		{Role: "user", Content: "user3-open-and-dynamic"},
	}
	out := RewriteMessagesForProvider(in, "tongyi", "qwen3.6-plus")
	if !jsonEq(t, in, out) {
		t.Fatalf("4-segment client cc must be passed through verbatim, in=%v out=%v", in, out)
	}
	// 序列化后必须含 3 个 cache_control 字段
	bs, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if cnt := strings.Count(string(bs), `"cache_control":{"type":"ephemeral"}`); cnt != 3 {
		t.Fatalf("expected 3 cache_control entries in JSON, got %d. body=%s", cnt, bs)
	}
}

// TestRewriteMessages_FourSegmentStripsForNonTongyi 验证 4 段输出在非 tongyi
// provider (如 siliconflow / openai) 下三条 cc 都被强制 strip, text 完整保留.
// 关键词: 4 段 hijacker 输出, 非 tongyi provider strip 全量 cc
func TestRewriteMessages_FourSegmentStripsForNonTongyi(t *testing.T) {
	in := []aispec.ChatDetail{
		{Role: "system", Content: []*aispec.ChatContent{
			{Type: "text", Text: "system-text", CacheControl: map[string]any{"type": "ephemeral"}},
		}},
		{Role: "user", Content: []*aispec.ChatContent{
			{Type: "text", Text: "user1-frozen-prefix", CacheControl: map[string]any{"type": "ephemeral"}},
		}},
		{Role: "user", Content: []*aispec.ChatContent{
			{Type: "text", Text: "user2-semi-prefix", CacheControl: map[string]any{"type": "ephemeral"}},
		}},
		{Role: "user", Content: "user3-open-and-dynamic"},
	}
	out := RewriteMessagesForProvider(in, "siliconflow", "any-model")
	if len(out) != 4 {
		t.Fatalf("len mismatch: got %d want 4", len(out))
	}
	for i := 0; i < 3; i++ {
		arr, ok := out[i].Content.([]*aispec.ChatContent)
		if !ok || len(arr) == 0 {
			t.Fatalf("idx %d: content shape mismatch %T", i, out[i].Content)
		}
		if arr[0].CacheControl != nil {
			t.Fatalf("idx %d: cc must be stripped, got %v", i, arr[0].CacheControl)
		}
	}
	if s, _ := out[3].Content.(string); s != "user3-open-and-dynamic" {
		t.Fatalf("user3 string must be unchanged, got %v", out[3].Content)
	}
}
