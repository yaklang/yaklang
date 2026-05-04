package aicommon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

// TestUserUsageCallback_CtxBased_RoundTrip 验证 ctx 注入 / 取出 是否互逆.
//
// 关键词: WithUserUsageCallbackContext, GetUserUsageCallbackFromContext, P3-T5
func TestUserUsageCallback_CtxBased_RoundTrip(t *testing.T) {
	cb := func(*aispec.ChatUsage) {}

	ctx := WithUserUsageCallbackContext(context.Background(), cb)
	got := GetUserUsageCallbackFromContext(ctx)
	require.NotNil(t, got, "round-trip must return the callback")

	var nilCtx context.Context
	require.Nil(t, GetUserUsageCallbackFromContext(nilCtx), "nil ctx must yield nil")
	require.Nil(t, GetUserUsageCallbackFromContext(context.Background()),
		"empty ctx must yield nil")

	derived, cancel := context.WithCancel(ctx)
	defer cancel()
	require.NotNil(t, GetUserUsageCallbackFromContext(derived),
		"derived ctx must inherit user usage callback value")
}

// TestConfig_GetContext_InjectsUserUsageCallback 验证 Config.GetContext 在
// 注册了 userUsageCallback 后, 自动把 callback 注入返回的 ctx 中, 让所有
// 通过 cfg.GetContext() 派生的子调用都能拿到.
//
// 关键词: GetContext, ctx 透传 user usage callback, P3-T5
func TestConfig_GetContext_InjectsUserUsageCallback(t *testing.T) {
	cfg := NewTestConfig(context.Background())

	require.Nil(t, GetUserUsageCallbackFromContext(cfg.GetContext()),
		"before SetUserUsageCallback, ctx must NOT carry callback")

	cfg.SetUserUsageCallback(func(u *aispec.ChatUsage) {})
	require.NotNil(t, GetUserUsageCallbackFromContext(cfg.GetContext()),
		"after SetUserUsageCallback, GetContext must inject callback into ctx")
}

// TestExtractUserUsageCallbackOpts_FallbacksToContext 验证当子 Config 自身
// 没有 userUsageCallback 但其 ctx 中携带 callback 时, extractUserUsageCallbackOpts
// 仍然能取到并产出 aispec.WithUsageCallback 选项. 修复 enhancesearch HyDE 等
// 子 LiteForge 调用走 MustGetSpeedPriorityAIModelCallback 时 user callback 漏接的 BUG.
//
// 关键词: extractUserUsageCallbackOpts ctx fallback, enhancesearch usage 透传, P3-T5
func TestExtractUserUsageCallbackOpts_FallbacksToContext(t *testing.T) {
	called := false
	cb := func(*aispec.ChatUsage) { called = true }

	ctx := WithUserUsageCallbackContext(context.Background(), cb)
	child := NewTestConfig(ctx)
	require.Nil(t, child.GetUserUsageCallback(),
		"child must NOT have userUsageCallback set directly (only via ctx)")

	opts := extractUserUsageCallbackOpts(child)
	require.Len(t, opts, 1,
		"extractUserUsageCallbackOpts must fallback to ctx-based callback when cfg.userUsageCallback is nil")

	cfg := aispec.NewDefaultAIConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	require.NotNil(t, cfg.UsageCallback,
		"the generated option must set UsageCallback on aispec.AIConfig")

	cfg.UsageCallback(&aispec.ChatUsage{PromptTokens: 1})
	require.True(t, called, "ctx-based callback must be the same one invoked")
}

// TestExtractUserUsageCallbackOpts_PrefersCfgOverContext 验证当 cfg 自身
// 已注册 userUsageCallback 时, extractUserUsageCallbackOpts 优先使用 cfg 上的,
// 不被 ctx 上的 callback 覆盖 (避免父子混用导致重复触发).
//
// 关键词: extractUserUsageCallbackOpts cfg-priority, P3-T5
func TestExtractUserUsageCallbackOpts_PrefersCfgOverContext(t *testing.T) {
	cfgCalled := false
	ctxCalled := false
	cfgCb := func(*aispec.ChatUsage) { cfgCalled = true }
	ctxCb := func(*aispec.ChatUsage) { ctxCalled = true }

	ctx := WithUserUsageCallbackContext(context.Background(), ctxCb)
	cfg := NewTestConfig(ctx)
	cfg.SetUserUsageCallback(cfgCb)

	opts := extractUserUsageCallbackOpts(cfg)
	require.Len(t, opts, 1)
	ac := aispec.NewDefaultAIConfig()
	for _, opt := range opts {
		opt(ac)
	}
	require.NotNil(t, ac.UsageCallback)
	ac.UsageCallback(&aispec.ChatUsage{PromptTokens: 1})

	require.True(t, cfgCalled, "must use cfg.userUsageCallback when both present")
	require.False(t, ctxCalled, "must NOT use ctx-based callback when cfg has its own")
}
