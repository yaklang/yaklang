package yak

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/yaklang/yaklang/common/log"
)

// PluginExecutionStatus 插件执行状态
type PluginExecutionStatus string

const (
	PluginStatusPending   PluginExecutionStatus = "pending"   // 等待执行
	PluginStatusRunning   PluginExecutionStatus = "running"   // 正在执行
	PluginStatusCompleted PluginExecutionStatus = "completed" // 执行完成
	PluginStatusFailed    PluginExecutionStatus = "failed"    // 执行失败
	PluginStatusCancelled PluginExecutionStatus = "cancelled" // 被取消
)

// PluginExecutionTrace 插件执行跟踪信息
type PluginExecutionTrace struct {
	TraceID    string                `json:"trace_id"`    // 跟踪ID，唯一标识一次执行
	PluginID   string                `json:"plugin_id"`   // 插件ID
	HookName   string                `json:"hook_name"`   // Hook函数名
	Status     PluginExecutionStatus `json:"status"`      // 执行状态
	LoadedTime time.Time             `json:"loaded_time"` // 插件加载时间
	StartTime  time.Time             `json:"start_time"`  // 执行开始时间
	EndTime    time.Time             `json:"end_time"`    // 执行结束时间
	Duration   time.Duration         `json:"duration"`    // 执行耗时
	Args       []interface{}         `json:"args"`        // 执行参数
	Result     interface{}           `json:"result"`      // 执行结果
	Error      string                `json:"error"`       // 错误信息
	CancelFunc context.CancelFunc    `json:"-"`           // 取消函数
	RuntimeCtx context.Context       `json:"-"`           // 运行时上下文
}

// PluginExecutionTracker 插件执行跟踪器
type PluginExecutionTracker struct {
	traces       *sync.Map                     // map[traceID]*PluginExecutionTrace
	pluginTraces *sync.Map                     // map[pluginID][]traceID 按插件ID索引
	hookTraces   *sync.Map                     // map[hookName][]traceID 按Hook名索引
	callbacks    []func(*PluginExecutionTrace) // 回调函数列表
	mu           sync.RWMutex
}

// NewPluginExecutionTracker 创建新的插件执行跟踪器
func NewPluginExecutionTracker() *PluginExecutionTracker {
	return &PluginExecutionTracker{
		traces:       &sync.Map{},
		pluginTraces: &sync.Map{},
		hookTraces:   &sync.Map{},
		callbacks:    make([]func(*PluginExecutionTrace), 0),
	}
}

// makePluginHookKey 创建插件和Hook的复合键（保留用于兼容性，但不再用于唯一性标识）
func makePluginHookKey(pluginID, hookName string) string {
	return pluginID + "_" + hookName
}

// addToIndex 添加到索引
func (t *PluginExecutionTracker) addToIndex(pluginID, hookName, traceID string) {
	// 添加到插件索引
	if value, ok := t.pluginTraces.Load(pluginID); ok {
		traceIDs := value.([]string)
		traceIDs = append(traceIDs, traceID)
		t.pluginTraces.Store(pluginID, traceIDs)
	} else {
		t.pluginTraces.Store(pluginID, []string{traceID})
	}

	// 添加到Hook索引
	if value, ok := t.hookTraces.Load(hookName); ok {
		traceIDs := value.([]string)
		traceIDs = append(traceIDs, traceID)
		t.hookTraces.Store(hookName, traceIDs)
	} else {
		t.hookTraces.Store(hookName, []string{traceID})
	}
}

// removeFromIndex 从索引中移除
func (t *PluginExecutionTracker) removeFromIndex(pluginID, hookName, traceID string) {
	// 从插件索引中移除
	if value, ok := t.pluginTraces.Load(pluginID); ok {
		traceIDs := value.([]string)
		for i, id := range traceIDs {
			if id == traceID {
				traceIDs = append(traceIDs[:i], traceIDs[i+1:]...)
				break
			}
		}
		if len(traceIDs) == 0 {
			t.pluginTraces.Delete(pluginID)
		} else {
			t.pluginTraces.Store(pluginID, traceIDs)
		}
	}

	// 从Hook索引中移除
	if value, ok := t.hookTraces.Load(hookName); ok {
		traceIDs := value.([]string)
		for i, id := range traceIDs {
			if id == traceID {
				traceIDs = append(traceIDs[:i], traceIDs[i+1:]...)
				break
			}
		}
		if len(traceIDs) == 0 {
			t.hookTraces.Delete(hookName)
		} else {
			t.hookTraces.Store(hookName, traceIDs)
		}
	}
}

// AddCallback 添加跟踪回调函数
func (t *PluginExecutionTracker) AddCallback(callback func(*PluginExecutionTrace)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.callbacks = append(t.callbacks, callback)
}

// notifyCallbacks 通知所有回调函数
func (t *PluginExecutionTracker) notifyCallbacks(trace *PluginExecutionTrace) {
	t.mu.RLock()
	callbacks := make([]func(*PluginExecutionTrace), len(t.callbacks))
	copy(callbacks, t.callbacks)
	t.mu.RUnlock()

	for _, callback := range callbacks {
		go func(cb func(*PluginExecutionTrace)) {
			defer func() {
				if err := recover(); err != nil {
					log.Errorf("plugin execution trace callback panic: %v", err)
				}
			}()
			cb(trace)
		}(callback)
	}
}

// CreateTrace 创建新的插件执行跟踪记录（每次调用都创建新的Trace）
func (t *PluginExecutionTracker) CreateTrace(pluginID, hookName string, runtimeCtx context.Context) *PluginExecutionTrace {
	traceID := uuid.NewString()

	ctx, cancel := context.WithCancel(runtimeCtx)

	trace := &PluginExecutionTrace{
		TraceID:    traceID,
		PluginID:   pluginID,
		HookName:   hookName,
		Status:     PluginStatusPending,
		LoadedTime: time.Now(),
		CancelFunc: cancel,
		RuntimeCtx: ctx,
	}

	// 存储跟踪记录
	t.traces.Store(traceID, trace)
	// 添加到索引
	t.addToIndex(pluginID, hookName, traceID)

	t.notifyCallbacks(trace)
	return trace
}

// StartTrace 开始跟踪插件加载（保留用于向后兼容，但现在总是创建新的Trace）
func (t *PluginExecutionTracker) StartTrace(pluginID, hookName string, runtimeCtx context.Context) *PluginExecutionTrace {
	return t.CreateTrace(pluginID, hookName, runtimeCtx)
}

// StartExecution 开始执行跟踪（从Pending转为Running）
func (t *PluginExecutionTracker) StartExecution(traceID string, args []interface{}) bool {
	if value, ok := t.traces.Load(traceID); ok {
		trace := value.(*PluginExecutionTrace)
		trace.Status = PluginStatusRunning
		trace.StartTime = time.Now()
		trace.Args = args
		t.notifyCallbacks(trace)
		return true
	}
	return false
}

// FindTraceByPluginAndHook 根据插件ID和Hook名查找最新的跟踪记录
// 注意：此方法现在返回最新创建的Trace记录，主要用于兼容性
func (t *PluginExecutionTracker) FindTraceByPluginAndHook(pluginID, hookName string) *PluginExecutionTrace {
	// 获取该插件和Hook的所有Trace记录
	if value, ok := t.pluginTraces.Load(pluginID); ok {
		traceIDs := value.([]string)
		var latestTrace *PluginExecutionTrace
		var latestTime time.Time

		for _, traceID := range traceIDs {
			if trace, exists := t.traces.Load(traceID); exists {
				traceObj := trace.(*PluginExecutionTrace)
				if traceObj.HookName == hookName {
					if latestTrace == nil || traceObj.LoadedTime.After(latestTime) {
						latestTrace = traceObj
						latestTime = traceObj.LoadedTime
					}
				}
			}
		}
		return latestTrace
	}
	return nil
}

// FindLatestRunningTraceByPluginAndHook 查找指定插件和Hook的最新正在运行的Trace
func (t *PluginExecutionTracker) FindLatestRunningTraceByPluginAndHook(pluginID, hookName string) *PluginExecutionTrace {
	if value, ok := t.pluginTraces.Load(pluginID); ok {
		traceIDs := value.([]string)
		var latestTrace *PluginExecutionTrace
		var latestTime time.Time

		for _, traceID := range traceIDs {
			if trace, exists := t.traces.Load(traceID); exists {
				traceObj := trace.(*PluginExecutionTrace)
				if traceObj.HookName == hookName && traceObj.Status == PluginStatusRunning {
					if latestTrace == nil || traceObj.StartTime.After(latestTime) {
						latestTrace = traceObj
						latestTime = traceObj.StartTime
					}
				}
			}
		}
		return latestTrace
	}
	return nil
}

// UpdateTraceStatus 更新跟踪状态
func (t *PluginExecutionTracker) UpdateTraceStatus(traceID string, status PluginExecutionStatus, result interface{}, err error) {
	if value, ok := t.traces.Load(traceID); ok {
		trace := value.(*PluginExecutionTrace)
		trace.Status = status
		trace.Result = result
		if err != nil {
			trace.Error = err.Error()
		}
		if status == PluginStatusCompleted || status == PluginStatusFailed || status == PluginStatusCancelled {
			trace.EndTime = time.Now()
			trace.Duration = trace.EndTime.Sub(trace.StartTime)
		}
		t.notifyCallbacks(trace)
	}
}

// GetTrace 获取跟踪信息
func (t *PluginExecutionTracker) GetTrace(traceID string) (*PluginExecutionTrace, bool) {
	if value, ok := t.traces.Load(traceID); ok {
		return value.(*PluginExecutionTrace), true
	}
	return nil, false
}

// GetAllTraces 获取所有跟踪信息
func (t *PluginExecutionTracker) GetAllTraces() []*PluginExecutionTrace {
	var traces []*PluginExecutionTrace
	t.traces.Range(func(key, value interface{}) bool {
		traces = append(traces, value.(*PluginExecutionTrace))
		return true
	})
	return traces
}

// GetTracesByPlugin 根据插件ID获取跟踪信息
func (t *PluginExecutionTracker) GetTracesByPlugin(pluginID string) []*PluginExecutionTrace {
	if value, ok := t.pluginTraces.Load(pluginID); ok {
		traceIDs := value.([]string)
		traces := make([]*PluginExecutionTrace, 0, len(traceIDs))
		for _, traceID := range traceIDs {
			if trace, exists := t.traces.Load(traceID); exists {
				traces = append(traces, trace.(*PluginExecutionTrace))
			}
		}
		return traces
	}
	return []*PluginExecutionTrace{}
}

// GetTracesByHook 根据Hook名获取跟踪信息
func (t *PluginExecutionTracker) GetTracesByHook(hookName string) []*PluginExecutionTrace {
	if value, ok := t.hookTraces.Load(hookName); ok {
		traceIDs := value.([]string)
		traces := make([]*PluginExecutionTrace, 0, len(traceIDs))
		for _, traceID := range traceIDs {
			if trace, exists := t.traces.Load(traceID); exists {
				traces = append(traces, trace.(*PluginExecutionTrace))
			}
		}
		return traces
	}
	return []*PluginExecutionTrace{}
}

// GetRunningTraces 获取正在运行的跟踪信息
func (t *PluginExecutionTracker) GetRunningTraces() []*PluginExecutionTrace {
	var traces []*PluginExecutionTrace
	t.traces.Range(func(key, value interface{}) bool {
		trace := value.(*PluginExecutionTrace)
		if trace.Status == PluginStatusRunning {
			traces = append(traces, trace)
		}
		return true
	})
	return traces
}

// CancelTrace 取消跟踪
func (t *PluginExecutionTracker) CancelTrace(traceID string) bool {
	if value, ok := t.traces.Load(traceID); ok {
		trace := value.(*PluginExecutionTrace)
		if trace.CancelFunc != nil {
			trace.CancelFunc()
		}
		t.UpdateTraceStatus(traceID, PluginStatusCancelled, nil, nil)
		return true
	}
	return false
}

// CancelAllTraces 取消所有跟踪
func (t *PluginExecutionTracker) CancelAllTraces() {
	t.traces.Range(func(key, value interface{}) bool {
		trace := value.(*PluginExecutionTrace)
		if trace.Status == PluginStatusRunning || trace.Status == PluginStatusPending {
			if trace.CancelFunc != nil {
				trace.CancelFunc()
			}
			t.UpdateTraceStatus(trace.TraceID, PluginStatusCancelled, nil, nil)
		}
		return true
	})
}

// CleanupCompletedTraces 清理已完成的跟踪信息
func (t *PluginExecutionTracker) CleanupCompletedTraces(olderThan time.Duration) {
	cutoff := time.Now().Add(-olderThan)
	var toDelete []string

	t.traces.Range(func(key, value interface{}) bool {
		trace := value.(*PluginExecutionTrace)
		if (trace.Status == PluginStatusCompleted || trace.Status == PluginStatusFailed || trace.Status == PluginStatusCancelled) &&
			trace.EndTime.Before(cutoff) {
			toDelete = append(toDelete, trace.TraceID)
		}
		return true
	})

	// 批量删除
	for _, traceID := range toDelete {
		t.RemoveTrace(traceID) // 使用RemoveTrace确保同时删除映射
	}
}

// RemoveTrace 删除指定的跟踪记录
func (t *PluginExecutionTracker) RemoveTrace(traceID string) bool {
	if value, ok := t.traces.Load(traceID); ok {
		trace := value.(*PluginExecutionTrace)
		if trace.CancelFunc != nil {
			trace.CancelFunc()
		}

		// 删除跟踪记录
		t.traces.Delete(traceID)
		// 从索引中移除
		t.removeFromIndex(trace.PluginID, trace.HookName, traceID)

		return true
	}
	return false
}

// RemoveTracesByPluginAndHook 删除指定插件和Hook的所有跟踪记录
func (t *PluginExecutionTracker) RemoveTracesByPluginAndHook(pluginID, hookName string) int {
	removed := 0
	if value, ok := t.pluginTraces.Load(pluginID); ok {
		traceIDs := value.([]string)
		var toRemove []string

		for _, traceID := range traceIDs {
			if trace, exists := t.traces.Load(traceID); exists {
				traceObj := trace.(*PluginExecutionTrace)
				if traceObj.HookName == hookName {
					toRemove = append(toRemove, traceID)
				}
			}
		}

		for _, traceID := range toRemove {
			if t.RemoveTrace(traceID) {
				removed++
			}
		}
	}
	return removed
}

// GetPluginExecutionStatistics 获取插件执行统计信息
func GetPluginExecutionStatistics(tracker *PluginExecutionTracker) map[string]interface{} {
	if tracker == nil {
		return map[string]interface{}{
			"error": "插件执行跟踪器为空",
		}
	}

	allTraces := tracker.GetAllTraces()
	runningTraces := tracker.GetRunningTraces()

	stats := make(map[string]interface{})
	stats["total_traces"] = len(allTraces)
	stats["running_traces"] = len(runningTraces)

	// 按状态统计
	statusCount := make(map[PluginExecutionStatus]int)
	for _, trace := range allTraces {
		statusCount[trace.Status]++
	}
	stats["status_count"] = statusCount

	// 按插件统计
	pluginCount := make(map[string]int)
	for _, trace := range allTraces {
		pluginCount[trace.PluginID]++
	}
	stats["plugin_count"] = pluginCount

	// 按Hook统计
	hookCount := make(map[string]int)
	for _, trace := range allTraces {
		hookCount[trace.HookName]++
	}
	stats["hook_count"] = hookCount

	// 计算平均执行时间
	var totalDuration time.Duration
	completedCount := 0
	for _, trace := range allTraces {
		if trace.Status == PluginStatusCompleted && trace.Duration > 0 {
			totalDuration += trace.Duration
			completedCount++
		}
	}
	if completedCount > 0 {
		stats["average_duration"] = totalDuration / time.Duration(completedCount)
	}

	return stats
}
