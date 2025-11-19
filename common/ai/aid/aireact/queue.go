package aireact

import (
	"container/list"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
)

var (
	REACT_TASK_cancelled = "react_task_cancelled"
	REACT_TASK_enqueue   = "react_task_enqueue"
	REACT_TASK_dequeue   = "react_task_dequeue"
	REACT_TASK_clear     = "react_task_cleared"
)

func (r *ReAct) EmitEnqueueReActTask(t aicommon.AIStatefulTask) {
	if t == nil {
		return
	}
	if r.taskQueue == nil {
		log.Warnf("ReAct task queue is not initialized, cannot emit enqueue event for task [%s]", t.GetId())
		return
	}
	r.EmitStructured(REACT_TASK_enqueue, map[string]interface{}{
		"react_task_id":    t.GetId(),
		"react_task_input": t.GetUserInput(),
		"queue_len":        r.taskQueue.Len(),
	})
}

func (r *ReAct) EmitDequeueReActTask(t aicommon.AIStatefulTask, reason string) {
	if t == nil {
		return
	}
	if r.taskQueue == nil {
		log.Warnf("ReAct task queue is not initialized, cannot emit dequeue event for task [%s]", t.GetId())
		return
	}
	r.EmitStructured(REACT_TASK_dequeue, map[string]interface{}{
		"react_task_id":    t.GetId(),
		"react_task_input": t.GetUserInput(),
		"reason":           reason,
		"queue_len":        r.taskQueue.Len(),
	})
}

// taskEnqueueHook 定义任务预处理钩子函数类型
// 钩子函数可以修改任务状态，返回值决定是否继续入队
type taskEnqueueHook func(task aicommon.AIStatefulTask) (shouldQueue bool, err error)

type taskDequeueHook func(task aicommon.AIStatefulTask, reason string)

// TaskQueue 任务队列结构
type TaskQueue struct {
	mutex        sync.RWMutex
	queue        *list.List        // 使用链表实现队列
	dequeueHooks []taskDequeueHook // 取消入队钩子集合
	enqueueHook  []taskEnqueueHook // 预处理钩子集合
	queueName    string            // 队列名称，用于日志记录
}

// NewTaskQueue 创建新的任务队列
func NewTaskQueue(name string) *TaskQueue {
	return &TaskQueue{
		queue:        list.New(),
		enqueueHook:  make([]taskEnqueueHook, 0),
		dequeueHooks: make([]taskDequeueHook, 0),
		queueName:    name,
	}
}

func (tq *TaskQueue) Len() int {
	return tq.queue.Len()
}

// executeHooks 执行所有预处理钩子
func (tq *TaskQueue) executeHooks(task aicommon.AIStatefulTask) (bool, error) {
	for _, hook := range tq.enqueueHook {
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

// executeDequeueHooks 执行所有出队钩子 warning 不要在再有锁的情况下调用这个函数
func (tq *TaskQueue) executeDequeueHooks(task aicommon.AIStatefulTask, reason string) (bool, error) {
	for _, hook := range tq.dequeueHooks {
		hook(task, reason)
	}
	return true, nil
}

// GetFirst 获取并移除队列中的第一个任务
func (tq *TaskQueue) GetFirst() aicommon.AIStatefulTask {
	tq.mutex.Lock()
	defer tq.mutex.Unlock()
	front := tq.queue.Front()
	if front == nil {
		return nil
	}

	task := front.Value.(aicommon.AIStatefulTask)

	// 执行出队钩子
	shouldDequeue, err := tq.executeDequeueHooks(task, "normal")
	if err != nil {
		log.Errorf("Task dequeue hook failed: %v", err)
		return nil
	}
	if !shouldDequeue {
		return nil
	}

	tq.queue.Remove(front)

	log.Debugf("Task queue [%s]: dequeued task [%s]", tq.queueName, task.GetId())
	return task
}

// Append 将任务添加到队列末尾
func (tq *TaskQueue) Append(task aicommon.AIStatefulTask) error {
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
func (tq *TaskQueue) PrependToFirst(task aicommon.AIStatefulTask) error {
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
func (tq *TaskQueue) GetQueueingTasks() []aicommon.AIStatefulTask {
	tq.mutex.RLock()
	defer tq.mutex.RUnlock()

	tasks := make([]aicommon.AIStatefulTask, 0, tq.queue.Len())
	for e := tq.queue.Front(); e != nil; e = e.Next() {
		tasks = append(tasks, e.Value.(aicommon.AIStatefulTask))
	}

	return tasks
}

// GetQueueingCount 获取队列中任务的数量
func (tq *TaskQueue) GetQueueingCount() int {
	tq.mutex.RLock()
	defer tq.mutex.RUnlock()

	return tq.queue.Len()
}

// AddEnqueueHook 添加预处理钩子
func (tq *TaskQueue) AddEnqueueHook(hook taskEnqueueHook) {
	tq.mutex.Lock()
	defer tq.mutex.Unlock()

	tq.enqueueHook = append(tq.enqueueHook, hook)
	log.Debugf("Task queue [%s]: added new enqueue hook", tq.queueName)
}

// AddDequeueHook 添加出队钩子
func (tq *TaskQueue) AddDequeueHook(hook taskDequeueHook) {
	tq.mutex.Lock()
	defer tq.mutex.Unlock()

	tq.dequeueHooks = append(tq.dequeueHooks, hook)
	log.Debugf("Task queue [%s]: added new remove from queue hook", tq.queueName)
}

// ClearHooks 清除所有预处理钩子
func (tq *TaskQueue) ClearHooks() {
	tq.mutex.Lock()
	defer tq.mutex.Unlock()

	tq.enqueueHook = make([]taskEnqueueHook, 0)
	log.Debugf("Task queue [%s]: cleared all enqueueHook", tq.queueName)
}

// ClearRemoveFromQueueHooks 清除所有出队钩子
func (tq *TaskQueue) ClearRemoveFromQueueHooks() {
	tq.mutex.Lock()
	defer tq.mutex.Unlock()

	tq.dequeueHooks = make([]taskDequeueHook, 0)
	log.Debugf("Task queue [%s]: cleared all dequeueHooks", tq.queueName)
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
func (tq *TaskQueue) PeekFirst() aicommon.AIStatefulTask {
	tq.mutex.RLock()
	defer tq.mutex.RUnlock()

	front := tq.queue.Front()
	if front == nil {
		return nil
	}

	return front.Value.(aicommon.AIStatefulTask)
}

// GetQueueName 获取队列名称
func (tq *TaskQueue) GetQueueName() string {
	return tq.queueName
}

// 预定义的常用Hook函数

// TaskDuplicateFilter 创建一个过滤重复任务的Hook
// 基于任务ID进行去重
func TaskDuplicateFilter() taskEnqueueHook {
	taskIds := make(map[string]bool)
	return func(task aicommon.AIStatefulTask) (bool, error) {
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
func TaskPriorityFilter(allowedIds []string) taskEnqueueHook {
	allowedMap := make(map[string]bool)
	for _, id := range allowedIds {
		allowedMap[id] = true
	}

	return func(task aicommon.AIStatefulTask) (bool, error) {
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
func TaskLogger() taskEnqueueHook {
	return func(task aicommon.AIStatefulTask) (bool, error) {
		log.Infof("Processing task: id=[%s], input=[%s], status=[%s]",
			task.GetId(), task.GetUserInput(), task.GetStatus())
		return true, nil
	}
}

// MoveTaskToFirst 将指定 task_id 的任务移动到队列最前面
// 如果找到任务则返回 true，否则返回 false
func (tq *TaskQueue) MoveTaskToFirst(taskId string) bool {
	tq.mutex.Lock()
	defer tq.mutex.Unlock()

	// 遍历队列查找指定的任务
	for e := tq.queue.Front(); e != nil; e = e.Next() {
		task := e.Value.(aicommon.AIStatefulTask)
		if task.GetId() == taskId {
			// 找到任务，将其移动到队列最前面
			tq.queue.Remove(e)       // 先从当前位置移除
			tq.queue.PushFront(task) // 再添加到队列最前面

			log.Infof("Task queue [%s]: moved task [%s] to front of queue", tq.queueName, taskId)
			return true
		}
	}

	log.Warnf("Task queue [%s]: task [%s] not found in queue", tq.queueName, taskId)
	return false
}

// RemoveTask 从队列中移除指定 task_id 的任务
// 如果找到并移除任务则返回 true，否则返回 false
func (tq *TaskQueue) RemoveTask(taskId string) bool {
	tq.mutex.Lock()
	defer tq.mutex.Unlock()

	// 遍历队列查找指定的任务
	for e := tq.queue.Front(); e != nil; e = e.Next() {
		task := e.Value.(aicommon.AIStatefulTask)
		if task.GetId() == taskId {
			// 找到任务，从队列中移除
			tq.queue.Remove(e)

			// 执行 dequeue hooks 来发送事件
			for _, hook := range tq.dequeueHooks {
				hook(task, "manual_remove")
			}

			log.Infof("Task queue [%s]: removed task [%s] from queue", tq.queueName, taskId)
			return true
		}
	}

	log.Warnf("Task queue [%s]: task [%s] not found in queue, cannot remove", tq.queueName, taskId)
	return false
}
