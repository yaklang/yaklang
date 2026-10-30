package orchestrator

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
	_ "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_report_generating"
)

func TestReportFallbackPreservesFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := aicommon.NewConfig(ctx, aicommon.WithWorkdir(t.TempDir()), aicommon.WithDisableAutoSkills(true),
		aicommon.WithAIRetryWaitFunc(func(ctx context.Context, _ time.Duration) error { return ctx.Err() }),
		aicommon.WithDisablePerception(true), aicommon.WithDisallowMCPServers(true), aicommon.WithAITransactionAutoRetry(1),
		aicommon.WithAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return nil, fmt.Errorf("report unavailable")
		}))
	runtime := mock.NewMockInvoker(ctx)
	runtime.SetConfig(cfg)
	loop, err := reactloops.NewReActLoop("audit-parent", runtime, reactloops.WithAllowToolCall(false))
	require.NoError(t, err)
	defer loop.Release()
	task := aicommon.NewStatefulTaskBase("audit", "audit fixture", ctx, cfg.GetEmitter(), true)
	state := model.NewAuditState()
	state.WorkDir = t.TempDir()
	state.ProjectPath = t.TempDir()
	err = runPhase4(loop, runtime, task, state, state.WorkDir)
	require.ErrorContains(t, err, "report unavailable", "saving a fallback must not turn failure into success")
	written, err := os.ReadFile(state.GetFinalReportPath())
	require.NoError(t, err)
	require.Contains(t, string(written), "报告生成阶段未完成")
	require.Equal(t, string(written), state.GetFinalReport())
}
