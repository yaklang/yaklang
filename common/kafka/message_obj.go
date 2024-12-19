package kafka

import "github.com/google/uuid"

func NewRegisterRequest(id, token string) *Request {
	return &Request{
		Message: Message{
			Type: Register,
		},
		id:        id,
		Token:     token,
		RequestId: uuid.NewString(),
	}
}
func NewHeartRequest(id, token string, msg []byte) *Request {
	return &Request{
		Message: Message{
			Type: Heart,
			Msg:  msg,
		},
		id:        id,
		Token:     token,
		RequestId: uuid.NewString(),
	}
}
func NewTaskRequest(id, token string, msg []byte) *Request {
	return &Request{
		Message: Message{
			Type: TaskRequest,
			Msg:  msg,
		},
		id:    id,
		Token: token,
	}
}

func NewTaskResponse(id, token, requestId string, msg []byte) *Request {
	return &Request{
		Message: Message{
			Type: TaskResponse,
			Msg:  msg,
		},
		id:        id,
		Token:     token,
		RequestId: requestId,
	}
}
