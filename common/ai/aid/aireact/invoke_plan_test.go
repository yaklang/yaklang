package aireact

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/schema"
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
