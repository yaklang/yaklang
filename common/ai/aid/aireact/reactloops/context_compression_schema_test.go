package reactloops

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
)

func TestContextCompressionRebuildPinsActionSchemas(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, constraint := range []string{"init_must_use", "init_disabled", "next_must_use", "next_disabled"} {
			t.Run(fmt.Sprintf("native=%v/%s", native, constraint), func(t *testing.T) {
				ctx := context.Background()
				cfg := aicommon.NewConfig(ctx, aicommon.WithDisableAutoSkills(true),
					aicommon.WithDisableCreateDBRuntime(true), aicommon.WithTimelineContentLimit(6000))
				cfg.Timeline.PushText(cfg.AcquireId(), "ordinary history")
				loop := makeSchemaStabilityTestLoop(cfg)
				loop.functionCallMode = native
				loop.actions.Set(aicommon.ContextCompressionAction, loopAction_CompressContext)
				op := NewActionHandlerOperator(nil)
				switch constraint {
				case "init_must_use":
					loop.initActionMustUse = []string{"finish"}
				case "init_disabled":
					loop.initActionDisabled = []string{"finish"}
				case "next_must_use":
					op.NextAction("finish")
				case "next_disabled":
					op.RemoveNextAction("finish")
				}
				invoker := &compressionPromptInvoker{MockInvoker: mock.NewMockInvoker(ctx), timeline: cfg.Timeline,
					overflowPadding: strings.Repeat("unfolded ", 9000)}
				invoker.SetConfig(cfg)
				loop.invoker = invoker
				input := loop.captureLoopPromptBuildInput("same-retry", "", "", nil, "", op)
				_, err := loop.generateLoopPromptFromInput(input)
				require.NoError(t, err)
				require.False(t, loop.initActionApplied, "rendering must leave initialization constraints active")
				before := invoker.inputs[len(invoker.inputs)-1]
				beforeTools := append([]string(nil), loop.lastNativeActionNames...)
				// A concurrent policy/inventory update is for the next business turn;
				// an overflow rebuild must keep this request's admitted tool snapshot.
				loop.actions.Set("later_action", &LoopAction{ActionType: "later_action"})
				loop.initActionApplied = true
				_, err = loop.generateLoopPromptFromInput(input)
				require.NoError(t, err)
				after := invoker.inputs[len(invoker.inputs)-1]
				require.Equal(t, before.Schema, after.Schema)
				require.Equal(t, before.FunctionCallSchemas, after.FunctionCallSchemas)
				require.Equal(t, beforeTools, loop.lastNativeActionNames)
			})
		}
	}
}
