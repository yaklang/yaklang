package aireact

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func compressionRequestActionNames(t *testing.T, req *aicommon.AIRequest) []string {
	t.Helper()
	wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
	if wire.ToolCallCallback != nil {
		var names []string
		for _, tool := range wire.Tools {
			names = append(names, tool.Function.Name)
		}
		return names
	}
	_, body, found := strings.Cut(req.GetPrompt(), "<|SCHEMA|>")
	require.True(t, found)
	_, body, found = strings.Cut(body, "```jsonschema")
	require.True(t, found)
	body, _, found = strings.Cut(body, "```")
	require.True(t, found)
	var document struct{ Properties map[string]json.RawMessage }
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(body)), &document))
	var selector struct{ Enum []string }
	require.NoError(t, json.Unmarshal(document.Properties["@action"], &selector))
	return selector.Enum
}

func TestReAct_CompressContextPreservesActionConstraints(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, source := range []string{"init", "next_action"} {
			t.Run(fmt.Sprintf("native=%v/%s", native, source), func(t *testing.T) {
				var decisions atomic.Int32
				var react *ReAct
				var err error
				appendHistory := func() {
					for i := 0; i < 3; i++ {
						react.config.Timeline.PushText(react.config.AcquireId(), "%s", strings.Repeat(uuid.NewString()+" ", 900))
					}
				}
				react, err = NewTestReAct(
					aicommon.WithWorkdir(t.TempDir()), aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true),
					aicommon.WithEnableFunctionCallMode(native), aicommon.WithTimelineContentLimit(60*1024),
					aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1),
					aicommon.WithAICallback(func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
						if aicommon.IsVerifySatisfactionPrompt(req.GetPrompt()) {
							return compressionActionResponse(t, cfg, req, "verify-satisfaction", map[string]any{"user_satisfied": true, "reasoning": "done"}), nil
						}
						step := decisions.Add(1)
						if source == "next_action" {
							if step == 1 {
								return compressionActionResponse(t, cfg, req, "prime", map[string]any{}), nil
							}
							step--
						}
						names := compressionRequestActionNames(t, req)
						switch step {
						case 1, 2:
							require.Contains(t, names, "probe")
							require.NotContains(t, names, "finish", "maintenance must preserve the pending business constraints")
							if step == 1 {
								require.Equal(t, aicommon.CallerLabelContextCompress, req.GetCallerLabel())
								return compressionActionResponse(t, cfg, req, aicommon.ContextCompressionAction, map[string]any{
									"summary": uuid.NewString(), "retain_timeline": "", "retire_skill_names": []string{}, "retire_evidence_ids": []string{},
								}), nil
							}
							return compressionActionResponse(t, cfg, req, "probe", map[string]any{}), nil
						case 3:
							require.Contains(t, names, "finish", "the constraints are consumed by the probe business action")
							return compressionActionResponse(t, cfg, req, "finish", map[string]any{}), nil
						default:
							return nil, fmt.Errorf("unexpected decision %d", step)
						}
					}))
				require.NoError(t, err)
				loop, err := reactloops.NewReActLoop("compression-constraints", react,
					reactloops.WithFunctionCallMode(native), reactloops.WithDisablePeriodicVerification(true),
					reactloops.WithDisableLoopPerception(true), reactloops.WithMaxIterations(5),
					reactloops.WithInitTask(func(_ *reactloops.ReActLoop, _ aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
						if source == "init" {
							op.NextAction("probe")
							op.RemoveNextAction("finish")
						}
					}),
					reactloops.WithRegisterLoopAction("prime", "prepare a constrained turn", nil, nil,
						func(_ *reactloops.ReActLoop, _ *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
							appendHistory()
							op.NextAction("probe")
							op.RemoveNextAction("finish")
							op.Continue()
						}),
					reactloops.WithRegisterLoopAction("probe", "perform the required business action", nil, nil,
						func(_ *reactloops.ReActLoop, _ *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
							op.Continue()
						}))
				require.NoError(t, err)
				if source == "init" {
					appendHistory()
				}
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				require.NoError(t, loop.Execute("compression-constraints-task", ctx, "Continue after compression."))
				want := int32(3)
				if source == "next_action" {
					want++
				}
				require.Equal(t, want, decisions.Load())
			})
		}
	}
}
