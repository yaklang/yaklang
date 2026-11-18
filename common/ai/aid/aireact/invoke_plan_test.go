package aireact

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// The cached plan focus name enters the native coordinator. It permits
// bounded tools, but cannot recursively dispatch plans or blueprints.
func TestReAct_PlanLoop_Basic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	model := newNativePlanTestModel("")
	coordinatorCalls, workerCalls := 0, 0
	var reports atomic.Int32
	ins, err := NewTestReAct(aicommon.WithContext(ctx), aicommon.WithEnableFunctionCallMode(true), aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithDisableCreateDBRuntime(true), aicommon.WithNoOpMemoryTriage(), aicommon.WithAgreeYOLO(),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			if event.Type == "report_finish" {
				reports.Add(1)
			}
		}),
		aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
			require.NotNil(t, wire.ToolCallCallback)
			names := make([]string, 0, len(wire.Tools))
			for _, tool := range wire.Tools {
				names = append(names, tool.Function.Name)
			}
			for _, name := range []string{"request_plan", "request_plan_and_execution", "require_ai_blueprint", "load_capability", "dispatch_sub_react_agents"} {
				require.NotContains(t, names, name)
			}
			if strings.Contains(req.GetPrompt(), "执行已批准的冻结任务书。") {
				workerCalls++
				require.Contains(t, names, "submit_task_result")
			} else {
				coordinatorCalls++
				if strings.Contains(req.GetPrompt(), "阶段：PLAN") {
					require.Contains(t, names, "create_plan")
					require.Contains(t, names, "submit_plan")
					require.NotContains(t, names, "review_task")
				} else {
					require.Contains(t, req.GetPrompt(), "阶段：EXEC")
					require.NotContains(t, names, "create_plan")
					require.NotContains(t, names, "submit_plan")
					require.Contains(t, names, "review_task")
				}
			}
			rsp, handled, err := model(c, req, "native")
			require.True(t, handled)
			return rsp, err
		}))
	require.NoError(t, err)
	task := aicommon.NewStatefulTaskBase("plan-alias", "Run one deterministic check", ctx, ins.Emitter)
	_, err = ins.ExecuteLoopTask("plan", task)
	require.NoError(t, err)
	require.GreaterOrEqual(t, coordinatorCalls, 5)
	require.Equal(t, 2, workerCalls)
	require.Equal(t, int32(1), reports.Load())
}

func TestReAct_CoordinatorInputAttachments(t *testing.T) {
	const preview = "ONLY_IN_PINNED_FILE_7491"
	path := filepath.Join(t.TempDir(), "reference.txt")
	require.NoError(t, os.WriteFile(path, []byte(preview), 0o600))

	for _, focus := range []string{coordinator.Name, schema.AI_REACT_LOOP_NAME_DEFAULT} {
		t.Run(focus, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			firstPlanPrompt := make(chan string, 1)
			var pins, defaultRequests atomic.Int32
			r, err := NewTestReAct(aicommon.WithContext(ctx), aicommon.WithWorkdir(t.TempDir()),
				aicommon.WithDisableCreateDBRuntime(true), aicommon.WithNoOpMemoryTriage(), aicommon.WithEnableFunctionCallMode(false),
				aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
					if e.Type == schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME && e.GetContentJSONPath("$.path") == path {
						pins.Add(1)
					}
				}),
				aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					switch req.GetCallerLabel() {
					case "react-loop:default":
						defaultRequests.Add(1)
						resp := c.NewAIResponse()
						resp.EmitOutputStream(strings.NewReader(`{"@action":"request_plan_and_execution","plan_request_payload":"Plan using the pinned file"}`))
						resp.Close()
						return resp, nil
					case "react-loop:coordinator":
						// Stop at the first model request: no worker or later turn can supply the preview.
						select {
						case firstPlanPrompt <- req.GetPrompt():
						case <-ctx.Done():
						}
						<-ctx.Done()
						return nil, ctx.Err()
					default:
						return nil, fmt.Errorf("unexpected model request: %s", req.GetCallerLabel())
					}
				}))
			require.NoError(t, err)
			defer func() { cancel(); r.WaitLifecycleStopped() }()
			require.NoError(t, r.SendInputEvent(&ypb.AIInputEvent{
				IsFreeInput: true, FreeInput: "Plan using the pinned file", FocusModeLoop: focus,
				AttachedResourceInfo: []*ypb.AttachedResourceInfo{{Type: "file", Key: "file_path", Value: path}},
			}))
			select {
			case prompt := <-firstPlanPrompt:
				require.Equal(t, 1, strings.Count(prompt, "## Attached File\n"), "expected one file attachment in the first planning prompt")
				require.Equal(t, 1, strings.Count(prompt, preview), "render the file exactly once on either route")
				require.EqualValues(t, 1, pins.Load(), "only ingress should emit a pin for the original path")
				require.Equal(t, focus == schema.AI_REACT_LOOP_NAME_DEFAULT, defaultRequests.Load() > 0)
			case <-ctx.Done():
				t.Fatal("did not receive the first coordinator prompt")
			}
		})
	}
}
