package kafka

type ManagerConfig struct {
	OnConnectBeforeFunc func(requestId, msg string)
	OnConnectAfterFunc  func(requestId, msg string)
	OnAgentErrorFunc    func(requestId string, err error)
	OnHealthFunc        func(health []byte)
	debug               bool
	*KafkaConfig
	*AgentConfig
}

type AgentConfig struct {
	*TaskConfig
}
type TaskConfig struct {
	OnTaskStartFunc  func(requestId, taskId string, message TaskRequestMessage)
	OnTaskFinishFunc func(taskId string)
	OnTaskStopFunc   func(requestId, taskId string)

	OnTaskResultBackFunc func(requestId, taskId string, message []byte)
	TaskProcess          func(taskId string, msg []byte) //任务的扫描进度
}
