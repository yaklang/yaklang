package aireact

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func mustMarshalSyncInput(t *testing.T, content string) string {
	t.Helper()

	raw, err := json.Marshal(map[string]string{
		"content": content,
	})
	require.NoError(t, err)
	return string(raw)
}

func TestReAct_SyncUserIntervention(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *schema.AiOutputEvent, 16)

	ins, err := NewTestReAct(
		aicommon.WithContext(ctx),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e
		}),
	)
	require.NoError(t, err)

	syncID := uuid.NewString()
	content := uuid.NewString()
	in <- &ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      aicommon.SYNC_TYPE_USER_INTERVENTION,
		SyncID:        syncID,
		SyncJsonInput: mustMarshalSyncInput(t, content),
	}

	var result *schema.AiOutputEvent
LOOP:
	for {
		select {
		case event := <-out:
			if event != nil && event.IsSync && event.SyncID == syncID && event.NodeId == "user_intervention" {
				result = event
				break LOOP
			}
		case <-ctx.Done():
			t.Fatal("timeout waiting for user_intervention sync event")
		}
	}

	require.NotNil(t, result)
	require.Equal(t, schema.EVENT_TYPE_STRUCTURED, result.Type)

	var payload map[string]string
	require.NoError(t, json.Unmarshal(result.Content, &payload))
	require.Equal(t, content, payload["content"])

	require.Eventually(t, func() bool {
		return strings.Contains(ins.DumpTimeline(), "[User Intervention] "+content)
	}, time.Second, 20*time.Millisecond)

	history := ins.config.GetUserInputHistory()
	require.Len(t, history, 1)
	require.Equal(t, content, history[0].UserInput)

	nonce := uuid.NewString()
	ctxWithNonce := ins.promptManager.DynamicContextWithNonce(nonce)
	require.Contains(t, ctxWithNonce, "<|PREV_USER_INPUT_"+nonce+"|>")
	require.Contains(t, ctxWithNonce, "# Session User Input History")
	require.Contains(t, ctxWithNonce, "Round 1")
	require.Contains(t, ctxWithNonce, content)

	plainCtx := ins.promptManager.DynamicContext()
	require.Contains(t, plainCtx, "# Session User Input History")
	require.Contains(t, plainCtx, content)
}

func TestReAct_SyncUserIntervention_EmptyContent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *schema.AiOutputEvent, 16)

	ins, err := NewTestReAct(
		aicommon.WithContext(ctx),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e
		}),
	)
	require.NoError(t, err)

	in <- &ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      aicommon.SYNC_TYPE_USER_INTERVENTION,
		SyncJsonInput: mustMarshalSyncInput(t, ""),
	}

	var result *schema.AiOutputEvent
LOOP:
	for {
		select {
		case event := <-out:
			if event != nil && !event.IsSync && event.NodeId == "system" {
				var payload map[string]string
				if json.Unmarshal(event.Content, &payload) == nil &&
					payload["level"] == "error" &&
					payload["message"] == "content is empty in sync json input" {
					result = event
					break LOOP
				}
			}
		case <-ctx.Done():
			t.Fatal("timeout waiting for user_intervention error event")
		}
	}

	require.NotNil(t, result)
	require.Empty(t, ins.DumpTimeline())
	require.Nil(t, ins.config.GetUserInputHistory())

	nonce := uuid.NewString()
	ctxWithNonce := ins.promptManager.DynamicContextWithNonce(nonce)
	require.NotContains(t, ctxWithNonce, "Session User Input History")
	require.NotContains(t, ctxWithNonce, "<|PREV_USER_INPUT_"+nonce+"|>")
}

func TestReAct_SyncUserIntervention_PromptContainsHistoryForAI(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	in := make(chan *ypb.AIInputEvent, 10)
	promptCh := make(chan string, 1)

	content := uuid.NewString()
	userInput := "free-input-" + uuid.NewString()

	ins, err := NewTestReAct(
		aicommon.WithContext(ctx),
		aicommon.WithEventInputChan(in),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			select {
			case promptCh <- r.GetPrompt():
			default:
			}
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"directly_answer","answer_payload":"ok"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	in <- &ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      aicommon.SYNC_TYPE_USER_INTERVENTION,
		SyncID:        uuid.NewString(),
		SyncJsonInput: mustMarshalSyncInput(t, content),
	}

	require.Eventually(t, func() bool {
		history := ins.config.GetUserInputHistory()
		return len(history) == 1 && history[0].UserInput == content
	}, time.Second, 20*time.Millisecond)

	in <- &ypb.AIInputEvent{
		IsFreeInput: true,
		FreeInput:   userInput,
	}

	var prompt string
	select {
	case prompt = <-promptCh:
	case <-ctx.Done():
		t.Fatal("timeout waiting for ai prompt after sync user intervention")
	}

	require.Eventually(t, func() bool {
		history := ins.config.GetUserInputHistory()
		return len(history) == 2 &&
			history[0].UserInput == content &&
			history[1].UserInput == userInput
	}, time.Second, 20*time.Millisecond)

	userQueryBlock := mustExtractAITagBlock(t, prompt, "USER_QUERY")
	require.Equal(t, userInput, userQueryBlock.Body)

	prevUserInputBlock := mustExtractAITagBlock(t, prompt, "PREV_USER_INPUT")
	require.Equal(t, strings.TrimSpace(ins.config.FormatUserInputHistory()), prevUserInputBlock.Body)
	require.Less(t, prevUserInputBlock.StartIndex, userQueryBlock.StartIndex)
	require.NotContains(t, userQueryBlock.Body, content)
}
