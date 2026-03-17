package aireact

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
)

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
				nonceRe := regexp.MustCompile(`<\|FINAL_ANSWER_(\w{4})\|>`)
				matches := nonceRe.FindStringSubmatch(req.GetPrompt())
				if len(matches) != 2 {
					t.Fatalf("expected FINAL_ANSWER nonce in prompt, got: %s", req.GetPrompt())
				}
				rsp.EmitOutputStream(bytes.NewBufferString(
					`{"@action":"directly_answer"}` + "\n" +
						"<|FINAL_ANSWER_" + matches[1] + "|>third time lucky<|FINAL_ANSWER_END_" + matches[1] + "|>",
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
