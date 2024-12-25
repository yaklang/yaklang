package kafka

type ManagerConfig struct {
	OnConnectAfterFunc func(requestId, msg string)
	OnAgentErrorFunc   func(requestId string, err error)
	OnHealthFunc       func(health []byte)
	debug              bool
	*KafkaConfig
	*AgentConfig
}

type AgentConfig struct {
	*TaskConfig
	OnHealthFunc func(msg []byte)
}
type TaskConfig struct {
	OnTaskStartFunc  func(requestId, taskId string, message TaskRequestMessage)
	OnTaskFinishFunc func(taskId string)
	OnTaskStopFunc   func(requestId, taskId string)

	OnTaskResultBackFunc func(requestId, taskId string, message any)
	TaskProcess          func(taskId string, msg []byte) //任务的扫描进度
}

type KafkaConfig struct {
	timeout  int
	maxBytes int64
	retry    int
}
