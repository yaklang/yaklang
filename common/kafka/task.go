package kafka

import (
	"context"
	"sync"
	"sync/atomic"
)

type TaskType int
type TaskStatus int
type TaskResultType int

const (
	Script TaskType = iota + 1
)

const (
	FingerprintResponse TaskResultType = iota + 1
	PortScanResponse
	AssetsResponse
	Process
)

const (
	Prepare TaskStatus = iota + 1
	Running
	Stop
	Done
)

// TasksItem 最小的任务执行单元
type TasksItem struct {
	typ         TaskType
	ctx         context.Context
	cancel      context.CancelFunc
	mux         *sync.Mutex
	wg          *sync.WaitGroup
	status      TaskStatus
	Content     []byte //yak脚本
	taskId      string
	items       chan *TaskRequestMessage
	Total       *atomic.Int64
	ReceiveTask *atomic.Bool
	config      *TaskConfig
}

func NewTasksItem(id string, typ TaskType, config *TaskConfig) *TasksItem {
	return &TasksItem{
		typ:         typ,
		mux:         &sync.Mutex{},
		status:      Prepare,
		taskId:      id,
		items:       make(chan *TaskRequestMessage, 1024),
		ReceiveTask: &atomic.Bool{},
		wg:          &sync.WaitGroup{},
		config:      config,
	}
}

// StopReceive 停止接收任务，当manager发送完毕的时候，就关闭管道
func (t *TasksItem) StopReceive() {
	if t.ReceiveTask.Load() {
		t.mux.Lock()
		t.ReceiveTask.Store(false)
		go func() {
			t.wg.Wait()
			close(t.items)
		}()
		t.mux.Unlock()
	}
}

func (t *TasksItem) Start(ctx context.Context) {
	childCtx, cancelFunc := context.WithCancel(ctx)
	t.ctx = childCtx
	t.cancel = cancelFunc
	t.SetStatus(Running)
	t.mux.Lock()
	defer t.SetStatus(Done)
	defer t.mux.Unlock()
	var wg = &sync.WaitGroup{}
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range t.items {
				select {
				case <-t.ctx.Done():
					return
				default:
					processor, exist := registerProcess[t.typ]
					if !exist {
						return
					}
					processor.Process(t.ctx, item)
					_ = item
				}
			}
		}()
	}
	wg.Wait()
}

func (t *TasksItem) AddTask(message *TaskRequestMessage) {
	if t.ReceiveTask.Load() {
		t.wg.Add(1)
		go func() {
			defer t.wg.Done()
			t.items <- message
		}()
	}
}

func (t *TasksItem) SetStatus(status TaskStatus) {
	t.mux.Lock()
	t.status = status
	t.mux.Unlock()
}

func (t *TasksItem) Stop() {
	t.mux.Lock()
	t.cancel()
	t.SetStatus(Stop)
	t.mux.Unlock()
}
