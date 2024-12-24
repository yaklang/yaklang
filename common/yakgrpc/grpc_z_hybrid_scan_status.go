package yakgrpc

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type HybridScanStatusManager struct {
	// 任务的总量
	TargetTotal    int64
	PluginTotal    int64
	TotalTaskCount int64

	// 完成的任务
	TargetFinished int64
	TaskFinished   int64

	// dynamic
	ActiveTask    int64
	ActiveTarget  int64
	ActiveTaskMap *sync.Map

	TaskId string

	// Task 计数器，作为索引
	TaskCount int64

	// 恢复任务的时候使用
	minTaskCount int64

	// 任务状态
	Status string

	ManagerMutex *sync.Mutex
}

func (h *HybridScanStatusManager) SetCurrentTaskIndex(i int64) {
	h.minTaskCount = i
}

func newHybridScanStatusManager(id string, targets int, plugins int, status string) *HybridScanStatusManager {
	return &HybridScanStatusManager{
		TargetTotal:   int64(targets),
		PluginTotal:   int64(plugins),
		TaskId:        id,
		ActiveTaskMap: new(sync.Map),
		Status:        status,
		ManagerMutex:  new(sync.Mutex),
	}
}

func fitStatusToHybridScanTaskRecord(status *ypb.HybridScanResponse, task *schema.HybridScanTask) {
	task.TotalTargets = status.TotalTargets
	task.TotalPlugins = status.TotalPlugins
	task.TotalTasks = status.TotalTasks
	task.FinishedTargets = status.FinishedTargets
	task.FinishedTasks = status.FinishedTasks
	task.Status = status.Status
}

func (h *HybridScanStatusManager) GetStatus(r ...*schema.HybridScanTask) *ypb.HybridScanResponse {
	h.ManagerMutex.Lock()
	defer h.ManagerMutex.Unlock()
	status := &ypb.HybridScanResponse{
		TotalTargets:     h.TargetTotal,
		TotalPlugins:     h.PluginTotal,
		TotalTasks:       int64(h.TargetTotal) * int64(h.PluginTotal),
		FinishedTasks:    h.TaskFinished,
		FinishedTargets:  h.TargetFinished,
		ActiveTasks:      h.ActiveTask,
		ActiveTargets:    h.ActiveTarget,
		HybridScanTaskId: h.TaskId,
		Status:           h.Status,
	}
	if h.minTaskCount > 0 {
		if status.FinishedTasks < h.minTaskCount {
			status.FinishedTasks = h.minTaskCount
		}
	}
	for _, data := range r {
		fitStatusToHybridScanTaskRecord(status, data)
	}
	return status
}

func (h *HybridScanStatusManager) DoActiveTarget() int64 {
	return atomic.AddInt64(&h.ActiveTarget, 1)
}

// DoActiveTask returns index of task
func (h *HybridScanStatusManager) DoActiveTask(task ...*schema.HybridScanTask) int64 {
	atomic.AddInt64(&h.ActiveTask, 1)
	index := atomic.AddInt64(&h.TotalTaskCount, 1)
	h.ActiveTaskMap.Store(index, struct{}{})
	for _, item := range task { // update task survival indexes
		item.SurvivalTaskIndexes = strings.Join(h.GetCurrentActiveTaskIndexes(), ",")
	}
	return index
}

func (h *HybridScanStatusManager) PushActiveTask(index int64, t *HybridScanTarget, pluginName string, stream HybridScanRequestStream) {
	rsp := h.GetStatus()
	rsp.UpdateActiveTask = &ypb.HybridScanUpdateActiveTaskTable{
		Operator:    "create",
		Index:       fmt.Sprint(index),
		IsHttps:     t.IsHttps,
		HTTPRequest: t.Request,
		Url:         utils.EscapeInvalidUTF8Byte([]byte(t.Url)),
		PluginName:  pluginName,
	}
	stream.Send(rsp)
}

func (h *HybridScanStatusManager) RemoveActiveTask(index int64, t *HybridScanTarget, pluginName string, stream HybridScanRequestStream) {
	rsp := h.GetStatus()
	rsp.UpdateActiveTask = &ypb.HybridScanUpdateActiveTaskTable{
		Operator:    "remove",
		Index:       fmt.Sprint(index),
		IsHttps:     t.IsHttps,
		HTTPRequest: t.Request,
		Url:         utils.EscapeInvalidUTF8Byte([]byte(t.Url)),
		PluginName:  pluginName,
	}
	stream.Send(rsp)
}

func (h *HybridScanStatusManager) DoneTask(index int64, task ...*schema.HybridScanTask) {
	atomic.AddInt64(&h.TaskFinished, 1)
	atomic.AddInt64(&h.ActiveTask, -1)
	h.ActiveTaskMap.Delete(index)
	for _, item := range task { // update task survival indexes
		item.SurvivalTaskIndexes = strings.Join(h.GetCurrentActiveTaskIndexes(), ",")
	}
}

func (h *HybridScanStatusManager) DoneTarget() {
	atomic.AddInt64(&h.TargetFinished, 1)
	atomic.AddInt64(&h.ActiveTarget, -1)
}

func (h *HybridScanStatusManager) DoneFailureTarget() {
	atomic.AddInt64(&h.TaskFinished, h.PluginTotal)
	h.DoneTarget()
}

func (h *HybridScanStatusManager) Feedback(stream HybridScanRequestStream) error {
	return stream.Send(h.GetStatus())
}

func (h *HybridScanStatusManager) GetCurrentActiveTaskIndexes() []string { // save to db ,use string
	var vals []string
	h.ActiveTaskMap.Range(func(key, value any) bool {
		if ret := codec.Atoi(fmt.Sprint(key)); ret > 0 {
			vals = append(vals, fmt.Sprint(key))
		}
		return true
	})
	return vals
}
