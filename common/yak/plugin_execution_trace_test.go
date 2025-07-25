package yak

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

// TestPluginExecutionTracing 测试插件执行跟踪功能
func TestPluginExecutionTracing(t *testing.T) {
	// 创建 YakToCallerManager
	manager := NewYakToCallerManager()

	// 启用执行跟踪
	manager.EnableExecutionTracing(true)

	// 添加跟踪回调
	traceEvents := make([]string, 0)
	manager.AddExecutionTraceCallback(func(trace *PluginExecutionTrace) {
		traceEvents = append(traceEvents, string(trace.Status))
		log.Infof("跟踪事件: 插件[%s] Hook[%s] 状态[%s] TraceID[%s]",
			trace.PluginID, trace.HookName, trace.Status, trace.TraceID)
	})

	// 创建测试插件
	testScript := &schema.YakScript{
		ScriptName: "test-plugin",
		Content: `
mirrorHTTPFlow = func(isHttps, url, req, rsp, body) {
	yakit_output("测试插件执行: " + url)
}
`,
		Type: "mitm",
	}

	// 添加插件 (现在不会立即创建跟踪记录)
	err := manager.Add(context.Background(), testScript, map[string]any{}, testScript.Content, nil, "mirrorHTTPFlow")
	if err != nil {
		t.Fatalf("添加插件失败: %v", err)
	}

	// 等待一下让回调处理完成
	time.Sleep(100 * time.Millisecond)

	// 验证插件注册后没有跟踪记录（因为还没有执行）
	allTraces := manager.GetAllExecutionTraces()
	if len(allTraces) != 0 {
		t.Errorf("插件注册后应该没有跟踪记录，实际有%d个", len(allTraces))
	}

	// 执行插件 (此时应该创建新的Trace记录)
	manager.CallByName("mirrorHTTPFlow", true, "http://example.com", []byte("request"), []byte("response"), []byte("body"))
	time.Sleep(100 * time.Millisecond)

	// 验证执行后创建了跟踪记录
	allTraces = manager.GetAllExecutionTraces()
	if len(allTraces) != 1 {
		t.Fatalf("执行后应该有1个跟踪记录，实际有%d个", len(allTraces))
	}

	trace := allTraces[0]
	if trace.PluginID != "test-plugin" {
		t.Errorf("期望插件ID为test-plugin, 实际为: %s", trace.PluginID)
	}

	if trace.HookName != "mirrorHTTPFlow" {
		t.Errorf("期望Hook名为mirrorHTTPFlow, 实际为: %s", trace.HookName)
	}

	// 再次执行相同的插件 (应该创建新的Trace记录)
	manager.CallByName("mirrorHTTPFlow", true, "http://example2.com", []byte("request2"), []byte("response2"), []byte("body2"))
	time.Sleep(100 * time.Millisecond)

	// 验证现在有两个独立的跟踪记录
	allTraces = manager.GetAllExecutionTraces()
	if len(allTraces) != 2 {
		t.Fatalf("两次执行后应该有2个跟踪记录，实际有%d个", len(allTraces))
	}

	// 验证两个Trace记录有不同的TraceID
	trace1 := allTraces[0]
	trace2 := allTraces[1]
	if trace1.TraceID == trace2.TraceID {
		t.Error("两次执行应该有不同的TraceID")
	}

	// 验证两个Trace记录都是同一个插件和Hook
	if trace1.PluginID != trace2.PluginID || trace1.HookName != trace2.HookName {
		t.Error("两个Trace记录应该属于同一个插件和Hook")
	}

	// 验证每个Trace都有独立的执行参数
	if len(trace1.Args) == 0 || len(trace2.Args) == 0 {
		t.Error("每个Trace应该都有执行参数记录")
	}

	t.Logf("测试完成，总跟踪记录数: %d", len(allTraces))
}

// TestPluginExecutionTracingLifecycle 测试插件调用生命周期跟踪
func TestPluginExecutionTracingLifecycle(t *testing.T) {
	manager := NewYakToCallerManager()
	manager.EnableExecutionTracing(true)

	// 创建测试插件
	testScript := &schema.YakScript{
		ScriptName: "lifecycle-test-plugin",
		Content: `
beforeRequest = func(isHttps, originReq, req) {
	yakit_output("beforeRequest 执行")
	return req
}

afterRequest = func(isHttps, originReq, req, originRsp, rsp) {
	yakit_output("afterRequest 执行")
	return rsp
}
`,
		Type: "mitm",
	}

	// 添加插件
	err := manager.Add(context.Background(), testScript, map[string]any{}, testScript.Content, nil, "beforeRequest", "afterRequest")
	if err != nil {
		t.Fatalf("添加插件失败: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// 验证插件注册后没有跟踪记录（因为还没有执行）
	allTraces := manager.GetAllExecutionTraces()
	if len(allTraces) != 0 {
		t.Errorf("插件注册后应该没有跟踪记录，实际有%d个", len(allTraces))
	}

	// 执行beforeRequest Hook
	manager.CallByName("beforeRequest", true, []byte("original"), []byte("modified"))
	time.Sleep(100 * time.Millisecond)

	// 验证执行后创建了跟踪记录
	allTraces = manager.GetAllExecutionTraces()
	if len(allTraces) != 1 {
		t.Fatalf("执行beforeRequest后应该有1个跟踪记录, 实际有%d个", len(allTraces))
	}

	// 验证第一个跟踪记录
	trace1 := allTraces[0]
	if trace1.HookName != "beforeRequest" {
		t.Errorf("期望Hook名为beforeRequest, 实际为: %s", trace1.HookName)
	}

	// 执行afterRequest Hook
	manager.CallByName("afterRequest", true, []byte("original"), []byte("request"), []byte("original"), []byte("response"))
	time.Sleep(100 * time.Millisecond)

	// 验证现在有两个跟踪记录
	allTraces = manager.GetAllExecutionTraces()
	if len(allTraces) != 2 {
		t.Fatalf("执行两个Hook后应该有2个跟踪记录, 实际有%d个", len(allTraces))
	}

	// 测试按Hook名查询
	beforeTraces := manager.GetExecutionTracesByHook("beforeRequest")
	if len(beforeTraces) != 1 {
		t.Errorf("beforeRequest应该有1个跟踪记录, 实际有%d个", len(beforeTraces))
	}

	// 测试按插件ID查询
	pluginTraces := manager.GetExecutionTracesByPlugin("lifecycle-test-plugin")
	if len(pluginTraces) != 2 {
		t.Errorf("插件应该有2个跟踪记录, 实际有%d个", len(pluginTraces))
	}

	// 再次执行beforeRequest Hook（应该创建新的Trace记录）
	manager.CallByName("beforeRequest", true, []byte("original2"), []byte("modified2"))
	time.Sleep(100 * time.Millisecond)

	// 验证现在beforeRequest有2个跟踪记录
	updatedBeforeTraces := manager.GetExecutionTracesByHook("beforeRequest")
	if len(updatedBeforeTraces) != 2 {
		t.Errorf("beforeRequest应该有2个跟踪记录, 实际有%d个", len(updatedBeforeTraces))
	}

	// 验证总跟踪记录数增加到3个
	allTraces = manager.GetAllExecutionTraces()
	if len(allTraces) != 3 {
		t.Errorf("总共应该有3个跟踪记录, 实际有%d个", len(allTraces))
	}

	t.Logf("生命周期测试完成，总跟踪记录数: %d", len(manager.GetAllExecutionTraces()))
}

// TestPluginExecutionTracingPerformance 测试插件执行跟踪的性能优化
func TestPluginExecutionTracingPerformance(t *testing.T) {
	manager := NewYakToCallerManager()
	manager.EnableExecutionTracing(true)

	// 创建多个测试插件来验证索引性能
	plugins := []string{"plugin1", "plugin2", "plugin3"}
	hooks := []string{"mirrorHTTPFlow", "beforeRequest", "afterRequest"}

	// 添加多个插件和Hook组合
	for _, pluginName := range plugins {
		for _, hookName := range hooks {
			var funcContent string
			switch hookName {
			case "mirrorHTTPFlow":
				funcContent = fmt.Sprintf(`
%s = func(isHttps, url, req, rsp, body) {
	yakit_output("测试插件 %s 的 %s Hook: " + url)
}
`, hookName, pluginName, hookName)
			case "beforeRequest":
				funcContent = fmt.Sprintf(`
%s = func(isHttps, originReq, req) {
	yakit_output("测试插件 %s 的 %s Hook")
	return req
}
`, hookName, pluginName, hookName)
			case "afterRequest":
				funcContent = fmt.Sprintf(`
%s = func(isHttps, originReq, req, originRsp, rsp) {
	yakit_output("测试插件 %s 的 %s Hook")
	return rsp
}
`, hookName, pluginName, hookName)
			default:
				funcContent = fmt.Sprintf(`
%s = func() {
	yakit_output("测试插件 %s 的 %s Hook")
}
`, hookName, pluginName, hookName)
			}

			testScript := &schema.YakScript{
				ScriptName: pluginName,
				Content:    funcContent,
				Type:       "mitm",
			}

			err := manager.Add(context.Background(), testScript, map[string]any{}, testScript.Content, nil, hookName)
			if err != nil {
				t.Fatalf("添加插件 %s 失败: %v", pluginName, err)
			}
		}
	}

	time.Sleep(100 * time.Millisecond)

	// 验证插件注册后没有跟踪记录
	allTraces := manager.GetAllExecutionTraces()
	if len(allTraces) != 0 {
		t.Errorf("插件注册后应该没有跟踪记录，实际有%d个", len(allTraces))
	}

	// 执行每个插件的每个Hook一次来创建跟踪记录
	for _, pluginName := range plugins {
		for _, hookName := range hooks {
			// 构造插件特定的Hook名称
			specificHookName := hookName
			switch hookName {
			case "mirrorHTTPFlow":
				manager.CallByName(specificHookName, true, fmt.Sprintf("http://%s.com", pluginName), []byte("request"), []byte("response"), []byte("body"))
			case "beforeRequest":
				manager.CallByName(specificHookName, true, []byte("original"), []byte("modified"))
			case "afterRequest":
				manager.CallByName(specificHookName, true, []byte("original"), []byte("request"), []byte("original"), []byte("response"))
			}
		}
	}

	time.Sleep(200 * time.Millisecond)

	// 验证总跟踪记录数
	// 每次调用Hook时，所有注册了该Hook的插件都会被执行
	// 所以总数 = plugins数量 × hooks数量 × plugins数量（因为每个plugin名调用一次，触发所有plugin）
	allTraces = manager.GetAllExecutionTraces()
	expectedCount := len(plugins) * len(hooks) * len(plugins)
	if len(allTraces) != expectedCount {
		t.Errorf("期望有 %d 个跟踪记录, 实际有 %d 个", expectedCount, len(allTraces))
	}

	// 测试按插件ID查询的性能 - 应该是O(1)查找
	// 每个插件被调用了 hooks数量 × plugins数量 次
	plugin1Traces := manager.GetExecutionTracesByPlugin("plugin1")
	expectedPlugin1Count := len(hooks) * len(plugins)
	if len(plugin1Traces) != expectedPlugin1Count {
		t.Errorf("plugin1 应该有 %d 个跟踪记录, 实际有 %d 个", expectedPlugin1Count, len(plugin1Traces))
	}

	// 测试按Hook名查询的性能 - 应该是O(1)查找
	// 每个Hook被调用了 plugins数量 × plugins数量 次
	mirrorTraces := manager.GetExecutionTracesByHook("mirrorHTTPFlow")
	expectedMirrorCount := len(plugins) * len(plugins)
	if len(mirrorTraces) != expectedMirrorCount {
		t.Errorf("mirrorHTTPFlow 应该有 %d 个跟踪记录, 实际有 %d 个", expectedMirrorCount, len(mirrorTraces))
	}

	// 测试精确查找特定插件和Hook组合 - 应该是O(1)查找
	tracker := manager.GetExecutionTracker()
	specificTrace := tracker.FindTraceByPluginAndHook("plugin2", "beforeRequest")
	if specificTrace == nil {
		t.Error("应该能找到 plugin2 的 beforeRequest 跟踪记录")
	} else {
		if specificTrace.PluginID != "plugin2" || specificTrace.HookName != "beforeRequest" {
			t.Errorf("查找结果不正确: 期望 plugin2.beforeRequest, 实际 %s.%s",
				specificTrace.PluginID, specificTrace.HookName)
		}
	}

	// 测试不存在的组合
	nonExistentTrace := tracker.FindTraceByPluginAndHook("nonexistent", "nonexistent")
	if nonExistentTrace != nil {
		t.Error("不应该找到不存在的跟踪记录")
	}

	t.Logf("性能测试完成，创建了 %d 个跟踪记录", len(allTraces))
}

// TestPluginExecutionErrorHandling 测试插件执行错误处理
func TestPluginExecutionErrorHandling(t *testing.T) {
	manager := NewYakToCallerManager()
	manager.EnableExecutionTracing(true)

	traceEvents := make([]*PluginExecutionTrace, 0)
	var mu sync.Mutex
	manager.AddExecutionTraceCallback(func(trace *PluginExecutionTrace) {
		mu.Lock()
		defer mu.Unlock()
		traceEvents = append(traceEvents, trace)
	})

	t.Run("插件语法错误", func(t *testing.T) {
		testScript := &schema.YakScript{
			ScriptName: "syntax-error-plugin",
			Content: `
beforeRequest = func(isHttps, originReq, req) {
	// 故意的语法错误 - 缺少闭合括号
	if (true {
		yakit_output("语法错误测试")
	}
	return req
}
`,
			Type: "mitm",
		}

		// 这个插件在Add阶段就应该失败，因为有语法错误
		err := manager.Add(context.Background(), testScript, map[string]any{}, testScript.Content, nil, "beforeRequest")
		if err != nil {
			t.Logf("成功捕获语法错误: %s", err)
			return // 语法错误在Add阶段就被捕获了，这是正确的行为
		}

		t.Logf("语法错误插件意外添加成功，将测试执行阶段的错误处理")

		time.Sleep(100 * time.Millisecond)

		mu.Lock()
		traceEvents = traceEvents[:0]
		mu.Unlock()

		manager.CallByName("beforeRequest", true, []byte("original"), []byte("request"))
		time.Sleep(200 * time.Millisecond)

		mu.Lock()
		defer mu.Unlock()

		// 如果Add成功了，那么执行应该会失败
		found := false
		for _, trace := range traceEvents {
			if trace.Status == PluginStatusFailed && trace.Error != "" {
				found = true
				t.Logf("成功捕获执行错误: %s", trace.Error)
				break
			}
		}

		if !found {
			t.Error("没有捕获到预期的错误")
			for _, trace := range traceEvents {
				t.Logf("跟踪事件: 状态=%s, 错误=%s", trace.Status, trace.Error)
			}
		}
	})

	t.Run("运行时错误", func(t *testing.T) {
		testScript := &schema.YakScript{
			ScriptName: "error-plugin-runtime",
			Content: `
beforeRequest = func(isHttps, originReq, req) {
	// 故意的运行时错误
	undefined_variable.call()
	return req
}
`,
			Type: "mitm",
		}

		err := manager.Add(context.Background(), testScript, map[string]any{}, testScript.Content, nil, "beforeRequest")
		if err != nil {
			t.Fatalf("添加插件失败: %v", err)
		}

		time.Sleep(100 * time.Millisecond)

		mu.Lock()
		traceEvents = traceEvents[:0]
		mu.Unlock()

		manager.CallByName("beforeRequest", true, []byte("original"), []byte("request"))
		time.Sleep(200 * time.Millisecond)

		mu.Lock()
		defer mu.Unlock()
		found := false
		for _, trace := range traceEvents {
			if trace.Status == PluginStatusFailed && trace.Error != "" {
				found = true
				t.Logf("成功捕获运行时错误: %s", trace.Error)
				break
			}
		}

		if !found {
			t.Error("没有捕获到预期的运行时错误")
			for _, trace := range traceEvents {
				t.Logf("跟踪事件: 状态=%s, 错误=%s", trace.Status, trace.Error)
			}
		}
	})

	t.Run("超时错误", func(t *testing.T) {
		manager.SetCallPluginTimeout(0.1)      // 100ms
		defer manager.SetCallPluginTimeout(10) // 恢复

		testScript := &schema.YakScript{
			ScriptName: "timeout-plugin",
			Content: `
beforeRequest = func(isHttps, originReq, req) {
	sleep(0.5) // 睡眠500ms，会超过100ms的超时限制
	yakit_output("这不应该被执行到")
	return req
}
`,
			Type: "mitm",
		}

		err := manager.Add(context.Background(), testScript, map[string]any{}, testScript.Content, nil, "beforeRequest")
		if err != nil {
			t.Fatalf("添加插件失败: %v", err)
		}

		time.Sleep(100 * time.Millisecond)

		mu.Lock()
		traceEvents = traceEvents[:0]
		mu.Unlock()

		manager.CallByName("beforeRequest", true, []byte("original"), []byte("request"))
		time.Sleep(300 * time.Millisecond)

		mu.Lock()
		defer mu.Unlock()
		found := false
		for _, trace := range traceEvents {
			if trace.Status == PluginStatusFailed &&
				(strings.Contains(trace.Error, "timeout") || strings.Contains(trace.Error, "deadline exceeded")) {
				found = true
				t.Logf("成功捕获超时错误: %s", trace.Error)
				break
			}
		}

		if !found {
			t.Error("没有捕获到预期的超时错误")
			for _, trace := range traceEvents {
				t.Logf("跟踪事件: 状态=%s, 错误=%s", trace.Status, trace.Error)
			}
		}
	})
}

// TestPluginExecutionCancellation 测试插件执行取消
func TestPluginExecutionCancellation(t *testing.T) {
	manager := NewYakToCallerManager()
	manager.EnableExecutionTracing(true)

	traceEvents := make([]*PluginExecutionTrace, 0)
	var mu sync.Mutex
	manager.AddExecutionTraceCallback(func(trace *PluginExecutionTrace) {
		mu.Lock()
		defer mu.Unlock()
		traceEvents = append(traceEvents, trace)
		t.Logf("跟踪事件: 插件[%s] Hook[%s] 状态[%s]", trace.PluginID, trace.HookName, trace.Status)
	})

	// 创建一个会长时间运行的插件
	testScript := &schema.YakScript{
		ScriptName: "long-running-plugin",
		Content: `
mirrorHTTPFlow = func(isHttps, url, req, rsp, body) {
	for i = 0; i < 100; i++ {
		sleep(0.1) // 每次睡眠100ms，总共10秒
		yakit_output(sprintf("执行中... %d/100", i))
	}
	yakit_output("执行完成")
}
`,
		Type: "mitm",
	}

	err := manager.Add(context.Background(), testScript, map[string]any{}, testScript.Content, nil, "mirrorHTTPFlow")
	if err != nil {
		t.Fatalf("添加插件失败: %v", err)
	}

	// 启动插件执行
	go func() {
		manager.CallByName("mirrorHTTPFlow", true, "http://example.com", []byte("request"), []byte("response"), []byte("body"))
	}()

	// 等待插件开始执行
	time.Sleep(2 * time.Second)
	// 获取跟踪记录
	traces := manager.GetAllExecutionTraces()
	if len(traces) == 0 {
		t.Fatal("应该有跟踪记录")
	}
	trace := traces[0]
	// 验证插件正在运行
	runningTraces := manager.GetRunningExecutionTraces()
	if len(runningTraces) == 0 {
		t.Error("应该有正在运行的跟踪记录")
	}

	// 取消执行
	success := manager.CancelExecutionTrace(trace.TraceID)
	if !success {
		t.Error("取消执行应该成功")
	}

	// 等待取消生效
	time.Sleep(300 * time.Millisecond)

	// 验证状态变为取消
	mu.Lock()
	defer mu.Unlock()

	cancelledFound := false
	for _, event := range traceEvents {
		if event.Status == PluginStatusCancelled {
			cancelledFound = true
			t.Logf("成功取消插件执行: %s", event.Error)
			break
		}
	}

	if !cancelledFound {
		t.Error("没有找到取消状态的跟踪事件")
		for _, event := range traceEvents {
			t.Logf("事件: 状态=%s, 错误=%s", event.Status, event.Error)
		}
	}

	// 验证没有正在运行的跟踪记录
	runningTraces = manager.GetRunningExecutionTraces()
	if len(runningTraces) > 0 {
		t.Error("取消后不应该有正在运行的跟踪记录")
	}
}

// TestPluginExecutionConcurrency 测试插件并发执行跟踪
func TestPluginExecutionConcurrency(t *testing.T) {
	manager := NewYakToCallerManager()
	manager.EnableExecutionTracing(true)

	var traceEvents []*PluginExecutionTrace
	var mu sync.Mutex
	manager.AddExecutionTraceCallback(func(trace *PluginExecutionTrace) {
		mu.Lock()
		defer mu.Unlock()
		traceEvents = append(traceEvents, trace)
	})

	// 创建多个插件
	pluginCount := 5
	for i := 0; i < pluginCount; i++ {
		testScript := &schema.YakScript{
			ScriptName: fmt.Sprintf("concurrent-plugin-%d", i),
			Content: fmt.Sprintf(`
mirrorHTTPFlow = func(isHttps, url, req, rsp, body) {
	sleep(0.1)
	yakit_output("并发插件 %d 执行完成")
}
`, i),
			Type: "mitm",
		}

		err := manager.Add(context.Background(), testScript, map[string]any{}, testScript.Content, nil, "mirrorHTTPFlow")
		if err != nil {
			t.Fatalf("添加插件 %d 失败: %v", i, err)
		}
	}

	// 并发执行所有插件
	var wg sync.WaitGroup
	for i := 0; i < pluginCount; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			manager.CallByName("mirrorHTTPFlow", true, fmt.Sprintf("http://example%d.com", index),
				[]byte("request"), []byte("response"), []byte("body"))
		}(i)
	}

	wg.Wait()
	// 验证所有插件都创建了跟踪记录
	allTraces := manager.GetAllExecutionTraces()
	if len(allTraces) != pluginCount*pluginCount {
		t.Errorf("期望有 %d 个跟踪记录, 实际有 %d 个", pluginCount, len(allTraces))
	}

	// 验证所有插件都执行完成
	mu.Lock()
	traceEvents = allTraces
	defer mu.Unlock()

	completedCount := 0
	pluginCompletedMap := make(map[string]bool)
	for _, event := range traceEvents {
		if event.Status == PluginStatusCompleted {
			completedCount++
			pluginCompletedMap[event.PluginID] = true
		}
	}

	uniqueCompletedCount := len(pluginCompletedMap)
	if uniqueCompletedCount != pluginCount {
		t.Errorf("期望有 %d 个不同插件完成执行, 实际有 %d 个", pluginCount, uniqueCompletedCount)
	}
	t.Logf("并发测试完成，处理了 %d 个并发插件执行", completedCount)
}

// TestPluginExecutionCleanup 测试插件执行跟踪清理
func TestPluginExecutionCleanup(t *testing.T) {
	manager := NewYakToCallerManager()
	manager.EnableExecutionTracing(true)

	// 创建测试插件
	testScript := &schema.YakScript{
		ScriptName: "cleanup-test-plugin",
		Content: `
mirrorHTTPFlow = func(isHttps, url, req, rsp, body) {
	yakit_output("清理测试插件执行")
}
`,
		Type: "mitm",
	}

	err := manager.Add(context.Background(), testScript, map[string]any{}, testScript.Content, nil, "mirrorHTTPFlow")
	if err != nil {
		t.Fatalf("添加插件失败: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// 执行插件
	manager.CallByName("mirrorHTTPFlow", true, "http://example.com", []byte("request"), []byte("response"), []byte("body"))
	time.Sleep(100 * time.Millisecond)

	// 验证有跟踪记录
	allTraces := manager.GetAllExecutionTraces()
	if len(allTraces) == 0 {
		t.Fatal("应该有跟踪记录")
	}

	// 清理已完成的跟踪记录
	manager.CleanupCompletedExecutionTraces(0) // 清理所有已完成的

	time.Sleep(100 * time.Millisecond)

	// 验证跟踪记录被清理
	allTraces = manager.GetAllExecutionTraces()
	for _, trace := range allTraces {
		if trace.Status == PluginStatusCompleted {
			t.Error("已完成的跟踪记录应该被清理")
		}
	}

	t.Logf("清理测试完成，剩余跟踪记录数: %d", len(allTraces))
}

// TestPluginExecutionTracingDisabled 测试禁用跟踪功能
func TestPluginExecutionTracingDisabled(t *testing.T) {
	manager := NewYakToCallerManager()
	// 不启用跟踪

	testScript := &schema.YakScript{
		ScriptName: "disabled-tracing-plugin",
		Content: `
mirrorHTTPFlow = func(isHttps, url, req, rsp, body) {
	yakit_output("禁用跟踪测试")
}
`,
		Type: "mitm",
	}

	err := manager.Add(context.Background(), testScript, map[string]any{}, testScript.Content, nil, "mirrorHTTPFlow")
	if err != nil {
		t.Fatalf("添加插件失败: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// 执行插件
	manager.CallByName("mirrorHTTPFlow", true, "http://example.com", []byte("request"), []byte("response"), []byte("body"))
	time.Sleep(100 * time.Millisecond)

	// 验证没有跟踪记录
	allTraces := manager.GetAllExecutionTraces()
	if len(allTraces) > 0 {
		t.Error("禁用跟踪时不应该有跟踪记录")
	}

	// 验证跟踪功能确实被禁用
	if manager.IsExecutionTracingEnabled() {
		t.Error("跟踪功能应该被禁用")
	}

	t.Log("禁用跟踪测试完成")
}

// TestPluginExecutionMixedScenarios 测试混合场景
func TestPluginExecutionMixedScenarios(t *testing.T) {
	manager := NewYakToCallerManager()
	manager.EnableExecutionTracing(true)

	var traceEvents []*PluginExecutionTrace
	var mu sync.Mutex
	manager.AddExecutionTraceCallback(func(trace *PluginExecutionTrace) {
		mu.Lock()
		defer mu.Unlock()
		traceEvents = append(traceEvents, trace)
	})

	// 创建正常插件
	normalScript := &schema.YakScript{
		ScriptName: "normal-plugin",
		Content: `
mirrorHTTPFlow = func(isHttps, url, req, rsp, body) {
	yakit_output("正常插件执行")
}
`,
		Type: "mitm",
	}

	// 创建错误插件
	errorScript := &schema.YakScript{
		ScriptName: "error-plugin",
		Content: `
mirrorHTTPFlow = func(isHttps, url, req, rsp, body) {
	undefined_variable.call() // 运行时错误
}
`,
		Type: "mitm",
	}

	// 添加插件
	err := manager.Add(context.Background(), normalScript, map[string]any{}, normalScript.Content, nil, "mirrorHTTPFlow")
	if err != nil {
		t.Fatalf("添加正常插件失败: %v", err)
	}

	err = manager.Add(context.Background(), errorScript, map[string]any{}, errorScript.Content, nil, "mirrorHTTPFlow")
	if err != nil {
		t.Fatalf("添加错误插件失败: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// 清空事件记录
	mu.Lock()
	traceEvents = traceEvents[:0]
	mu.Unlock()

	// 执行所有插件
	manager.CallByName("mirrorHTTPFlow", true, "http://example.com", []byte("request"), []byte("response"), []byte("body"))
	time.Sleep(200 * time.Millisecond)

	// 验证结果
	mu.Lock()
	defer mu.Unlock()

	completedCount := 0
	failedCount := 0
	for _, event := range traceEvents {
		switch event.Status {
		case PluginStatusCompleted:
			completedCount++
		case PluginStatusFailed:
			failedCount++
		}
	}

	if completedCount == 0 {
		t.Error("应该有成功完成的插件")
	}
	if failedCount == 0 {
		t.Error("应该有失败的插件")
	}

	t.Logf("混合场景测试完成: 成功 %d 个, 失败 %d 个", completedCount, failedCount)
}

// TestMultipleExecutionTraces 测试同一插件多次执行创建独立Trace记录
func TestMultipleExecutionTraces(t *testing.T) {
	manager := NewYakToCallerManager()
	manager.EnableExecutionTracing(true)

	// 创建测试插件
	testScript := &schema.YakScript{
		ScriptName: "multi-exec-plugin",
		Content: `
mirrorHTTPFlow = func(isHttps, url, req, rsp, body) {
	yakit_output("处理URL: " + url)
	return "processed: " + url
}
`,
		Type: "mitm",
	}

	// 添加插件
	err := manager.Add(context.Background(), testScript, map[string]any{}, testScript.Content, nil, "mirrorHTTPFlow")
	if err != nil {
		t.Fatalf("添加插件失败: %v", err)
	}

	// 执行同一插件的同一Hook多次，每次使用不同的参数
	urls := []string{
		"http://example1.com",
		"http://example2.com",
		"http://example3.com",
	}

	for i, url := range urls {
		t.Logf("执行第%d次调用，URL: %s", i+1, url)
		manager.CallByName("mirrorHTTPFlow", true, url, []byte("request"), []byte("response"), []byte("body"))
		time.Sleep(50 * time.Millisecond) // 确保每次调用完成
	}

	// 等待所有执行完成
	time.Sleep(200 * time.Millisecond)

	// 验证创建了3个独立的Trace记录
	allTraces := manager.GetAllExecutionTraces()
	if len(allTraces) != 3 {
		t.Fatalf("期望有3个跟踪记录，实际有%d个", len(allTraces))
	}

	// 验证每个Trace都有唯一的TraceID
	traceIDs := make(map[string]bool)
	for _, trace := range allTraces {
		if traceIDs[trace.TraceID] {
			t.Errorf("发现重复的TraceID: %s", trace.TraceID)
		}
		traceIDs[trace.TraceID] = true

		// 验证基本信息
		if trace.PluginID != "multi-exec-plugin" {
			t.Errorf("期望插件ID为multi-exec-plugin，实际为: %s", trace.PluginID)
		}
		if trace.HookName != "mirrorHTTPFlow" {
			t.Errorf("期望Hook名为mirrorHTTPFlow，实际为: %s", trace.HookName)
		}

		// 验证执行参数不同
		if len(trace.Args) < 1 {
			t.Error("Trace应该包含执行参数")
		}
	}

	// 验证按插件ID查询能获取到所有3个Trace
	pluginTraces := manager.GetExecutionTracesByPlugin("multi-exec-plugin")
	if len(pluginTraces) != 3 {
		t.Errorf("按插件ID查询应该返回3个Trace，实际返回%d个", len(pluginTraces))
	}

	// 验证按Hook名查询能获取到所有3个Trace
	hookTraces := manager.GetExecutionTracesByHook("mirrorHTTPFlow")
	if len(hookTraces) != 3 {
		t.Errorf("按Hook名查询应该返回3个Trace，实际返回%d个", len(hookTraces))
	}

	// 验证FindTraceByPluginAndHook返回最新的Trace
	latestTrace := manager.GetExecutionTracker().FindTraceByPluginAndHook("multi-exec-plugin", "mirrorHTTPFlow")
	if latestTrace == nil {
		t.Error("FindTraceByPluginAndHook应该返回最新的Trace")
	} else {
		// 验证返回的是最新的Trace（LoadedTime最晚的）
		isLatest := true
		for _, trace := range allTraces {
			if trace.TraceID != latestTrace.TraceID && trace.LoadedTime.After(latestTrace.LoadedTime) {
				isLatest = false
				break
			}
		}
		if !isLatest {
			t.Error("FindTraceByPluginAndHook应该返回最新创建的Trace")
		}
	}

	t.Logf("多次执行测试完成，创建了%d个独立的Trace记录", len(allTraces))
}
