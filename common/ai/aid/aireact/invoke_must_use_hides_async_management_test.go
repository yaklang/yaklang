package aireact

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
)

func TestReAct_MustUseHidesAsyncManagement(t *testing.T) {
	manage := schema.AI_REACT_LOOP_ACTION_MANAGE_ASYNC_EXECUTION_RUNTIME
	for _, native := range []bool{false, true} {
		for _, source := range []string{"init", "next_action", "disabled_only", "unmatched", "named_async"} {
			t.Run(fmt.Sprintf("native=%v/%s", native, source), func(t *testing.T) {
				var decisions atomic.Int32
				missingAction := "missing-" + uuid.NewString()
				react, err := NewTestReAct(
					aicommon.WithWorkdir(t.TempDir()), aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true),
					aicommon.WithEnableFunctionCallMode(native), aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1),
					aicommon.WithAICallback(func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
						if aicommon.IsVerifySatisfactionPrompt(req.GetPrompt()) {
							return compressionActionResponse(t, cfg, req, "verify-satisfaction", map[string]any{"user_satisfied": true, "reasoning": "done"}), nil
						}
						step := decisions.Add(1)
						names := compressionRequestActionNames(t, req)
						switch source {
						case "init":
							switch step {
							case 1:
								require.Contains(t, names, "probe")
								require.Contains(t, names, aicommon.ContextCompressionAction)
								require.NotContains(t, names, manage)
								return compressionActionResponse(t, cfg, req, "probe", map[string]any{}), nil
							case 2:
								require.Contains(t, names, manage, "management returns after the required action runs")
								return compressionActionResponse(t, cfg, req, "finish", map[string]any{}), nil
							default:
								return nil, fmt.Errorf("unexpected decision %d", step)
							}
						case "next_action":
							switch step {
							case 1:
								require.Contains(t, names, manage)
								return compressionActionResponse(t, cfg, req, "prime", map[string]any{}), nil
							case 2:
								require.Contains(t, names, "probe")
								require.Contains(t, names, aicommon.ContextCompressionAction)
								require.NotContains(t, names, manage)
								return compressionActionResponse(t, cfg, req, "probe", map[string]any{}), nil
							case 3:
								require.Contains(t, names, manage, "management returns after the required action runs")
								return compressionActionResponse(t, cfg, req, "finish", map[string]any{}), nil
							default:
								return nil, fmt.Errorf("unexpected decision %d", step)
							}
						case "disabled_only":
							switch step {
							case 1:
								require.Contains(t, names, manage)
								require.NotContains(t, names, "finish")
								return compressionActionResponse(t, cfg, req, "probe", map[string]any{}), nil
							case 2:
								return compressionActionResponse(t, cfg, req, "finish", map[string]any{}), nil
							default:
								return nil, fmt.Errorf("unexpected decision %d", step)
							}
						case "unmatched":
							if step != 1 {
								return nil, fmt.Errorf("unexpected decision %d", step)
							}
							require.Contains(t, names, manage)
							require.Contains(t, names, "finish")
							return compressionActionResponse(t, cfg, req, "finish", map[string]any{}), nil
						case "named_async":
							switch step {
							case 1:
								require.Contains(t, names, manage)
								require.Contains(t, names, aicommon.ContextCompressionAction)
								require.NotContains(t, names, "finish")
								return compressionActionResponse(t, cfg, req, manage, map[string]any{"op": "list"}), nil
							case 2:
								require.Contains(t, names, "finish")
								return compressionActionResponse(t, cfg, req, "finish", map[string]any{}), nil
							default:
								return nil, fmt.Errorf("unexpected decision %d", step)
							}
						default:
							return nil, fmt.Errorf("unknown source %s", source)
						}
					}))
				require.NoError(t, err)
				loop, err := reactloops.NewReActLoop("must-use-hides-async", react,
					reactloops.WithFunctionCallMode(native), reactloops.WithDisablePeriodicVerification(true),
					reactloops.WithDisableLoopPerception(true), reactloops.WithMaxIterations(4),
					reactloops.WithInitTask(func(_ *reactloops.ReActLoop, _ aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
						switch source {
						case "init":
							op.NextAction("probe")
						case "disabled_only":
							op.RemoveNextAction("finish")
						case "unmatched":
							op.NextAction(missingAction)
						case "named_async":
							op.NextAction(manage)
						}
					}),
					reactloops.WithRegisterLoopAction("prime", "prepare a constrained turn", nil, nil,
						func(_ *reactloops.ReActLoop, _ *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
							op.NextAction("probe")
							op.Continue()
						}),
					reactloops.WithRegisterLoopAction("probe", "perform the required business action", nil, nil,
						func(_ *reactloops.ReActLoop, _ *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
							op.Continue()
						}))
				require.NoError(t, err)
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				require.NoError(t, loop.Execute("must-use-hides-async-"+uuid.NewString(), ctx, uuid.NewString()))
				want := int32(2)
				switch source {
				case "next_action":
					want = 3
				case "unmatched":
					want = 1
				}
				require.Equal(t, want, decisions.Load())
			})
		}
	}
}
