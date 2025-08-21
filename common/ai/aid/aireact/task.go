package aireact

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

type TaskStatus string

const (
	TaskStatus_Created    TaskStatus = "created"
	TaskStatus_Queueing   TaskStatus = "queueing"
	TaskStatus_Evaluating TaskStatus = "evaluating"
	TaskStatus_Processing TaskStatus = "processing"
	TaskStatus_Completed  TaskStatus = "completed"
	TaskStatus_Aborted    TaskStatus = "aborted"
)

// each single query/input create a task
type Task struct {
	emitter *aicommon.Emitter
	*sync.RWMutex

	Id        string
	UserInput string
	Status    string
	CreatedAt time.Time
}

func (t *Task) SetEmitter(e *aicommon.Emitter) {
	t.emitter = e
}

func (t *Task) GetId() string {
	return t.Id
}

func (t *Task) GetUserInput() string {
	return t.UserInput
}

func (t *Task) GetStatus() string {
	return t.Status
}

func (t *Task) SetStatus(status string) {
	t.Lock()
	defer t.Unlock()

	oldStatus := t.Status
	t.Status = status

	// 输出调试日志记录状态变化
	if oldStatus != status {
		log.Debugf("Task %s status changed: %s -> %s", t.Id, oldStatus, status)
	} else {
		if t.emitter != nil {
			t.emitter.EmitStructured("react_task_status_changed", map[string]any{
				"react_task_id":         t.Id,
				"react_task_old_status": oldStatus,
				"react_task_now_status": status,
			})
		}
	}
}

func (t *Task) GetCreatedAt() time.Time {
	return t.CreatedAt
}

// IsRelatedTo 检查当前任务是否与另一个任务相关
// 这个方法可以在未来扩展为更复杂的相关性算法
func (t *Task) IsRelatedTo(currentTask *Task) bool {
	return false
}

func NewTask(id string, userInput string, emitter *aicommon.Emitter) *Task {
	task := &Task{
		RWMutex:   &sync.RWMutex{},
		Id:        id,
		UserInput: userInput,
		Status:    string(TaskStatus_Created),
		CreatedAt: time.Now(),
		emitter:   emitter,
	}
	if task.emitter != nil {
		task.emitter.EmitStructured("react_task_created", map[string]any{
			"react_task_id":     task.Id,
			"react_user_input":  task.UserInput,
			"react_task_status": task.Status,
		})
		log.Debugf("Task created: %s with input: %s", task.Id, task.UserInput)
	} else {
		log.Warnf("Task created without emitter: %s with input: %s", task.Id, task.UserInput)
	}
	return task
}
