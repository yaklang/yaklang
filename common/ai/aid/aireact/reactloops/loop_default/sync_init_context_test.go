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
