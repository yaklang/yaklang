package coordinator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
)

func TestSubForgeScopeAndLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	allowed := aitool.NewWithoutCallback("approved")
	impostor := aitool.NewWithoutCallback("approved")
	outside := aitool.NewWithoutCallback("outside")
	manager := buildinaitools.NewToolManagerByToolGetter(func() []*aitool.Tool { return []*aitool.Tool{allowed} },
		buildinaitools.WithEnabledTools([]string{allowed.Name}),
		buildinaitools.WithSearchToolEnabled(false), buildinaitools.WithForgeSearchToolEnabled(false))
	session := &Session{Config: aicommon.NewConfig(ctx, aicommon.WithWorkdir(t.TempDir()), aicommon.WithAiToolManager(manager))}
	task := aicommon.NewStatefulTaskBase("parent-task", "inspect", ctx, session.GetEmitter(), true)
	entered := make(chan struct{})
	finished := make(chan error, 1)
	binding := SubForgeTool{
		Schema: mcp.NewTool("sub_forge_inspect"),
		Tools:  []*aitool.Tool{allowed, impostor, outside},
		Run: func(childCtx context.Context, _ aitool.InvokeParams, tools []*aitool.Tool, _ ...aicommon.ConfigOption) (any, error) {
			close(entered)
			if len(tools) != 1 || tools[0] != allowed || !IsSubForgeContext(childCtx) {
				return nil, errors.New("child lost its scope")
			}
			_, err := session.invokeSubForge(childCtx, task, "recursive", SubForgeTool{}, nil)
			if err == nil {
				return nil, errors.New("recursion allowed")
			}
			<-childCtx.Done()
			return nil, childCtx.Err()
		},
	}
	session.subForgeTools = []SubForgeTool{binding}
	worker, descriptions, err := session.subForgeWorkerManager(task)
	require.NoError(t, err)
	require.Len(t, descriptions, 1)
	_, err = manager.GetToolByName(binding.Schema.Name)
	require.Error(t, err, "worker binding escaped into parent")
	tool, err := worker.GetToolByName(binding.Schema.Name)
	require.NoError(t, err)
	require.True(t, tool.RequireForeground)
	require.True(t, tool.RejectInterruptedReplay)
	go func() {
		_, err := session.invokeSubForge(ctx, task, "call", binding, nil)
		finished <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("child did not start")
	}
	_, err = session.invokeSubForge(ctx, task, "parallel", binding, nil)
	require.ErrorContains(t, err, "already running")
	cancel()
	select {
	case err := <-finished:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("child did not stop")
	}
}

func TestSubForgeDeadlineAndFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	session := &Session{Config: aicommon.NewConfig(ctx, aicommon.WithWorkdir(t.TempDir()))}
	task := aicommon.NewStatefulTaskBase("task", "query", ctx, session.GetEmitter(), true)
	failure := errors.New("child failed")
	binding := SubForgeTool{Run: func(ctx context.Context, _ aitool.InvokeParams, _ []*aitool.Tool, _ ...aicommon.ConfigOption) (any, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > subForgeTimeout {
			return nil, errors.New("missing default deadline")
		}
		return nil, failure
	}}
	_, err := session.invokeSubForge(ctx, task, "fail", binding, nil)
	require.ErrorIs(t, err, failure)
	childCtx, stop := context.WithTimeout(ctx, 50*time.Millisecond)
	defer stop()
	binding.Run = func(ctx context.Context, _ aitool.InvokeParams, _ []*aitool.Tool, _ ...aicommon.ConfigOption) (any, error) {
		actual, _ := ctx.Deadline()
		want, _ := childCtx.Deadline()
		if !actual.Equal(want) {
			return nil, errors.New("parent deadline was not preserved")
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	_, err = session.invokeSubForge(childCtx, task, "timeout", binding, nil)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}
