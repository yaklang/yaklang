package aireact

import "sync"

type TaskStatus string

const (
	TaskStatus_Queueing   TaskStatus = "queueing"
	TaskStatus_Processing TaskStatus = "processing"
	TaskStatus_Completed  TaskStatus = "completed"
	TaskStatus_Aborted    TaskStatus = "aborted"
)

// each single query/input create a task
type Task struct {
	*sync.RWMutex

	Id        string
	UserInput string
	Status    string
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
	t.Status = status
}

func NewTask(id string, userInput string) *Task {
	return &Task{
		Id:        id,
		UserInput: userInput,
		Status:    string(TaskStatus_Queueing),
	}
}
