package reactloops

import (
	"context"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"io"
	"testing"
)

type managedChildRuntime struct{}

func (*managedChildRuntime) AuthorizedTarget() string                               { return "https://workspace.invalid/test/" }
func (*managedChildRuntime) Execute(string, map[string]any) (map[string]any, error) { return nil, nil }
func (*managedChildRuntime) ManagedInputRestricted() bool                           { return true }

func TestManagedInputSubAgentInheritsResourceBoundary(t *testing.T) {
	ctx := context.Background()
	tool, err := aitool.New("read_file", aitool.WithNoRuntimeCallback(func(context.Context, aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
		return "authorized page", nil
	}))
	require.NoError(t, err)
	manager := buildinaitools.NewToolManagerByToolGetter(func() []*aitool.Tool { return []*aitool.Tool{tool} }, buildinaitools.WithOnlyTools(tool))
	runtime := &managedChildRuntime{}
	parent := aicommon.NewConfig(ctx, aicommon.WithDisableAutoSkills(true), aicommon.WithAiToolManager(manager), aicommon.WithLegionResultRuntime(runtime), aicommon.WithDisallowMCPServers(true))
	timeline := aicommon.NewTimeline(nil, nil)
	fork, err := timeline.ForkForTask("child", "child", parent, parent)
	require.NoError(t, err)
	getter := aicommon.AIRuntimeInvokerGetter
	t.Cleanup(func() { aicommon.AIRuntimeInvokerGetter = getter })
	aicommon.AIRuntimeInvokerGetter = func(ctx context.Context, opts ...aicommon.ConfigOption) (aicommon.AITaskInvokeRuntime, error) {
		return newSubAgentTestConfigInvoker(ctx, aicommon.NewConfig(ctx, opts...)), nil
	}
	child, err := BuildSubAgentInvokerForTest(parent, fork, ctx, parent.GetEmitter())
	require.NoError(t, err)
	cfg := child.GetConfig().(*aicommon.Config)
	require.Same(t, runtime, cfg.GetLegionResultRuntime())
	require.True(t, cfg.DisallowMCPServers)
	require.True(t, aicommon.HasManagedInputRestriction(cfg))
	require.Same(t, manager, cfg.GetAiToolManager())
	inherited, err := cfg.GetAiToolManager().GetToolByName("read_file")
	require.NoError(t, err)
	require.Same(t, tool, inherited)
	_, err = cfg.GetAiToolManager().GetToolByName("bash")
	require.Error(t, err)
}
