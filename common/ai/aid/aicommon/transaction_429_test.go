package aicommon

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

type transactionTestConfig struct {
	*KeyValueConfig
	*BaseInteractiveHandler
	*BaseCheckpointableStorage
	ctx       context.Context
	emitter   *Emitter
	idSeq     int64
	retryMax  int64
}

var _ AICallerConfigIf = (*transactionTestConfig)(nil)

func newTransactionTestConfig(ctx context.Context) *transactionTestConfig {
	emitter := NewEmitter("txn-test", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		return e, nil
	})
	return &transactionTestConfig{
		KeyValueConfig:            NewKeyValueConfig(),
		BaseInteractiveHandler:    NewBaseInteractiveHandler(),
		BaseCheckpointableStorage: NewBaseCheckpointableStorage(),
		ctx:                       ctx,
		emitter:                   emitter,
		retryMax:                  3,
	}
}

func (t *transactionTestConfig) CallAI(req *AIRequest) (*AIResponse, error)                { panic("unused") }
func (t *transactionTestConfig) CallSpeedPriorityAI(req *AIRequest) (*AIResponse, error)   { return t.CallAI(req) }
func (t *transactionTestConfig) CallQualityPriorityAI(req *AIRequest) (*AIResponse, error) { return t.CallAI(req) }
func (t *transactionTestConfig) AcquireId() int64                     { return atomic.AddInt64(&t.idSeq, 1) }
func (t *transactionTestConfig) GetRuntimeId() string                 { return "txn-test-runtime" }
func (t *transactionTestConfig) IsCtxDone() bool {
	select {
	case <-t.ctx.Done():
		return true
	default:
		return false
	}
}
func (t *transactionTestConfig) GetContext() context.Context           { return t.ctx }
func (t *transactionTestConfig) CallAIResponseConsumptionCallback(int) {}
func (t *transactionTestConfig) GetAITransactionAutoRetryCount() int64 { return t.retryMax }
func (t *transactionTestConfig) GetToolComposeConcurrency() int        { return 2 }
func (t *transactionTestConfig) GetTimelineContentSizeLimit() int64    { return 1000 }
func (t *transactionTestConfig) GetUserInteractiveLimitedTimes() int64 { return 3 }
func (t *transactionTestConfig) GetMaxIterationCount() int64           { return 100 }
func (t *transactionTestConfig) GetAllowUserInteraction() bool         { return false }
func (t *transactionTestConfig) RetryPromptBuilder(prompt string, err error) string {
	if err == nil {
		return prompt
	}
	return fmt.Sprintf("retry for: %v\n%s", err, prompt)
}
func (t *transactionTestConfig) GetEmitter() *Emitter                            { return t.emitter }
func (t *transactionTestConfig) NewAIResponse() *AIResponse                      { return NewAIResponse(t) }
func (t *transactionTestConfig) CallAIResponseOutputFinishedCallback(string)      {}
func (t *transactionTestConfig) GetAiToolManager() *buildinaitools.AiToolManager  { return nil }
func (t *transactionTestConfig) OriginOptions() []ConfigOption                    { return nil }
func (t *transactionTestConfig) GetOrCreateWorkDir() string                       { return "" }
func (t *transactionTestConfig) GetContextProviderManager() *ContextProviderManager {
	return NewContextProviderManager()
}
func (t *transactionTestConfig) GetSessionEvidenceRendered() string        { return "" }
func (t *transactionTestConfig) ApplySessionEvidenceOps([]EvidenceOperation) {}

// --- tests ---

func TestCallAITransaction_429DoesNotCountRetry(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cfg := newTransactionTestConfig(ctx)
	cfg.retryMax = 3

	var callCount int64
	const num429Responses = 5

	callAi := func(req *AIRequest) (*AIResponse, error) {
		n := atomic.AddInt64(&callCount, 1)
		if n <= num429Responses {
			rsp := make429Response()
			return rsp, utils.Errorf("429 rate limited")
		}
		rsp := NewUnboundAIResponse()
		rsp.Close()
		return rsp, nil
	}

	postHandler := func(rsp *AIResponse) error {
		return nil
	}

	err := CallAITransaction(cfg, "test prompt", callAi, postHandler)
	require.NoError(t, err, "transaction should succeed after 429 retries")

	totalCalls := atomic.LoadInt64(&callCount)
	assert.Greater(t, totalCalls, cfg.retryMax,
		"total calls (%d) should exceed retry limit (%d) because 429s are not counted",
		totalCalls, cfg.retryMax)
}

func TestCallAITransaction_429ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg := newTransactionTestConfig(ctx)

	var callCount int64
	callAi := func(req *AIRequest) (*AIResponse, error) {
		n := atomic.AddInt64(&callCount, 1)
		if n >= 3 {
			cancel()
		}
		rsp := make429Response()
		return rsp, utils.Errorf("429 rate limited")
	}

	postHandler := func(rsp *AIResponse) error {
		return nil
	}

	start := time.Now()
	err := CallAITransaction(cfg, "test prompt", callAi, postHandler)
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.Less(t, elapsed, 30*time.Second, "should exit promptly after context cancellation")
	t.Logf("exited after %v with %d calls", elapsed, atomic.LoadInt64(&callCount))
}

func TestCallAITransaction_Non429ErrorCountsRetry(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := newTransactionTestConfig(ctx)
	cfg.retryMax = 3

	var callCount int64
	callAi := func(req *AIRequest) (*AIResponse, error) {
		atomic.AddInt64(&callCount, 1)
		rsp := make200Response()
		return rsp, utils.Errorf("some non-429 error")
	}

	postHandler := func(rsp *AIResponse) error {
		return nil
	}

	err := CallAITransaction(cfg, "test prompt", callAi, postHandler)
	require.Error(t, err)

	totalCalls := atomic.LoadInt64(&callCount)
	assert.Equal(t, cfg.retryMax, totalCalls,
		"non-429 errors should be counted, total calls should equal retry limit")
}

func TestIs429Response_Nil(t *testing.T) {
	assert.False(t, is429Response(context.Background(), nil))
}

func TestIs429Response_Non429(t *testing.T) {
	rsp := make200Response()
	assert.False(t, is429Response(context.Background(), rsp))
}

func TestIs429Response_429(t *testing.T) {
	rsp := make429Response()
	assert.True(t, is429Response(context.Background(), rsp))
}
