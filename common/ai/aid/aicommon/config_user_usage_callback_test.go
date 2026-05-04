package aicommon

import (
	"context"
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

// drainAIResponse 把 AIChatToAICallbackType 异步 goroutine 的下游 chat 输出
// 读到 EOF, 等价于"等到 chat 函数执行完". 避免测试断言早于 goroutine.
func drainAIResponse(t *testing.T, rsp *AIResponse) {
	t.Helper()
	if rsp == nil {
		return
	}
	reasonReader, outputReader := rsp.GetUnboundStreamReaderEx(nil, nil, nil)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(io.Discard, reasonReader)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(io.Discard, outputReader)
	}()
	wg.Wait()
}

// TestAIChatToAICallbackType_PropagatesUserUsageCallback 验证 P1-D1 修复:
// 当 caller config 注册了 user UsageCallback 时, AIChatToAICallbackType 在调用
// 下游 chat function 之前必须把它通过 aispec.WithUsageCallback 注入到 opts 中,
// 否则 ai.usageCallback(...) 在 React loop 内不会被触发, missing usage 80/130
// 的 bug 仍会复现.
//
// 关键词: AIChatToAICallbackType, ai.usageCallback 透传, P1-D1 回归
func TestAIChatToAICallbackType_PropagatesUserUsageCallback(t *testing.T) {
	cfg := NewTestConfig(context.Background())

	expectedUsage := &aispec.ChatUsage{
		PromptTokens:     12,
		CompletionTokens: 34,
		TotalTokens:      46,
	}
	gotFromUser := make(chan *aispec.ChatUsage, 1)
	cfg.SetUserUsageCallback(func(u *aispec.ChatUsage) {
		gotFromUser <- u
	})

	var sawUsageOpt bool
	chatFn := func(prompt string, opts ...aispec.AIConfigOption) (string, error) {
		ac := aispec.NewDefaultAIConfig()
		for _, opt := range opts {
			opt(ac)
		}
		if ac.UsageCallback != nil {
			sawUsageOpt = true
			ac.UsageCallback(expectedUsage)
		}
		return "ok", nil
	}

	cb := AIChatToAICallbackType(chatFn)
	rsp, err := cb(cfg, NewAIRequest("ping"))
	require.NoError(t, err)
	require.NotNil(t, rsp)
	drainAIResponse(t, rsp)

	require.True(t, sawUsageOpt,
		"AIChatToAICallbackType must propagate WithUsageCallback when caller registered userUsageCallback")
	select {
	case got := <-gotFromUser:
		require.Equal(t, expectedUsage.PromptTokens, got.PromptTokens)
		require.Equal(t, expectedUsage.CompletionTokens, got.CompletionTokens)
		require.Equal(t, expectedUsage.TotalTokens, got.TotalTokens)
	default:
		t.Fatal("user usage callback was not invoked through propagated opts")
	}
}

// TestAIChatToAICallbackType_NoCallbackWhenUserUsageNil 验证当 user 端未注册
// UsageCallback 时, AIChatToAICallbackType 不会泄漏一个 noop callback 到下游
// (避免下游误以为 user 想要 stream_options.include_usage 而额外计费).
//
// 关键词: AIChatToAICallbackType, 无 user usage 时不注入, P1-D1 回归
func TestAIChatToAICallbackType_NoCallbackWhenUserUsageNil(t *testing.T) {
	cfg := NewTestConfig(context.Background())

	var injectedCallback func(*aispec.ChatUsage)
	chatFn := func(prompt string, opts ...aispec.AIConfigOption) (string, error) {
		ac := aispec.NewDefaultAIConfig()
		for _, opt := range opts {
			opt(ac)
		}
		injectedCallback = ac.UsageCallback
		return "ok", nil
	}

	cb := AIChatToAICallbackType(chatFn)
	rsp, err := cb(cfg, NewAIRequest("ping"))
	require.NoError(t, err)
	require.NotNil(t, rsp)
	drainAIResponse(t, rsp)
	require.Nil(t, injectedCallback,
		"AIChatToAICallbackType must NOT inject WithUsageCallback when user did not register one")
}

// TestWithInheritTieredAICallback_InheritsUserUsageCallback 验证 P1-D2 修复:
// 子 Config 通过 WithInheritTieredAICallback 继承父 Config 时, 父注册的
// userUsageCallback 也必须被同步继承, 否则子 coordinator 走 OriginalAICallback
// 路径时 extractUserUsageCallbackOpts 取不到 callback, ai.usageCallback 不触发.
//
// 关键词: WithInheritTieredAICallback, userUsageCallback 继承, P1-D2 回归
func TestWithInheritTieredAICallback_InheritsUserUsageCallback(t *testing.T) {
	parent := NewTestConfig(context.Background())
	parent.SetUserUsageCallback(func(u *aispec.ChatUsage) {})
	require.NotNil(t, parent.GetUserUsageCallback(), "parent must register usage callback")

	child := NewTestConfig(context.Background())
	require.Nil(t, child.GetUserUsageCallback(), "child must start without usage callback")

	require.NoError(t, WithInheritTieredAICallback(parent, true)(child))
	require.NotNil(t, child.GetUserUsageCallback(),
		"child must inherit parent's userUsageCallback after WithInheritTieredAICallback")
}
