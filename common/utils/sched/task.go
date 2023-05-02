package sched

import (
	"context"
	"github.com/pkg/errors"
	"time"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type Task struct {
	// 任务间隔
	interval time.Duration

	// 任务 ID
	ID string

	// 任务的启动时间
	Start time.Time

	// 任务的停止时间
	End time.Time

	// 任务执行函数
	f func()

	isDisabled *utils.AtomicBool

	// 是否已经执行过了，只会执行一次
	isExecuted *utils.AtomicBool

	// 是否正在运行中？
	isWorking *utils.AtomicBool

	// 调度是否生效？
	isScheduling *utils.AtomicBool

	// 是否结束了
	isFinished *utils.AtomicBool

	// context
	ctx context.Context

	// 取消任务
	cancel context.CancelFunc

	// 第一次是否执行？
	first *utils.AtomicBool

	// 上一次执行时间和下一次执行时间
	last, next time.Time

	// 钩子函数
	onFinished        map[string]TaskCallback
	onBeforeExecuting map[string]TaskCallback
	onEveryExecuted   map[string]TaskCallback
	onScheduleStart   map[string]TaskCallback
	onCanceled        map[string]TaskCallback
}

func NewTask(interval time.Duration, id string, start, end time.Time, f func(), first bool) *Task {
	return &Task{
		interval: interval, ID: id, Start: start, End: end, f: f,
		isExecuted:        utils.NewAtomicBool(),
		isWorking:         utils.NewAtomicBool(),
		isScheduling:      utils.NewAtomicBool(),
		isFinished:        utils.NewAtomicBool(),
		isDisabled:        utils.NewAtomicBool(),
		ctx:               context.Background(),
		first:             utils.NewBool(first),
		onFinished:        make(map[string]TaskCallback),
		onBeforeExecuting: make(map[string]TaskCallback),
		onCanceled:        make(map[string]TaskCallback),
		onScheduleStart:   make(map[string]TaskCallback),
		onEveryExecuted:   make(map[string]TaskCallback),
	}
}

func (t *Task) JustExecuteNotRecording() {
	t.f()
}

func (t *Task) SetDisabled(b bool) {
	t.isDisabled.SetTo(b)
}

func (t *Task) runWithContext(ctx context.Context) {
	t.isScheduling.Set()
	callbackLock.Lock()
	for _, f := range t.onScheduleStart {
		f(t)
	}
	callbackLock.Unlock()

	defer func() {
		t.isScheduling.UnSet()
		t.isFinished.Set()

		callbackLock.Lock()
		for _, f := range t.onFinished {
			f(t)
		}
		callbackLock.Unlock()
	}()

	// 设置 hook 来记录上次执行的时间
	taskFunc := func() {
		t.isWorking.Set()
		t.last = time.Now()

		// 设置执行任务前的回调函数
		callbackLock.Lock()
		for _, f := range t.onBeforeExecuting {
			f(t)
		}
		callbackLock.Unlock()

		// 如果已经禁用了，就不能执行
		if !t.isDisabled.IsSet() {
			t.f()
		}

		callbackLock.Lock()
		for _, f := range t.onEveryExecuted {
			f(t)
		}
		t.next = time.Now().Add(t.interval)
		callbackLock.Unlock()

		t.isWorking.UnSet()
	}

	// 设置时间执行时间
	var taskCtx = ctx
	if t.End.After(time.Now()) {
		taskCtx, _ = context.WithDeadline(ctx, t.End)
	}

	if t.Start.After(time.Now()) {
		startCtx, _ := context.WithDeadline(ctx, t.Start)
		select {
		case <-startCtx.Done():
			break
		case <-ctx.Done():
			callbackLock.Lock()
			for _, f := range t.onCanceled {
				f(t)
			}
			callbackLock.Unlock()
			return
		}
	}

	// 如果设置了第一次执行的话，则立即执行
	if t.first.IsSet() {
		taskFunc()
	}

	// 进入循环模式
	ticker := time.Tick(t.interval)
	for {
		if t.isFinished.IsSet() || !t.isScheduling.IsSet() {
			if t.cancel != nil {
				t.cancel()
			}
			return
		}

		select {
		case <-taskCtx.Done():
			callbackLock.Lock()
			for _, f := range t.onCanceled {
				f(t)
			}
			callbackLock.Unlock()
			return
		case <-ticker:
			taskFunc()
		}
	}
}

func (t *Task) ExecuteWithContext(ctx context.Context) error {
	if t.isExecuted.IsSet() {
		return errors.Errorf("execute failed: %s is executed", t.ID)
	}
	t.isExecuted.Set()

	var c context.Context
	c, t.cancel = context.WithCancel(ctx)
	go t.runWithContext(c)
	return nil
}

func (t *Task) Execute() error {
	return t.ExecuteWithContext(t.ctx)
}

func (t *Task) Cancel() {
	log.Infof("schedule task in memory: %v is canceled", t.ID)
	if t.cancel != nil {
		t.cancel()
	}
	t.isScheduling.UnSet()
	t.isFinished.Set()
}

// 状态函数
func (t *Task) IsFinished() bool {
	return t.isFinished.IsSet()
}

func (t *Task) IsDisabled() bool {
	return t.isDisabled.IsSet()
}

func (t *Task) GetIntervalSeconds() int64 {
	return int64(t.interval.Seconds())
}

func (t *Task) IsExecuted() bool {
	return t.isExecuted.IsSet()
}

func (t *Task) IsWorking() bool {
	return t.isWorking.IsSet()
}

func (t *Task) IsInScheduling() bool {
	return t.isScheduling.IsSet()
}

// 其他参数
func (t *Task) LastExecutedDate() (time.Time, error) {
	if t.last.IsZero() {
		return time.Time{}, errors.New("not executed yet")
	}
	return t.last, nil
}

func (t *Task) NextExecuteDate() (time.Time, error) {
	if !t.IsInScheduling() || t.IsFinished() {
		return t.next, nil
	}
	return time.Time{}, errors.New("sched is finished")
}
