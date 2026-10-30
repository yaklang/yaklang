package aicommon

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
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
	retryWait func(context.Context, time.Duration) error
}

func (t *transactionTestConfig) RecordRecentlyUsedTool(*aitool.Tool) buildinaitools.RecentToolCacheMutation {
	return buildinaitools.RecentToolCacheMutation{}
}

var _ AICallerConfigIf = (*transactionTestConfig)(nil)

func (t *transactionTestConfig) IsSingleAIModelMode() bool { return false }

func (t *transactionTestConfig) ScheduleAuxiliaryTask(
	context.Context,
	string,
	func() string,
	func(*Action),
	...AuxiliaryTaskOption,
) {
}

func (t *transactionTestConfig) ResolveAuxiliaryTask(_ string) AuxiliaryTaskDecision {
	return AuxiliaryTaskDecision{Action: SingleModelRun}
}

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

func (t *transactionTestConfig) CallAI(req *AIRequest) (*AIResponse, error) { panic("unused") }
func (t *transactionTestConfig) CallSpeedPriorityAI(req *AIRequest) (*AIResponse, error) {
	return t.CallAI(req)
}
func (t *transactionTestConfig) CallQualityPriorityAI(req *AIRequest) (*AIResponse, error) {
	return t.CallAI(req)
}
func (t *transactionTestConfig) AcquireId() int64     { return atomic.AddInt64(&t.idSeq, 1) }
func (t *transactionTestConfig) GetRuntimeId() string { return "txn-test-runtime" }
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
func (t *transactionTestConfig) waitBeforeAIRetry(ctx context.Context, delay time.Duration) error {
	if t.retryWait != nil {
		return t.retryWait(ctx, delay)
	}
	return nil
}
func (t *transactionTestConfig) GetToolComposeConcurrency() int        { return 2 }
func (t *transactionTestConfig) GetPlanExecTaskConcurrency() int       { return 1 }
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
func (t *transactionTestConfig) GetBrowserSessionTracker() BrowserSessionTracker { return nil }
func (t *transactionTestConfig) AppendReportedRisk(risk *schema.Risk) bool       { return false }
func (t *transactionTestConfig) GetReportedRisksRendered() string                { return "" }
func (t *transactionTestConfig) GetReportedRisks() string                        { return "" }
func (t *transactionTestConfig) SetReportedRisks(string)                         {}
func (t *transactionTestConfig) NewAIResponse() *AIResponse                      { return NewAIResponse(t) }
func (t *transactionTestConfig) CallAIResponseOutputFinishedCallback(string)     {}
func (t *transactionTestConfig) GetAiToolManager() *buildinaitools.AiToolManager { return nil }
func (t *transactionTestConfig) OriginOptions() []ConfigOption                   { return nil }
func (t *transactionTestConfig) GetOrCreateWorkDir() string                      { return "" }
func (t *transactionTestConfig) AppendRelatedRuntimeID(string)                   {}
func (t *transactionTestConfig) GetContextProviderManager() *ContextProviderManager {
	return NewContextProviderManager()
}
func (t *transactionTestConfig) GetSessionEvidenceRendered() string          { return "" }
func (t *transactionTestConfig) ApplySessionEvidenceOps([]EvidenceOperation) {}
func (t *transactionTestConfig) GetVerificationTodoRendered(_ VerificationTodoScope) string {
	return ""
}
func (t *transactionTestConfig) ApplyTodoDelta(VerificationTodoScope, *TodoDelta) []VerificationTodoApplyResult {
	return nil
}
func (t *transactionTestConfig) ValidateTodoDelta(VerificationTodoScope, *TodoDelta) error {
	return nil
}
func (t *transactionTestConfig) GetVerificationTodoMarkdownDelta(VerificationTodoScope, *TodoDelta) string {
	return ""
}
func (t *transactionTestConfig) SnapshotCanonicalTodos(VerificationTodoScope) ([]TodoOpenItem, string, []TodoClosedItem) {
	return nil, "", nil
}
func (t *transactionTestConfig) SnapshotVerificationTodoItems() []VerificationTodoItem {
	return nil
}
func (t *transactionTestConfig) SnapshotVerificationTodoItemsByScope(VerificationTodoScope) []VerificationTodoItem {
	return nil
}
func (t *transactionTestConfig) GetVerificationTodoStats() VerificationTodoStats {
	return VerificationTodoStats{}
}
func (t *transactionTestConfig) GetVerificationTodoStatsByScope(VerificationTodoScope) VerificationTodoStats {
	return VerificationTodoStats{}
}
func (t *transactionTestConfig) HasActiveVerificationTodosByScope(VerificationTodoScope) bool {
	return false
}
func (t *transactionTestConfig) ActiveVerificationTodoItemsByScope(VerificationTodoScope) []VerificationTodoItem {
	return nil
}

// --- tests ---

// Transport errors have already exhausted the request executor's policy.
// Agent validation must neither replay them nor parse their HTTP error bodies.
func TestCallAITransaction_RequestFailureIsNotRetried(t *testing.T) {
	for _, status := range []int{401, 429, 502} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			cfg := newTransactionTestConfig(context.Background())
			cfg.retryMax = 9
			cfg.retryWait = func(context.Context, time.Duration) error {
				t.Fatal("Agent must not wait for transport replay")
				return nil
			}
			calls := 0
			err := CallAITransaction(cfg, "test", func(*AIRequest) (*AIResponse, error) {
				calls++
				rsp := NewAIResponse(nil)
				rsp.SetRawHTTPResponseData([]byte(fmt.Sprintf("HTTP/1.1 %d failure\r\n\r\n", status)), []byte(`{"error":"provider failure"}`))
				rsp.Close()
				return rsp, nil
			}, func(*AIResponse) error { t.Fatal("HTTP failure is not model output"); return nil })
			require.ErrorContains(t, err, "provider failure")
			require.Equal(t, 1, calls)
		})
	}
}

func TestCallAITransaction_RequestContextCancelDoesNotRetryOrEmitError(t *testing.T) {
	configCtx := context.Background()
	cfg := newTransactionTestConfig(configCtx)
	cfg.retryMax = 5

	var captured []*schema.AiOutputEvent
	cfg.emitter = NewEmitter("txn-test", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		captured = append(captured, e)
		return e, nil
	})

	requestCtx, cancelRequest := context.WithCancel(context.Background())
	started := make(chan context.Context, 1)
	var callCount atomic.Int64
	callAi := func(req *AIRequest) (*AIResponse, error) {
		callCount.Add(1)
		started <- req.GetContext()
		<-req.GetContext().Done()
		return nil, req.GetContext().Err()
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- CallAITransaction(
			cfg,
			"request-scoped cancellation",
			callAi,
			func(*AIResponse) error { return nil },
			WithAIRequest_Context(requestCtx),
		)
	}()

	select {
	case actualRequestCtx := <-started:
		require.NotNil(t, actualRequestCtx)
		cancelRequest()
	case <-time.After(3 * time.Second):
		t.Fatal("AI request did not start")
	}

	select {
	case err := <-errCh:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(3 * time.Second):
		t.Fatal("transaction did not stop after request context cancellation")
	}

	require.Equal(t, int64(1), callCount.Load(), "context cancellation must not be retried")
	for _, event := range captured {
		if event == nil {
			continue
		}
		content := string(event.Content)
		require.NotContains(t, content, "call ai api error")
		require.NotContains(t, content, "postHandler error")
	}
}

func TestCallAITransaction_PostHandlerCancellationDoesNotRetryOrEmitError(t *testing.T) {
	cfg := newTransactionTestConfig(context.Background())
	cfg.retryMax = 5

	var captured []*schema.AiOutputEvent
	cfg.emitter = NewEmitter("txn-test", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		captured = append(captured, e)
		return e, nil
	})

	requestCtx, cancelRequest := context.WithCancel(context.Background())
	var callCount atomic.Int64
	var postHandlerCount atomic.Int64
	callAi := func(*AIRequest) (*AIResponse, error) {
		callCount.Add(1)
		rsp := NewUnboundAIResponse()
		rsp.Close()
		return rsp, nil
	}
	postHandler := func(*AIResponse) error {
		postHandlerCount.Add(1)
		cancelRequest()
		return utils.Errorf("failed to parse action: %v", context.Canceled)
	}

	err := CallAITransaction(
		cfg,
		"post-handler cancellation",
		callAi,
		postHandler,
		WithAIRequest_Context(requestCtx),
	)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, int64(1), callCount.Load(), "post-handler cancellation must not call AI again")
	require.Equal(t, int64(1), postHandlerCount.Load(), "post-handler cancellation must not be retried")
	for _, event := range captured {
		if event == nil {
			continue
		}
		content := string(event.Content)
		require.NotContains(t, content, "call ai api error")
		require.NotContains(t, content, "postHandler error")
	}
}

func TestCallAITransaction_PostHandlerErrorEmitsBoundAIInfo(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := newTransactionTestConfig(ctx)
	cfg.retryMax = 1

	var captured []*schema.AiOutputEvent
	cfg.emitter = NewEmitter("txn-test", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		captured = append(captured, e)
		return e, nil
	})

	callAi := func(req *AIRequest) (*AIResponse, error) {
		rsp := NewUnboundAIResponse()
		rsp.SetModelInfo("openai", "gpt-4o-mini")
		rsp.Close()
		return rsp, nil
	}

	postHandler := func(rsp *AIResponse) error {
		return utils.Errorf("post handler failed")
	}

	err := CallAITransaction(cfg, "test prompt", callAi, postHandler)
	require.Error(t, err)

	var found bool
	for _, event := range captured {
		if event == nil || event.Type != schema.EVENT_TYPE_STRUCTURED {
			continue
		}
		if !strings.Contains(string(event.Content), "postHandler error") {
			continue
		}
		found = true
		assert.Equal(t, "openai", event.AIService)
		assert.Equal(t, "gpt-4o-mini", event.AIModelName)
		assert.NotEmpty(t, event.AIModelVerboseName)
	}
	require.True(t, found, "expected postHandler error event to be emitted")
}
