package yakgrpc

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// 测试插件执行超过一定时间被推送到客户端后被用户手动取消
func TestGRPCMUSTPASS_PluginTraceLongRunningAndCancel(t *testing.T) {
	timeThreshold := 3 // 使用3秒，确保有足够时间被检测为长时间运行
	testScript := &schema.YakScript{
		ScriptName: "time-exhausted-hook-call",
		Content: fmt.Sprintf(`
mirrorHTTPFlow = func(isHttps, url, req, rsp, body) {
	time.sleep(%v)
	yakit_output("耗时调用完成")
}
`, timeThreshold),
		Type: "mitm",
	}
	// 创建测试服务器
	client, err := NewLocalClient(true)
	require.NoError(t, err)

	// 设置测试用MixPluginCaller
	testCaller, err := yak.NewMixPluginCaller()
	require.NoError(t, err)
	testCaller.SetCallPluginTimeout(60)
	// 设置较短的长时间运行阈值，加速测试
	testCaller.SetLongRunningThreshold(consts.PluginCallDurationThresholdSecondsForTest)
	mitmPluginCallerGlobal = testCaller
	mitmPluginCallerNotifyChan = make(chan struct{})
	defer func() {
		mitmPluginCallerGlobal = nil
		mitmPluginCallerNotifyChan = nil
	}()

	// 启动PluginTrace流
	traceCtx, traceCancel := context.WithCancel(context.Background())
	defer traceCancel()

	traceStream, err := client.PluginTrace(traceCtx)
	require.NoError(t, err)

	// 用于收集trace响应的通道
	traceResponses := make(chan *ypb.PluginTraceResponse, 100)
	traceErrors := make(chan error, 1)

	// 启动goroutine接收trace响应
	go func() {
		defer close(traceResponses)
		for {
			resp, err := traceStream.Recv()
			if err != nil {
				if err != io.EOF {
					traceErrors <- err
				}
				return
			}
			traceResponses <- resp
		}
	}()

	// 启动stream
	err = traceStream.Send(&ypb.PluginTraceRequest{
		ControlMode:   "start_stream",
		EnableTracing: true,
	})
	require.NoError(t, err)

	waitForResponse := func(expectedType string) {
		select {
		case resp := <-traceResponses:
			assert.Equal(t, expectedType, resp.ResponseType)
			assert.True(t, resp.Success)
		case err := <-traceErrors:
			t.Fatalf("接收trace响应失败: %v", err)
		case <-time.After(5 * time.Second):
			t.Fatal("未收到trace响应")
		}
	}

	// 等待第一个响应
	waitForResponse("control_result")

	err = testCaller.LoadPluginEx(traceCtx, testScript)
	require.NoError(t, err)

	// 启动长时间运行的插件
	go func() {
		// 调用长时间运行的插件
		testCaller.GetNativeCaller().CallByName("mirrorHTTPFlow",
			false, "http://example.com/long", []byte("GET / HTTP/1.1"), []byte("HTTP/1.1 200 OK"), []byte("body"))
	}()

	var longRunningTraceID string
	var statsReceived bool

	// 等待并验证trace响应
	timeout := time.After(10 * time.Second)
	for {
		select {
		case resp := <-traceResponses:
			switch resp.ResponseType {
			case "trace_update":
				if len(resp.Traces) > 0 {
					trace := resp.Traces[0]
					log.Infof("收到trace更新: ID=%s, Plugin=%s, Status=%s, Duration=%dms",
						trace.TraceID, trace.PluginID, trace.Status, trace.DurationMs)

					if trace.PluginID == "time-exhausted-hook-call" && trace.Status == "running" {
						longRunningTraceID = trace.TraceID
						assert.NotEmpty(t, trace.TraceID)
						assert.Equal(t, "mirrorHTTPFlow", trace.HookName)
						assert.Equal(t, "running", trace.Status)
						assert.Greater(t, trace.DurationMs, int64(consts.PluginCallDurationThresholdSecondsForTest*1000)) // 应该超过1秒阈值

						log.Infof("检测到长时间运行的trace: %s，准备取消", longRunningTraceID)

						// 发送取消请求
						err = traceStream.Send(&ypb.PluginTraceRequest{
							ControlMode: "cancel_trace",
							TraceID:     longRunningTraceID,
						})
						require.NoError(t, err)
						continue
					}

					if trace.TraceID == longRunningTraceID && trace.Status == "cancelled" {
						log.Infof("成功收到取消后的trace状态: %s", trace.Status)
						assert.Equal(t, "cancelled", trace.Status)
						// 测试成功，可以退出
						goto testComplete
					}
					t.Fatal("Unexpected Trace rsp")
				}

			case "stats_update":
				if !statsReceived {
					statsReceived = true
					log.Infof("收到stats更新: Total=%d, Running=%d, Completed=%d, Failed=%d, Cancelled=%d",
						resp.Stats.TotalTraces, resp.Stats.RunningTraces, resp.Stats.CompletedTraces,
						resp.Stats.FailedTraces, resp.Stats.CancelledTraces)
					assert.NotNil(t, resp.Stats)
				}

			case "control_result":
				if longRunningTraceID != "" {
					log.Infof("收到控制操作结果: Success=%t, Message=%s", resp.Success, resp.Message)
					assert.True(t, resp.Success, "取消操作应该成功")
				}

			case "tracing_status":
				log.Infof("收到tracing状态: Success=%t", resp.Success)
				assert.True(t, resp.Success)
			}

		case err := <-traceErrors:
			t.Fatalf("接收trace响应出错: %v", err)

		case <-timeout:
			if longRunningTraceID == "" {
				t.Fatal("10秒内未检测到长时间运行的trace")
			} else {
				t.Fatal("10秒内未收到取消确认")
			}
		}
	}

testComplete:
	log.Info("PluginTrace长时间运行和取消测试完成")
}

// 测试插件执行失败的情况 先因长时间调用推送给前端后执行失败
func TestGRPCMUSTPASS_PluginTraceExecutionFailed(t *testing.T) {
	timeThreshold := 3  // 使用2秒，超过1秒阈值
	maxNotifyTime := 10 // 总等待时间
	testScript := &schema.YakScript{
		ScriptName: "error-plugin",
		Content: fmt.Sprintf(`
mirrorHTTPFlow = func(isHttps, url, req, rsp, body) {
	yakit_output("插件开始执行，%v秒后出错")
	time.sleep(%v)  // 超过5秒阈值
	die("模拟插件执行错误")
}
`, timeThreshold, timeThreshold),
		Type: "mitm",
	}

	client, err := NewLocalClient(true)
	require.NoError(t, err)

	testCaller, err := yak.NewMixPluginCaller()
	require.NoError(t, err)
	testCaller.SetCallPluginTimeout(60)
	// 设置较短的长时间运行阈值，加速测试
	testCaller.SetLongRunningThreshold(consts.PluginCallDurationThresholdSecondsForTest)
	mitmPluginCallerGlobal = testCaller
	mitmPluginCallerNotifyChan = make(chan struct{})
	defer func() {
		mitmPluginCallerGlobal = nil
		mitmPluginCallerNotifyChan = nil
	}()

	traceCtx, traceCancel := context.WithCancel(context.Background())
	defer traceCancel()

	traceStream, err := client.PluginTrace(traceCtx)
	require.NoError(t, err)

	traceResponses := make(chan *ypb.PluginTraceResponse, 100)
	go func() {
		defer close(traceResponses)
		for {
			resp, err := traceStream.Recv()
			if err != nil {
				return
			}
			traceResponses <- resp
		}
	}()

	// 启用tracing和stream
	err = traceStream.Send(&ypb.PluginTraceRequest{
		ControlMode:   "start_stream",
		EnableTracing: true,
	})
	require.NoError(t, err)

	// 等待初始响应
	<-traceResponses // control_result

	err = testCaller.LoadPluginEx(traceCtx, testScript)
	require.NoError(t, err)

	// 启动会失败的插件
	go func() {
		testCaller.GetNativeCaller().CallByName("mirrorHTTPFlow",
			false, "http://example.com/error", []byte("GET / HTTP/1.1"), []byte("HTTP/1.1 200 OK"), []byte("body"))
	}()

	receivedStatuses := []string{}
	timeout := time.After(time.Duration(maxNotifyTime) * time.Second)
	var traceID string
	for {
		select {
		case resp := <-traceResponses:
			if resp.ResponseType == "trace_update" && len(resp.Traces) > 0 {
				trace := resp.Traces[0]
				if trace.PluginID == "error-plugin" {
					traceID = trace.TraceID
					receivedStatuses = append(receivedStatuses, trace.Status)
					log.Infof("收到trace状态: %s, 错误信息: %s", trace.Status, trace.ErrorMessage)

					if trace.Status == "failed" {
						assert.Equal(t, traceID, trace.TraceID) // Running -> Failed TraceID不变
						assert.NotEmpty(t, trace.ErrorMessage, "failed状态应该包含错误信息")
						assert.Contains(t, trace.ErrorMessage, "模拟插件执行错误")
						goto testComplete
					}
				}
			}
		case <-timeout:
			t.Fatal("10秒内未收到failed状态")
		}
	}

testComplete:
	// 验证状态变化序列
	assert.Contains(t, receivedStatuses, "running", "应该收到running状态")
	assert.Contains(t, receivedStatuses, "failed", "应该收到failed状态")
	log.Info("插件执行失败测试完成")
}

// 测试快速完成的插件不会被推送
func TestGRPCMUSTPASS_PluginTraceCompletionNotPushed(t *testing.T) {
	testScript := &schema.YakScript{
		ScriptName: "fast-plugin",
		Content: `
mirrorHTTPFlow = func(isHttps, url, req, rsp, body) {
	yakit_output("快速完成的插件")
	// 不添加sleep，应该快速完成，不会被推送
}
`,
		Type: "mitm",
	}

	client, err := NewLocalClient(true)
	require.NoError(t, err)

	testCaller, err := yak.NewMixPluginCaller()
	require.NoError(t, err)
	testCaller.SetCallPluginTimeout(60)
	// 设置较短的长时间运行阈值，加速测试
	testCaller.SetLongRunningThreshold(1)
	mitmPluginCallerGlobal = testCaller
	mitmPluginCallerNotifyChan = make(chan struct{})
	defer func() {
		mitmPluginCallerGlobal = nil
		mitmPluginCallerNotifyChan = nil
	}()

	traceCtx, traceCancel := context.WithCancel(context.Background())
	defer traceCancel()

	traceStream, err := client.PluginTrace(traceCtx)
	require.NoError(t, err)

	traceResponses := make(chan *ypb.PluginTraceResponse, 100)
	go func() {
		defer close(traceResponses)
		for {
			resp, err := traceStream.Recv()
			if err != nil {
				return
			}
			traceResponses <- resp
		}
	}()

	// 启用tracing和stream
	err = traceStream.Send(&ypb.PluginTraceRequest{
		ControlMode:   "start_stream",
		EnableTracing: true,
	})
	require.NoError(t, err)

	// 等待初始响应
	<-traceResponses // control_result

	err = testCaller.LoadPluginEx(traceCtx, testScript)
	require.NoError(t, err)

	// 执行快速插件
	go func() {
		testCaller.GetNativeCaller().CallByName("mirrorHTTPFlow",
			false, "http://example.com/fast", []byte("GET / HTTP/1.1"), []byte("HTTP/1.1 200 OK"), []byte("body"))
	}()

	// 等待足够时间，确保如果有trace会被推送
	timeout := time.After(3 * time.Second) // 快速插件应该在1秒内完成
	receivedFastPluginTrace := false

	for {
		select {
		case resp := <-traceResponses:
			if resp.ResponseType == "trace_update" {
				for _, trace := range resp.Traces {
					if trace.PluginID == "fast-plugin" {
						receivedFastPluginTrace = true
						t.Errorf("快速完成的插件不应该被推送，但收到了trace: %s", trace.Status)
					}
				}
				t.Fatal("Unexpected trace update rsp")
			}
		case <-timeout:
			goto testComplete
		}
	}

testComplete:
	assert.False(t, receivedFastPluginTrace, "快速完成的插件不应该被推送")
	log.Info("快速完成插件测试完成")
}

// 测试多个并发插件执行
func TestGRPCMUSTPASS_PluginTraceConcurrentPlugins(t *testing.T) {
	timeThreshold := 3 // 使用3秒，超过1秒阈值
	// 创建多个不同的插件
	longPlugin := &schema.YakScript{
		ScriptName: "long-plugin-1",
		Content: fmt.Sprintf(`
mirrorHTTPFlow = func(isHttps, url, req, rsp, body) {
	yakit_output("长时间插件1开始")
	time.sleep(%v)
	yakit_output("长时间插件1完成")
}
`, timeThreshold),
		Type: "mitm",
	}

	errorPlugin := &schema.YakScript{
		ScriptName: "error-plugin-2",
		Content: fmt.Sprintf(`
mirrorHTTPFlow = func(isHttps, url, req, rsp, body) {
	yakit_output("错误插件开始")
	time.sleep(%v)
	die("并发测试错误")
}
`, timeThreshold),
		Type: "mitm",
	}

	client, err := NewLocalClient(true)
	require.NoError(t, err)

	testCaller, err := yak.NewMixPluginCaller()
	require.NoError(t, err)
	testCaller.SetCallPluginTimeout(60)
	// 设置较短的长时间运行阈值，加速测试
	testCaller.SetLongRunningThreshold(consts.PluginCallDurationThresholdSecondsForTest)
	err = testCaller.SetConcurrent(20)
	require.NoError(t, err)
	mitmPluginCallerGlobal = testCaller
	mitmPluginCallerNotifyChan = make(chan struct{})
	defer func() {
		mitmPluginCallerGlobal = nil
		mitmPluginCallerNotifyChan = nil
	}()

	traceCtx, traceCancel := context.WithCancel(context.Background())
	defer traceCancel()

	traceStream, err := client.PluginTrace(traceCtx)
	require.NoError(t, err)

	traceResponses := make(chan *ypb.PluginTraceResponse, 100)
	go func() {
		defer close(traceResponses)
		for {
			resp, err := traceStream.Recv()
			if err != nil {
				return
			}
			traceResponses <- resp
		}
	}()

	// 启用tracing和stream
	err = traceStream.Send(&ypb.PluginTraceRequest{
		ControlMode:   "start_stream",
		EnableTracing: true,
	})
	require.NoError(t, err)

	// 等待初始响应
	<-traceResponses // control_result

	// 加载插件
	err = testCaller.LoadPluginEx(traceCtx, longPlugin)
	require.NoError(t, err)
	err = testCaller.LoadPluginEx(traceCtx, errorPlugin)
	require.NoError(t, err)

	// 并发执行插件
	go func() {
		// 同时启动两个插件
		testCaller.GetNativeCaller().CallByName("mirrorHTTPFlow",
			false, "http://example.com/", []byte("GET / HTTP/1.1"), []byte("HTTP/1.1 200 OK"), []byte("body"))
	}()

	receivedTraces := make(map[string][]string) // pluginID -> statuses
	timeout := time.After(10 * time.Second)     // 基于2秒插件执行时间
	expectedPlugins := map[string]bool{"long-plugin-1": false, "error-plugin-2": false}

	for {
		select {
		case resp := <-traceResponses:
			if resp.ResponseType == "trace_update" && len(resp.Traces) > 0 {
				for _, trace := range resp.Traces {
					pluginID := trace.PluginID
					if _, exists := expectedPlugins[pluginID]; exists {
						if _, exists := receivedTraces[pluginID]; !exists {
							receivedTraces[pluginID] = []string{}
						}
						receivedTraces[pluginID] = append(receivedTraces[pluginID], trace.Status)
						log.Infof("收到并发trace: Plugin=%s, Status=%s", pluginID, trace.Status)

						// 检查是否收到终态
						if trace.Status == "completed" || trace.Status == "failed" {
							expectedPlugins[pluginID] = true
						}
					}
				}
			}
		case <-timeout:
			goto testComplete
		}

		// 检查是否所有插件都完成了
		allCompleted := true
		for _, completed := range expectedPlugins {
			if !completed {
				allCompleted = false
				break
			}
		}
		if allCompleted {
			break
		}
	}

testComplete:
	// 验证结果
	assert.GreaterOrEqual(t, len(receivedTraces), 2, "应该收到两个插件的trace")

	for pluginID, statuses := range receivedTraces {
		log.Infof("插件 %s 的状态变化: %v", pluginID, statuses)
		assert.Contains(t, statuses, "running", "每个插件都应该有running状态")

		if pluginID == "long-plugin-1" {
			assert.Contains(t, statuses, "completed", "长时间插件应该完成")
		} else if pluginID == "error-plugin-2" {
			assert.Contains(t, statuses, "failed", "错误插件应该失败")
		}
	}

	log.Info("并发插件测试完成")
}

// 测试统计信息的准确性同时验证客户端被推送Running状态的Trace后会受到其Complete的状态变更
func TestGRPCMUSTPASS_PluginTraceStatsAccuracy(t *testing.T) {
	timeThreshold := consts.PluginCallDurationThresholdSecondsForTest + 5
	testScript := &schema.YakScript{
		ScriptName: "stats-test-plugin",
		Content: fmt.Sprintf(`
mirrorHTTPFlow = func(isHttps, url, req, rsp, body) {
	yakit_output("统计测试插件")
	time.sleep(%v)
	yakit_output("统计测试完成")
}
`, timeThreshold),
		Type: "mitm",
	}

	client, err := NewLocalClient(true)
	require.NoError(t, err)

	testCaller, err := yak.NewMixPluginCaller()
	require.NoError(t, err)
	testCaller.SetCallPluginTimeout(60)
	// 设置较短的长时间运行阈值，加速测试
	testCaller.SetLongRunningThreshold(consts.PluginCallDurationThresholdSecondsForTest)
	mitmPluginCallerGlobal = testCaller
	mitmPluginCallerNotifyChan = make(chan struct{})
	defer func() {
		mitmPluginCallerGlobal = nil
		mitmPluginCallerNotifyChan = nil
	}()

	traceCtx, traceCancel := context.WithCancel(context.Background())
	defer traceCancel()

	traceStream, err := client.PluginTrace(traceCtx)
	require.NoError(t, err)

	traceResponses := make(chan *ypb.PluginTraceResponse, 100)
	go func() {
		defer close(traceResponses)
		for {
			resp, err := traceStream.Recv()
			if err != nil {
				return
			}
			traceResponses <- resp
		}
	}()

	// 启用stream
	err = traceStream.Send(&ypb.PluginTraceRequest{
		ControlMode:   "start_stream",
		EnableTracing: true,
	})
	require.NoError(t, err)

	// 等待初始响应
	<-traceResponses // control_result

	err = testCaller.LoadPluginEx(traceCtx, testScript)
	require.NoError(t, err)

	// 执行插件
	go func() {
		time.Sleep(time.Second)
		testCaller.GetNativeCaller().CallByName("mirrorHTTPFlow",
			false, "http://example.com/stats", []byte("GET / HTTP/1.1"), []byte("HTTP/1.1 200 OK"), []byte("body"))
	}()

	var finalStats *ypb.PluginTraceStats
	seenRunning := false
	seenCompleted := false
	timeout := time.After(35 * time.Second)

	for {
		select {
		case resp := <-traceResponses:
			if resp.ResponseType == "stats_update" && resp.Stats != nil {
				finalStats = resp.Stats
				log.Infof("收到统计更新: Total=%d, Running=%d, Completed=%d",
					resp.Stats.TotalTraces, resp.Stats.RunningTraces, resp.Stats.CompletedTraces)
				if seenCompleted && resp.Stats.CompletedTraces == 1 {
					goto testComplete
				}
			} else if resp.ResponseType == "trace_update" && len(resp.Traces) > 0 {
				trace := resp.Traces[0]
				log.Infof("收到trace状态: %s, 错误信息: %s", trace.Status, trace.ErrorMessage)
				if trace.PluginID == "stats-test-plugin" {
					if trace.Status == "running" {
						seenRunning = true
					} else if trace.Status == "completed" {
						seenCompleted = true
					}
				}
			}
		case <-timeout:
			goto testComplete
		}
	}

testComplete:
	// 验证统计信息
	require.NotNil(t, finalStats, "应该收到统计信息")
	assert.True(t, seenRunning, "应该看到running状态")
	assert.True(t, seenCompleted, "应该看到completed状态")

	// 验证统计数字的合理性
	assert.GreaterOrEqual(t, finalStats.TotalTraces, int64(1), "总trace数应该>=1")
	assert.GreaterOrEqual(t, finalStats.CompletedTraces, int64(1), "完成的trace数应该>=1")

	log.Info("统计信息准确性测试完成")
}

// 测试无效的取消请求 模拟用户手动取消Running的Trace时后端接收到的时候Trace已经完成或者取消/失败
func TestGRPCMUSTPASS_PluginTraceInvalidCancelRequest(t *testing.T) {
	client, err := NewLocalClient(true)
	require.NoError(t, err)

	testCaller, err := yak.NewMixPluginCaller()
	require.NoError(t, err)
	mitmPluginCallerGlobal = testCaller
	mitmPluginCallerNotifyChan = make(chan struct{})
	defer func() {
		mitmPluginCallerGlobal = nil
		mitmPluginCallerNotifyChan = nil
	}()

	traceCtx, traceCancel := context.WithCancel(context.Background())
	defer traceCancel()

	traceStream, err := client.PluginTrace(traceCtx)
	require.NoError(t, err)

	traceResponses := make(chan *ypb.PluginTraceResponse, 10)
	go func() {
		defer close(traceResponses)
		for {
			resp, err := traceStream.Recv()
			if err != nil {
				return
			}
			traceResponses <- resp
		}
	}()

	// 启用tracing和stream
	err = traceStream.Send(&ypb.PluginTraceRequest{
		ControlMode:   "start_stream",
		EnableTracing: true,
	})
	require.NoError(t, err)

	// 等待初始响应
	<-traceResponses // control_result

	// 发送无效的取消请求
	err = traceStream.Send(&ypb.PluginTraceRequest{
		ControlMode: "cancel_trace",
		TraceID:     "non-existent-trace-id",
	})
	require.NoError(t, err)

	// 验证收到失败响应
	select {
	case resp := <-traceResponses:
		if resp.ResponseType == "control_result" {
			assert.False(t, resp.Success, "取消不存在的trace应该失败")
			assert.Contains(t, resp.Message, "找不到", "错误消息应该说明找不到trace")
			log.Infof("收到预期的失败响应: %s", resp.Message)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("未收到取消响应")
	}

	log.Info("无效取消请求测试完成")
}

// 测试MITM未启动时的行为
func TestGRPCMUSTPASS_PluginTraceWithoutMITM(t *testing.T) {
	client, err := NewLocalClient(true)
	require.NoError(t, err)

	// 不设置MixPluginCaller，模拟MITM未启动
	traceCtx, traceCancel := context.WithCancel(context.Background())
	defer traceCancel()

	traceStream, err := client.PluginTrace(traceCtx)
	require.NoError(t, err)

	traceResponses := make(chan *ypb.PluginTraceResponse, 10)
	go func() {
		defer close(traceResponses)
		for {
			resp, err := traceStream.Recv()
			if err != nil {
				return
			}
			traceResponses <- resp
		}
	}()

	// 应该收到空的trace列表
	select {
	case resp := <-traceResponses:
		assert.Equal(t, "control_result", resp.ResponseType)
		assert.Equal(t, resp.Success, false, "未启动MITM时应该返回control_result且success为false")
	case <-time.After(3 * time.Second):
		t.Fatal("未收到初始响应")
	}

	// 测试set_tracing命令应该失败
	err = traceStream.Send(&ypb.PluginTraceRequest{
		ControlMode:   "set_tracing",
		EnableTracing: true,
	})
	require.NoError(t, err)

	select {
	case resp := <-traceResponses:
		if resp.Success {
			assert.False(t, resp.Success, "MITM未启动时任何请求相应的Success字段都应该为false")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("未收到响应")
	}

	log.Info("无MITM测试完成")
}

// 测试客户端取消时插件已经失败的竞态条件
func TestGRPCMUSTPASS_PluginTraceCancelRaceCondition_AlreadyFailed(t *testing.T) {
	timeThreshold := 3 // make sure client received this running trace
	// after received running trace client try to cancel the trace whereas the trace failed before actually cancel been called
	cancelTiming := 6
	testScript := &schema.YakScript{
		ScriptName: "race-condition-fail-plugin",
		Content: fmt.Sprintf(`
mirrorHTTPFlow = func(isHttps, url, req, rsp, body) {
	yakit_output("插件开始执行，即将在%v秒后失败")
	time.sleep(%v)  // 超过5秒阈值，会被推送
	die("插件执行失败")
}
`, timeThreshold, timeThreshold),
		Type: "mitm",
	}

	client, err := NewLocalClient(true)
	require.NoError(t, err)

	testCaller, err := yak.NewMixPluginCaller()
	require.NoError(t, err)
	testCaller.SetCallPluginTimeout(60)
	// 设置较短的长时间运行阈值，加速测试
	testCaller.SetLongRunningThreshold(consts.PluginCallDurationThresholdSecondsForTest)
	mitmPluginCallerGlobal = testCaller
	mitmPluginCallerNotifyChan = make(chan struct{})
	defer func() {
		mitmPluginCallerGlobal = nil
		mitmPluginCallerNotifyChan = nil
	}()

	traceCtx, traceCancel := context.WithCancel(context.Background())
	defer traceCancel()

	traceStream, err := client.PluginTrace(traceCtx)
	require.NoError(t, err)

	traceResponses := make(chan *ypb.PluginTraceResponse, 100)
	go func() {
		defer close(traceResponses)
		for {
			resp, err := traceStream.Recv()
			if err != nil {
				return
			}
			traceResponses <- resp
		}
	}()

	// 启用tracing和stream
	err = traceStream.Send(&ypb.PluginTraceRequest{
		ControlMode:   "start_stream",
		EnableTracing: true,
	})
	require.NoError(t, err)

	// 等待初始响应
	<-traceResponses // tracing_status
	<-traceResponses // trace_list

	err = testCaller.LoadPluginEx(traceCtx, testScript)
	require.NoError(t, err)

	// 启动插件
	go func() {
		testCaller.GetNativeCaller().CallByName("mirrorHTTPFlow",
			false, "http://example.com/race", []byte("GET / HTTP/1.1"), []byte("HTTP/1.1 200 OK"), []byte("body"))
	}()

	var runningTraceID string
	receivedStatuses := []string{}
	timeout := time.After(20 * time.Second)

	for {
		select {
		case resp := <-traceResponses:
			switch resp.ResponseType {
			case "trace_update":
				if len(resp.Traces) > 0 {
					trace := resp.Traces[0]
					if trace.PluginID == "race-condition-fail-plugin" {
						receivedStatuses = append(receivedStatuses, trace.Status)
						log.Infof("收到trace状态: %s, TraceID: %s", trace.Status, trace.TraceID)

						if trace.Status == "running" {
							runningTraceID = trace.TraceID
							// 模拟用户在插件失败时点击取消
							// 这时插件可能已经失败了，但前端还不知道
							go func() {
								time.Sleep(time.Second * time.Duration(cancelTiming))
								log.Info("客户端尝试取消trace")
								err = traceStream.Send(&ypb.PluginTraceRequest{
									ControlMode: "cancel_trace",
									TraceID:     runningTraceID,
								})
								if err != nil {
									log.Errorf("发送取消请求失败: %v", err)
								}
							}()
						} else if trace.Status == "failed" {
							assert.Equal(t, runningTraceID, trace.TraceID, "failed状态的TraceID应该与running状态一致")
							assert.NotEmpty(t, trace.ErrorMessage, "failed状态应该包含错误信息")
							goto testComplete
						} else if trace.Status == "cancelled" {
							t.Fatal("Trace失败后不应该被取消")
						}
					}
				}

			case "control_result":
				log.Infof("收到控制操作结果: Success=%t, Message=%s", resp.Success, resp.Message)
			}

		case <-timeout:
			t.Fatal("20秒内未完成测试")
		}
	}

testComplete:
	// 验证收到了running状态
	assert.Contains(t, receivedStatuses, string(yak.PluginStatusRunning), "应该收到running状态")
	// 最终状态应该是failed
	finalStatus := receivedStatuses[len(receivedStatuses)-1]
	assert.Equal(t, string(yak.PluginStatusFailed), finalStatus, "最终状态应该是failed")
	assert.NotEqual(t, testCaller.GetAllExecutionTraces()[0].Status, string(yak.PluginStatusCancelled))

	log.Infof("竞态条件测试完成，状态序列: %v", receivedStatuses)
}

// 测试客户端取消时插件已经正常完成的竞态条件
func TestGRPCMUSTPASS_PluginTraceCancelRaceCondition_AlreadyCompleted(t *testing.T) {
	timeThreshold := 3 // make sure client received this running trace
	// after received running trace client try to cancel the trace whereas the trace complete before actually cancel been called
	cancelTiming := 10
	testScript := &schema.YakScript{
		ScriptName: "race-condition-complete-plugin",
		Content: fmt.Sprintf(`
mirrorHTTPFlow = func(isHttps, url, req, rsp, body) {
	yakit_output("插件开始执行，将在%v秒后完成")
	time.sleep(%v)
	yakit_output("插件执行完成")
}
`, timeThreshold, timeThreshold),
		Type: "mitm",
	}

	client, err := NewLocalClient(true)
	require.NoError(t, err)

	testCaller, err := yak.NewMixPluginCaller()
	require.NoError(t, err)
	testCaller.SetCallPluginTimeout(60)
	// 设置较短的长时间运行阈值，加速测试
	testCaller.SetLongRunningThreshold(consts.PluginCallDurationThresholdSecondsForTest)
	mitmPluginCallerGlobal = testCaller
	mitmPluginCallerNotifyChan = make(chan struct{})
	defer func() {
		mitmPluginCallerGlobal = nil
		mitmPluginCallerNotifyChan = nil
	}()

	traceCtx, traceCancel := context.WithCancel(context.Background())
	defer traceCancel()

	traceStream, err := client.PluginTrace(traceCtx)
	require.NoError(t, err)

	traceResponses := make(chan *ypb.PluginTraceResponse, 100)
	go func() {
		defer close(traceResponses)
		for {
			resp, err := traceStream.Recv()
			if err != nil {
				return
			}
			traceResponses <- resp
		}
	}()

	// 启用tracing和stream
	err = traceStream.Send(&ypb.PluginTraceRequest{
		ControlMode:   "start_stream",
		EnableTracing: true,
	})
	require.NoError(t, err)

	// 等待初始响应
	<-traceResponses // tracing_status
	<-traceResponses // trace_list

	err = testCaller.LoadPluginEx(traceCtx, testScript)
	require.NoError(t, err)

	// 启动插件
	go func() {
		testCaller.GetNativeCaller().CallByName("mirrorHTTPFlow",
			false, "http://example.com/complete", []byte("GET / HTTP/1.1"), []byte("HTTP/1.1 200 OK"), []byte("body"))
	}()

	var runningTraceID string
	receivedStatuses := []string{}
	cancelRequestSent := false
	timeout := time.After(15 * time.Second)

	for {
		select {
		case resp := <-traceResponses:
			switch resp.ResponseType {
			case "trace_update":
				if len(resp.Traces) > 0 {
					trace := resp.Traces[0]
					if trace.PluginID == "race-condition-complete-plugin" {
						receivedStatuses = append(receivedStatuses, trace.Status)
						log.Infof("收到trace状态: %s, TraceID: %s", trace.Status, trace.TraceID)

						if trace.Status == "running" && !cancelRequestSent {
							runningTraceID = trace.TraceID
							cancelRequestSent = true
							// 模拟用户在插件完成时点击取消
							go func() {
								time.Sleep(time.Duration(cancelTiming) * time.Second)
								log.Info("客户端尝试取消即将完成的trace")
								err = traceStream.Send(&ypb.PluginTraceRequest{
									ControlMode: "cancel_trace",
									TraceID:     runningTraceID,
								})
								if err != nil {
									log.Errorf("发送取消请求失败: %v", err)
								}
							}()
						} else if trace.Status == "completed" {
							assert.Equal(t, runningTraceID, trace.TraceID, "completed状态的TraceID应该与running状态一致")
							log.Info("插件正常完成（取消请求发送但后端取消失败因为trace状态已经处于执行完成状态）")
							goto testComplete
						} else if trace.Status == "cancelled" {
							t.Fatal("Trace完成后不应该被取消")
						}
					}
				}

			case "control_result":
				log.Infof("收到控制操作结果: Success=%t, Message=%s", resp.Success, resp.Message)
			}

		case <-timeout:
			t.Fatal("15秒内未完成测试")
		}
	}

testComplete:
	// 验证收到了running状态
	assert.Contains(t, receivedStatuses, string(yak.PluginStatusRunning), "应该收到running状态")
	// 最终状态应该是completed
	finalStatus := receivedStatuses[len(receivedStatuses)-1]
	assert.Equal(t, string(yak.PluginStatusCompleted), finalStatus, "最终状态应该是completed")
	assert.NotEqual(t, testCaller.GetAllExecutionTraces()[0].Status, string(yak.PluginStatusCancelled))
	log.Infof("完成竞态条件测试完成，状态序列: %v", receivedStatuses)
}

// 测试 grpc 插件取消加载时的 trace 状态更新
func TestGRPCMUSTPASS_PluginTraceUnloadPluginCancelTraces(t *testing.T) {
	timeThreshold := 3 // make sure we block long enough
	log.SetLevel(log.DebugLevel)
	// 创建自定义插件脚本，包含多个mirror函数
	customPluginScript := fmt.Sprintf(`
# mirrorHTTPFlow 会镜像所有的流量到这里，包括 .js / .css / .jpg 这类一般会被劫持程序过滤的请求
mirrorHTTPFlow = func(isHttps /*bool*/, url /*string*/, req /*[]byte*/, rsp /*[]byte*/, body /*[]byte*/) {
    time.sleep(%v)
}

# mirrorFilteredHTTPFlow 劫持到的流量为 MITM 自动过滤出的可能和 "业务" 有关的流量，会自动过滤掉 js / css 等流量
mirrorFilteredHTTPFlow = func(isHttps /*bool*/, url /*string*/, req /*[]byte*/, rsp /*[]byte*/, body /*[]byte*/) {
    time.sleep(%v)
}

# mirrorNewWebsite 每新出现一个网站，这个网站的第一个请求，将会在这里被调用！
mirrorNewWebsite = func(isHttps /*bool*/, url /*string*/, req /*[]byte*/, rsp /*[]byte*/, body /*[]byte*/) {
    time.sleep(%v)
}

# mirrorNewWebsitePath 每新出现一个网站路径，关于这个网站路径的第一个请求，将会在这里被传入回调
mirrorNewWebsitePath = func(isHttps /*bool*/, url /*string*/, req /*[]byte*/, rsp /*[]byte*/, body /*[]byte*/) {
    time.sleep(%v)
}

# mirrorNewWebsitePathParams 每新出现一个网站路径且带有一些参数，参数通过常见位置和参数名去重，去重的第一个 HTTPFlow 在这里被调用
mirrorNewWebsitePathParams = func(isHttps /*bool*/, url /*string*/, req /*[]byte*/, rsp /*[]byte*/, body /*[]byte*/) {
    time.sleep(%v)
}
`, timeThreshold, timeThreshold, timeThreshold, timeThreshold, timeThreshold)

	client, err := NewLocalClient(true)
	require.NoError(t, err)

	// 创建测试用的 MixPluginCaller
	testCaller, err := yak.NewMixPluginCaller()
	require.NoError(t, err)
	err = testCaller.SetConcurrent(20)
	if err != nil {
		t.Fatal(err)
	}
	testCaller.SetCallPluginTimeout(300)
	testCaller.SetLongRunningThreshold(consts.PluginCallDurationThresholdSecondsForTest)
	testCaller.SetDividedContext(true)
	mitmPluginCallerGlobal = testCaller
	mitmPluginCallerNotifyChan = make(chan struct{})
	defer func() {
		mitmPluginCallerGlobal = nil
		mitmPluginCallerNotifyChan = nil
	}()

	// 加载自定义插件到 testCaller
	testScript := &schema.YakScript{
		ScriptName: "trace-unload-test-plugin",
		Content:    customPluginScript,
		Type:       "mitm",
	}

	// 启动 PluginTrace 流
	traceCtx, traceCancel := context.WithCancel(context.Background())
	defer traceCancel()

	traceStream, err := client.PluginTrace(traceCtx)
	require.NoError(t, err)

	traceResponses := make(chan *ypb.PluginTraceResponse, 100)
	go func() {
		defer close(traceResponses)
		for {
			resp, err := traceStream.Recv()
			if err != nil {
				return
			}
			traceResponses <- resp
		}
	}()

	// 启用 tracing
	err = traceStream.Send(&ypb.PluginTraceRequest{
		ControlMode:   "start_stream",
		EnableTracing: true,
	})
	require.NoError(t, err)

	// 等待初始响应
	<-traceResponses // control_result

	err = testCaller.LoadPluginEx(traceCtx, testScript)
	require.NoError(t, err)

	// 等待插件加载完成
	time.Sleep(2 * time.Second)

	// 触发插件执行，创建长时间运行的 trace
	go func() {
		// 模拟 HTTP 请求触发所有 mirror 函数
		for i := 0; i < 5; i++ {
			url := fmt.Sprintf("http://example%d.com/path%d", i, i)
			// 这会触发多个 mirror 函数，每个都会 sleep 180 秒
			go testCaller.GetNativeCaller().CallByName("mirrorHTTPFlow",
				false, url, []byte("GET / HTTP/1.1"), []byte("HTTP/1.1 200 OK"), []byte("body"))
			go testCaller.GetNativeCaller().CallByName("mirrorFilteredHTTPFlow",
				false, url, []byte("GET / HTTP/1.1"), []byte("HTTP/1.1 200 OK"), []byte("body"))
			go testCaller.GetNativeCaller().CallByName("mirrorNewWebsite",
				false, url, []byte("GET / HTTP/1.1"), []byte("HTTP/1.1 200 OK"), []byte("body"))
			go testCaller.GetNativeCaller().CallByName("mirrorNewWebsitePath",
				false, url, []byte("GET / HTTP/1.1"), []byte("HTTP/1.1 200 OK"), []byte("body"))
			go testCaller.GetNativeCaller().CallByName("mirrorNewWebsitePathParams",
				false, url, []byte("GET / HTTP/1.1"), []byte("HTTP/1.1 200 OK"), []byte("body"))
		}
	}()

	// 收集运行中的 trace ID
	runningTraces := make(map[string]bool)
	timeout := time.After(30 * time.Second)
	waitingForRunning := true

	for waitingForRunning {
		select {
		case resp := <-traceResponses:
			if resp.ResponseType == "trace_update" && len(resp.Traces) > 0 {
				for _, trace := range resp.Traces {
					if trace.Status == "running" {
						runningTraces[trace.TraceID] = true
						log.Infof("收到运行中的 trace: ID=%s, Hook=%s", trace.TraceID, trace.HookName)
					}
				}
				// 当我们有足够多的运行中的 trace 时，开始取消加载插件
				if len(runningTraces) >= 5 {
					waitingForRunning = false
				}
			}
		case <-timeout:
			t.Fatal("30秒内未收到足够的运行中 trace")
		}
	}

	log.Infof("收集到 %d 个运行中的 trace，开始取消加载插件", len(runningTraces))

	// 取消加载插件 - 使用 Remove 方法清除所有插件
	testCaller.GetNativeCaller().Remove(&ypb.RemoveHookParams{
		ClearAll: true,
	})
	log.Info("已清除所有插件")

	// 验证所有 trace 都变成 cancelled 状态
	cancelledTraces := make(map[string]bool)
	timeout = time.After(15 * time.Second)
	allCancelled := false

	for !allCancelled {
		select {
		case resp := <-traceResponses:
			if resp.ResponseType == "trace_update" && len(resp.Traces) > 0 {
				for _, trace := range resp.Traces {
					if trace.Status == "cancelled" {
						cancelledTraces[trace.TraceID] = true
						log.Infof("收到取消的 trace: ID=%s, Hook=%s", trace.TraceID, trace.HookName)
					}
				}

				// 检查是否所有运行中的 trace 都被取消了
				allCancelled = true
				for traceID := range runningTraces {
					if !cancelledTraces[traceID] {
						allCancelled = false
						break
					}
				}
			}
		case <-timeout:
			goto testComplete
		}
	}

testComplete:
	// 验证结果
	log.Infof("运行中的 trace 数量: %d, 取消的 trace 数量: %d", len(runningTraces), len(cancelledTraces))

	// 至少应该有一些 trace 被取消
	assert.Greater(t, len(cancelledTraces), 0, "应该有 trace 被取消")

	// 验证大部分运行中的 trace 都被取消了（考虑到时序问题，可能不是100%）
	cancelledRatio := float64(len(cancelledTraces)) / float64(len(runningTraces))
	assert.Greater(t, cancelledRatio, 0.5, "至少一半的运行中 trace 应该被取消")

	log.Info("插件取消加载 trace 状态更新测试完成")
}
