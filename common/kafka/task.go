package kafka

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"
)

type TaskType int
type TaskStatus int

const (
	Scan TaskType = iota + 1
	FingerPrint
)
const (
	Start TaskStatus = iota + 1
	Stop
	Recover
	Done
)

type Task struct {
	ctx        context.Context
	TaskType   TaskType
	TaskId     string
	SubTaskId  string
	CreateTime time.Time
	UpdateTime time.Time
	DeleteTime time.Time
	Runtime    time.Time
	Content    json.RawMessage

	status TaskStatus //记录任务状态
}

func (t *Task) GetTaskStatus() TaskStatus {
	return t.status
}

type Tasks struct {
	ctx               context.Context
	wg                *sync.WaitGroup
	currentTaskNumber atomic.Int64
	allTaskNumber     atomic.Int64
	taskChannel       chan *Task
}

func NewTasks(ctx context.Context) *Tasks {
	t := new(Tasks)
	t.ctx = ctx
	t.wg = &sync.WaitGroup{}
	t.taskChannel = make(chan *Task, 1024)
	return t
}
func (t *Tasks) AddTask(task *Task) {
	go func() {
		select {
		case <-t.ctx.Done():
			close(t.taskChannel)
			return
		case t.taskChannel <- task:
			t.allTaskNumber.Add(1)
		}
	}()
}

func (t *Tasks) GetTaskProcess() float64 {
	return float64(t.currentTaskNumber.Load()) / float64(t.allTaskNumber.Load())
}
