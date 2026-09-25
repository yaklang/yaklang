package reactloops

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils/omap"
)

// The scheduler is intercepted here to test the loop/response-protocol boundary.
// Real Config -> LiteForge execution is covered in aiforge's integration tests.
func TestSpeedLoopAuxiliaryProtocol(t *testing.T) {
	for _, mode := range []string{"text", "functioncall", "retry", "error", "skip", "intelligence"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			base := mock.NewMockedAIConfig(ctx).(*mock.MockedAIConfig)
			base.SetConfig("AiTransactionAutoRetry", 1)
			var scheduled, calls, verified int
			var loop *ReActLoop
			raw := `{"action":"accept","text":"streamed thought"}` + "\n<|BODY_nonce|>tag body<|BODY_END_nonce|>"
			cfg := &fcTestConfig{MockedAIConfig: base, aiCallback: func(req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				calls++
				require.Same(t, loop.GetCurrentTask().GetContext(), req.GetContext())
				require.Equal(t, "react-loop:test-auxiliary", req.GetCallerLabel())
				require.False(t, req.IsToolCallArgumentsStreamEnabled())
				response := base.NewAIResponse()
				if mode == "functioncall" {
					cfg := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
					cfg.ToolCallCallback([]*aispec.ToolCall{{Index: 0, ID: "call_accept", Type: "function",
						Function: aispec.FuncReturn{Name: "accept", Arguments: `{"text":"streamed thought","body":"tag body"}`}}})
					cfg.FinishReasonCallback("tool_calls", nil)
				}
				response.EmitOutputStream(strings.NewReader(raw))
				response.Close()
				return response, nil
			}}
			invoker := mock.NewMockInvoker(ctx)
			invoker.SetConfig(cfg)
			loop = NewMinimalReActLoop(cfg, invoker)
			loop.loopName = "test-auxiliary"
			loop.useSpeedPriorityAI = mode != "intelligence"
			loop.functionCallMode = mode == "functioncall"
			loop.SetCurrentTask(aicommon.NewStatefulTaskBase("active", "task", ctx, cfg.GetEmitter()))
			loop.actions = omap.NewEmptyOrderedMap[string, *LoopAction]()
			loop.actions.Set("accept", &LoopAction{ActionType: "accept", ActionVerifier: func(_ *ReActLoop, action *aicommon.Action) error {
				verified++
				if mode == "retry" && verified == 1 {
					return errors.New("retry validation")
				}
				require.Equal(t, "streamed thought", action.GetString("text"))
				return nil
			}})
			loop.aiTagFields = omap.NewEmptyOrderedMap[string, *LoopAITagField]()
			WithAITagField("BODY", "body")(loop)
			base.ScheduleAuxiliaryTaskFunc = func(gotCtx context.Context, name string, build func() string, onResult func(*aicommon.Action), opts ...aicommon.AuxiliaryTaskOption) {
				scheduled++
				require.Equal(t, "react-loop:test-auxiliary", name)
				require.Same(t, loop.GetCurrentTask().GetContext(), gotCtx)
				if mode == "skip" {
					return
				}
				spec := &aicommon.AuxiliaryTaskSpec{}
				for _, opt := range opts {
					opt(spec)
				}
				require.NotNil(t, spec.ResponseHandler)
				require.Same(t, loop.GetEmitter(), spec.Emitter)
				if mode == "error" {
					spec.OnError(context.Canceled)
					return
				}
				requestOpts := aicommon.NewGeneralKVConfig(spec.Opts...).GetExtraRequestOpts()
				for attempt := 0; attempt < 2; attempt++ {
					response, err := cfg.CallSpeedPriorityAI(aicommon.NewAIRequest(build(), requestOpts...))
					require.NoError(t, err)
					action, err := spec.ResponseHandler(response)
					if err != nil {
						require.EqualError(t, err, "retry validation")
						continue
					}
					onResult(action)
					return
				}
				t.Fatal("no accepted response")
			}
			var wg sync.WaitGroup
			loopCalls, _, _, err := loop.callAILoopTransaction(&wg, "protocol prompt", "nonce", nil,
				loop.emitLoopGeneralOutput, loop.emitLoopFunctionCallOutput)
			var action *aicommon.Action
			var handler *LoopAction
			if len(loopCalls) == 1 {
				action, handler = loopCalls[0].Action, loopCalls[0].LoopAction
			}
			wg.Wait()
			if mode == "intelligence" {
				require.Zero(t, scheduled)
			} else {
				require.Equal(t, 1, scheduled)
			}
			if mode == "skip" || mode == "error" {
				require.Error(t, err)
				require.Nil(t, action)
				require.Zero(t, calls)
				require.Zero(t, verified)
				if mode == "error" {
					require.ErrorIs(t, err, context.Canceled)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, "accept", handler.ActionType)
			require.Equal(t, "tag body", action.GetString("body"))
			require.Equal(t, raw, loop.Get("last_ai_decision_response"))
			if mode == "retry" {
				require.Equal(t, 2, verified)
			} else {
				require.Equal(t, 1, verified)
			}
		})
	}
}
