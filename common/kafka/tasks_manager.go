package kafka

import (
	"context"
	"github.com/yaklang/yaklang/common/utils"
)

type TaskManagerConfig struct {
	OnTaskBeforeFunc      func(taskId, nodeId string)
	OnTaskRunProcess      func(taskId string, process int, msg string)
	OnTaskRunErrorFunc    func(taskId string, err error)
	OnTaskOtherStatusFunc func(taskId string, msg string)
}
type TaskOptions func(config *TaskManagerConfig)
type TaskManager struct {
	ctx     context.Context
	cancel  context.CancelFunc
	manager utils.SafeMap[*Tasks]
	config  *TaskManagerConfig
}

func NewTaskManager(ctx context.Context, opts ...TaskOptions) *TaskManager {
	childCtx, cancelFunc := context.WithCancel(ctx)
	t := new(TaskManager)
	t.ctx = childCtx
	t.cancel = cancelFunc
	config := new(TaskManagerConfig)
	for _, opt := range opts {
		opt(config)
	}
	return t
}
func (t *TaskManager) AddTask(task *Task) {
	if tasks, b := t.manager.Get(task.TaskId); b {
		tasks.AddTask(task)
	} else {
		t.manager.Set(task.TaskId, NewTasks(t.ctx))
	}
}
func (t *TaskManager) GetTasksByTaskId(id string) (Tasks, bool) {
	return t.GetTasksByTaskId(id)
}
