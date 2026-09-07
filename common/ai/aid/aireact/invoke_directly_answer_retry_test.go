package aireact

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	aicommon_testutil "github.com/yaklang/yaklang/common/ai/aid/aicommon/testutil"
	"github.com/yaklang/yaklang/common/utils"
)

func TestReAct_DirectlyAnswerBindsProviderRequestToCallerContext(t *testing.T) {
	requestStarted := make(chan context.Context, 1)
	release := make(chan struct{})
	defer close(release)

	ins, err := NewTestReAct(
		aicommon.WithAITransactionAutoRetry(1),
		aicommon.WithAICallback(func(_ aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			requestCtx := req.GetContext()
			requestStarted <- requestCtx
			if requestCtx == nil {
				<-release
				return nil, utils.Error("provider request context is nil")
			}
			select {
			case <-requestCtx.Done():
				return nil, requestCtx.Err()
			case <-release:
				return nil, utils.Error("test released provider request")
			}
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	callerCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, directErr := ins.DirectlyAnswer(callerCtx, "cancel this provider request", nil)
		done <- directErr
	}()

	var requestCtx context.Context
	select {
	case requestCtx = <-requestStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("provider request did not start")
	}
	if requestCtx == nil {
		t.Fatal("provider request did not inherit the directly-answer caller context")
	}

	cancel()
	select {
	case <-requestCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("provider request context was not cancelled")
	}
	select {
	case directErr := <-done:
		if !errors.Is(directErr, context.Canceled) {
			t.Fatalf("DirectlyAnswer error = %v, want context canceled", directErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("DirectlyAnswer did not return after caller cancellation")
	}
}

func TestReAct_DirectlyAnswer_RetryIncludesLastErrorAndAITAGHint(t *testing.T) {
	var prompts []string
	var promptMu sync.Mutex
	var attempts int32

	ins, err := NewTestReAct(
		aicommon.WithAIAutoRetry(1),
		aicommon.WithAITransactionAutoRetry(3),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			promptMu.Lock()
			prompts = append(prompts, req.GetPrompt())
			promptMu.Unlock()

			attempt := atomic.AddInt32(&attempts, 1)
			rsp := i.NewAIResponse()

			switch attempt {
			case 1, 2:
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"directly_answer"}`))
			case 3:
				nonce := aicommon_testutil.MustExtractPromptNonce(t, req.GetPrompt(), "FINAL_ANSWER")
				rsp.EmitOutputStream(bytes.NewBufferString(
					`{"@action":"directly_answer"}` + "\n" +
						"<|FINAL_ANSWER_" + nonce + "|>third time lucky<|FINAL_ANSWER_END_" + nonce + "|>",
				))
			default:
				t.Fatalf("unexpected retry attempt: %d", attempt)
			}

			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	answer, err := ins.DirectlyAnswer(context.Background(), "请直接回答这个问题，必要时用 AITAG 输出长内容", nil)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "third time lucky" {
		t.Fatalf("unexpected final answer: %q", answer)
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Fatalf("expected 3 attempts, got %d", got)
	}

	promptMu.Lock()
	capturedPrompts := append([]string(nil), prompts...)
	promptMu.Unlock()

	if len(capturedPrompts) != 3 {
		t.Fatalf("expected 3 captured prompts, got %d", len(capturedPrompts))
	}
	if strings.Contains(capturedPrompts[0], "AITAG retry hint:") {
		t.Fatalf("first prompt should not contain retry hint: %s", capturedPrompts[0])
	}
	if !utils.MatchAllOfSubString(
		capturedPrompts[1],
		"Retry due to error:",
		"no answer_payload key in stream",
		"AITAG retry hint:",
		"MUST use AITAG",
		"<|FINAL_ANSWER_",
	) {
		t.Fatalf("second prompt should include retry reason and AITAG hint, got: %s", capturedPrompts[1])
	}
}
