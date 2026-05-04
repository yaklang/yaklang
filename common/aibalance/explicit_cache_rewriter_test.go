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
// 双 cc 注入测试 (§7.7 配套: aicache hijacker 3 段拆分时给 user1 也打 cc)
// ---------------------------------------------------------------------------

// TestRewriteMessages_DualCC_SystemUserUser 验证 [system, user1, user2] 形态下
// system + user1 都被注入 cc, user2 不被注入。这正是 aicache hijacker
// 3 段拆分后送到这里的标准形态。
// 关键词: 双 cc 注入, system + user1, §7.7
func TestRewriteMessages_DualCC_SystemUserUser(t *testing.T) {
	in := []aispec.ChatDetail{
		{Role: "system", Content: "system-prefix"},
		{Role: "user", Content: "frozen-user-1"},
		{Role: "user", Content: "open-user-2"},
	}
	out := RewriteMessagesForExplicitCache(in, "tongyi", "qwen3.6-plus")
	if len(out) != 3 {
		t.Fatalf("len mismatch: got %d want 3", len(out))
	}

	sysContents, ok := out[0].Content.([]*aispec.ChatContent)
	if !ok {
		t.Fatalf("system content type mismatch: %T", out[0].Content)
	}
	if cc, ok := sysContents[0].CacheControl.(map[string]any); !ok || cc["type"] != "ephemeral" {
		t.Fatalf("system cache_control mismatch: %+v", sysContents[0].CacheControl)
	}

	user1Contents, ok := out[1].Content.([]*aispec.ChatContent)
	if !ok {
		t.Fatalf("user1 content should be rewritten to []*ChatContent, got %T", out[1].Content)
	}
	if cc, ok := user1Contents[0].CacheControl.(map[string]any); !ok || cc["type"] != "ephemeral" {
		t.Fatalf("user1 cache_control mismatch: %+v", user1Contents[0].CacheControl)
	}

	if s, ok := out[2].Content.(string); !ok || s != "open-user-2" {
		t.Fatalf("user2 (last) must NOT be rewritten, got %T %v", out[2].Content, out[2].Content)
	}
}

// TestRewriteMessages_DualCC_SerializeBothInJSON 验证最终 JSON 中
// system + user1 都序列化出 cache_control 字段。
// 关键词: 双 cc 注入, JSON 字节断言
func TestRewriteMessages_DualCC_SerializeBothInJSON(t *testing.T) {
	in := []aispec.ChatDetail{
		{Role: "system", Content: "S"},
		{Role: "user", Content: "U1"},
		{Role: "user", Content: "U2"},
	}
	out := RewriteMessagesForExplicitCache(in, "tongyi", "qwen3.6-plus")
	js, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	count := strings.Count(string(js), `"cache_control":{"type":"ephemeral"}`)
	if count != 2 {
		t.Fatalf("expected 2 cache_control occurrences in JSON, got %d, body=%s", count, js)
	}
}

// TestRewriteMessages_DualCC_SingleUserOnlySystemCC 单 user 场景 (旧形态),
// 只注入 system cc, 不强行给唯一的 user 注入 (与现有 TestRewriteMessages_StringContent
// 行为完全一致, 这里再加显式断言避免回归)。
// 关键词: 双 cc 注入, 单 user 退化为单 cc
func TestRewriteMessages_DualCC_SingleUserOnlySystemCC(t *testing.T) {
	in := []aispec.ChatDetail{
		{Role: "system", Content: "S"},
		{Role: "user", Content: "U-only"},
	}
	out := RewriteMessagesForExplicitCache(in, "tongyi", "qwen3.6-plus")
	if _, ok := out[0].Content.([]*aispec.ChatContent); !ok {
		t.Fatalf("system should be rewritten, got %T", out[0].Content)
	}
	if s, ok := out[1].Content.(string); !ok || s != "U-only" {
		t.Fatalf("single user must NOT be rewritten, got %T %v", out[1].Content, out[1].Content)
	}
}

// TestRewriteMessages_DualCC_AssistantBetweenUsers multi-turn 历史:
// [system, user, assistant, user, assistant, user] —— 末位 user 是当前轮,
// 末位之前最近的 user (倒数第 2 个 user) 是上一轮的提问; 该上一轮 user 应
// 被注入 cc (这能让"system + 上一轮 user"作为前缀缓存)。
// 关键词: 双 cc 注入, multi-turn 历史, assistant 间隔
func TestRewriteMessages_DualCC_AssistantBetweenUsers(t *testing.T) {
	in := []aispec.ChatDetail{
		{Role: "system", Content: "S"},
		{Role: "user", Content: "U1"},
		{Role: "assistant", Content: "A1"},
		{Role: "user", Content: "U2"},
		{Role: "assistant", Content: "A2"},
		{Role: "user", Content: "U3-current"},
	}
	out := RewriteMessagesForExplicitCache(in, "tongyi", "qwen3.6-plus")
	// 期望: out[0] (system) 改写, out[3] (U2 = 末位之前最近 user) 改写,
	// 其它 (U1, A1, A2, U3) 全部保持 string 不变
	if _, ok := out[0].Content.([]*aispec.ChatContent); !ok {
		t.Fatalf("system (idx 0) should be rewritten, got %T", out[0].Content)
	}
	if s, ok := out[1].Content.(string); !ok || s != "U1" {
		t.Fatalf("U1 (idx 1) should be untouched, got %T %v", out[1].Content, out[1].Content)
	}
	if s, ok := out[2].Content.(string); !ok || s != "A1" {
		t.Fatalf("A1 (idx 2) should be untouched, got %T %v", out[2].Content, out[2].Content)
	}
	if _, ok := out[3].Content.([]*aispec.ChatContent); !ok {
		t.Fatalf("U2 (idx 3) should be rewritten as penultimate user, got %T", out[3].Content)
	}
	if s, ok := out[4].Content.(string); !ok || s != "A2" {
		t.Fatalf("A2 (idx 4) should be untouched, got %T %v", out[4].Content, out[4].Content)
	}
	if s, ok := out[5].Content.(string); !ok || s != "U3-current" {
		t.Fatalf("U3-current (idx 5, last) should be untouched, got %T %v", out[5].Content, out[5].Content)
	}
}

// TestRewriteMessages_DualCC_NoSystemPassThrough 维持旧契约: 没有 system
// 时即使存在多个 user 也完全不动。
// 关键词: 双 cc 注入, 无 system 绝对 pass-through
func TestRewriteMessages_DualCC_NoSystemPassThrough(t *testing.T) {
	in := []aispec.ChatDetail{
		{Role: "user", Content: "U1"},
		{Role: "user", Content: "U2"},
		{Role: "user", Content: "U3"},
	}
	out := RewriteMessagesForExplicitCache(in, "tongyi", "qwen3.6-plus")
	if !jsonEq(t, in, out) {
		t.Fatalf("no-system multi-user must be untouched, in=%v out=%v", in, out)
	}
}

// TestRewriteMessages_DualCC_DoesNotMutateInput 验证双 cc 注入仍然零副作用。
// 关键词: 双 cc 注入, 零副作用
func TestRewriteMessages_DualCC_DoesNotMutateInput(t *testing.T) {
	in := []aispec.ChatDetail{
		{Role: "system", Content: "S"},
		{Role: "user", Content: "U1"},
		{Role: "user", Content: "U2"},
	}
	snapshot, _ := json.Marshal(in)
	_ = RewriteMessagesForExplicitCache(in, "tongyi", "qwen3.6-plus")
	after, _ := json.Marshal(in)
	if string(snapshot) != string(after) {
		t.Fatalf("input mutated\nbefore=%s\nafter =%s", snapshot, after)
	}
}
