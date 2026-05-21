package aireact

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestReAct_ConfigHotpatch_DisablePlan(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	in := make(chan *ypb.AIInputEvent, 16)
	configUpdated := make(chan struct{}, 1)
	promptCh := make(chan string, 2)

	ins, err := NewTestReAct(
		aicommon.WithContext(ctx),
		aicommon.WithEventInputChan(in),
		aicommon.WithEnablePlanAndExec(true),
		aicommon.WithDisableIntentRecognition(true),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			if e != nil && e.Type == schema.EVENT_TYPE_AID_CONFIG {
				select {
				case configUpdated <- struct{}{}:
				default:
				}
			}
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

	in <- &ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     aicommon.HotPatchType_EnablePlan,
		Params: &ypb.AIStartParams{
			EnablePlan: false,
		},
	}

	require.Eventually(t, func() bool {
		select {
		case <-configUpdated:
			return true
		default:
			return false
		}
	}, 2*time.Second, 20*time.Millisecond)

	require.False(t, ins.config.GetEnablePlanAndExec())

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

	require.NotContains(t, prompt, "申请分步计划")
	require.NotContains(t, prompt, `{"@action": "request_plan_and_execution"`)
}

func TestReAct_ConfigHotpatch_SyncPerceptionTrigger(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	in := make(chan *ypb.AIInputEvent, 16)
	ins, err := NewTestReAct(
		aicommon.WithContext(ctx),
		aicommon.WithEventInputChan(in),
	)
	require.NoError(t, err)
	require.False(t, ins.config.GetSyncPerceptionTrigger())

	in <- &ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     aicommon.HotPatchType_SyncPerceptionTrigger,
		Params: &ypb.AIStartParams{
			SyncPerceptionTrigger: true,
		},
	}

	require.Eventually(t, func() bool {
		return ins.config.GetSyncPerceptionTrigger()
	}, time.Second, 20*time.Millisecond)
}
