package aireact

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func mustMarshalJSONMap(t *testing.T, v map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return string(raw)
}

func TestReAct_SyncUpdateConfig_DisableIntentRecognition(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	in := make(chan *ypb.AIInputEvent, 16)

	var intentLoopCalled int32
	var mainLoopCalled int32

	ins, err := NewTestReAct(
		aicommon.WithContext(ctx),
		aicommon.WithEventInputChan(in),
		aicommon.WithDisableIntentRecognition(false),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := r.GetPrompt()
			rsp := i.NewAIResponse()

			if strings.Contains(prompt, "finalize_enrichment") && strings.Contains(prompt, "query_capabilities") {
				atomic.AddInt32(&intentLoopCalled, 1)
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"finalize_enrichment","human_readable_thought":"done","final_query":"test"}`))
				rsp.Close()
				return rsp, nil
			}

			atomic.AddInt32(&mainLoopCalled, 1)
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"directly_answer","answer_payload":"ok"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	syncID := uuid.NewString()
	in <- &ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      aicommon.SYNC_TYPE_UPDATE_CONFIG,
		SyncID:        syncID,
		SyncJsonInput: mustMarshalJSONMap(t, map[string]any{
			"DisableIntentRecognition": true,
		}),
	}

	require.Eventually(t, func() bool {
		return ins.config.GetConfigBool("DisableIntentRecognition")
	}, time.Second, 20*time.Millisecond)

	in <- &ypb.AIInputEvent{
		IsFreeInput: true,
		FreeInput:   "security audit " + uuid.NewString() + " with enough context to normally trigger deep intent recognition in default loop flow",
	}

	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&mainLoopCalled) > 0
	}, 3*time.Second, 20*time.Millisecond)

	require.Equal(t, int32(0), atomic.LoadInt32(&intentLoopCalled))
}

func TestReAct_SyncUpdateConfig_DisablePlan(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	in := make(chan *ypb.AIInputEvent, 16)
	out := make(chan *schema.AiOutputEvent, 16)
	promptCh := make(chan string, 2)

	_, err := NewTestReAct(
		aicommon.WithContext(ctx),
		aicommon.WithEventInputChan(in),
		aicommon.WithEnablePlanAndExec(true),
		aicommon.WithDisableIntentRecognition(true),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e
		}),
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

	syncID := uuid.NewString()
	in <- &ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      "update_config",
		SyncID:        syncID,
		SyncJsonInput: mustMarshalJSONMap(t, map[string]any{
			"EnablePlan": false,
		}),
	}

	require.Eventually(t, func() bool {
		select {
		case event := <-out:
			return event != nil && event.IsSync && event.SyncID == syncID && event.NodeId == "update_config"
		default:
			return false
		}
	}, 2*time.Second, 20*time.Millisecond)

	in <- &ypb.AIInputEvent{
		IsFreeInput: true,
		FreeInput:   "help me build a step-by-step plan for this repo",
	}

	var prompt string
	select {
	case prompt = <-promptCh:
	case <-ctx.Done():
		t.Fatal("timeout waiting for main loop prompt")
	}

	require.NotContains(t, prompt, "request_plan_and_execution")
	require.NotContains(t, prompt, "申请分步计划")
}

func TestReAct_SyncUpdateConfig_EmitStructuredResponse(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	in := make(chan *ypb.AIInputEvent, 16)
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
	in <- &ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      aicommon.SYNC_TYPE_UPDATE_CONFIG,
		SyncID:        syncID,
		SyncJsonInput: mustMarshalJSONMap(t, map[string]any{
			"EnablePlan":               false,
			"DisableIntentRecognition": true,
		}),
	}

	var result *schema.AiOutputEvent
	require.Eventually(t, func() bool {
		select {
		case event := <-out:
			if event != nil && event.IsSync && event.SyncID == syncID && event.NodeId == "update_config" {
				result = event
				return true
			}
		default:
		}
		return false
	}, 2*time.Second, 20*time.Millisecond)

	require.NotNil(t, result)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(result.Content, &payload))
	require.Equal(t, true, payload["applied"])

	current, ok := payload["current"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, false, current["EnablePlan"])
	require.Equal(t, true, current["DisableIntentRecognition"])

	require.True(t, ins.config.GetConfigBool("DisableIntentRecognition"))
}
