package scannode

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/node"
	"github.com/yaklang/yaklang/common/utils"
)

type TaskManager struct {
	tasks *sync.Map
}

func newTaskManager() *TaskManager {
	return &TaskManager{tasks: new(sync.Map)}
}

func (t *TaskManager) Add(taskID string, task *Task) {
	now := time.Now().UTC()
	task.StartTimestamp = now.Unix()
	ddl, ok := task.Ctx.Deadline()
	if ok {
		task.DeadlineTimestamp = ddl.Unix()
	}
	task.MarkRunningAt(now)
	t.tasks.Store(taskID, task)
}

func (t *TaskManager) Remove(taskID string) {
	t.tasks.Delete(taskID)
}

func (t *TaskManager) GetTaskById(taskID string) (*Task, error) {
	ins, ok := t.tasks.Load(taskID)
	if ok {
		return ins.(*Task), nil
	}
	return nil, utils.Errorf("no existed task: %s", taskID)
}

func (t *TaskManager) Count() int {
	count := 0
	t.tasks.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}

func (t *TaskManager) Touch(taskID string) {
	task, err := t.GetTaskById(taskID)
	if err != nil {
		return
	}
	task.Touch()
}

func (t *TaskManager) MarkCancelRequested(taskID string) {
	task, err := t.GetTaskById(taskID)
	if err != nil {
		return
	}
	task.MarkCancelRequested()
}

func (t *TaskManager) ActiveAttemptHeartbeats(now time.Time) []node.ActiveAttemptHeartbeat {
	if now.IsZero() {
		now = time.Now().UTC()
	}

	items := make([]node.ActiveAttemptHeartbeat, 0)
	t.tasks.Range(func(_, value interface{}) bool {
		task, ok := value.(*Task)
		if !ok {
			return true
		}
		item, ok := task.activeAttemptHeartbeat(now)
		if ok {
			items = append(items, item)
		}
		return true
	})
	sort.Slice(items, func(i, j int) bool {
		if items[i].AttemptID == items[j].AttemptID {
			return items[i].SubtaskID < items[j].SubtaskID
		}
		return items[i].AttemptID < items[j].AttemptID
	})
	return items
}

type Task struct {
	TaskType          string
	TaskId            string
	JobID             string
	SubtaskID         string
	AttemptID         string
	Ctx               context.Context
	Cancel            context.CancelFunc
	StartTimestamp    int64
	DeadlineTimestamp int64
	cancelReason      string
	cancelReasonMu    sync.RWMutex
	status            string
	statusMu          sync.RWMutex
	lastActivityAt    time.Time
	activityMu        sync.RWMutex
	completedUnits    uint32
	totalUnits        uint32
	progressMu        sync.RWMutex
}

func (t *Task) SetCancelReason(reason string) {
	t.cancelReasonMu.Lock()
	defer t.cancelReasonMu.Unlock()
	t.cancelReason = reason
}

func (t *Task) CancelReason() string {
	t.cancelReasonMu.RLock()
	defer t.cancelReasonMu.RUnlock()
	return t.cancelReason
}

func (t *Task) MarkRunning() {
	t.MarkRunningAt(time.Now().UTC())
}

func (t *Task) MarkRunningAt(now time.Time) {
	t.statusMu.Lock()
	t.status = "running"
	t.statusMu.Unlock()
	t.TouchAt(now)
}

func (t *Task) MarkCancelRequested() {
	t.statusMu.Lock()
	t.status = "cancel_requested"
	t.statusMu.Unlock()
	t.Touch()
}

func (t *Task) Touch() {
	t.TouchAt(time.Now().UTC())
}

func (t *Task) TouchAt(now time.Time) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	t.activityMu.Lock()
	t.lastActivityAt = now
	t.activityMu.Unlock()
}

func (t *Task) UpdateProgressAt(completedUnits uint32, totalUnits uint32, now time.Time) {
	if totalUnits > 0 && completedUnits > totalUnits {
		completedUnits = totalUnits
	}
	t.progressMu.Lock()
	t.completedUnits = completedUnits
	t.totalUnits = totalUnits
	t.progressMu.Unlock()
	t.TouchAt(now)
}

func (t *Task) Status() string {
	t.statusMu.RLock()
	defer t.statusMu.RUnlock()
	return t.status
}

func (t *Task) Progress() (uint32, uint32) {
	t.progressMu.RLock()
	defer t.progressMu.RUnlock()
	return t.completedUnits, t.totalUnits
}

func (t *Task) LastActivityAt(now time.Time) time.Time {
	t.activityMu.RLock()
	last := t.lastActivityAt
	t.activityMu.RUnlock()
	if last.IsZero() {
		return now
	}
	return last
}

func (t *Task) activeAttemptHeartbeat(now time.Time) (node.ActiveAttemptHeartbeat, bool) {
	if t == nil || t.AttemptID == "" {
		return node.ActiveAttemptHeartbeat{}, false
	}
	status := t.Status()
	if status == "" {
		status = "running"
	}
	completedUnits, totalUnits := t.Progress()
	return node.ActiveAttemptHeartbeat{
		AttemptID:      t.AttemptID,
		JobID:          t.JobID,
		SubtaskID:      t.SubtaskID,
		Status:         status,
		CompletedUnits: completedUnits,
		TotalUnits:     totalUnits,
		LastActivityAt: t.LastActivityAt(now),
	}, true
}

func taskIDForSubtask(subtaskID string) string {
	return "script-task-" + subtaskID
}
