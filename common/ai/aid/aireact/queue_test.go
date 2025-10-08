package aireact

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
)

// TestNewTaskQueue 测试队列创建
func TestNewTaskQueue(t *testing.T) {
	queueName := "test-queue"
	queue := NewTaskQueue(queueName)

	if queue == nil {
		t.Fatal("NewTaskQueue should not return nil")
	}

	if queue.GetQueueName() != queueName {
		t.Errorf("Expected queue name %s, got %s", queueName, queue.GetQueueName())
	}

	if !queue.IsEmpty() {
		t.Error("New queue should be empty")
	}

	if queue.GetQueueingCount() != 0 {
		t.Errorf("New queue should have 0 tasks, got %d", queue.GetQueueingCount())
	}
}

// TestTaskQueue_BasicOperations 测试基本队列操作
func TestTaskQueue_BasicOperations(t *testing.T) {
	queue := NewTaskQueue("test")

	// 测试空队列
	if !queue.IsEmpty() {
		t.Error("Queue should be empty initially")
	}

	if queue.GetFirst() != nil {
		t.Error("GetFirst on empty queue should return nil")
	}

	if queue.PeekFirst() != nil {
		t.Error("PeekFirst on empty queue should return nil")
	}

	// 添加任务
	task1 := aicommon.NewStatefulTaskBase("task1", "test input 1", nil, nil)
	err := queue.Append(task1)
	if err != nil {
		t.Fatalf("Failed to append task: %v", err)
	}

	if queue.IsEmpty() {
		t.Error("Queue should not be empty after adding task")
	}

	if queue.GetQueueingCount() != 1 {
		t.Errorf("Expected 1 task in queue, got %d", queue.GetQueueingCount())
	}

	// 测试 PeekFirst
	peeked := queue.PeekFirst()
	if peeked == nil {
		t.Error("PeekFirst should return the first task")
	}
	if peeked.GetId() != "task1" {
		t.Errorf("Expected task1, got %s", peeked.GetId())
	}

	// PeekFirst 不应该移除任务
	if queue.GetQueueingCount() != 1 {
		t.Error("PeekFirst should not remove task from queue")
	}

	// 获取任务
	retrieved := queue.GetFirst()
	if retrieved == nil {
		t.Error("GetFirst should return the task")
	}
	if retrieved.GetId() != "task1" {
		t.Errorf("Expected task1, got %s", retrieved.GetId())
	}

	// 队列应该为空
	if !queue.IsEmpty() {
		t.Error("Queue should be empty after removing task")
	}
}

// TestTaskQueue_AppendAndPrepend 测试添加和插队操作
func TestTaskQueue_AppendAndPrepend(t *testing.T) {
	queue := NewTaskQueue("test")

	task1 := aicommon.NewStatefulTaskBase("task1", "input1", nil, nil)
	task2 := aicommon.NewStatefulTaskBase("task2", "input2", nil, nil)
	task3 := aicommon.NewStatefulTaskBase("task3", "input3", nil, nil)

	// 添加到末尾
	queue.Append(task1)
	queue.Append(task2)

	// 插队到前面
	queue.PrependToFirst(task3)

	// 验证顺序：task3, task1, task2
	tasks := queue.GetQueueingTasks()
	if len(tasks) != 3 {
		t.Fatalf("Expected 3 tasks, got %d", len(tasks))
	}

	if tasks[0].GetId() != "task3" {
		t.Errorf("First task should be task3, got %s", tasks[0].GetId())
	}
	if tasks[1].GetId() != "task1" {
		t.Errorf("Second task should be task1, got %s", tasks[1].GetId())
	}
	if tasks[2].GetId() != "task2" {
		t.Errorf("Third task should be task2, got %s", tasks[2].GetId())
	}
}

// TestTaskQueue_GetQueueingTasks 测试获取排队任务
func TestTaskQueue_GetQueueingTasks(t *testing.T) {
	queue := NewTaskQueue("test")

	// 空队列
	tasks := queue.GetQueueingTasks()
	if len(tasks) != 0 {
		t.Errorf("Empty queue should return 0 tasks, got %d", len(tasks))
	}

	// 添加多个任务
	for i := 1; i <= 5; i++ {
		task := aicommon.NewStatefulTaskBase(fmt.Sprintf("task%d", i), fmt.Sprintf("input%d", i), nil, nil)
		queue.Append(task)
	}

	tasks = queue.GetQueueingTasks()
	if len(tasks) != 5 {
		t.Errorf("Expected 5 tasks, got %d", len(tasks))
	}

	// 验证任务顺序和内容
	for i, task := range tasks {
		expectedId := fmt.Sprintf("task%d", i+1)
		if task.GetId() != expectedId {
			t.Errorf("Task %d should have id %s, got %s", i, expectedId, task.GetId())
		}
	}
}

// TestTaskQueue_Clear 测试清空队列
func TestTaskQueue_Clear(t *testing.T) {
	queue := NewTaskQueue("test")

	// 添加多个任务
	for i := 1; i <= 3; i++ {
		task := aicommon.NewStatefulTaskBase(fmt.Sprintf("task%d", i), fmt.Sprintf("input%d", i), nil, nil)
		queue.Append(task)
	}

	if queue.GetQueueingCount() != 3 {
		t.Errorf("Expected 3 tasks before clear, got %d", queue.GetQueueingCount())
	}

	// 清空队列
	queue.Clear()

	if !queue.IsEmpty() {
		t.Error("Queue should be empty after clear")
	}

	if queue.GetQueueingCount() != 0 {
		t.Errorf("Queue should have 0 tasks after clear, got %d", queue.GetQueueingCount())
	}
}

// TestTaskQueue_NilTaskHandling 测试空任务处理
func TestTaskQueue_NilTaskHandling(t *testing.T) {
	queue := NewTaskQueue("test")

	// 测试添加 nil 任务
	err := queue.Append(nil)
	if err != nil {
		t.Errorf("Append nil should not return error, got %v", err)
	}

	err = queue.PrependToFirst(nil)
	if err != nil {
		t.Errorf("PrependToFirst nil should not return error, got %v", err)
	}

	if !queue.IsEmpty() {
		t.Error("Queue should remain empty after adding nil tasks")
	}
}

// TestTaskQueue_EnqueueHooks 测试入队钩子
func TestTaskQueue_EnqueueHooks(t *testing.T) {
	queue := NewTaskQueue("test")

	// 测试允许所有任务的钩子
	allowAllHook := func(task aicommon.AIStatefulTask) (bool, error) {
		log.Infof("Allow all hook called for task: %s", task.GetId())
		return true, nil
	}

	queue.AddEnqueueHook(allowAllHook)

	task := aicommon.NewStatefulTaskBase("task1", "input1", nil, nil)
	err := queue.Append(task)
	if err != nil {
		t.Errorf("Should not fail with allow-all hook: %v", err)
	}

	if queue.GetQueueingCount() != 1 {
		t.Error("Task should be in queue with allow-all hook")
	}

	// 清空并测试拒绝所有任务的钩子
	queue.Clear()
	queue.ClearHooks()

	denyAllHook := func(task aicommon.AIStatefulTask) (bool, error) {
		log.Infof("Deny all hook called for task: %s", task.GetId())
		return false, nil
	}

	queue.AddEnqueueHook(denyAllHook)

	task2 := aicommon.NewStatefulTaskBase("task2", "input2", nil, nil)
	err = queue.Append(task2)
	if err != nil {
		t.Errorf("Should not return error with deny-all hook: %v", err)
	}

	if queue.GetQueueingCount() != 0 {
		t.Error("Task should not be in queue with deny-all hook")
	}
}

// TestTaskQueue_EnqueueHookError 测试入队钩子错误处理
func TestTaskQueue_EnqueueHookError(t *testing.T) {
	queue := NewTaskQueue("test")

	errorHook := func(task aicommon.AIStatefulTask) (bool, error) {
		return false, errors.New("hook error")
	}

	queue.AddEnqueueHook(errorHook)

	task := aicommon.NewStatefulTaskBase("task1", "input1", nil, nil)
	err := queue.Append(task)
	if err == nil {
		t.Error("Should return error when hook fails")
	}

	if queue.GetQueueingCount() != 0 {
		t.Error("Task should not be in queue when hook fails")
	}
}

// TestTaskQueue_MultipleHooks 测试多个钩子
func TestTaskQueue_MultipleHooks(t *testing.T) {
	queue := NewTaskQueue("test")

	hook1Called := false
	hook2Called := false

	hook1 := func(task *Task) (bool, error) {
		hook1Called = true
		return true, nil
	}

	hook2 := func(task *Task) (bool, error) {
		hook2Called = true
		return true, nil
	}

	queue.AddEnqueueHook(hook1)
	queue.AddEnqueueHook(hook2)

	task := aicommon.NewStatefulTaskBase("task1", "input1", nil, nil)
	queue.Append(task)

	if !hook1Called {
		t.Error("Hook1 should be called")
	}
	if !hook2Called {
		t.Error("Hook2 should be called")
	}

	// 测试多个出队钩子
	dequeueHook1Called := false
	dequeueHook2Called := false

	dequeueHook1 := func(task *Task, reason string) {
		dequeueHook1Called = true
	}

	dequeueHook2 := func(task *Task, reason string) {
		dequeueHook2Called = true
	}

	queue.AddDequeueHook(dequeueHook1)
	queue.AddDequeueHook(dequeueHook2)

	queue.GetFirst()

	if !dequeueHook1Called {
		t.Error("Dequeue hook1 should be called")
	}
	if !dequeueHook2Called {
		t.Error("Dequeue hook2 should be called")
	}
}

// TestTaskQueue_PredefinedHooks 测试预定义的钩子函数
func TestTaskQueue_PredefinedHooks(t *testing.T) {
	queue := NewTaskQueue("test")

	// 测试重复任务过滤器
	duplicateFilter := TaskDuplicateFilter()
	queue.AddEnqueueHook(duplicateFilter)

	task1 := aicommon.NewStatefulTaskBase("duplicate", "input1", nil, nil)
	task2 := aicommon.NewStatefulTaskBase("duplicate", "input2", nil, nil) // 相同ID
	task3 := aicommon.NewStatefulTaskBase("unique", "input3", nil, nil)

	queue.Append(task1)
	queue.Append(task2) // 应该被过滤
	queue.Append(task3)

	if queue.GetQueueingCount() != 2 {
		t.Errorf("Expected 2 tasks (duplicate filtered), got %d", queue.GetQueueingCount())
	}

	tasks := queue.GetQueueingTasks()
	if tasks[0].GetId() != "duplicate" {
		t.Error("First task should be the original duplicate task")
	}
	if tasks[1].GetId() != "unique" {
		t.Error("Second task should be the unique task")
	}
}

// TestTaskQueue_PriorityFilter 测试优先级过滤器
func TestTaskQueue_PriorityFilter(t *testing.T) {
	queue := NewTaskQueue("test")

	// 只允许特定ID的任务
	allowedIds := []string{"high", "medium"}
	priorityFilter := TaskPriorityFilter(allowedIds)
	queue.AddEnqueueHook(priorityFilter)

	taskHigh := aicommon.NewStatefulTaskBase("high", "high priority task", nil, nil)
	taskMedium := aicommon.NewStatefulTaskBase("medium", "medium priority task", nil, nil)
	taskLow := aicommon.NewStatefulTaskBase("low", "low priority task", nil, nil)

	queue.Append(taskHigh)
	queue.Append(taskMedium)
	queue.Append(taskLow) // 应该被过滤

	if queue.GetQueueingCount() != 2 {
		t.Errorf("Expected 2 tasks (low priority filtered), got %d", queue.GetQueueingCount())
	}

	tasks := queue.GetQueueingTasks()
	for _, task := range tasks {
		if task.GetId() == "low" {
			t.Error("Low priority task should be filtered out")
		}
	}
}

// TestTaskQueue_TaskLogger 测试任务日志钩子
func TestTaskQueue_TaskLogger(t *testing.T) {
	queue := NewTaskQueue("test")

	logger := TaskLogger()
	queue.AddEnqueueHook(logger)

	task := aicommon.NewStatefulTaskBase("logged-task", "test input", nil, nil)
	err := queue.Append(task)
	if err != nil {
		t.Errorf("Logger hook should not cause error: %v", err)
	}

	if queue.GetQueueingCount() != 1 {
		t.Error("Task should be queued with logger hook")
	}
}

// TestTaskQueue_ConcurrentOperations 测试并发操作
func TestTaskQueue_ConcurrentOperations(t *testing.T) {
	queue := NewTaskQueue("concurrent-test")

	// 并发添加任务
	numGoroutines := 10
	tasksPerGoroutine := 10

	done := make(chan bool, numGoroutines)

	// 启动多个 goroutine 并发添加任务
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			for j := 0; j < tasksPerGoroutine; j++ {
				task := aicommon.NewStatefulTaskBase(fmt.Sprintf("task-%d-%d", id, j), fmt.Sprintf("input-%d-%d", id, j), nil, nil)
				queue.Append(task)
			}
			done <- true
		}(i)
	}

	// 等待所有 goroutine 完成
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	expectedCount := numGoroutines * tasksPerGoroutine
	actualCount := queue.GetQueueingCount()

	if actualCount != expectedCount {
		t.Errorf("Expected %d tasks, got %d", expectedCount, actualCount)
	}

	// 并发获取任务
	retrievedTasks := make(chan *Task, expectedCount)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			for {
				task := queue.GetFirst()
				if task == nil {
					break
				}
				retrievedTasks <- task
			}
			done <- true
		}()
	}

	// 等待一段时间让 goroutines 处理
	time.Sleep(100 * time.Millisecond)

	close(retrievedTasks)
	retrievedCount := len(retrievedTasks)

	if retrievedCount != expectedCount {
		t.Errorf("Expected to retrieve %d tasks, got %d", expectedCount, retrievedCount)
	}

	if !queue.IsEmpty() {
		t.Error("Queue should be empty after all tasks retrieved")
	}
}

// TestTaskQueue_HookWithPrependToFirst 测试插队操作的钩子处理
func TestTaskQueue_HookWithPrependToFirst(t *testing.T) {
	queue := NewTaskQueue("test")

	hookCalled := false
	hook := func(task *Task) (bool, error) {
		hookCalled = true
		return true, nil
	}

	queue.AddEnqueueHook(hook)

	task := aicommon.NewStatefulTaskBase("prepend-task", "prepend input", nil, nil)
	err := queue.PrependToFirst(task)
	if err != nil {
		t.Errorf("PrependToFirst should not fail: %v", err)
	}

	if !hookCalled {
		t.Error("Hook should be called for PrependToFirst")
	}

	if queue.GetQueueingCount() != 1 {
		t.Error("Task should be in queue after PrependToFirst")
	}
}
