package yakvm

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestConcurrentGetVar 测试并发访问 GetVar 的安全性
func TestConcurrentGetVar(t *testing.T) {
	vm := New()
	ctx := context.Background()
	_ = ctx
	// 设置一些全局变量
	vm.SetVars(map[string]any{
		"testVar1": "value1",
		"testVar2": 42,
		"testVar3": map[string]any{"key": "value"},
	})

	// 并发读取变量
	concurrentCount := 50
	var wg sync.WaitGroup
	errChan := make(chan error, concurrentCount)

	for i := 0; i < concurrentCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// 读取变量
			val1, ok1 := vm.GetVar("testVar1")
			if !ok1 {
				errChan <- errors.New("testVar1 not found")
				return
			}
			if val1 != "value1" {
				errChan <- errors.New("testVar1 value mismatch")
				return
			}

			val2, ok2 := vm.GetVar("testVar2")
			if !ok2 {
				errChan <- errors.New("testVar2 not found")
				return
			}
			if val2 != 42 {
				errChan <- errors.New("testVar2 value mismatch")
				return
			}

			val3, ok3 := vm.GetVar("testVar3")
			if !ok3 {
				errChan <- errors.New("testVar3 not found")
				return
			}
			if val3 == nil {
				errChan <- errors.New("testVar3 is nil")
				return
			}
		}(i)
	}

	wg.Wait()
	close(errChan)

	// 检查错误
	errorCount := 0
	for err := range errChan {
		if err != nil {
			errorCount++
		}
	}

	require.Equal(t, 0, errorCount, "并发读取变量不应该有错误")
}

// TestVMStackConcurrentAccess 测试 VMStack 的并发访问安全性
func TestVMStackConcurrentAccess(t *testing.T) {
	vm := New()
	ctx := context.Background()

	// 创建多个 frame 并并发访问
	concurrentCount := 30
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

			// 执行代码，会创建 frame 并推入 VMStack
			// 使用 Exec 方法测试并发访问
			err := vm.Exec(ctx, func(frame *Frame) {
				// 在 frame 中执行一些操作
				_ = frame
			}, None)
			if err != nil {
				// 忽略执行错误，我们只关心并发访问
				return
			}

			// 尝试访问 VMStack（通过 GetVar 间接访问）
			_, _ = vm.GetVar("__test_var__")
		}(i)
	}

	wg.Wait()
	close(panics)

	// 检查 panic
	panicCount := 0
	for p := range panics {
		panicCount++
		t.Logf("发生 panic: %v", p)
	}

	// 注意：由于 ExecYakCode 内部已经有锁保护，这里主要是测试 GetVar 的并发访问
	t.Logf("Panic 数量: %d", panicCount)
	require.Equal(t, 0, panicCount, "并发读取变量不应该有错误")
}
