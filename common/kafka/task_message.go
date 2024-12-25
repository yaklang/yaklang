package kafka

import (
	"encoding/json"
	"time"
)

// TaskRequestMessage 任务消息
type TaskRequestMessage struct {
	typ        TaskType
	Content    []byte
	Params     []byte //脚本参数
	CreateTime time.Time
	taskId     string
}

func (t *TaskRequestMessage) String() string {
	marshal, err := json.Marshal(t)
	if err != nil {
		return ""
	}
	return string(marshal)
}

type TaskResponseMessage struct {
	typ    TaskResultType
	TaskId string
	Msg    []byte //根据type进行区分
}

func (t *TaskResponseMessage) String() string {
	marshal, err := json.Marshal(t)
	if err != nil {
		return ""
	}
	return string(marshal)
}

func NewTaskRequestMessage(typ TaskType, taskId string, Content []byte) *TaskRequestMessage {
	return &TaskRequestMessage{
		typ:        typ,
		Content:    Content,
		CreateTime: time.Now(),
		taskId:     taskId,
	}
}

func NewTaskResponseMessage(taskType TaskResultType, taskId string, Msg []byte) *TaskResponseMessage {
	return &TaskResponseMessage{
		typ: taskType,
		Msg: Msg,
	}
}
