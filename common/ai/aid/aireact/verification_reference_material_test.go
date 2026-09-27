package aireact

import (
	"bytes"
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestVerifyUserSatisfaction_TaskCancellationStopsRequestWithoutRetry(t *testing.T) {
	var (
		events   []*schema.AiOutputEvent
		eventsMu sync.Mutex
		calls    atomic.Int64
	)
	requestStarted := make(chan context.Context, 1)

	ins, err := NewTestReAct(
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			eventsMu.Lock()
			defer eventsMu.Unlock()
			events = append(events, e)
		}),
		aicommon.WithAICallback(func(_ aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			calls.Add(1)
			requestStarted <- req.GetContext()
			<-req.GetContext().Done()
			return nil, req.GetContext().Err()
		}),
	)
	require.NoError(t, err)

	taskCtx, cancelTask := context.WithCancel(context.Background())
	type verifyOutcome struct {
		result *aicommon.VerifySatisfactionResult
		err    error
	}
	outcomeCh := make(chan verifyOutcome, 1)
	go func() {
		result, verifyErr := ins.VerifyUserSatisfaction(taskCtx, "completed report", false, "report saved")
		outcomeCh <- verifyOutcome{result: result, err: verifyErr}
	}()

	select {
	case requestCtx := <-requestStarted:
		require.NotNil(t, requestCtx)
		cancelTask()
	case <-time.After(3 * time.Second):
		t.Fatal("verification request did not start")
	}

	select {
	case outcome := <-outcomeCh:
		require.NoError(t, outcome.err)
		require.Nil(t, outcome.result, "cancelled observation should be quietly skipped")
	case <-time.After(3 * time.Second):
		t.Fatal("verification did not stop after task completion")
	}

	require.Equal(t, int64(1), calls.Load(), "cancelled verification must not retry")
	eventsMu.Lock()
	defer eventsMu.Unlock()
	for _, event := range events {
		if event == nil {
			continue
		}
		content := string(event.Content)
		require.NotContains(t, content, "call ai api error")
		require.NotContains(t, content, "postHandler error")
	}
}

func TestVerifyUserSatisfaction_DoesNotEmitModelExchangeReferences(t *testing.T) {
	var (
		events   []*schema.AiOutputEvent
		eventsMu sync.Mutex
	)

	queryToken := "verify-query-" + utils.RandStringBytes(8)
	payloadToken := "verify-payload-" + utils.RandStringBytes(8)
	rawResponse := `{"@action":"verify-satisfaction","user_satisfied":true,"reasoning":"verified"}`

	ins, err := NewTestReAct(
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			eventsMu.Lock()
			defer eventsMu.Unlock()
			events = append(events, e)
		}),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()
			require.Contains(t, prompt, queryToken)
			require.Contains(t, prompt, payloadToken)

			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(rawResponse))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	result, err := ins.VerifyUserSatisfaction(context.Background(), queryToken, false, payloadToken)
	require.NoError(t, err)
	require.True(t, result.Satisfied)

	ins.WaitForStream()

	eventsMu.Lock()
	defer eventsMu.Unlock()

	var sawVerificationStream bool
	for _, event := range events {
		require.NotEqual(t, schema.EVENT_TYPE_REFERENCE_MATERIAL, event.Type,
			"verification prompts and raw responses are not reference materials")
		if event.Type == schema.EVENT_TYPE_STREAM_START && event.NodeId == "re-act-verify" {
			sawVerificationStream = true
		}
	}
	require.True(t, sawVerificationStream, "verification progress must remain visible")

}
