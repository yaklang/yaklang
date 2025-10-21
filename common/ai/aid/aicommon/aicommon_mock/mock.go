package aicommon_mock

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"

	"sync/atomic"
)

type MockedAIConfig struct {
	*aicommon.KeyValueConfig
	*aicommon.BaseInteractiveHandler
	*aicommon.BaseCheckpointableStorage

	Ctx context.Context

	IdSequence int64
	RuntimeId  string

	Emitter *aicommon.Emitter

	TimelineContentSizeLimit int64
}

func (m *MockedAIConfig) CallAI(request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
	//TODO implement me
	panic("implement me")
}

var _ aicommon.AICallerConfigIf = (*MockedAIConfig)(nil)

func NewMockedAIConfig(ctx context.Context) aicommon.AICallerConfigIf {
	emitter := aicommon.NewEmitter("mock-emitter", func(e *schema.AiOutputEvent) error {
		return nil
	})

	config := &MockedAIConfig{
		KeyValueConfig:            aicommon.NewKeyValueConfig(),
		BaseInteractiveHandler:    &aicommon.BaseInteractiveHandler{},
		BaseCheckpointableStorage: aicommon.NewBaseCheckpointableStorage(),
		Ctx:                       ctx,
		RuntimeId:                 "mock-runtime-id",
		Emitter:                   emitter,
		TimelineContentSizeLimit:  1000,
	}

	config.BaseInteractiveHandler = aicommon.NewBaseInteractiveHandler()

	return config
}

func (m *MockedAIConfig) AcquireId() int64 {
	return atomic.AddInt64(&m.IdSequence, 1)
}

func (m *MockedAIConfig) GetRuntimeId() string {
	return m.RuntimeId
}

func (m *MockedAIConfig) IsCtxDone() bool {
	select {
	case <-m.Ctx.Done():
		return true
	default:
		return false
	}
}

func (m *MockedAIConfig) GetContext() context.Context {
	return m.Ctx
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

func (m *MockedAIConfig) GetEmitter() *aicommon.Emitter {
	return m.Emitter
}

func (m *MockedAIConfig) NewAIResponse() *aicommon.AIResponse {
	return aicommon.NewAIResponse(m)
}

func (m *MockedAIConfig) CallAIResponseOutputFinishedCallback(s string) {
	// Mock implementation - do nothing
}

func (m *MockedAIConfig) GetTimelineContentSizeLimit() int64 {
	return m.TimelineContentSizeLimit
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
