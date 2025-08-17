package aireact

import (
	"container/list"
	"sync"

	"github.com/yaklang/yaklang/common/log"
)

// TaskHook 定义任务预处理钩子函数类型
// 钩子函数可以修改任务状态，返回值决定是否继续入队
type TaskHook func(task *Task) (shouldQueue bool, err error)

// TaskQueue 任务队列结构
type TaskQueue struct {
	mutex     sync.RWMutex
	queue     *list.List // 使用链表实现队列
	hooks     []TaskHook // 预处理钩子集合
	queueName string     // 队列名称，用于日志记录
}

// NewTaskQueue 创建新的任务队列
func NewTaskQueue(name string) *TaskQueue {
	return &TaskQueue{
		queue:     list.New(),
		hooks:     make([]TaskHook, 0),
		queueName: name,
	}
}

// executeHooks 执行所有预处理钩子
func (tq *TaskQueue) executeHooks(task *Task) (bool, error) {
	for _, hook := range tq.hooks {
		shouldQueue, err := hook(task)
		if err != nil {
			log.Errorf("Task queue hook execution failed: %v", err)
			return false, err
		}
		if !shouldQueue {
			log.Infof("Task [%s] was filtered out by hook", task.GetId())
			return false, nil
		}
	}
	return true, nil
}

// GetFirst 获取并移除队列中的第一个任务
func (tq *TaskQueue) GetFirst() *Task {
	tq.mutex.Lock()
	defer tq.mutex.Unlock()

	front := tq.queue.Front()
	if front == nil {
		return nil
	}

	task := front.Value.(*Task)
	tq.queue.Remove(front)

	log.Debugf("Task queue [%s]: dequeued task [%s]", tq.queueName, task.GetId())
	return task
}

// Append 将任务添加到队列末尾
func (tq *TaskQueue) Append(task *Task) error {
	if task == nil {
		return nil
	}

	// 执行预处理钩子
	shouldQueue, err := tq.executeHooks(task)
	if err != nil {
		return err
	}
	if !shouldQueue {
		return nil
	}

	tq.mutex.Lock()
	defer tq.mutex.Unlock()

	tq.queue.PushBack(task)
	log.Debugf("Task queue [%s]: enqueued task [%s] at end", tq.queueName, task.GetId())
	return nil
}

// PrependToFirst 将任务插队到队列最前面
func (tq *TaskQueue) PrependToFirst(task *Task) error {
	if task == nil {
		return nil
	}

	// 执行预处理钩子
	shouldQueue, err := tq.executeHooks(task)
	if err != nil {
		return err
	}
	if !shouldQueue {
		return nil
	}

	tq.mutex.Lock()
	defer tq.mutex.Unlock()

	tq.queue.PushFront(task)
	log.Debugf("Task queue [%s]: prepended task [%s] to front", tq.queueName, task.GetId())
	return nil
}

// GetQueueingTasks 获取所有排队中的任务（不移除）
func (tq *TaskQueue) GetQueueingTasks() []*Task {
	tq.mutex.RLock()
	defer tq.mutex.RUnlock()

	tasks := make([]*Task, 0, tq.queue.Len())
	for e := tq.queue.Front(); e != nil; e = e.Next() {
		tasks = append(tasks, e.Value.(*Task))
	}

	return tasks
}

// GetQueueingCount 获取队列中任务的数量
func (tq *TaskQueue) GetQueueingCount() int {
	tq.mutex.RLock()
	defer tq.mutex.RUnlock()

	return tq.queue.Len()
}

// AddHook 添加预处理钩子
func (tq *TaskQueue) AddHook(hook TaskHook) {
	tq.mutex.Lock()
	defer tq.mutex.Unlock()

	tq.hooks = append(tq.hooks, hook)
	log.Debugf("Task queue [%s]: added new hook", tq.queueName)
}

// ClearHooks 清除所有预处理钩子
func (tq *TaskQueue) ClearHooks() {
	tq.mutex.Lock()
	defer tq.mutex.Unlock()

	tq.hooks = make([]TaskHook, 0)
	log.Debugf("Task queue [%s]: cleared all hooks", tq.queueName)
}

// Clear 清空队列中的所有任务
func (tq *TaskQueue) Clear() {
	tq.mutex.Lock()
	defer tq.mutex.Unlock()

	count := tq.queue.Len()
	tq.queue.Init() // 重新初始化链表，清空所有元素
	log.Infof("Task queue [%s]: cleared %d tasks", tq.queueName, count)
}

// IsEmpty 检查队列是否为空
func (tq *TaskQueue) IsEmpty() bool {
	tq.mutex.RLock()
	defer tq.mutex.RUnlock()

	return tq.queue.Len() == 0
}

// PeekFirst 查看队列第一个任务但不移除
func (tq *TaskQueue) PeekFirst() *Task {
	tq.mutex.RLock()
	defer tq.mutex.RUnlock()

	front := tq.queue.Front()
	if front == nil {
		return nil
	}

	return front.Value.(*Task)
}

// GetQueueName 获取队列名称
func (tq *TaskQueue) GetQueueName() string {
	return tq.queueName
}

// 预定义的常用Hook函数

// TaskDuplicateFilter 创建一个过滤重复任务的Hook
// 基于任务ID进行去重
func TaskDuplicateFilter() TaskHook {
	taskIds := make(map[string]bool)
	return func(task *Task) (bool, error) {
		id := task.GetId()
		if taskIds[id] {
			log.Infof("Duplicate task filtered: %s", id)
			return false, nil
		}
		taskIds[id] = true
		return true, nil
	}
}

// TaskPriorityFilter 创建一个基于任务优先级的Hook
// 可以根据任务属性设置优先级规则
func TaskPriorityFilter(allowedIds []string) TaskHook {
	allowedMap := make(map[string]bool)
	for _, id := range allowedIds {
		allowedMap[id] = true
	}

	return func(task *Task) (bool, error) {
		if len(allowedMap) == 0 {
			return true, nil // 如果没有限制，允许所有任务
		}

		id := task.GetId()
		allowed := allowedMap[id]
		if !allowed {
			log.Infof("Task [%s] filtered by priority filter", id)
		}
		return allowed, nil
	}
}

// TaskLogger 创建一个记录任务信息的Hook
func TaskLogger() TaskHook {
	return func(task *Task) (bool, error) {
		log.Infof("Processing task: id=[%s], input=[%s], status=[%s]",
			task.GetId(), task.GetUserInput(), task.GetStatus())
		return true, nil
	}
}
