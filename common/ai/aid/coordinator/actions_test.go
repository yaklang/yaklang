package coordinator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

type actionFixture struct {
	t    *testing.T
	c    *Controller
	cfg  *aicommon.Config
	loop *reactloops.ReActLoop
	task aicommon.AIStatefulTask
}

type actionArtifactInvoker struct {
	*mock.MockInvoker
	path string
}

func (r *actionArtifactInvoker) EmitFileArtifactWithExt(_ string, _ string, body any) string {
	if err := os.WriteFile(r.path, []byte(body.(string)), 0600); err != nil {
		return ""
	}
	return r.path
}

func newActionFixture(t *testing.T, approved bool) *actionFixture {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cfg := aicommon.NewConfig(ctx, aicommon.WithDisableCreateDBRuntime(true), aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true), aicommon.WithWorkdir(t.TempDir()), aicommon.WithGenerateReport(false))
	cfg.Timeline.SetTimelineBucketByteSize(-1)
	runtime := &actionArtifactInvoker{MockInvoker: mock.NewMockInvoker(ctx), path: filepath.Join(t.TempDir(), "report.md")}
	runtime.SetConfig(cfg)
	c := New(ctx, &testHost{plan: testPlan()}, 1)
	t.Cleanup(c.Close)
	if approved {
		v, err := c.CreatePlan(ctx, "plan", "document")
		require.NoError(t, err)
		require.NoError(t, c.SubmitPlan(ctx, v))
	}
	loop, err := NewLoop(runtime, WithController(c))
	require.NoError(t, err)
	task := aicommon.NewStatefulTaskBase("action-test", "verify action", ctx, cfg.GetEmitter(), true)
	loop.SetCurrentTask(task)
	return &actionFixture{t, c, cfg, loop, task}
}

func actionForTest(t *testing.T, name string, params map[string]any) *aicommon.Action {
	t.Helper()
	copy := map[string]any{"@action": name}
	for k, v := range params {
		copy[k] = v
	}
	data, err := json.Marshal(copy)
	require.NoError(t, err)
	a, err := aicommon.ExtractAction(string(data), name)
	require.NoError(t, err)
	return a
}

func (f *actionFixture) invoke(name string, params map[string]any, rejected bool) *reactloops.LoopActionHandlerOperator {
	f.t.Helper()
	h, err := f.loop.GetActionHandler(name)
	require.NoError(f.t, err)
	a := actionForTest(f.t, name, params)
	if h.ActionVerifier != nil {
		require.NoError(f.t, h.ActionVerifier(f.loop, a))
	}
	op := reactloops.NewActionHandlerOperator(f.task)
	h.ActionHandler(f.loop, a, op)
	_, err = op.IsTerminated()
	require.NoError(f.t, err)
	if rejected {
		require.Contains(f.t, op.GetFeedback().String(), "rejected")
	}
	if name != "directly_answer" {
		if !rejected {
			require.Contains(f.t, op.GetFeedback().String(), "completed")
		}
		require.Contains(f.t, f.cfg.GetSessionEvidenceRendered(), `"action":"`+name+`"`)
		require.NotContains(f.t, op.GetFeedback().String(), `"result"`)
	}
	return op
}
