package yak

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
