package kafka

import (
	"context"
	"sync"
	"sync/atomic"
)

type AgentConfig struct {
}
type Agent struct {
	ctx     context.Context
	mux     *sync.Mutex
	status  atomic.Int64
	manager *TaskManager
	AgentInfo
}

type AgentInfo struct {
}
