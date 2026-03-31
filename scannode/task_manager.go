package scannode

import (
	"context"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

type TaskManager struct {
	tasks *sync.Map
}

func newTaskManager() *TaskManager {
	return &TaskManager{tasks: new(sync.Map)}
}

func (t *TaskManager) Add(taskID string, task *Task) {
	task.StartTimestamp = time.Now().Unix()
	ddl, ok := task.Ctx.Deadline()
	if ok {
		task.DeadlineTimestamp = ddl.Unix()
	}
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

type Task struct {
	TaskType          string
	TaskId            string
	Ctx               context.Context
	Cancel            context.CancelFunc
	StartTimestamp    int64
	DeadlineTimestamp int64
	cancelReason      string
	cancelReasonMu    sync.RWMutex
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

func taskIDForSubtask(subtaskID string) string {
	return "script-task-" + subtaskID
}
