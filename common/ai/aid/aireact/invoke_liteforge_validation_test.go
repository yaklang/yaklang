package aireact

import (
	"bytes"
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestAuxiliaryOutputValidationRetriesBeforeReturning(t *testing.T) {
	for _, recoverOutput := range []bool{true, false} {
		t.Run(fmt.Sprint(recoverOutput), func(t *testing.T) {
			var calls atomic.Int32
			r, err := NewTestReAct(aicommon.WithAIAutoRetry(1), aicommon.WithAIRetryWaitFunc(func(context.Context, time.Duration) error { return nil }), aicommon.WithAITransactionAutoRetry(2), aicommon.WithSpeedPriorityAICallback(func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				n := calls.Add(1)
				body := `{"@action":"memory-triage","memory_entities":[{"content":"valid prefix"},[]]}`
				if n > 1 {
					require.Contains(t, req.GetPrompt(), "array item 1")
					if recoverOutput {
						body = `{"@action":"memory-triage","memory_entities":[]}`
					}
				}
				rsp := cfg.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(body))
				rsp.Close()
				return rsp, nil
			}))
			require.NoError(t, err)
			action, err := r.InvokeSpeedPriorityLiteForge(context.Background(), "memory-triage", "extract facts", []aitool.ToolOption{aitool.WithRawParam("memory_entities", map[string]any{})}, aicommon.WithLiteForgeDisableTimeline(), aicommon.WithLiteForgeOutputValidator(func(action *aicommon.Action) error {
				_, present, err := action.GetCanonicalObjectArray("memory_entities")
				if err != nil {
					return err
				}
				if !present {
					return fmt.Errorf("missing memory_entities")
				}
				return nil
			}))
			require.EqualValues(t, 2, calls.Load(), "repair is bounded by existing transaction budget")
			if recoverOutput {
				require.NoError(t, err)
				require.NotNil(t, action)
			} else {
				require.ErrorContains(t, err, "array item 1")
				require.Nil(t, action)
			}
		})
	}
}
