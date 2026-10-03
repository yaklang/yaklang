package loop_task

import (
	"context"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"testing"
)

type initContextInvoker struct {
	*mock.MockInvoker
	calls int
}

func (i *initContextInvoker) ExecuteLoopTaskIF(_ string, _ aicommon.AIStatefulTask, _ ...any) (bool, error) {
	i.calls++
	return true, nil
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
