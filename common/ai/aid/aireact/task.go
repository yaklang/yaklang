package aireact

import (
	"strings"
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
	*sync.RWMutex

	Id        string
	UserInput string
	Status    string
	CreatedAt time.Time
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
	}
}

func (t *Task) GetCreatedAt() time.Time {
	return t.CreatedAt
}

// IsRelatedTo 检查当前任务是否与另一个任务相关
// 这个方法可以在未来扩展为更复杂的相关性算法
func (t *Task) IsRelatedTo(currentTask *Task) bool {
	if currentTask == nil || t == nil {
		return false
	}

	// 基本的文本相似性检查（简化版）
	// 在实际应用中，这里可以使用更复杂的语义相似性算法
	currentInput := currentTask.GetUserInput()
	newInput := t.GetUserInput()

	if currentInput == "" || newInput == "" {
		return false
	}

	// 简单的关键词重叠检查
	// 在实际应用中可以集成AI模型进行语义相似性判断
	return hasSignificantOverlap(currentInput, newInput)
}

// hasSignificantOverlap 检查两个字符串是否有显著重叠
func hasSignificantOverlap(text1, text2 string) bool {
	// 这是一个简化的实现，实际应用中可以使用更复杂的算法
	// 比如：词向量相似性、编辑距离、或者调用AI接口进行语义判断

	if len(text1) < 3 || len(text2) < 3 {
		return false
	}

	// 转换为小写进行比较
	text1 = strings.ToLower(text1)
	text2 = strings.ToLower(text2)

	// 简单的包含关系检查
	if strings.Contains(text1, text2) || strings.Contains(text2, text1) {
		return true
	}

	// 检查关键词重叠（至少有3个字符的公共子串）
	for i := 0; i < len(text1)-2; i++ {
		for j := 3; j <= len(text1)-i && j <= 10; j++ { // 限制子串长度
			substr := text1[i : i+j]
			if len(substr) >= 3 && strings.Contains(text2, substr) {
				return true
			}
		}
	}

	return false
}

func NewTask(id string, userInput string) *Task {
	return &Task{
		RWMutex:   &sync.RWMutex{},
		Id:        id,
		UserInput: userInput,
		Status:    string(TaskStatus_Created),
		CreatedAt: time.Now(),
	}
}
