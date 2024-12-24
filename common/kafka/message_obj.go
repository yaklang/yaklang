package kafka

func NewTaskRequest(id, token string, msg []byte) *Request {
	return newRequest(TaskRequest, id, token, msg)
}

func NewManagerRequest(id, token string, msg []byte) *Request {
	return newRequest(ManagerRequest, id, token, msg)
}

// NewTaskResponse 里面还得对task进行细致划分
func NewTaskResponse(id, token, requestId string, msg []byte) *Response {
	return NewResponse(TaskResponse, id, requestId, token, msg)
}
func NewManagerResponse(id, token, fromRequestId string, msg []byte) *Response {
	return NewResponse(ManagerResponse, id, fromRequestId, token, msg)
}
