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
	"github.com/yaklang/yaklang/common/schema"
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
	require.Equal(t, "sub-Forge", tool.ExclusiveBatchGroup)
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

func TestSubForgeTimeoutKeepsSlotUntilCallbackSettles(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	session := &Session{Config: aicommon.NewConfig(ctx, aicommon.WithWorkdir(t.TempDir()))}
	task := aicommon.NewStatefulTaskBase("task", "query", ctx, session.GetEmitter(), true)
	entered, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	binding := SubForgeTool{Run: func(_ context.Context, _ aitool.InvokeParams, _ []*aitool.Tool, _ ...aicommon.ConfigOption) (any, error) {
		close(entered)
		<-release
		return "settled", nil
	}}
	callCtx, stop := context.WithCancel(ctx)
	defer stop()
	finished := make(chan error, 1)
	go func() {
		_, err := session.invokeSubForge(callCtx, task, "blocking", binding, nil)
		finished <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("child did not enter callback")
	}
	stop()
	select {
	case err := <-finished:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("caller remained blocked behind child callback")
	}
	_, err := session.invokeSubForge(ctx, task, "another", binding, nil)
	require.ErrorContains(t, err, "already running")
}

func TestSubForgeOverridesAttachedSessionOptions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	parent := aicommon.NewConfig(ctx, aicommon.WithWorkdir(t.TempDir()), aicommon.WithAgreeAIScoreLowMid(0.1, 0.2),
		aicommon.WithToolBatchMaxCalls(2), aicommon.WithToolBatchInvokeConcurrency(1),
		aicommon.WithAllowRequireForUserInteract(true),
		aicommon.WithClientUI(`{"protocol":"memfit.ui/1","questions":true}`))
	require.True(t, parent.UIQuestionsEnabled())
	session := &Session{Config: parent}
	task := aicommon.NewStatefulTaskBase("task", "query", ctx, parent.GetEmitter(), true)
	priorWorkdir := t.TempDir()
	binding := SubForgeTool{Run: func(ctx context.Context, _ aitool.InvokeParams, _ []*aitool.Tool, opts ...aicommon.ConfigOption) (any, error) {
		childOptions := []aicommon.ConfigOption{
			aicommon.WithPersistentSessionId("previous-session"),
			aicommon.WithWorkdir(priorWorkdir),
			aicommon.WithTimeline(parent.Timeline),
			aicommon.WithSessionPromptState(parent.GetSessionPromptState()),
		}
		return aicommon.NewConfig(ctx, append(childOptions, opts...)...), nil
	}}
	out, err := session.invokeSubForge(ctx, task, "isolate", binding, nil)
	require.NoError(t, err)
	child := out.(*aicommon.Config)
	require.Empty(t, child.PersistentSessionId)
	require.True(t, parent.UIQuestionsEnabled())
	require.False(t, child.UIQuestionsEnabled())
	require.NotSame(t, parent.Timeline, child.Timeline)
	require.NotEqual(t, priorWorkdir, child.Workdir)
	require.Equal(t, 0.1, child.AgreeAIScoreLow)
	require.Equal(t, 0.2, child.AgreeAIScoreMiddle)
	require.Equal(t, 2, child.GetConfigInt(aicommon.ConfigKeyToolBatchMaxCalls))
	require.Equal(t, 1, child.GetConfigInt(aicommon.ConfigKeyToolBatchInvokeConcurrency))
}

func TestSubForgeCancelReleasesPendingReview(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan *schema.AiOutputEvent, 16)
	unblockHandler := make(chan struct{})
	defer func() {
		select {
		case <-unblockHandler:
		default:
			close(unblockHandler)
		}
	}()
	parent := aicommon.NewConfig(ctx, aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			if event.GetInteractiveId() == "child-review" {
				events <- event
				if event.Type == schema.EVENT_TYPE_REQUIRE_USER_INTERACTIVE {
					<-unblockHandler
				}
			}
		}))
	session := &Session{Config: parent}
	task := aicommon.NewStatefulTaskBase("task", "query", ctx, parent.GetEmitter(), true)
	allowRelease := make(chan struct{})
	defer func() {
		select {
		case <-allowRelease:
		default:
			close(allowRelease)
		}
	}()
	childReleased := make(chan struct{})
	binding := SubForgeTool{Run: func(ctx context.Context, _ aitool.InvokeParams, _ []*aitool.Tool, opts ...aicommon.ConfigOption) (any, error) {
		child := aicommon.NewConfig(ctx, opts...)
		_, err := child.GetEmitter().EmitInteractiveJSON("child-review", schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE,
			"review-require", map[string]any{"id": "child-review"})
		if err != nil {
			return nil, err
		}
		<-allowRelease
		child.ReleaseInteractiveEvent("child-review", nil)
		close(childReleased)
		return nil, ctx.Err()
	}}
	finished := make(chan error, 1)
	go func() {
		_, err := session.invokeSubForge(ctx, task, "call", binding, nil)
		finished <- err
	}()
	readEvent := func() *schema.AiOutputEvent {
		t.Helper()
		select {
		case event := <-events:
			return event
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for a child event")
			return nil
		}
	}
	require.Equal(t, schema.EVENT_TYPE_REQUIRE_USER_INTERACTIVE, readEvent().Type)
	cancel()
	select {
	case err := <-finished:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("child cancellation did not reach the caller")
	}
	close(unblockHandler)
	release := readEvent()
	require.Equal(t, schema.EVENT_TYPE_REVIEW_RELEASE, release.Type)
	require.Equal(t, "call", release.CallToolID)
	close(allowRelease)
	select {
	case <-childReleased:
	case <-time.After(time.Second):
		t.Fatal("child did not release its endpoint")
	}
	require.Empty(t, events, "the child must not emit a second release after cleanup")
}
