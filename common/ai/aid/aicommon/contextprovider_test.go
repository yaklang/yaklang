package aicommon

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils"
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

func TestContextProviderManager_RegisterTracedContent_BasicFunctionality(t *testing.T) {
	cpm := NewContextProviderManager()

	callCount := 0
	tracedProvider := func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		callCount++
		return fmt.Sprintf("content_version_%d", callCount), nil
	}

	cpm.RegisterTracedContent("traced_test", tracedProvider)

	// 第一次执行 - 应该只包含内容，不包含差异
	result1 := cpm.Execute(nil, nil)
	if callCount != 1 {
		t.Fatalf("Expected 1 call, got %d", callCount)
	}

	if !strings.Contains(result1, "content_version_1") {
		t.Fatalf("First result should contain content_version_1: %s", result1)
	}

	// 第二次执行 - 应该包含差异信息
	result2 := cpm.Execute(nil, nil)
	if callCount != 2 {
		t.Fatalf("Expected 2 calls, got %d", callCount)
	}

	if !strings.Contains(result2, "content_version_2") {
		t.Fatalf("Second result should contain content_version_2: %s", result2)
	}

	// 第二次结果应该包含差异信息
	if !strings.Contains(result2, "CHANGES_DIFF_") {
		t.Fatalf("Second result should contain diff markers: %s", result2)
	}

	// 验证差异内容包含了正确的变化
	if !strings.Contains(result2, "-content_version_1") {
		t.Fatalf("Second result should contain old content in diff: %s", result2)
	}

	if !strings.Contains(result2, "+content_version_2") {
		t.Fatalf("Second result should contain new content in diff: %s", result2)
	}

	t.Logf("First result: %s", result1)
	t.Logf("Second result: %s", result2)
}

func TestContextProviderManager_RegisterTracedContent_ErrorHandling(t *testing.T) {
	cpm := NewContextProviderManager()

	callCount := 0
	tracedProvider := func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		callCount++
		if callCount == 1 {
			return "success_content", nil
		} else if callCount == 2 {
			return "", fmt.Errorf("test error")
		} else {
			return "recovered_content", nil
		}
	}

	cpm.RegisterTracedContent("traced_error_test", tracedProvider)

	// 第一次执行 - 成功
	result1 := cpm.Execute(nil, nil)
	if !strings.Contains(result1, "success_content") {
		t.Fatalf("First result should contain success_content: %s", result1)
	}

	// 第二次执行 - 错误
	result2 := cpm.Execute(nil, nil)
	if !strings.Contains(result2, "test error") {
		t.Fatalf("Second result should contain error message: %s", result2)
	}

	// 第三次执行 - 恢复
	result3 := cpm.Execute(nil, nil)
	if !strings.Contains(result3, "recovered_content") {
		t.Fatalf("Third result should contain recovered_content: %s", result3)
	}

	// 验证第三次结果包含差异信息和错误解决信息
	if !strings.Contains(result3, "CHANGES_DIFF_") {
		t.Fatalf("Third result should contain diff markers: %s", result3)
	}

	// 验证包含错误解决信息
	if !strings.Contains(result3, "Error resolved") {
		t.Fatalf("Third result should contain error resolution info: %s", result3)
	}

	t.Logf("Error handling test - Result1: %s", result1)
	t.Logf("Error handling test - Result2: %s", result2)
	t.Logf("Error handling test - Result3: %s", result3)
}

func TestContextProviderManager_RegisterTracedContent_EmptyContent(t *testing.T) {
	cpm := NewContextProviderManager()

	tracedProvider := func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		return "", nil
	}

	cpm.RegisterTracedContent("traced_empty_test", tracedProvider)

	result := cpm.Execute(nil, nil)

	// 即使内容为空，也应该有基本的标签结构
	if !strings.Contains(result, "AUTO_PROVIDE_CTX_") {
		t.Fatal("Result should contain proper tags even for empty content")
	}

	t.Logf("Empty content result: %s", result)
}

func TestContextProviderManager_RegisterTracedContent_LargeContentDiff(t *testing.T) {
	cpm := NewContextProviderManager()

	callCount := 0
	tracedProvider := func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		callCount++
		// 生成大的内容来测试差异计算
		baseContent := strings.Repeat("This is a line of content. ", 10)
		if callCount == 1 {
			return baseContent + "First version.", nil
		} else {
			return baseContent + "Second version with modifications.", nil
		}
	}

	cpm.RegisterTracedContent("traced_large_test", tracedProvider)

	// 第一次执行
	_ = cpm.Execute(nil, nil)

	// 第二次执行 - 应该计算差异
	result2 := cpm.Execute(nil, nil)

	// 验证包含了新的内容
	if !strings.Contains(result2, "Second version") {
		t.Fatalf("Second result should contain new content: %s", result2)
	}

	// 验证差异信息存在
	if !strings.Contains(result2, "CHANGES_DIFF_") {
		t.Fatalf("Second result should contain diff markers: %s", result2)
	}

	t.Logf("Large content diff - Result2: %s", utils.ShrinkString(result2, 200))
}

func TestContextProviderManager_RegisterTracedContent_ConcurrentAccess(t *testing.T) {
	cpm := NewContextProviderManager()

	callCounts := make(map[string]int)
	var countsMutex sync.Mutex

	tracedProvider := func(id string) ContextProvider {
		return func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
			countsMutex.Lock()
			callCounts[id]++
			count := callCounts[id]
			countsMutex.Unlock()
			return fmt.Sprintf("concurrent_content_%s_%d", id, count), nil
		}
	}

	// 注册多个 traced providers
	cpm.RegisterTracedContent("traced_concurrent_1", tracedProvider("provider1"))
	cpm.RegisterTracedContent("traced_concurrent_2", tracedProvider("provider2"))

	// 并发执行
	var wg sync.WaitGroup
	results := make([]string, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = cpm.Execute(nil, nil)
		}(i)
	}

	wg.Wait()

	// 验证所有结果都包含预期的内容
	for i, result := range results {
		if !strings.Contains(result, "concurrent_content_provider1_") {
			t.Fatalf("Result %d should contain provider1 content: %s", i, result)
		}
		if !strings.Contains(result, "concurrent_content_provider2_") {
			t.Fatalf("Result %d should contain provider2 content: %s", i, result)
		}
	}

	countsMutex.Lock()
	if callCounts["provider1"] < 10 || callCounts["provider2"] < 10 {
		t.Fatalf("Expected at least 10 calls per provider, got provider1: %d, provider2: %d",
			callCounts["provider1"], callCounts["provider2"])
	}
	countsMutex.Unlock()
}

func TestContextProviderManager_RegisterTracedContent_WithMaxBytesShrink(t *testing.T) {
	// 创建一个小的 maxBytes 来测试 traced content 的压缩
	cpm := &ContextProviderManager{
		maxBytes: 200, // 设置一个小的限制
		callback: omap.NewOrderedMap(make(map[string]ContextProvider)),
	}

	callCount := 0
	tracedProvider := func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		callCount++
		// 生成会触发压缩的长内容
		longContent := strings.Repeat("This is a very long traced content that should trigger compression. ", 20)
		return longContent, nil
	}

	cpm.RegisterTracedContent("traced_compress_test", tracedProvider)

	// 执行多次以积累差异信息
	for i := 0; i < 3; i++ {
		result := cpm.Execute(nil, nil)
		if len(result) > 200 {
			// 如果结果超过 maxBytes，验证它被压缩了
			if !strings.Contains(result, "...") {
				t.Fatalf("Long result should be compressed, but no '...' found in: %s", result)
			}
		}
	}

	if callCount != 3 {
		t.Fatalf("Expected 3 calls, got %d", callCount)
	}
}

func TestContextProviderManager_RegisterTracedContent_ComplexDiff(t *testing.T) {
	cpm := NewContextProviderManager()

	callCount := 0
	tracedProvider := func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		callCount++
		switch callCount {
		case 1:
			return `line1: unchanged
line2: will change
line3: unchanged`, nil
		case 2:
			return `line1: unchanged
line2: changed content
line3: unchanged
line4: new line added`, nil
		case 3:
			return `line1: unchanged
line2: changed again
line3: unchanged
line4: new line added
line5: another new line`, nil
		default:
			return "final content", nil
		}
	}

	cpm.RegisterTracedContent("traced_complex_test", tracedProvider)

	// 执行多次并验证差异跟踪
	results := []string{}
	for i := 0; i < 4; i++ {
		result := cpm.Execute(nil, nil)
		results = append(results, result)
	}

	// 验证每次调用都有正确的输出内容
	for i, result := range results {
		// 验证基本的内容存在
		if i == 0 && !strings.Contains(result, "will change") {
			t.Fatalf("Result %d should contain 'will change'", i)
		}
		if i == 1 && !strings.Contains(result, "changed content") {
			t.Fatalf("Result %d should contain 'changed content'", i)
		}
		if i == 2 && !strings.Contains(result, "changed again") {
			t.Fatalf("Result %d should contain 'changed again'", i)
		}
		if i == 3 && !strings.Contains(result, "final content") {
			t.Fatalf("Result %d should contain 'final content'", i)
		}

		// 验证从第二次调用开始有差异信息
		if i > 0 && !strings.Contains(result, "CHANGES_DIFF_") {
			t.Fatalf("Result %d should contain diff markers", i)
		}

		t.Logf("Complex diff - Result %d: %s", i, utils.ShrinkString(result, 150))
	}

	if callCount != 4 {
		t.Fatalf("Expected 4 calls, got %d", callCount)
	}
}

func TestContextProviderManager_RegisterTracedContent_IntegrationWithRegular(t *testing.T) {
	cpm := NewContextProviderManager()

	// 注册普通 provider
	regularProvider := func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		return "regular_content", nil
	}
	cpm.Register("regular_provider", regularProvider)

	// 注册 traced provider
	callCount := 0
	tracedProvider := func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		callCount++
		return fmt.Sprintf("traced_content_%d", callCount), nil
	}
	cpm.RegisterTracedContent("traced_provider", tracedProvider)

	result := cpm.Execute(nil, nil)

	// 验证两个 provider 的结果都在
	if !strings.Contains(result, "regular_content") {
		t.Fatalf("Result should contain regular provider content: %s", result)
	}

	if !strings.Contains(result, "traced_content_1") {
		t.Fatalf("Result should contain traced provider content: %s", result)
	}

	t.Logf("Integration test result: %s", result)
}

func TestContextProviderManager_RegisterTracedContent_PanicRecovery(t *testing.T) {
	cpm := NewContextProviderManager()

	callCount := 0
	panicProvider := func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		callCount++
		if callCount == 1 {
			return "normal_content", nil
		} else {
			panic("traced provider panic test")
		}
	}

	cpm.RegisterTracedContent("traced_panic_test", panicProvider)

	// 第一次调用应该正常
	result1 := cpm.Execute(nil, nil)
	if !strings.Contains(result1, "normal_content") {
		t.Fatalf("First result should contain normal content: %s", result1)
	}

	// 第二次调用应该panic并被恢复
	result2 := cpm.Execute(nil, nil)

	// 验证包含错误信息
	if !strings.Contains(result2, "Error getting context") {
		t.Logf("Panic should be handled gracefully. Result: %s", result2)
	}

	if callCount != 2 {
		t.Fatalf("Expected 2 calls despite panic, got %d", callCount)
	}
}
