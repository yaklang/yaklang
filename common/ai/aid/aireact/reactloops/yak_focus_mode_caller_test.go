package reactloops

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// 验证基本的 dunder 常量与 focusXxx 钩子能从 yak 脚本中被正确读取与调用。
// 关键词: yak focus mode caller test, dunder extraction, hook invocation
func TestFocusModeCaller_ExtractAndCall(t *testing.T) {
	code := `
__VERBOSE_NAME__ = "TestCaller"
__MAX_ITERATIONS__ = 7
__ALLOW_RAG__ = true
__VARS__ = {"counter": 1}

focusInitTask = func(loop, task, op) {
    return "init-called"
}
`
	caller, err := NewFocusModeYakHookCaller("test_caller.ai-focus.yak", code,
		WithFocusModeCallerCallTimeout(2*time.Second))
	require.NoError(t, err)
	defer caller.Close()

	require.Equal(t, "TestCaller", caller.GetString(FocusDunder_VerboseName))

	maxIter, ok := caller.GetInt(FocusDunder_MaxIterations)
	require.True(t, ok)
	require.Equal(t, 7, maxIter)

	allowRAG, ok := caller.GetBool(FocusDunder_AllowRAG)
	require.True(t, ok)
	require.True(t, allowRAG)

	vars := caller.GetMap(FocusDunder_Vars)
	require.NotNil(t, vars)
	require.Contains(t, vars, "counter")

	require.True(t, caller.HasHook(FocusHook_InitTask))

	val, err := caller.CallByName(FocusHook_InitTask, nil, nil, nil)
	require.NoError(t, err)
	require.Equal(t, "init-called", val)
}

// 验证 hook 不存在时返回特定 sentinel 错误。
// 关键词: yak focus mode caller test, hook not found sentinel
func TestFocusModeCaller_HookNotFound(t *testing.T) {
	caller, err := NewFocusModeYakHookCaller("nohook.ai-focus.yak", `__VERBOSE_NAME__ = "noop"`)
	require.NoError(t, err)
	defer caller.Close()

	_, err = caller.CallByName("focusDoesNotExist", nil)
	require.Error(t, err)
	require.True(t, IsFocusModeHookNotFound(err), "expect sentinel hook not found, got %v", err)

	val, err := caller.CallByNameIgnoreNotFound("focusDoesNotExist", nil)
	require.NoError(t, err)
	require.Nil(t, val)
}

// 验证 hook 内 panic 不会向上传播。
// 关键词: yak focus mode caller test, panic recovery
func TestFocusModeCaller_PanicRecover(t *testing.T) {
	code := `
focusPanic = func() {
    panic("boom")
}
`
	caller, err := NewFocusModeYakHookCaller("panic.ai-focus.yak", code,
		WithFocusModeCallerCallTimeout(2*time.Second))
	require.NoError(t, err)
	defer caller.Close()

	_, err = caller.CallByName("focusPanic")
	require.Error(t, err)
	require.Contains(t, err.Error(), "panic")
}

// 验证 hook 内死循环时超时机制能让调用返回。
// 关键词: yak focus mode caller test, ctx timeout, hook hang
func TestFocusModeCaller_Timeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skip timeout test in short mode")
	}
	code := `
focusHang = func() {
    counter = 0
    for {
        counter++
    }
}
`
	caller, err := NewFocusModeYakHookCaller("hang.ai-focus.yak", code,
		WithFocusModeCallerCallTimeout(500*time.Millisecond))
	require.NoError(t, err)
	defer caller.Close()

	start := time.Now()
	_, err = caller.CallByName("focusHang")
	cost := time.Since(start)
	require.Error(t, err)
	require.Less(t, cost.Milliseconds(), int64(3000), "should timeout fast, cost=%v", cost)
}

// 验证 sidekick 拼接后函数对主入口可见。
// 关键词: yak focus mode caller test, sidekick bundle, symbol resolution
func TestFocusModeCaller_BundleSidekicks(t *testing.T) {
	main := `
focusBuild = func() {
    return helperGreeting("focus")
}
`
	sidekick := `
helperGreeting = func(name) {
    return "hello, " + name
}
`
	bundle := BundleSidekicks(main, sidekick)
	require.True(t, strings.Contains(bundle, "helperGreeting"))

	caller, err := NewFocusModeYakHookCaller("bundle.ai-focus.yak", bundle,
		WithFocusModeCallerCallTimeout(2*time.Second))
	require.NoError(t, err)
	defer caller.Close()

	val, err := caller.CallByName("focusBuild")
	require.NoError(t, err)
	require.Equal(t, "hello, focus", val)
}

// 验证 caller 实例之间互相隔离：在 caller A 中定义的全局符号
// 不会泄漏到 caller B。
// 关键词: yak focus mode caller test, isolation, no global leak
func TestFocusModeCaller_Isolation(t *testing.T) {
	codeA := `secretMarker = "A-side"`
	codeB := `secretMarker = "B-side"`

	a, err := NewFocusModeYakHookCaller("a.ai-focus.yak", codeA)
	require.NoError(t, err)
	defer a.Close()

	b, err := NewFocusModeYakHookCaller("b.ai-focus.yak", codeB)
	require.NoError(t, err)
	defer b.Close()

	require.Equal(t, "A-side", a.GetString("secretMarker"))
	require.Equal(t, "B-side", b.GetString("secretMarker"))
}

// 验证多 caller 并发执行 hook 时，不会因共享 yak 全局而互相干扰。
// 关键词: yak focus mode caller test, concurrent isolation
func TestFocusModeCaller_ConcurrentIsolation(t *testing.T) {
	code := `
focusEcho = func(s) {
    return s
}
`
	const N = 8
	callers := make([]*FocusModeYakHookCaller, 0, N)
	for i := 0; i < N; i++ {
		c, err := NewFocusModeYakHookCaller("conc.ai-focus.yak", code,
			WithFocusModeCallerCallTimeout(2*time.Second))
		require.NoError(t, err)
		defer c.Close()
		callers = append(callers, c)
	}

	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			val, err := callers[i].CallByName("focusEcho", "msg")
			require.NoError(t, err)
			require.Equal(t, "msg", val)
		}()
	}
	wg.Wait()
}

// 验证 Close 后再调用 hook 立即返回错误。
// 关键词: yak focus mode caller test, close idempotent
func TestFocusModeCaller_CloseIdempotent(t *testing.T) {
	caller, err := NewFocusModeYakHookCaller("close.ai-focus.yak", `focusNoop = func() { return 1 }`)
	require.NoError(t, err)

	caller.Close()
	caller.Close() // 再次关闭应当是 no-op

	_, err = caller.CallByName("focusNoop")
	require.Error(t, err)
}

// 验证父 ctx 取消时所有 caller 调用立即结束。
// 关键词: yak focus mode caller test, parent ctx cancel propagation
func TestFocusModeCaller_ParentCtxCancel(t *testing.T) {
	parentCtx, cancelParent := context.WithCancel(context.Background())
	code := `
focusBlocking = func() {
    counter = 0
    for {
        counter++
    }
}
`
	caller, err := NewFocusModeYakHookCaller("parent.ai-focus.yak", code,
		WithFocusModeCallerParentContext(parentCtx),
		WithFocusModeCallerCallTimeout(5*time.Second))
	require.NoError(t, err)
	defer caller.Close()

	doneCh := make(chan struct{})
	go func() {
		_, _ = caller.CallByName("focusBlocking")
		close(doneCh)
	}()

	time.Sleep(100 * time.Millisecond)
	cancelParent()

	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("expected hook to abort after parent ctx cancelled")
	}
}
