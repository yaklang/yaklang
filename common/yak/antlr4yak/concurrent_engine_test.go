package antlr4yak

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestConcurrentEvalAndGetVar 测试并发执行 eval 和 GetVar
func TestConcurrentEvalAndGetVar(t *testing.T) {
	engine := New()
	ctx := context.Background()

	// 初始代码
	initialCode := `
counter = 0
testFunc = func() {
	return counter
}
`
	err := engine.SafeEval(ctx, initialCode)
	require.NoError(t, err)

	// 并发执行 eval 和调用函数
	concurrentCount := 30
	var wg sync.WaitGroup
	var lock sync.Mutex
	panics := make(chan interface{}, concurrentCount)
	successCount := 0

	for i := 0; i < concurrentCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					panics <- r
				}
			}()

			lock.Lock()
			// 执行 eval 修改变量
			hotCode := `
counter = counter + 1
testFunc = func() {
	return counter * 2
}
`
			err := engine.SafeEval(ctx, hotCode)
			if err != nil {
				lock.Unlock()
				return
			}

			// 调用函数
			result, err := engine.CallYakFunction(ctx, "testFunc", []interface{}{})
			lock.Unlock()

			if err != nil {
				t.Logf("调用失败: %v", err)
				return
			}

			if result != nil {
				lock.Lock()
				successCount++
				lock.Unlock()
			}
		}(i)
	}

	wg.Wait()
	close(panics)

	// 检查 panic
	panicCount := 0
	for p := range panics {
		panicCount++
		t.Errorf("发生 panic: %v", p)
	}

	require.Equal(t, 0, panicCount, "不应该有 panic")
	require.Greater(t, successCount, 0, "至少应该有一些成功的调用")
}

// TestConcurrentCallYakFunction 测试并发调用 CallYakFunction 的安全性
func TestConcurrentCallYakFunction(t *testing.T) {
	engine := New()
	ctx := context.Background()

	// 定义函数
	code := `
add = func(a, b) {
	return a + b
}
`
	err := engine.SafeEval(ctx, code)
	require.NoError(t, err)

	// 并发调用函数
	concurrentCount := 50
	var wg sync.WaitGroup
	errChan := make(chan error, concurrentCount)
	successCount := 0
	var mu sync.Mutex

	for i := 0; i < concurrentCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// 调用函数
			result, err := engine.CallYakFunction(ctx, "add", []interface{}{id, id * 2})
			if err != nil {
				errChan <- err
				return
			}

			// 验证结果
			if result != nil {
				expected := id + id*2
				if result != expected {
					errChan <- errors.New("result value mismatch")
					return
				}
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()
	close(errChan)

	// 统计错误
	errorCount := 0
	for err := range errChan {
		if err != nil {
			errorCount++
			t.Logf("错误: %v", err)
		}
	}

	t.Logf("成功: %d, 错误: %d", successCount, concurrentCount)
	require.Equal(t, concurrentCount, successCount, "所有调用都应该成功")
	require.Equal(t, 0, errorCount, "不应该有错误")
}

// TestConcurrentEvalWithFunctionRedefinition 测试并发重新定义函数
func TestConcurrentEvalWithFunctionRedefinition(t *testing.T) {
	engine := New()
	ctx := context.Background()

	initialCode := `
beforeRequest = func(pack) {
	return pack + "_v1"
}
`
	err := engine.SafeEval(ctx, initialCode)
	require.NoError(t, err)

	var lock sync.Mutex
	concurrentCount := 20
	var wg sync.WaitGroup
	panics := make(chan interface{}, concurrentCount)

	for i := 0; i < concurrentCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					panics <- r
				}
			}()

			lock.Lock()
			// 重新定义函数
			hotCode := `
beforeRequest = func(pack) {
	return pack + "_v2"
}
`
			err := engine.SafeEval(ctx, hotCode)
			if err != nil {
				lock.Unlock()
				return
			}

			// 调用函数
			result, err := engine.CallYakFunction(ctx, "beforeRequest", []interface{}{"test"})
			lock.Unlock()

			if err != nil {
				t.Logf("调用失败: %v", err)
				return
			}

			// 验证结果
			if result == nil {
				t.Logf("结果为 nil")
			}
		}(i)
	}

	wg.Wait()
	close(panics)

	// 检查 panic
	panicCount := 0
	for p := range panics {
		panicCount++
		t.Errorf("发生 panic: %v", p)
	}

	require.Equal(t, 0, panicCount, "不应该有 panic")
}

// TestCallYakFunctionNilSafety 测试调用 nil 函数的安全性
func TestCallYakFunctionNilSafety(t *testing.T) {
	engine := New()
	ctx := context.Background()

	// 测试不存在的函数
	_, err := engine.CallYakFunction(ctx, "nonexistent", []interface{}{"test"})
	require.Error(t, err, "调用不存在的函数应该返回错误")
	require.Contains(t, err.Error(), "not found", "错误信息应该包含 'not found'")

	// 定义函数然后删除
	err = engine.SafeEval(ctx, `testFunc = func() { return "ok" }`)
	require.NoError(t, err)

	err = engine.SafeEval(ctx, `testFunc = undefined`)
	require.NoError(t, err)

	// 尝试调用已删除的函数
	_, err = engine.CallYakFunction(ctx, "testFunc", []interface{}{})
	require.Error(t, err, "调用已删除的函数应该返回错误")
}

