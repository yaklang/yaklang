package kafka

import "time"

// TaskRequestMessage 任务消息
type TaskRequestMessage struct {
	typ        TaskType
	Content    []byte
	Params     []byte //脚本参数
	CreateTime time.Time
}

type TaskResponseMessage struct {
	typ TaskResultType
	Msg []byte //根据type进行区分
}

func NewTaskRequestMessage(typ TaskType, Content []byte) *TaskRequestMessage {
	return &TaskRequestMessage{
		typ:        typ,
		Content:    Content,
		CreateTime: time.Now(),
	}
}

func NewTaskResponseMessage(taskType TaskResultType, Msg []byte) *TaskResponseMessage {
	return &TaskResponseMessage{
		typ: taskType,
		Msg: Msg,
	}
}
