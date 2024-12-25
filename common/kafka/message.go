package kafka

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
)

type MessageType int

const (
	TaskRequest MessageType = iota + 1
	ManagerRequest

	TaskResponse
	ManagerResponse
	Health
	Register
	AgentLog
	TaskProcess
)

type Message struct {
	Type MessageType
	Msg  []byte
}
type Request struct {
	Message
	Token     string //指定token去进行执行
	RequestId string
}

type Response struct {
	Message
	ResponseId    string
	id            string
	Token         string
	FromRequestId string
}

type TopicResponse struct {
	Topic    Topic
	Response *Response
}

func (r *Request) String() string {
	marshal, err := json.Marshal(r)
	if err != nil {
		log.Errorf("request string error: %s", err)
	}
	return string(marshal)
}

func newRequest(typ MessageType, token string, msg []byte) *Request {
	return &Request{
		Message: Message{
			Type: typ,
			Msg:  msg,
		},
		Token:     token,
		RequestId: uuid.NewString(),
	}
}

func NewResponse(typ MessageType, id string, fromRequestId, token string, msg []byte) *Response {
	return &Response{
		Message: Message{
			Type: typ,
			Msg:  msg,
		},
		ResponseId:    uuid.NewString(),
		id:            id,
		Token:         token,
		FromRequestId: fromRequestId,
	}
}

func NewLogResponse(id, token string, msg []byte) *Response {
	return &Response{
		Message: Message{
			Type: AgentLog,
			Msg:  msg,
		},
		ResponseId: uuid.NewString(),
		id:         id,
		Token:      token,
	}
}
