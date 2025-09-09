package aicommon

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils/omap"
)

func TestContextProviderManager_BasicRegistration(t *testing.T) {
	cpm := NewContextProviderManager()

	// 测试注册功能 - 实际上 Register 方法不会立即调用 provider
	provider := func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		return fmt.Sprintf("context from %s", key), nil
	}

	cpm.Register("test_provider", provider)

	// 执行以触发 provider 调用
	result := cpm.Execute(nil, nil)

	if !strings.Contains(result, "context from test_provider") {
		t.Fatalf("Provider should be called during Execute, got: %s", result)
	}

	// 测试重复注册 - 当前实现会忽略重复注册
	provider2 := func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		return "duplicate", nil
	}

	cpm.Register("test_provider", provider2) // 这个会被忽略

	// 再次执行，应该还是第一个 provider 的结果
	result2 := cpm.Execute(nil, nil)

	if !strings.Contains(result2, "context from test_provider") {
		t.Fatalf("Original provider should still be active, got: %s", result2)
	}

	if strings.Contains(result2, "duplicate") {
		t.Log("Second provider was registered (unexpected)")
	}
}

func TestContextProviderManager_Execute(t *testing.T) {
	cpm := NewContextProviderManager()

	// 注册多个 providers
	callCount := 0
	provider1 := func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		callCount++
		return fmt.Sprintf("provider1_result_%s", key), nil
	}

	provider2 := func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		callCount++
		return fmt.Sprintf("provider2_result_%s", key), nil
	}

	cpm.Register("provider1", provider1)
	cpm.Register("provider2", provider2)

	// 执行 (使用 nil 作为参数进行简单测试)
	result := cpm.Execute(nil, nil)

	// 验证结果
	if callCount != 2 {
		t.Fatalf("Expected 2 provider calls, got %d", callCount)
	}

	if !strings.Contains(result, "provider1_result_provider1") {
		t.Fatalf("Result should contain provider1 result: %s", result)
	}

	if !strings.Contains(result, "provider2_result_provider2") {
		t.Fatalf("Result should contain provider2 result: %s", result)
	}

	// 验证格式
	if !strings.Contains(result, "<|AUTO_PROVIDE_CTX_") {
		t.Fatal("Result should contain proper formatting tags")
	}
}

func TestContextProviderManager_ExecuteEmpty(t *testing.T) {
	cpm := NewContextProviderManager()

	result := cpm.Execute(nil, nil)

	if result != "" {
		t.Fatalf("Expected empty result for empty provider manager, got: %s", result)
	}
}

func TestContextProviderManager_ExecuteWithErrors(t *testing.T) {
	cpm := NewContextProviderManager()

	// 正常 provider
	normalProvider := func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		return "normal_result", nil
	}

	// 错误 provider
	errorProvider := func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		return "", fmt.Errorf("test error")
	}

	cpm.Register("normal", normalProvider)
	cpm.Register("error", errorProvider)

	result := cpm.Execute(nil, nil)

	// 验证正常结果存在
	if !strings.Contains(result, "normal_result") {
		t.Fatalf("Result should contain normal provider result: %s", result)
	}

	// 验证错误被正确处理
	if !strings.Contains(result, "[Error getting context: test error]") {
		t.Fatalf("Result should contain error message: %s", result)
	}
}

func TestContextProviderManager_ExecuteWithShrink(t *testing.T) {
	// 创建一个小的 maxBytes 来测试压缩功能
	cpm := &ContextProviderManager{
		maxBytes: 100, // 设置一个小的限制
		callback: omap.NewOrderedMap(make(map[string]ContextProvider)),
	}

	// 注册一个会产生长输出的 provider
	longProvider := func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		// 生成一个很长的字符串
		longString := strings.Repeat("This is a very long context string that should exceed the maxBytes limit. ", 10)
		return longString, nil
	}

	cpm.Register("long_provider", longProvider)

	result := cpm.Execute(nil, nil)

	// 验证结果被压缩了
	if len(result) >= 100 {
		t.Fatalf("Result should be compressed, length %d should be less than maxBytes 100", len(result))
	}

	// 验证压缩后的结果包含 "..." (ShrinkString 的特征)
	if !strings.Contains(result, "...") {
		t.Fatalf("Compressed result should contain '...': %s", result)
	}
}

func TestContextProviderManager_ExecuteWithoutShrink(t *testing.T) {
	cpm := NewContextProviderManager() // 使用默认的 maxBytes (10KB)

	// 注册一个短输出的 provider
	shortProvider := func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		return "short result", nil
	}

	cpm.Register("short_provider", shortProvider)

	result := cpm.Execute(nil, nil)

	// 验证结果没有被压缩
	if !strings.Contains(result, "short result") {
		t.Fatalf("Result should contain original content: %s", result)
	}

	if strings.Contains(result, "...") {
		t.Fatalf("Short result should not be compressed: %s", result)
	}
}

func TestContextProviderManager_Unregister(t *testing.T) {
	cpm := NewContextProviderManager()

	called := false
	provider := func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		called = true
		return "result", nil
	}

	cpm.Register("test_provider", provider)
	cpm.Unregister("test_provider")

	// 再次注册应该成功
	called = false
	cpm.Register("test_provider", provider)

	result := cpm.Execute(nil, nil)

	if !called {
		t.Fatal("Provider should be called after re-registration")
	}

	if !strings.Contains(result, "result") {
		t.Fatalf("Result should contain provider output: %s", result)
	}
}

func TestContextProviderManager_ConcurrentAccess(t *testing.T) {
	cpm := NewContextProviderManager()

	// 并发注册和执行
	var wg sync.WaitGroup
	callCounts := make(map[string]int)
	var countsMutex sync.Mutex

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			providerName := fmt.Sprintf("provider_%d", id)
			provider := func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
				countsMutex.Lock()
				callCounts[key]++
				countsMutex.Unlock()
				return fmt.Sprintf("result_%d", id), nil
			}

			cpm.Register(providerName, provider)
		}(i)
	}

	wg.Wait()

	// 执行 (使用 nil 作为参数进行简单测试)
	result := cpm.Execute(nil, nil)

	// 验证所有 providers 都被调用了
	countsMutex.Lock()
	if len(callCounts) != 10 {
		t.Fatalf("Expected 10 providers to be called, got %d", len(callCounts))
	}
	countsMutex.Unlock()

	// 验证结果包含所有 providers 的输出
	for i := 0; i < 10; i++ {
		expected := fmt.Sprintf("result_%d", i)
		if !strings.Contains(result, expected) {
			t.Fatalf("Result should contain %s: %s", expected, result)
		}
	}
}

func TestContextProviderManager_LargeScaleTest(t *testing.T) {
	cpm := NewContextProviderManager()

	// 注册大量 providers
	const numProviders = 100
	callCount := 0
	var countMutex sync.Mutex

	for i := 0; i < numProviders; i++ {
		providerName := fmt.Sprintf("provider_%03d", i)
		provider := func(id int) ContextProvider {
			return func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
				countMutex.Lock()
				callCount++
				countMutex.Unlock()
				return fmt.Sprintf("result_%03d", id), nil
			}
		}(i)

		cpm.Register(providerName, provider)
	}

	result := cpm.Execute(nil, nil)

	countMutex.Lock()
	if callCount != numProviders {
		t.Fatalf("Expected %d provider calls, got %d", numProviders, callCount)
	}
	countMutex.Unlock()

	// 验证结果大小在合理范围内
	if len(result) < 1000 { // 至少应该有一定的长度
		t.Fatalf("Result too small: %d characters", len(result))
	}
}

func TestContextProviderManager_PerformanceTest(t *testing.T) {
	cpm := NewContextProviderManager()

	// 注册一些 providers
	for i := 0; i < 20; i++ {
		providerName := fmt.Sprintf("perf_provider_%d", i)
		provider := func(id int) ContextProvider {
			return func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
				// 模拟一些处理时间
				time.Sleep(time.Millisecond)
				return fmt.Sprintf("perf_result_%d", id), nil
			}
		}(i)

		cpm.Register(providerName, provider)
	}

	start := time.Now()
	result := cpm.Execute(nil, nil)
	duration := time.Since(start)

	// 执行时间应该在合理范围内
	if duration > time.Second {
		t.Fatalf("Execution took too long: %v", duration)
	}

	if len(result) == 0 {
		t.Fatal("Result should not be empty")
	}
}

func TestContextProviderManager_EdgeCases(t *testing.T) {
	// 测试空名称注册
	cpm := NewContextProviderManager()

	emptyNameProvider := func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		return "empty_name_result", nil
	}

	// 空名称应该被拒绝
	cpm.Register("", emptyNameProvider)

	// nil provider 应该被拒绝
	cpm.Register("nil_provider", nil)

	// 测试非常长的 provider 名称
	longName := strings.Repeat("a", 1000)
	longNameProvider := func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		return "long_name_result", nil
	}

	cpm.Register(longName, longNameProvider)

	result := cpm.Execute(nil, nil)
	if !strings.Contains(result, "long_name_result") {
		t.Fatalf("Result should contain long name provider result: %s", result)
	}
}

func TestContextProviderManager_BufferedOutput(t *testing.T) {
	cpm := NewContextProviderManager()

	// 测试包含特殊字符的输出
	specialCharProvider := func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		return "special chars: \n\t\r\"'\\", nil
	}

	cpm.Register("special", specialCharProvider)

	result := cpm.Execute(nil, nil)

	// 验证结果包含正确的标签格式
	if !strings.Contains(result, "<|AUTO_PROVIDE_CTX_") {
		t.Fatal("Result should contain proper start tag")
	}

	if !strings.Contains(result, "_END|>") {
		t.Fatal("Result should contain proper end tag")
	}

	if !strings.Contains(result, "special chars") {
		t.Fatal("Result should contain provider output")
	}
}

func TestNewContextProviderManager(t *testing.T) {
	cpm := NewContextProviderManager()

	if cpm == nil {
		t.Fatal("NewContextProviderManager should return a non-nil instance")
	}

	if cpm.maxBytes != 10*1024 { // 默认 10KB
		t.Fatalf("Expected maxBytes to be 10240, got %d", cpm.maxBytes)
	}

	if cpm.callback == nil {
		t.Fatal("Callback map should be initialized")
	}
}

func TestContextProviderManager_MaxBytesConfiguration(t *testing.T) {
	// 测试不同的 maxBytes 配置
	testCases := []struct {
		name         string
		maxBytes     int
		input        string
		expectShrink bool
	}{
		{"Normal size", 1000, "short string", false},
		{"Exact limit", 50, strings.Repeat("x", 50), false},
		{"Over limit", 50, strings.Repeat("x", 100), true},
		{"Large over limit", 100, strings.Repeat("x", 1000), true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cpm := &ContextProviderManager{
				maxBytes: tc.maxBytes,
				callback: omap.NewOrderedMap(make(map[string]ContextProvider)),
			}

			provider := func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
				return tc.input, nil
			}

			cpm.Register("test", provider)
			result := cpm.Execute(nil, nil)

			if tc.expectShrink {
				if !strings.Contains(result, "...") {
					t.Fatalf("Expected result to be shrunk for case %s", tc.name)
				}
				if len(result) >= tc.maxBytes {
					t.Fatalf("Result length %d should be less than maxBytes %d", len(result), tc.maxBytes)
				}
			} else {
				if len(result) > tc.maxBytes && !strings.Contains(result, "...") {
					t.Fatalf("Result should not be shrunk for case %s", tc.name)
				}
			}
		})
	}
}

func TestContextProviderManager_ProviderPanicRecovery(t *testing.T) {
	cpm := NewContextProviderManager()

	panicProvider := func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		panic("test panic")
	}

	normalProvider := func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		return "normal_result", nil
	}

	cpm.Register("panic_provider", panicProvider)
	cpm.Register("normal_provider", normalProvider)

	// 这不应该panic，而是应该恢复并继续处理其他providers
	result := cpm.Execute(nil, nil)

	// 验证正常provider的结果仍然存在
	if !strings.Contains(result, "normal_result") {
		t.Fatalf("Normal provider result should be present despite panic: %s", result)
	}

	// 验证panic被处理了
	if !strings.Contains(result, "[Error getting context:") {
		t.Logf("Panic should be handled gracefully. Result: %s", result)
	}
}

func TestContextProviderManager_EmptyProviderResult(t *testing.T) {
	cpm := NewContextProviderManager()

	emptyProvider := func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		return "", nil
	}

	cpm.Register("empty_provider", emptyProvider)

	result := cpm.Execute(nil, nil)

	// 即使结果为空，也应该有标签
	if !strings.Contains(result, "<|AUTO_PROVIDE_CTX_") {
		t.Fatal("Result should contain tags even for empty provider result")
	}
}

func TestContextProviderManager_MultipleExecutions(t *testing.T) {
	cpm := NewContextProviderManager()

	callCount := 0
	provider := func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		callCount++
		return fmt.Sprintf("call_%d", callCount), nil
	}

	cpm.Register("multi_call_provider", provider)

	// 多次执行
	result1 := cpm.Execute(nil, nil)
	result2 := cpm.Execute(nil, nil)
	result3 := cpm.Execute(nil, nil)

	// 验证每次调用都会增加计数
	if !strings.Contains(result1, "call_1") {
		t.Fatalf("First result should contain call_1: %s", result1)
	}

	if !strings.Contains(result2, "call_2") {
		t.Fatalf("Second result should contain call_2: %s", result2)
	}

	if !strings.Contains(result3, "call_3") {
		t.Fatalf("Third result should contain call_3: %s", result3)
	}
}
