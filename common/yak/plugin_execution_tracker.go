package yak

import (
	"container/ring"
	"context"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/utils/omap"

	"github.com/google/uuid"

	"github.com/yaklang/yaklang/common/log"
)

// 默认 stale 队列大小
const (
	DefaultStaleQueueSize = 1000  // 默认保留1000个已完成的trace
	MaxStaleQueueSize     = 10000 // 最大保留10000个已完成的trace
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

// isStaleStatus 判断是否为已完成状态（需要移入stale队列）
func (t *PluginExecutionTrace) isStaleStatus() bool {
	return t.Status == PluginStatusCompleted || t.Status == PluginStatusFailed || t.Status == PluginStatusCancelled
}

// StaleTraceQueue stale trace 队列，使用 container/ring 实现
type StaleTraceQueue struct {
	ring     *ring.Ring   // 环形队列
	capacity int          // 队列容量
	size     int          // 当前队列大小
	mutex    sync.RWMutex // 读写锁
}

// NewStaleTraceQueue 创建新的stale trace队列
func NewStaleTraceQueue(capacity int) *StaleTraceQueue {
	if capacity <= 0 {
		capacity = DefaultStaleQueueSize
	}
	if capacity > MaxStaleQueueSize {
		capacity = MaxStaleQueueSize
	}
	return &StaleTraceQueue{
		ring:     ring.New(capacity),
		capacity: capacity,
		size:     0,
	}
}

// Push 添加trace到队列，如果队列满了则覆盖最老的trace
func (q *StaleTraceQueue) Push(trace *PluginExecutionTrace) *PluginExecutionTrace {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	var evicted *PluginExecutionTrace

	// 如果队列满了，保存即将被覆盖的trace
	if q.size == q.capacity {
		if q.ring.Value != nil {
			evicted = q.ring.Value.(*PluginExecutionTrace)
		}
	} else {
		q.size++
	}

	// 添加新trace并移动到下一个位置
	q.ring.Value = trace
	q.ring = q.ring.Next()

	return evicted
}

// GetAll 获取所有stale traces（按时间顺序，最新的在前面）
func (q *StaleTraceQueue) GetAll() []*PluginExecutionTrace {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	if q.size == 0 {
		return []*PluginExecutionTrace{}
	}

	result := make([]*PluginExecutionTrace, 0, q.size)

	// 从当前位置往前遍历，获取最新添加的trace
	current := q.ring.Prev() // 最新添加的trace在当前位置的前一个
	for i := 0; i < q.size; i++ {
		if current.Value != nil {
			result = append(result, current.Value.(*PluginExecutionTrace))
		}
		current = current.Prev()
	}

	return result
}

// GetByTraceID 根据traceID查找stale trace
func (q *StaleTraceQueue) GetByTraceID(traceID string) *PluginExecutionTrace {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	if q.size == 0 {
		return nil
	}

	// 遍历整个环形队列查找
	current := q.ring
	for i := 0; i < q.capacity; i++ {
		if current.Value != nil {
			if trace := current.Value.(*PluginExecutionTrace); trace.TraceID == traceID {
				return trace
			}
		}
		current = current.Next()
	}

	return nil
}

// Size 获取队列当前大小
func (q *StaleTraceQueue) Size() int {
	q.mutex.RLock()
	defer q.mutex.RUnlock()
	return q.size
}

// Capacity 获取队列容量
func (q *StaleTraceQueue) Capacity() int {
	return q.capacity
}

// Clear 清空队列
func (q *StaleTraceQueue) Clear() {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	// 清理所有引用，帮助GC
	current := q.ring
	for i := 0; i < q.capacity; i++ {
		current.Value = nil
		current = current.Next()
	}

	q.size = 0
}

// PluginExecutionTracker 插件执行跟踪器
type PluginExecutionTracker struct {
	traces       *sync.Map                                             // map[traceID]*PluginExecutionTrace (活跃的trace)
	pluginTraces *sync.Map                                             // map[pluginID][]traceID 按插件ID索引
	hookTraces   *sync.Map                                             // map[hookName][]traceID 按Hook名索引
	callbacks    *omap.OrderedMap[string, func(*PluginExecutionTrace)] // 回调函数列表
	staleQueue   *StaleTraceQueue                                      // 已完成的trace队列
}

// NewPluginExecutionTracker 创建新的插件执行跟踪器
func NewPluginExecutionTracker() *PluginExecutionTracker {
	return NewPluginExecutionTrackerWithStaleSize(DefaultStaleQueueSize)
}

// NewPluginExecutionTrackerWithStaleSize 创建指定stale队列大小的插件执行跟踪器
func NewPluginExecutionTrackerWithStaleSize(staleQueueSize int) *PluginExecutionTracker {
	return &PluginExecutionTracker{
		traces:       &sync.Map{},
		pluginTraces: &sync.Map{},
		hookTraces:   &sync.Map{},
		callbacks:    omap.NewOrderedMap(make(map[string]func(*PluginExecutionTrace))),
		staleQueue:   NewStaleTraceQueue(staleQueueSize),
	}
}

// moveToStaleQueue 将trace移动到stale队列
func (t *PluginExecutionTracker) moveToStaleQueue(trace *PluginExecutionTrace) {
	// 从活跃traces中移除
	t.traces.Delete(trace.TraceID)
	// 从索引中移除
	t.removeFromIndex(trace.PluginID, trace.HookName, trace.TraceID)

	// 添加到stale队列，如果有被驱逐的trace，清理其CancelFunc
	if evicted := t.staleQueue.Push(trace); evicted != nil {
		if evicted.CancelFunc != nil {
			evicted.CancelFunc()
		}
		log.Debugf("Evicted old stale trace: %s (plugin: %s, hook: %s)",
			evicted.TraceID, evicted.PluginID, evicted.HookName)
	}
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
func (t *PluginExecutionTracker) AddCallback(callback func(*PluginExecutionTrace)) (callbackID string, remove func()) {
	callbackID = uuid.New().String()
	t.callbacks.Set(callbackID, callback)
	return callbackID, func() {
		t.callbacks.Delete(callbackID)
	}
}

// notifyCallbacks 通知所有回调函数
func (t *PluginExecutionTracker) notifyCallbacks(trace *PluginExecutionTrace) {
	callbacks := t.callbacks.Copy()
	callbacks.ForEach(func(callbackID string, callback func(*PluginExecutionTrace)) bool {
		go func(cb func(*PluginExecutionTrace)) {
			defer func() {
				if err := recover(); err != nil {
					log.Errorf("plugin execution trace callback panic: %v", err)
				}
			}()
			cb(trace)
		}(callback)
		return true
	})
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

			// 将已完成的trace移动到stale队列
			t.moveToStaleQueue(trace)
		}
		t.notifyCallbacks(trace)
	}
}

// GetTrace 获取跟踪信息（先从活跃traces查找，再从stale队列查找）
func (t *PluginExecutionTracker) GetTrace(traceID string) (*PluginExecutionTrace, bool) {
	// 先从活跃traces中查找
	if value, ok := t.traces.Load(traceID); ok {
		return value.(*PluginExecutionTrace), true
	}

	// 再从stale队列中查找
	if trace := t.staleQueue.GetByTraceID(traceID); trace != nil {
		return trace, true
	}

	return nil, false
}

// GetAllTraces 获取所有跟踪信息（包括活跃的和stale的）
func (t *PluginExecutionTracker) GetAllTraces() []*PluginExecutionTrace {
	var traces []*PluginExecutionTrace

	// 获取活跃的traces
	t.traces.Range(func(key, value interface{}) bool {
		traces = append(traces, value.(*PluginExecutionTrace))
		return true
	})

	// 获取stale traces
	staleTraces := t.staleQueue.GetAll()
	traces = append(traces, staleTraces...)

	return traces
}

// GetActiveTraces 获取所有活跃的跟踪信息（不包括stale的）
func (t *PluginExecutionTracker) GetActiveTraces() []*PluginExecutionTrace {
	var traces []*PluginExecutionTrace
	t.traces.Range(func(key, value interface{}) bool {
		traces = append(traces, value.(*PluginExecutionTrace))
		return true
	})
	return traces
}

// GetStaleTraces 获取所有stale的跟踪信息
func (t *PluginExecutionTracker) GetStaleTraces() []*PluginExecutionTrace {
	return t.staleQueue.GetAll()
}

// GetTracesByPlugin 根据插件ID获取跟踪信息（包括活跃的和stale的）
func (t *PluginExecutionTracker) GetTracesByPlugin(pluginID string) []*PluginExecutionTrace {
	var traces []*PluginExecutionTrace

	// 获取活跃的traces
	if value, ok := t.pluginTraces.Load(pluginID); ok {
		traceIDs := value.([]string)
		for _, traceID := range traceIDs {
			if trace, exists := t.traces.Load(traceID); exists {
				traces = append(traces, trace.(*PluginExecutionTrace))
			}
		}
	}

	// 获取stale traces中匹配的
	staleTraces := t.staleQueue.GetAll()
	for _, trace := range staleTraces {
		if trace.PluginID == pluginID {
			traces = append(traces, trace)
		}
	}

	return traces
}

// GetTracesByHook 根据Hook名获取跟踪信息（包括活跃的和stale的）
func (t *PluginExecutionTracker) GetTracesByHook(hookName string) []*PluginExecutionTrace {
	var traces []*PluginExecutionTrace

	// 获取活跃的traces
	if value, ok := t.hookTraces.Load(hookName); ok {
		traceIDs := value.([]string)
		for _, traceID := range traceIDs {
			if trace, exists := t.traces.Load(traceID); exists {
				traces = append(traces, trace.(*PluginExecutionTrace))
			}
		}
	}

	// 获取stale traces中匹配的
	staleTraces := t.staleQueue.GetAll()
	for _, trace := range staleTraces {
		if trace.HookName == hookName {
			traces = append(traces, trace)
		}
	}

	return traces
}

// GetRunningTraces 获取正在运行的跟踪信息（只从活跃traces中查找）
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

// CleanupCompletedTraces 清理已完成的跟踪信息（现在主要用于清理stale队列中过旧的数据）
func (t *PluginExecutionTracker) CleanupCompletedTraces(olderThan time.Duration) {
	cutoff := time.Now().Add(-olderThan)

	// 清理活跃traces中的已完成项（这些应该已经被移动到stale队列了，但以防万一）
	var toDelete []string
	t.traces.Range(func(key, value interface{}) bool {
		trace := value.(*PluginExecutionTrace)
		if trace.isStaleStatus() && trace.EndTime.Before(cutoff) {
			toDelete = append(toDelete, trace.TraceID)
		}
		return true
	})

	// 批量删除活跃traces中的过期项
	for _, traceID := range toDelete {
		t.RemoveTrace(traceID)
	}

	// 注意：stale队列会自动管理容量，不需要手动清理
	log.Debugf("Cleaned up %d expired active traces, stale queue size: %d/%d",
		len(toDelete), t.staleQueue.Size(), t.staleQueue.Capacity())
}

// RemoveTrace 删除指定的跟踪记录（只能删除活跃的trace）
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

// RemoveTracesByPluginAndHook 删除指定插件和Hook的所有跟踪记录（只能删除活跃的trace）
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

// ClearStaleTraces 清空stale队列
func (t *PluginExecutionTracker) ClearStaleTraces() {
	t.staleQueue.Clear()
}

// GetStaleQueueInfo 获取stale队列信息
func (t *PluginExecutionTracker) GetStaleQueueInfo() map[string]interface{} {
	return map[string]interface{}{
		"size":     t.staleQueue.Size(),
		"capacity": t.staleQueue.Capacity(),
		"usage":    float64(t.staleQueue.Size()) / float64(t.staleQueue.Capacity()),
	}
}

// GetPluginExecutionStatistics 获取插件执行统计信息
func GetPluginExecutionStatistics(tracker *PluginExecutionTracker) map[string]interface{} {
	if tracker == nil {
		return map[string]interface{}{
			"error": "插件执行跟踪器为空",
		}
	}

	allTraces := tracker.GetAllTraces()
	activeTraces := tracker.GetActiveTraces()
	staleTraces := tracker.GetStaleTraces()
	runningTraces := tracker.GetRunningTraces()

	stats := make(map[string]interface{})
	stats["total_traces"] = len(allTraces)
	stats["active_traces"] = len(activeTraces)
	stats["stale_traces"] = len(staleTraces)
	stats["running_traces"] = len(runningTraces)
	stats["stale_queue_info"] = tracker.GetStaleQueueInfo()

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
