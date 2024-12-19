package kafka

import (
	"context"
	"github.com/yaklang/yaklang/common/utils"
)

type TaskManagerConfig struct {
	debug              bool
	OnTaskBeforeFunc   func(taskId, nodeId string, agent *Agent)
	OnTaskRunProcess   func(taskId string, process int, agent *Agent)
	OnTaskRunErrorFunc func(taskId string, err error, agent *Agent)
}

func defaultTaskManagerConfig() *TaskManagerConfig {
	return &TaskManagerConfig{
		OnTaskBeforeFunc: func(taskId, nodeId string, agent *Agent) {
			agent.writeHeathMessage()
		},
		OnTaskRunProcess:   nil,
		OnTaskRunErrorFunc: nil,
	}
}

type TaskOptions func(config *TaskManagerConfig)
type TaskManager struct {
	ctx     context.Context
	cancel  context.CancelFunc
	manager utils.SafeMap[*Tasks]
	config  *TaskManagerConfig
}

func NewTaskManager(ctx context.Context, opts ...TaskOptions) *TaskManager {
	if opts == nil || len(opts) == 0 {

	}
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
