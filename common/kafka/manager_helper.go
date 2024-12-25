package kafka

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"strconv"
)

type managerHelper struct {
}

func (m *managerHelper) newLogMsg(token, id string, log []byte) *TopicResponse {
	msg := NewLogMsg(log)
	return &TopicResponse{
		Topic:    CallBack,
		Response: NewLogResponse(id, token, []byte(msg.String())),
	}
}
func (m *managerHelper) newRegister(token, id string, msg []byte) *TopicResponse {
	return &TopicResponse{
		Topic:    CallBack,
		Response: NewResponse(Register, id, "", token, msg),
	}
}
func (m *managerHelper) newHearth(token, id string) *TopicResponse {
	return &TopicResponse{
		Topic:    CallBack,
		Response: NewResponse(Health, id, "", token, nil),
	}
}
func (m *managerHelper) newTaskProcess(token, id string, taskId string, process float64) *TopicResponse {
	float := strconv.FormatFloat(process, 'g', 5, 64)
	message := NewTaskResponseMessage(Process, taskId, []byte(float))
	return &TopicResponse{
		Topic:    CallBack,
		Response: NewTaskResponse(id, token, "", []byte(message.String())),
	}
}

func (m *managerHelper) newTaskResponse(token, id string, msg any) *TopicResponse {
	marshal, err := json.Marshal(msg)
	if err != nil {
		return m.newLogMsg(token, id, []byte(fmt.Sprintf("writer task response fail:%s", err)))
	}
	return &TopicResponse{
		Topic:    CallBack,
		Response: NewTaskResponse(id, token, uuid.NewString(), marshal),
	}
}
