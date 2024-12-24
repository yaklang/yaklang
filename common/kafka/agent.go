package kafka

import (
	"context"
	"github.com/yaklang/yaklang/common/kafka/health"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"sync/atomic"
	"time"
)

var agentLogger = log.GetLogger("agent")

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
	go a.healthCallback()
	a.agentEnvironment = environment
	return nil
}
func (a *Agent) healthCallback() {
	ticker := time.NewTicker(time.Second)
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			a.config.OnHealthFunc([]byte(a.agentEnvironment.String()))
		}
	}
}
func (a *Agent) AddTask(taskId string, message *TaskRequestMessage) {
	if tasksItem, exit := a.taskManager.Get(taskId); exit {
		tasksItem.AddTask(message)
	} else {
		item := NewTasksItem(taskId, message.typ, message.Content)
		item.AddTask(message)
		a.taskManager.Set(taskId, item)
	}
}

func (a *Agent) starkTask(id string) {
	tasks, exit := a.taskManager.Get(id)
	if exit {
		go tasks.Start(a.ctx)
	}
}
func (a *Agent) StopTask(id string) {
	tasks, exit := a.taskManager.Get(id)
	if exit {
		tasks.Stop()
	}
}
func (a *Agent) StopReceive(id string) {
	tasks, exit := a.taskManager.Get(id)
	if exit {
		tasks.StopReceive()
	}
}
