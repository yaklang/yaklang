package yak

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/yak/antlr4yak"
)

type testEventSync struct {
	mu    sync.Mutex
	chans map[string]chan struct{}
	once  map[string]*sync.Once
}

func newTestEventSync() *testEventSync {
	return &testEventSync{
		chans: make(map[string]chan struct{}),
		once:  make(map[string]*sync.Once),
	}
}

func (s *testEventSync) get(name string) (chan struct{}, *sync.Once) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ch, ok := s.chans[name]
	if !ok {
		ch = make(chan struct{})
		s.chans[name] = ch
	}
	o, ok := s.once[name]
	if !ok {
		o = new(sync.Once)
		s.once[name] = o
	}
	return ch, o
}

func (s *testEventSync) signal(name string) {
	ch, o := s.get(name)
	o.Do(func() {
		close(ch)
	})
}

func (s *testEventSync) wait(t *testing.T, name string) {
	t.Helper()
	ch, _ := s.get(name)
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for test event %q", name)
	}
}

func newTestHookEngine(t *testing.T, script string, globals map[string]any) *antlr4yak.Engine {
	t.Helper()

	scriptEngine := NewScriptEngine(1)
	if len(globals) > 0 {
		scriptEngine.RegisterEngineHooks(func(engine *antlr4yak.Engine) error {
			engine.OverrideRuntimeGlobalVariables(globals)
			return nil
		})
	}

	engine, err := scriptEngine.ExecuteEx(script, map[string]any{})
	require.NoError(t, err)
	return engine
}

type hookCallResult struct {
	result string
	err    error
}

func invokeBeforeRequestAsync(engine *antlr4yak.Engine, name string) <-chan hookCallResult {
	resultCh := make(chan hookCallResult, 1)
	go func() {
		req := []byte(name)
		result, err := engine.CallYakFunction(context.Background(), "beforeRequest", []interface{}{false, req, req})
		if err != nil {
			resultCh <- hookCallResult{err: err}
			return
		}
		switch ret := result.(type) {
		case string:
			resultCh <- hookCallResult{result: ret}
		case []byte:
			resultCh <- hookCallResult{result: string(ret)}
		default:
			resultCh <- hookCallResult{result: fmt.Sprint(ret)}
		}
	}()
	return resultCh
}

// 并发调用含子函数的hook，验证VMStack不会串扰
func TestConcurrentEngineCall_VMStackCorruption(t *testing.T) {
	scriptEngine := NewScriptEngine(1)
	engine, err := scriptEngine.ExecuteEx(`
tag = "ok"
helper = func(s) {
    return s + "-" + tag
}
beforeRequest = func(isHttps, originReq, req) {
    return helper(string(req))
}
`, make(map[string]interface{}))
	require.NoError(t, err)

	const n = 30
	results := make([]string, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			req := []byte(fmt.Sprintf("req%d", i))
			result, callErr := engine.CallYakFunction(context.Background(), "beforeRequest", []interface{}{false, req, req})
			if callErr != nil {
				t.Errorf("goroutine %d error: %v", i, callErr)
				return
			}
			switch v := result.(type) {
			case string:
				results[i] = v
			case []byte:
				results[i] = string(v)
			}
		}()
	}
	wg.Wait()

	for i, res := range results {
		expected := fmt.Sprintf("req%d-ok", i)
		assert.Equal(t, expected, res, "result[%d] wrong (VMStack corrupted)", i)
	}
}

// 通过MutateHookCaller API并发调用beforeRequest
func TestMutateHookCaller_ConcurrentCorrectness(t *testing.T) {
	hookBefore, _, _, _, _, _ := MutateHookCaller(context.Background(), `
tag = "done"
helper = func(s) {
    return s + "-" + tag
}
beforeRequest = func(isHttps, originReq, req) {
    return helper(string(req))
}
`, nil)
	require.NotNil(t, hookBefore)

	const n = 30
	results := make([]string, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			req := []byte(fmt.Sprintf("req%d", i))
			res := hookBefore(false, req, req)
			results[i] = string(res)
		}()
	}
	wg.Wait()

	for i, res := range results {
		expected := fmt.Sprintf("req%d-done", i)
		assert.Equal(t, expected, res, "result[%d] wrong", i)
	}
}

func TestMutateHookCallerDoesNotPersistHotReloadCache(t *testing.T) {
	t.Setenv("YAKIT_HOME", t.TempDir())
	code := `
beforeRequest = func(isHttps, originReq, req) {
    return req
}
// ` + strings.Repeat("hot-reload-revision-", 20)

	hookBefore, _, _, _, _, _ := MutateHookCaller(context.Background(), code, nil)
	require.NotNil(t, hookBefore)
	_, cached := antlr4yak.HaveYakcCache(code)
	require.False(t, cached, "web fuzzer hot reload must not persist unique source revisions")
}

// 并发调用多层嵌套子函数(A→B→C)，验证context逐层传递正确
func TestConcurrentEngineCall_DeepNestedCalls(t *testing.T) {
	scriptEngine := NewScriptEngine(1)
	engine, err := scriptEngine.ExecuteEx(`
c = func(s) { return s + "-c" }
b = func(s) { return c(s) + "-b" }
a = func(s) { return b(s) + "-a" }
process = func(isHttps, originReq, req) {
    return a(string(req))
}
`, make(map[string]interface{}))
	require.NoError(t, err)

	const n = 30
	results := make([]string, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			req := []byte(fmt.Sprintf("req%d", i))
			result, callErr := engine.CallYakFunction(context.Background(), "process", []interface{}{false, req, req})
			if callErr != nil {
				t.Errorf("goroutine %d error: %v", i, callErr)
				return
			}
			if s, ok := result.(string); ok {
				results[i] = s
			}
		}()
	}
	wg.Wait()

	for i, res := range results {
		expected := fmt.Sprintf("req%d-c-b-a", i)
		assert.Equal(t, expected, res, "result[%d] wrong (deep nesting)", i)
	}
}

// 并发调用afterRequest hook，验证结果正确
func TestMutateHookCaller_AfterRequestConcurrent(t *testing.T) {
	_, hookAfter, _, _, _, _ := MutateHookCaller(context.Background(), `
tag = func(s) { return s + "-tagged" }
afterRequest = func(isHttps, originReq, req, originRsp, rsp) {
    return tag(string(rsp))
}
`, nil)
	require.NotNil(t, hookAfter)

	const n = 30
	results := make([]string, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			rsp := []byte(fmt.Sprintf("rsp%d", i))
			res := hookAfter(false, nil, nil, rsp, rsp)
			results[i] = string(res)
		}()
	}
	wg.Wait()

	for i, res := range results {
		expected := fmt.Sprintf("rsp%d-tagged", i)
		assert.Equal(t, expected, res, "result[%d] wrong", i)
	}
}

// 引擎并发风险：helper/eval 如果直接读取共享栈顶，会在并发下拿到别的 goroutine 当前 frame。
func TestMutateHookCaller_ConcurrentEvalUsesCorrectFrame(t *testing.T) {
	syncer := newTestEventSync()
	engine := newTestHookEngine(t, `
beforeRequest = func(isHttps, originReq, req) {
    localValue = string(req)
    if localValue == "fast" {
        signal("fast-entered")
        wait("allow-fast-eval")
        eval("result = localValue")
        return result
    }

    signal("slow-entered")
    wait("allow-slow-exit")
    return localValue
}
`, map[string]any{
		"signal": func(name string) { syncer.signal(name) },
		"wait":   func(name string) { syncer.wait(t, name) },
	})

	fastResultCh := invokeBeforeRequestAsync(engine, "fast")
	syncer.wait(t, "fast-entered")

	slowResultCh := invokeBeforeRequestAsync(engine, "slow")
	syncer.wait(t, "slow-entered")

	// 此时 slow 的 frame 已经入栈，fast 继续执行 eval 必须仍然读取到自己的 localValue。
	syncer.signal("allow-fast-eval")
	fastResult := <-fastResultCh
	require.NoError(t, fastResult.err)
	require.Equal(t, "fast", fastResult.result)

	syncer.signal("allow-slow-exit")
	slowResult := <-slowResultCh
	require.NoError(t, slowResult.err)
	require.Equal(t, "slow", slowResult.result)
}

// 引擎风险：先返回的 goroutine 不能把另一个 goroutine 仍在运行的当前 frame 弹掉。
func TestMutateHookCaller_OutOfOrderReturnDoesNotCorruptCurrentFrame(t *testing.T) {
	syncer := newTestEventSync()
	engine := newTestHookEngine(t, `
beforeRequest = func(isHttps, originReq, req) {
    localValue = string(req)
    if localValue == "fast" {
        signal("fast-entered")
        wait("allow-fast-return")
        return localValue
    }

    signal("slow-entered")
    wait("allow-slow-eval")
    eval("result = localValue")
    return result
}
`, map[string]any{
		"signal": func(name string) { syncer.signal(name) },
		"wait":   func(name string) { syncer.wait(t, name) },
	})

	fastResultCh := invokeBeforeRequestAsync(engine, "fast")
	syncer.wait(t, "fast-entered")

	slowResultCh := invokeBeforeRequestAsync(engine, "slow")
	syncer.wait(t, "slow-entered")

	// 先让 fast 返回；旧实现里这里会把 slow 的 frame 从共享栈顶错误弹掉。
	syncer.signal("allow-fast-return")
	fastResult := <-fastResultCh
	require.NoError(t, fastResult.err)
	require.Equal(t, "fast", fastResult.result)

	// slow 恢复后继续执行 eval，必须仍然读到自己的 localValue。
	syncer.signal("allow-slow-eval")
	slowResult := <-slowResultCh
	require.NoError(t, slowResult.err)
	require.Equal(t, "slow", slowResult.result)
}

func TestMutateHookCallerCancellationStopsAllHookKinds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	before, after, mirror, retry, failure, mock := MutateHookCaller(ctx, `
beforeRequest = func(isHttps, originReq, req) { for {} }
afterRequest = func(isHttps, originReq, req, originRsp, rsp) { for {} }
mirrorHTTPFlow = func(req, rsp) { for {} }
retryHandler = func(req, rsp, retry) { for {} }
customFailureChecker = func(rsp, fail) { for {} }
mockHTTPRequest = func(isHttps, url, req, mockResponse) { for {} }
`, nil)
	require.NotNil(t, before)
	require.NotNil(t, after)
	require.NotNil(t, mirror)
	require.NotNil(t, retry)
	require.NotNil(t, failure)
	require.NotNil(t, mock)

	done := make(chan string, 6)
	go func() {
		before(false, nil, []byte("request"))
		done <- "before"
	}()
	go func() {
		after(false, nil, nil, nil, []byte("response"))
		done <- "after"
	}()
	go func() {
		mirror(nil, nil, map[string]string{})
		done <- "mirror"
	}()
	go func() {
		retry(false, 0, nil, nil, func(...[]byte) {})
		done <- "retry"
	}()
	go func() {
		failure(false, nil, nil, func(string) {})
		done <- "failure"
	}()
	go func() {
		mock(false, "http://example.invalid", nil, func(interface{}) {})
		done <- "mock"
	}()

	// Give every hook a chance to enter Yak execution before canceling the
	// stream context that owns this hot patch.
	time.Sleep(50 * time.Millisecond)
	cancel()

	returned := make(map[string]bool, 6)
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for len(returned) < 6 {
		select {
		case name := <-done:
			returned[name] = true
		case <-timer.C:
			t.Fatalf("hot hooks did not stop after cancellation: returned=%v", returned)
		}
	}
}
