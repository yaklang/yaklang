package kafka

import (
	"context"
	"github.com/yaklang/yaklang/common/kafka/health"
	"github.com/yaklang/yaklang/common/utils"
	"sync/atomic"
	"time"
)

// Agent 负责执行任务，然后将结果返回
type Agent struct {
	ctx              context.Context
	cancel           context.CancelFunc
	status           *atomic.Int64
	config           *AgentConfig
	taskManager      *utils.SafeMap[*TasksItem]
	agentEnvironment *health.SystemMatrix //运行环境
}

func NewAgent(config *AgentConfig) *Agent {
	return &Agent{
		status:      &atomic.Int64{},
		config:      config,
		taskManager: utils.NewSafeMap[*TasksItem](),
	}
}
func (a *Agent) Start(ctx context.Context) error {
	childCtx, cancelFunc := context.WithCancel(ctx)
	a.ctx = childCtx
	a.cancel = cancelFunc
	a.status.Add(1)
	environment, err := health.NewSystemMatrixBase()
	if err != nil {
		return err
	}
	a.agentEnvironment = environment
	a.config.OnRegisterFunc([]byte(a.agentEnvironment.String()))
	go a.healthCallback()
	return nil
}

func (a *Agent) healthCallback() {
	ticker := time.NewTicker(time.Second)
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			a.config.OnHealthFunc()
		}
	}
}
func (a *Agent) AddTask(message *TaskRequestMessage) {
	if tasksItem, exist := a.taskManager.Get(message.TaskId); exist {
		tasksItem.AddTask(message)
	} else {
		item := NewTasksItem(message.TaskId, message.typ, a.config.TaskConfig)
		item.AddTask(message)
		a.taskManager.Set(message.TaskId, item)
	}
}

func (a *Agent) starkTask(id string) {
	tasks, exist := a.taskManager.Get(id)
	if exist {
		go tasks.Start(a.ctx)
	}
}
func (a *Agent) StopTask(id string) {
	tasks, exist := a.taskManager.Get(id)
	if exist {
		tasks.Stop()
	}
}
func (a *Agent) shutDown() {
	a.cancel()
}
func (a *Agent) StopReceive(id string) {
	tasks, exit := a.taskManager.Get(id)
	if exit {
		tasks.StopReceive()
	}
}
