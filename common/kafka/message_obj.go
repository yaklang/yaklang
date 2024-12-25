package kafka

import (
	"encoding/json"
	"time"
)

func NewTaskRequest(id, token string, msg []byte) *Request {
	return newRequest(TaskRequest, token, msg)
}

func NewManagerRequest(id, token string, msg []byte) *Request {
	return newRequest(ManagerRequest, token, msg)
}

// NewTaskResponse 里面还得对task进行细致划分
func NewTaskResponse(id, token, requestId string, msg []byte) *Response {
	return NewResponse(TaskResponse, id, requestId, token, msg)
}

type ManagerMsgType int

const (
	RestartAgent ManagerMsgType = iota + 1
	ShutDownAgent
	StopTask
	ReuseTask
	StartTask
)

type ManagerMsg struct {
	Typ    ManagerMsgType
	TaskId string
}

func (m *ManagerMsg) String() string {
	marshal, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(marshal)
}

func newManagerMessage(typ ManagerMsgType, id string) *ManagerMsg {
	return &ManagerMsg{
		Typ:    typ,
		TaskId: id,
	}
}
func NewManagerStopAgentMsg() *ManagerMsg {
	return newManagerMessage(ShutDownAgent, "")
}
func NewManagerRestartAgentMsg() *ManagerMsg {
	return newManagerMessage(RestartAgent, "")
}
func NewStopTask(taskId string) *ManagerMsg {
	return newManagerMessage(StopTask, taskId)
}
func NewReuseTask(taskId string) *ManagerMsg {
	return newManagerMessage(ReuseTask, taskId)
}
func NewStartTask(taskId string) *ManagerMsg {
	return newManagerMessage(StartTask, taskId)
}

type LogMsg struct {
	Timestamp time.Time
	Msg       []byte
}

func NewLogMsg(msg []byte) *LogMsg {
	return &LogMsg{
		Timestamp: time.Now(),
		Msg:       msg,
	}
}

func (l *LogMsg) String() string {
	msg, err := json.Marshal(l)
	if err != nil {
		return ""
	}
	return string(msg)
}
