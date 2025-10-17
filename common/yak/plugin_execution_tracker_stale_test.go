package yak

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestStaleTraceQueue(t *testing.T) {
	// 测试创建队列
	queue := NewStaleTraceQueue(3)
	assert.Equal(t, 3, queue.Capacity())
	assert.Equal(t, 0, queue.Size())

	// 创建测试trace
	trace1 := &PluginExecutionTrace{
		TraceID:  "trace1",
		PluginID: "plugin1",
		HookName: "hook1",
		Status:   PluginStatusCompleted,
		EndTime:  time.Now(),
	}

	trace2 := &PluginExecutionTrace{
		TraceID:  "trace2",
		PluginID: "plugin2",
		HookName: "hook2",
		Status:   PluginStatusCompleted,
		EndTime:  time.Now().Add(time.Second),
	}

	trace3 := &PluginExecutionTrace{
		TraceID:  "trace3",
		PluginID: "plugin3",
		HookName: "hook3",
		Status:   PluginStatusCompleted,
		EndTime:  time.Now().Add(2 * time.Second),
	}

	trace4 := &PluginExecutionTrace{
		TraceID:  "trace4",
		PluginID: "plugin4",
		HookName: "hook4",
		Status:   PluginStatusCompleted,
		EndTime:  time.Now().Add(3 * time.Second),
	}

	// 测试添加trace
	evicted := queue.Push(trace1)
	assert.Nil(t, evicted)
	assert.Equal(t, 1, queue.Size())

	evicted = queue.Push(trace2)
	assert.Nil(t, evicted)
	assert.Equal(t, 2, queue.Size())

	evicted = queue.Push(trace3)
	assert.Nil(t, evicted)
	assert.Equal(t, 3, queue.Size())

	// 队列满了，添加新的应该驱逐最老的
	evicted = queue.Push(trace4)
	assert.NotNil(t, evicted)
	assert.Equal(t, "trace1", evicted.TraceID)
	assert.Equal(t, 3, queue.Size())

	// 测试获取所有trace（应该按最新到最老的顺序）
	allTraces := queue.GetAll()
	assert.Equal(t, 3, len(allTraces))
	assert.Equal(t, "trace4", allTraces[0].TraceID) // 最新的
	assert.Equal(t, "trace3", allTraces[1].TraceID)
	assert.Equal(t, "trace2", allTraces[2].TraceID) // 最老的

	// 测试根据ID查找
	found := queue.GetByTraceID("trace3")
	assert.NotNil(t, found)
	assert.Equal(t, "trace3", found.TraceID)

	notFound := queue.GetByTraceID("trace1") // 已被驱逐
	assert.Nil(t, notFound)

	// 测试清空队列
	queue.Clear()
	assert.Equal(t, 0, queue.Size())
	assert.Equal(t, 0, len(queue.GetAll()))
}

func TestPluginExecutionTrackerWithStaleQueue(t *testing.T) {
	// 创建一个小容量的tracker用于测试
	tracker := NewPluginExecutionTrackerWithStaleSize(2)

	ctx := context.Background()

	// 创建并完成多个trace
	trace1 := tracker.CreateTrace("plugin1", "hook1", ctx)
	tracker.StartExecution(trace1.TraceID, []interface{}{"arg1"})
	tracker.UpdateTraceStatus(trace1.TraceID, PluginStatusCompleted, "result1", nil)

	trace2 := tracker.CreateTrace("plugin2", "hook2", ctx)
	tracker.StartExecution(trace2.TraceID, []interface{}{"arg2"})
	tracker.UpdateTraceStatus(trace2.TraceID, PluginStatusCompleted, "result2", nil)

	trace3 := tracker.CreateTrace("plugin3", "hook3", ctx)
	tracker.StartExecution(trace3.TraceID, []interface{}{"arg3"})
	tracker.UpdateTraceStatus(trace3.TraceID, PluginStatusFailed, nil, assert.AnError)

	// 检查活跃traces应该为空（都已完成）
	activeTraces := tracker.GetActiveTraces()
	assert.Equal(t, 0, len(activeTraces))

	// 检查stale traces
	staleTraces := tracker.GetStaleTraces()
	assert.Equal(t, 2, len(staleTraces)) // 容量为2，最多保留2个

	// 检查总traces数量
	allTraces := tracker.GetAllTraces()
	assert.Equal(t, 2, len(allTraces))

	// 第一个trace应该被驱逐了
	_, found := tracker.GetTrace(trace1.TraceID)
	assert.False(t, found)

	// 后两个trace应该还在
	foundTrace2, found := tracker.GetTrace(trace2.TraceID)
	assert.True(t, found)
	assert.Equal(t, trace2.TraceID, foundTrace2.TraceID)

	foundTrace3, found := tracker.GetTrace(trace3.TraceID)
	assert.True(t, found)
	assert.Equal(t, trace3.TraceID, foundTrace3.TraceID)

	// 测试按插件查找（包括stale的）
	plugin2Traces := tracker.GetTracesByPlugin("plugin2")
	assert.Equal(t, 1, len(plugin2Traces))
	assert.Equal(t, trace2.TraceID, plugin2Traces[0].TraceID)

	// 测试按hook查找（包括stale的）
	hook3Traces := tracker.GetTracesByHook("hook3")
	assert.Equal(t, 1, len(hook3Traces))
	assert.Equal(t, trace3.TraceID, hook3Traces[0].TraceID)

	// 测试stale队列信息
	staleInfo := tracker.GetStaleQueueInfo()
	assert.Equal(t, 2, staleInfo["size"])
	assert.Equal(t, 2, staleInfo["capacity"])
	assert.Equal(t, 1.0, staleInfo["usage"])

	// 测试清空stale队列
	tracker.ClearStaleTraces()
	assert.Equal(t, 0, len(tracker.GetStaleTraces()))
	assert.Equal(t, 0, len(tracker.GetAllTraces()))
}

func TestPluginExecutionTrackerMixedActiveAndStale(t *testing.T) {
	tracker := NewPluginExecutionTrackerWithStaleSize(3)
	ctx := context.Background()

	// 创建一些running的trace
	runningTrace1 := tracker.CreateTrace("plugin1", "hook1", ctx)
	tracker.StartExecution(runningTrace1.TraceID, []interface{}{"arg1"})

	runningTrace2 := tracker.CreateTrace("plugin2", "hook2", ctx)
	tracker.StartExecution(runningTrace2.TraceID, []interface{}{"arg2"})

	// 创建一些已完成的trace
	completedTrace1 := tracker.CreateTrace("plugin3", "hook3", ctx)
	tracker.StartExecution(completedTrace1.TraceID, []interface{}{"arg3"})
	tracker.UpdateTraceStatus(completedTrace1.TraceID, PluginStatusCompleted, "result3", nil)

	completedTrace2 := tracker.CreateTrace("plugin4", "hook4", ctx)
	tracker.StartExecution(completedTrace2.TraceID, []interface{}{"arg4"})
	tracker.UpdateTraceStatus(completedTrace2.TraceID, PluginStatusFailed, nil, assert.AnError)

	// 检查各种统计
	activeTraces := tracker.GetActiveTraces()
	assert.Equal(t, 2, len(activeTraces)) // 2个running

	staleTraces := tracker.GetStaleTraces()
	assert.Equal(t, 2, len(staleTraces)) // 2个completed/failed

	allTraces := tracker.GetAllTraces()
	assert.Equal(t, 4, len(allTraces)) // 总共4个

	runningTraces := tracker.GetRunningTraces()
	assert.Equal(t, 2, len(runningTraces)) // 2个running

	// 测试统计信息
	stats := GetPluginExecutionStatistics(tracker)
	assert.Equal(t, 4, stats["total_traces"])
	assert.Equal(t, 2, stats["active_traces"])
	assert.Equal(t, 2, stats["stale_traces"])
	assert.Equal(t, 2, stats["running_traces"])

	staleInfo := stats["stale_queue_info"].(map[string]interface{})
	assert.Equal(t, 2, staleInfo["size"])
	assert.Equal(t, 3, staleInfo["capacity"])
}

func TestStaleTraceQueueBoundaryConditions(t *testing.T) {
	// 测试边界条件

	// 测试容量为0的情况
	queue := NewStaleTraceQueue(0)
	assert.Equal(t, DefaultStaleQueueSize, queue.Capacity())

	// 测试容量超过最大值的情况
	queue = NewStaleTraceQueue(MaxStaleQueueSize + 1000)
	assert.Equal(t, MaxStaleQueueSize, queue.Capacity())

	// 测试容量为1的队列
	queue = NewStaleTraceQueue(1)
	trace1 := &PluginExecutionTrace{TraceID: "trace1", Status: PluginStatusCompleted}
	trace2 := &PluginExecutionTrace{TraceID: "trace2", Status: PluginStatusCompleted}

	evicted := queue.Push(trace1)
	assert.Nil(t, evicted)
	assert.Equal(t, 1, queue.Size())

	evicted = queue.Push(trace2)
	assert.NotNil(t, evicted)
	assert.Equal(t, "trace1", evicted.TraceID)
	assert.Equal(t, 1, queue.Size())

	allTraces := queue.GetAll()
	assert.Equal(t, 1, len(allTraces))
	assert.Equal(t, "trace2", allTraces[0].TraceID)
}

func TestIsStaleStatus(t *testing.T) {
	trace := &PluginExecutionTrace{}

	// 测试非stale状态
	trace.Status = PluginStatusPending
	assert.False(t, trace.isStaleStatus())

	trace.Status = PluginStatusRunning
	assert.False(t, trace.isStaleStatus())

	// 测试stale状态
	trace.Status = PluginStatusCompleted
	assert.True(t, trace.isStaleStatus())

	trace.Status = PluginStatusFailed
	assert.True(t, trace.isStaleStatus())

	trace.Status = PluginStatusCancelled
	assert.True(t, trace.isStaleStatus())
}
