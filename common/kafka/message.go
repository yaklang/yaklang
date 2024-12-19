package kafka

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
)

type MessageType int

const (
	Register MessageType = iota + 1
	Heart
	Log

	TaskRequest
	TaskResponse
)

type Message struct {
	Type MessageType
	Msg  []byte
}
type Request struct {
	Message
	id        string
	Token     string
	RequestId string
}

func (r *Request) String() string {
	marshal, err := json.Marshal(r)
	if err != nil {
		log.Errorf("request string error: %s", err)
	}
	return string(marshal)
}

func newRequest(typ MessageType, id string, token string, msg []byte) *Request {
	return &Request{
		Message: Message{
			Type: typ,
			Msg:  msg,
		},
		id:        id,
		Token:     token,
		RequestId: uuid.NewString(),
	}
}

type Response struct {
	Message
	ResponseId    string
	FromRequestId string
}
