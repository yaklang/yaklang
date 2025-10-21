package aicommon

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"sync"
	"sync/atomic"
)

type MockedAIConfig struct {
	*KeyValueConfig
	*BaseInteractiveHandler
	*BaseCheckpointableStorage

	ctx context.Context

	idSequence int64
	runtimeId  string

	emitter *Emitter

	timelineContentSizeLimit int64
}

func (m *MockedAIConfig) CallAI(request *AIRequest) (*AIResponse, error) {
	//TODO implement me
	panic("implement me")
}

var _ AICallerConfigIf = (*MockedAIConfig)(nil)

func NewMockedAIConfig(ctx context.Context) AICallerConfigIf {
	emitter := &Emitter{
		streamWG:            &sync.WaitGroup{},
		id:                  "mock-emitter",
		baseEmitter:         func(e *schema.AiOutputEvent) error { return nil },
		eventProcesserStack: utils.NewStack[EventProcesser](),
	}

	config := &MockedAIConfig{
		KeyValueConfig:            NewKeyValueConfig(),
		BaseInteractiveHandler:    &BaseInteractiveHandler{},
		BaseCheckpointableStorage: NewBaseCheckpointableStorage(),
		ctx:                       ctx,
		runtimeId:                 "mock-runtime-id",
		emitter:                   emitter,
		timelineContentSizeLimit:  1000,
	}

	config.BaseInteractiveHandler.endpointManager = NewEndpointManager()

	return config
}

func (m *MockedAIConfig) AcquireId() int64 {
	return atomic.AddInt64(&m.idSequence, 1)
}

func (m *MockedAIConfig) GetRuntimeId() string {
	return m.runtimeId
}

func (m *MockedAIConfig) IsCtxDone() bool {
	select {
	case <-m.ctx.Done():
		return true
	default:
		return false
	}
}

func (m *MockedAIConfig) GetContext() context.Context {
	return m.ctx
}

func (m *MockedAIConfig) CallAIResponseConsumptionCallback(current int) {
	// Mock implementation - do nothing
}

func (m *MockedAIConfig) GetAITransactionAutoRetryCount() int64 {
	return 3
}

func (m *MockedAIConfig) RetryPromptBuilder(originalPrompt string, err error) string {
	if err == nil {
		return originalPrompt
	}
	return fmt.Sprintf("Retry prompt for error: %v\nOriginal: %s", err, originalPrompt)
}

func (m *MockedAIConfig) GetEmitter() *Emitter {
	return m.emitter
}

func (m *MockedAIConfig) NewAIResponse() *AIResponse {
	return NewAIResponse(m)
}

func (m *MockedAIConfig) CallAIResponseOutputFinishedCallback(s string) {
	// Mock implementation - do nothing
}

func (m *MockedAIConfig) GetTimelineContentSizeLimit() int64 {
	return m.timelineContentSizeLimit
}

func (c *MockedAIConfig) GetUserInteractiveLimitedTimes() int64 {
	return 3
}

func (c *MockedAIConfig) GetMaxIterationCount() int64 {
	return int64(100)
}

func (c *MockedAIConfig) GetAllowUserInteraction() bool {
	return false
}
