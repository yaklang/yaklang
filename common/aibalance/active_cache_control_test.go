package aibalance

import (
	"testing"

	"github.com/yaklang/yaklang/common/ai/aispec"
)

// 本文件验证 RewriteMessagesForProviderInstance 的四档语义:
//
//  1. Flag=true + 客户端无 cc -> 不看 model 白名单, 给最末 system 注入 ephemeral
//  2. Flag=true + 客户端自带 cc -> 完全 pass-through, 切片头不变 (零浅复制)
//  3. Flag=false + tongyi + 白名单 model -> 走 legacy RewriteMessagesForExplicitCache 注入
//  4. Flag=false + 非 tongyi + 客户端有 cc -> 走 StripCacheControlFromMessages 全部剥离
//
// 以及一组边界用例:
//  - Flag=true + tongyi + 不在 dashscope 白名单的新 model -> 仍然注入 (验证绕过白名单)
//  - p == nil 防御性退化 -> 原样返回
//
// 关键词: RewriteMessagesForProviderInstance 单测, ActiveCacheControl Flag 四档语义,
//        compat_or always_inject, dashscope 白名单旁路

// TestRewriteMessagesForProviderInstance_FlagOn_NoClientCC_InjectsBaseline
// 验证 Flag=true 且客户端无 cache_control 时, 任意 type/model 组合都给最末
// system 消息注入 ephemeral cache_control. 这里特意用 type=openai + 普通
// gpt-4o model 验证「绕过 dashscope+白名单」.
// 关键词: ActiveCacheControl Flag-on 注入, 跳过 dashscope 白名单
func TestRewriteMessagesForProviderInstance_FlagOn_NoClientCC_InjectsBaseline(t *testing.T) {
	in := []aispec.ChatDetail{
		{Role: "system", Content: "you are a strict assistant"},
		{Role: "user", Content: "hello"},
	}
	p := &Provider{
		TypeName:           "openai",
		ModelName:          "gpt-4o",
		ActiveCacheControl: true,
	}
	out := RewriteMessagesForProviderInstance(in, p)
	if len(out) != len(in) {
		t.Fatalf("len mismatch: got %d want %d", len(out), len(in))
	}
	contents, ok := out[0].Content.([]*aispec.ChatContent)
	if !ok {
		t.Fatalf("system content should become []*ChatContent, got %T", out[0].Content)
	}
	if len(contents) != 1 {
		t.Fatalf("contents len: got %d want 1", len(contents))
	}
	cc, ok := contents[0].CacheControl.(map[string]any)
	if !ok || cc["type"] != "ephemeral" {
		t.Fatalf("CacheControl mismatch: %+v", contents[0].CacheControl)
	}
	// user 消息不应被改写
	if out[1].Content != in[1].Content {
		t.Fatalf("user message must not be modified")
	}
	// 原 messages 切片不被污染: 索引 0 仍指向原 string content
	if _, isString := in[0].Content.(string); !isString {
		t.Fatalf("original system content must remain string, got %T", in[0].Content)
	}
}

// TestRewriteMessagesForProviderInstance_FlagOn_ClientCC_Passthrough
// 验证 Flag=true 但客户端任意位置自带 cache_control 时, 整体 pass-through 退让,
// 不做任何改写. 这是「客户端自管缓存策略」契约的核心: 即使 Flag=on, 也尊重
// 客户端 (例如 aicache hijacker 主动打的双 cc).
// 关键词: ActiveCacheControl Flag-on 自带 cc 退让, pass-through 零浅复制
func TestRewriteMessagesForProviderInstance_FlagOn_ClientCC_Passthrough(t *testing.T) {
	clientContent := []*aispec.ChatContent{
		{
			Type:         "text",
			Text:         "client managed cache",
			CacheControl: map[string]any{"type": "ephemeral"},
		},
	}
	in := []aispec.ChatDetail{
		{Role: "system", Content: clientContent},
		{Role: "user", Content: "hello"},
	}
	p := &Provider{
		TypeName:           "anthropic",
		ModelName:          "claude-sonnet-4",
		ActiveCacheControl: true,
	}
	out := RewriteMessagesForProviderInstance(in, p)
	if len(out) != len(in) {
		t.Fatalf("len mismatch: got %d want %d", len(out), len(in))
	}
	// 切片完全未浅复制 -> 第一项指向原 ChatDetail (Role/Content 两个字段都和原引用一致)
	gotContents, ok := out[0].Content.([]*aispec.ChatContent)
	if !ok {
		t.Fatalf("system content should remain []*ChatContent, got %T", out[0].Content)
	}
	if len(gotContents) != 1 || gotContents[0] != clientContent[0] {
		t.Fatalf("Flag-on + client cc must pass-through original ChatContent pointer (zero copy)")
	}
}

// TestRewriteMessagesForProviderInstance_FlagOff_TongyiWhitelist_LegacyInject
// 验证 Flag=false + tongyi + dashscope 白名单 model (qwen3.6-plus) 时, 仍然走
// 老 RewriteMessagesForExplicitCache 路径注入. 这是 compat_or 兼容契约: 现有
// tongyi 行不需要 portal 上手动勾选 Flag 也能继续工作.
// 关键词: ActiveCacheControl Flag-off compat_or, legacy tongyi 白名单兼容
func TestRewriteMessagesForProviderInstance_FlagOff_TongyiWhitelist_LegacyInject(t *testing.T) {
	in := []aispec.ChatDetail{
		{Role: "system", Content: "you are a strict assistant"},
		{Role: "user", Content: "hello"},
	}
	p := &Provider{
		TypeName:           "tongyi",
		ModelName:          "qwen3.6-plus",
		ActiveCacheControl: false,
	}
	out := RewriteMessagesForProviderInstance(in, p)
	contents, ok := out[0].Content.([]*aispec.ChatContent)
	if !ok {
		t.Fatalf("legacy tongyi whitelist must inject, got %T", out[0].Content)
	}
	cc, ok := contents[0].CacheControl.(map[string]any)
	if !ok || cc["type"] != "ephemeral" {
		t.Fatalf("legacy tongyi whitelist CacheControl mismatch: %+v",
			contents[0].CacheControl)
	}
}

// TestRewriteMessagesForProviderInstance_FlagOff_NonTongyi_Strips
// 验证 Flag=false + 非 tongyi + 客户端自带 cc 时, 走 StripCacheControlFromMessages
// 把 cc 全部剥离 (跨 provider 安全硬约束保留). 这是 compat_or 契约的另一面.
// 关键词: ActiveCacheControl Flag-off 非 tongyi strip, 跨 provider 安全
func TestRewriteMessagesForProviderInstance_FlagOff_NonTongyi_Strips(t *testing.T) {
	in := []aispec.ChatDetail{
		{
			Role: "system",
			Content: []*aispec.ChatContent{
				{
					Type:         "text",
					Text:         "client wants cache",
					CacheControl: map[string]any{"type": "ephemeral"},
				},
			},
		},
		{Role: "user", Content: "hello"},
	}
	p := &Provider{
		TypeName:           "siliconflow",
		ModelName:          "deepseek-chat",
		ActiveCacheControl: false,
	}
	out := RewriteMessagesForProviderInstance(in, p)
	contents, ok := out[0].Content.([]*aispec.ChatContent)
	if !ok {
		t.Fatalf("system content shape should remain []*ChatContent, got %T", out[0].Content)
	}
	if len(contents) != 1 {
		t.Fatalf("contents len: got %d want 1", len(contents))
	}
	if contents[0].CacheControl != nil {
		t.Fatalf("Flag-off + non-tongyi must strip cache_control, got %+v",
			contents[0].CacheControl)
	}
}

// TestRewriteMessagesForProviderInstance_FlagOn_NonWhitelistTongyiModel_Injects
// 验证 Flag=true + tongyi + 不在 dashscope 白名单的新 model (qwen-turbo, 隐式
// 缓存模型, 老路径不会注入) 时, 仍然给最末 system 注入 baseline. 这是 always_inject
// 契约: Flag 一旦打开就跳过白名单 gate, 让运维能给任意 (type, model) 组合启用
// 主动 cache_control。
// 关键词: ActiveCacheControl Flag-on always_inject, 跳过 dashscope 白名单
func TestRewriteMessagesForProviderInstance_FlagOn_NonWhitelistTongyiModel_Injects(t *testing.T) {
	in := []aispec.ChatDetail{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "hi"},
	}
	p := &Provider{
		TypeName:           "tongyi",
		ModelName:          "qwen-turbo", // 不在 dashscopeExplicitCacheModels 白名单内
		ActiveCacheControl: true,
	}
	out := RewriteMessagesForProviderInstance(in, p)
	contents, ok := out[0].Content.([]*aispec.ChatContent)
	if !ok {
		t.Fatalf("Flag-on must inject regardless of dashscope whitelist, got %T",
			out[0].Content)
	}
	cc, ok := contents[0].CacheControl.(map[string]any)
	if !ok || cc["type"] != "ephemeral" {
		t.Fatalf("Flag-on injection CacheControl mismatch: %+v",
			contents[0].CacheControl)
	}
}

// TestRewriteMessagesForProviderInstance_NilProvider_Passthrough
// 验证 p == nil 防御性退化: 直接返回入参切片不做任何改写, 不 panic.
// 关键词: RewriteMessagesForProviderInstance nil 防御
func TestRewriteMessagesForProviderInstance_NilProvider_Passthrough(t *testing.T) {
	in := []aispec.ChatDetail{
		{Role: "system", Content: "system prompt"},
	}
	out := RewriteMessagesForProviderInstance(in, nil)
	if len(out) != len(in) {
		t.Fatalf("nil provider must pass-through, len: got %d want %d", len(out), len(in))
	}
	// content 仍然是原 string, 没有被包装为 []*ChatContent
	if _, isString := out[0].Content.(string); !isString {
		t.Fatalf("nil provider must not modify content, got %T", out[0].Content)
	}
}

// TestRewriteMessagesForProviderInstance_FlagOff_TongyiNonWhitelist_Passthrough
// 验证 Flag=false + tongyi + 非白名单 model (qwen-max, 走隐式缓存) 时,
// 入参切片完全 pass-through 不注入也不 strip (零副作用). 这是 compat_or
// 契约的隐式分支: tongyi 的隐式缓存模型本身不需要 cc 标记。
// 关键词: ActiveCacheControl Flag-off tongyi 非白名单 pass-through
func TestRewriteMessagesForProviderInstance_FlagOff_TongyiNonWhitelist_Passthrough(t *testing.T) {
	in := []aispec.ChatDetail{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "hi"},
	}
	p := &Provider{
		TypeName:           "tongyi",
		ModelName:          "qwen-max", // 隐式缓存白名单, 但不在显式缓存白名单
		ActiveCacheControl: false,
	}
	out := RewriteMessagesForProviderInstance(in, p)
	if len(out) != len(in) {
		t.Fatalf("len mismatch: got %d want %d", len(out), len(in))
	}
	// content 必须保持原 string 形态 (没被注入也没被 strip)
	if got, isString := out[0].Content.(string); !isString || got != "system prompt" {
		t.Fatalf("tongyi non-whitelist + Flag=off must pass-through string content, got %T:%v",
			out[0].Content, out[0].Content)
	}
}

// TestRewriteMessagesForProviderInstance_EmptyMessages_NoOp
// 验证 messages 为空时直接返回 (与老 RewriteMessagesForProvider 行为一致),
// 不论 Flag 与 type/model.
// 关键词: RewriteMessagesForProviderInstance 空切片防御
func TestRewriteMessagesForProviderInstance_EmptyMessages_NoOp(t *testing.T) {
	cases := []*Provider{
		{TypeName: "openai", ModelName: "gpt-4o", ActiveCacheControl: true},
		{TypeName: "tongyi", ModelName: "qwen3.6-plus", ActiveCacheControl: false},
		{TypeName: "anthropic", ModelName: "claude-3", ActiveCacheControl: true},
		nil,
	}
	for _, p := range cases {
		out := RewriteMessagesForProviderInstance(nil, p)
		if len(out) != 0 {
			t.Fatalf("empty input must return empty, got %d for provider=%+v", len(out), p)
		}
	}
}
