package reactloops

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/utils"
)

type taskRequestContextConfig struct {
	*mock.MockedAIConfig
	requestStarted chan context.Context
	release        <-chan struct{}
}

func (c *taskRequestContextConfig) waitForRequest(req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
	requestCtx := req.GetContext()
	c.requestStarted <- requestCtx
	if requestCtx == nil {
		<-c.release
		return nil, utils.Error("provider request context is nil")
	}
	select {
	case <-requestCtx.Done():
		return nil, requestCtx.Err()
	case <-c.release:
		return nil, utils.Error("test released provider request")
	}
}

func (c *taskRequestContextConfig) CallAI(req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
	return c.waitForRequest(req)
}

func (c *taskRequestContextConfig) CallSpeedPriorityAI(req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
	return c.waitForRequest(req)
}

func (c *taskRequestContextConfig) CallQualityPriorityAI(req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
	return c.waitForRequest(req)
}

func TestReActLoopTransactionBindsProviderRequestToActiveTask(t *testing.T) {
	requestStarted := make(chan context.Context, 1)
	release := make(chan struct{})
	defer close(release)

	baseConfig, ok := mock.NewMockedAIConfig(context.Background()).(*mock.MockedAIConfig)
	if !ok {
		t.Fatal("mock config has unexpected type")
	}
	baseConfig.SetConfig("AiTransactionAutoRetry", 1)
	config := &taskRequestContextConfig{
		MockedAIConfig: baseConfig,
		requestStarted: requestStarted,
		release:        release,
	}
	invoker := mock.NewMockInvoker(context.Background())
	invoker.SetConfig(config)
	loop := NewMinimalReActLoop(config, invoker)

	taskCtx, cancelTask := context.WithCancel(context.Background())
	task := aicommon.NewStatefulTaskBase("queue-root", "cancel the active provider request", taskCtx, config.GetEmitter())
	loop.SetCurrentTask(task)

	done := make(chan error, 1)
	go func() {
		_, _, transactionErr := loop.callAITransaction(&sync.WaitGroup{}, "prompt", "nonce")
		done <- transactionErr
	}()

	var requestCtx context.Context
	select {
	case requestCtx = <-requestStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("provider request did not start")
	}
	if requestCtx == nil {
		t.Fatal("provider request did not inherit the active task context")
	}

	cancelTask()
	select {
	case <-requestCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("provider request context was not cancelled with the active task")
	}
	select {
	case transactionErr := <-done:
		if !errors.Is(transactionErr, context.Canceled) {
			t.Fatalf("transaction error = %v, want context canceled", transactionErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("transaction did not stop after active task cancellation")
	}
}
