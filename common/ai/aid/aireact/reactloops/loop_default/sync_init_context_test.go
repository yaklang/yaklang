package loop_default

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

type initContextInvoker struct {
	*mock.MockInvoker
	calls int
}

func (i *initContextInvoker) ExecuteLoopTaskIF(_ string, _ aicommon.AIStatefulTask, _ ...any) (bool, error) {
	i.calls++
	return true, nil
}

func TestDefaultLoopSyncInitContextOptIn(t *testing.T) {
	for _, tc := range []struct {
		name           string
		allow, disable bool
		want           int
	}{
		{name: "default"},
		{name: "opt in", allow: true, want: 1},
		{name: "intent disabled", allow: true, disable: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inv := &initContextInvoker{MockInvoker: mock.NewMockInvoker(context.Background())}
			cfg := inv.GetConfig()
			cfg.SetConfig("AllowSyncInitContext", tc.allow)
			cfg.SetConfig("DisableIntentRecognition", tc.disable)
			loop := reactloops.NewMinimalReActLoop(cfg, inv)
			task := aicommon.NewStatefulTaskBase("test", strings.Repeat("Please help analyze this task in detail. ", 5), cfg.GetContext(), cfg.GetEmitter())
			loop.SetCurrentTask(task)
			buildInitTask(inv)(loop, task, &reactloops.InitTaskOperator{})
			require.Equal(t, tc.want, inv.calls)
		})
	}
}

func TestPETaskStillEnrichesContextAfterFastInitialization(t *testing.T) {
	inv := &initContextInvoker{MockInvoker: mock.NewMockInvoker(context.Background())}
	cfg := inv.GetConfig()
	loop := reactloops.NewMinimalReActLoop(cfg, inv)
	task := aicommon.NewStatefulTaskBase("pe-task", "execute the next step", cfg.GetContext(), cfg.GetEmitter())
	loop.SetCurrentTask(task)
	buildPETaskInitTask(inv)(loop, task, &reactloops.InitTaskOperator{})
	require.Equal(t, 1, inv.calls)
}

func TestGreetingCompletionWithoutSynchronousEnrichment(t *testing.T) {
	for _, tc := range []struct {
		name, query string
		attached    bool
		disable     bool
		wantSimple  bool
	}{
		{name: "Chinese greeting", query: "你好", wantSimple: true},
		{name: "English punctuation", query: " Hello! ", wantSimple: true},
		{name: "greeting with work", query: "你好，请读取 report.txt"},
		{name: "greeting with attachment", query: "你好", attached: true},
		{name: "intent explicitly disabled", query: "你好", disable: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inv := &initContextInvoker{MockInvoker: mock.NewMockInvoker(context.Background())}
			cfg := inv.GetConfig()
			cfg.SetConfig("AllowSyncInitContext", false)
			cfg.SetConfig("DisableIntentRecognition", tc.disable)
			loop := reactloops.NewMinimalReActLoop(cfg, inv)
			task := aicommon.NewStatefulTaskBase("greeting", tc.query, cfg.GetContext(), cfg.GetEmitter())
			if tc.attached {
				task.SetAttachedDatas([]*aicommon.AttachedResource{aicommon.NewAttachedResource("file", "file_content", "pending work")})
			}
			loop.SetCurrentTask(task)
			buildInitTask(inv)(loop, task, &reactloops.InitTaskOperator{})
			require.Zero(t, inv.calls, "cheap classification must not invoke an intent model")
			require.Equal(t, tc.wantSimple, loop.Get("intent_hint") == "simple_query")
			action, err := aicommon.ExtractAction(`{"@action":"directly_answer","answer_payload":"你好"}`, "directly_answer")
			require.NoError(t, err)
			op := reactloops.NewActionHandlerOperator(task)
			reactloops.DirectlyAnswerContinue(loop, action, op)
			done, err := op.IsTerminated()
			require.NoError(t, err)
			require.Equal(t, tc.wantSimple, done)
		})
	}
}
