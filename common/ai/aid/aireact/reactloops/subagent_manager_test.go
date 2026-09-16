package reactloops

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicache"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
)

type managedLoopBuilder func(*PreparedSubAgent) (*ReActLoop, error)

func (f managedLoopBuilder) Build(p *PreparedSubAgent) (*ReActLoop, error) { return f(p) }

func backgroundSubAgentFixture(t *testing.T, concurrency int) (*ReActLoop, aicommon.AIStatefulTask) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	cfg := aicommon.NewConfig(ctx, aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true), aicommon.WithWorkdir(t.TempDir()), aicommon.WithMaxSubAgents(int64(concurrency)))
	inv := mock.NewMockInvoker(ctx)
	inv.SetConfig(cfg)
	loop := NewMinimalReActLoop(cfg, inv)
	task := aicommon.NewStatefulTaskBase("background-parent", "investigate incident", ctx, cfg.GetEmitter(), true)
	loop.SetCurrentTask(task)
	loop.ensureLoopDirectory(task)
	original := aicommon.AIRuntimeInvokerGetter
	aicommon.AIRuntimeInvokerGetter = func(ctx context.Context, options ...aicommon.ConfigOption) (aicommon.AITaskInvokeRuntime, error) {
		child := mock.NewMockInvoker(ctx)
		child.SetConfig(aicommon.NewConfig(ctx, options...))
		return child, nil
	}
	t.Cleanup(func() { aicommon.AIRuntimeInvokerGetter = original })
	t.Cleanup(loop.shutdownSubAgents)
	return loop, task
}

func blockingSubAgentBuilder(started chan<- string, release <-chan struct{}, output string) managedLoopBuilder {
	return func(p *PreparedSubAgent) (*ReActLoop, error) {
		loop := NewMinimalReActLoop(p.Invoker.GetConfig(), p.Invoker)
		WithInitTask(func(_ *ReActLoop, task aicommon.AIStatefulTask, op *InitTaskOperator) {
			started <- task.GetId()
			select {
			case <-release:
				task.SetResult(output)
			case <-task.GetContext().Done():
			}
			op.Done()
		})(loop)
		return loop, nil
	}
}

func awaitBackgroundTerminal(t *testing.T, m *SubAgentManager, ids []string) []SubAgentSnapshot {
	t.Helper()
	require.Eventually(t, func() bool {
		jobs, err := m.Inspect(ids)
		if err != nil {
			return false
		}
		for _, job := range jobs {
			if !job.terminal() {
				return false
			}
		}
		return true
	}, 3*time.Second, time.Millisecond)
	jobs, err := m.Inspect(ids)
	require.NoError(t, err)
	return jobs
}

func TestBackgroundSubAgents_NonblockingQueueWaitCancellationAndReferences(t *testing.T) {
	loop, task := backgroundSubAgentFixture(t, 1)
	started := make(chan string, 3)
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	content := strings.Repeat("full-result-middle-evidence\n", 400)
	opts := SubAgentOptions{TimelineMode: SubAgentTimelineFork, LoopBuilder: blockingSubAgentBuilder(started, release, content)}
	first, err := loop.SubmitSubAgents(task, []SubAgentJob{{Identifier: "first", Goal: "payment"}}, opts, "action-1")
	require.NoError(t, err)
	require.Equal(t, first.Jobs[0].ID, <-started)
	second, err := loop.SubmitSubAgents(task, []SubAgentJob{{Identifier: "second", Goal: "inventory"}}, opts, "action-2")
	require.NoError(t, err)
	require.Equal(t, "queued", second.Jobs[0].State)
	m := loop.GetSubAgentManager()
	replay, err := loop.SubmitSubAgents(task, []SubAgentJob{{Identifier: "different"}}, opts, "action-2")
	require.NoError(t, err)
	require.Equal(t, second.BatchID, replay.BatchID)

	observation, err := m.Wait(task.GetContext(), []string{first.Jobs[0].ID}, time.Millisecond, 0)
	require.NoError(t, err)
	require.True(t, observation.TimedOut)
	require.False(t, observation.Jobs[0].terminal())
	require.NoError(t, task.GetContext().Err())
	require.NotEmpty(t, loop.SubAgentFinishBlockReason())
	_, err = m.Inspect([]string{"foreign-or-unknown"})
	require.Error(t, err)
	_, err = m.Cancel([]string{second.Jobs[0].ID})
	require.NoError(t, err)
	cancelled := awaitBackgroundTerminal(t, m, []string{second.Jobs[0].ID})
	require.Equal(t, "cancelled", cancelled[0].State)
	select {
	case id := <-started:
		t.Fatalf("cancelled queued worker started: %s", id)
	default:
	}

	once.Do(func() { close(release) })
	completed := awaitBackgroundTerminal(t, m, []string{first.Jobs[0].ID})
	require.Equal(t, "completed", completed[0].State)
	raw, err := os.ReadFile(completed[0].ResultReference)
	require.NoError(t, err)
	require.Equal(t, strings.TrimSpace(content), string(raw))
	require.NotEmpty(t, m.FinishBlockReason(0, false), "terminal but not model-visible")
	records, revision := m.pending(0)
	require.Len(t, records, 2)
	require.Empty(t, m.FinishBlockReason(revision, true))
	_, err = loop.SubmitSubAgents(task, []SubAgentJob{{Identifier: "late"}}, opts, "late")
	require.Error(t, err)
}

func TestBackgroundSubAgents_WaitWakeAndCleanupPending(t *testing.T) {
	loop, task := backgroundSubAgentFixture(t, 2)
	started := make(chan string, 2)
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	// Deliberately ignore cancellation to verify truthful cleanup reporting.
	builder := managedLoopBuilder(func(p *PreparedSubAgent) (*ReActLoop, error) {
		child := NewMinimalReActLoop(p.Invoker.GetConfig(), p.Invoker)
		WithInitTask(func(_ *ReActLoop, task aicommon.AIStatefulTask, op *InitTaskOperator) {
			started <- task.GetId()
			<-release
			task.SetResult("late evidence")
			op.Done()
		})(child)
		return child, nil
	})
	receipt, err := loop.SubmitSubAgents(task, []SubAgentJob{{Identifier: "stubborn"}}, SubAgentOptions{TimelineMode: SubAgentTimelineClean, LoopBuilder: builder}, "start")
	require.NoError(t, err)
	<-started
	m := loop.GetSubAgentManager()
	_, err = m.Wait(task.GetContext(), []string{receipt.Jobs[0].ID}, 10*time.Minute+time.Millisecond, 0)
	require.Error(t, err)
	snapshots := m.Close(time.Millisecond)
	require.True(t, snapshots[0].CleanupPending)
	require.Equal(t, "cancelling", snapshots[0].State)
	once.Do(func() { close(release) })
	// A ten-minute observation must still wake as soon as the worker settles.
	observation, err := m.Wait(task.GetContext(), []string{receipt.Jobs[0].ID}, 10*time.Minute, 0)
	require.NoError(t, err)
	require.False(t, observation.TimedOut)
	require.Equal(t, "cancelled", observation.Jobs[0].State)
	require.False(t, observation.Jobs[0].CleanupPending)
}

func TestBackgroundSubAgents_ParentContinuesAndFinishSeesInFlightResult(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	release := make(chan struct{})
	started := make(chan string, 1)
	var releaseOnce sync.Once
	releaseChild := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseChild()
	var calls atomic.Int32
	var loop *ReActLoop
	var localWork atomic.Bool
	cfg := aicommon.NewConfig(ctx, aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true), aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			step := calls.Add(1)
			action := "finish"
			switch step {
			case 1:
				action = "dispatch_test"
			case 2:
				require.NotContains(t, req.GetPrompt(), "PAYMENT_EVIDENCE")
				action = "read_deployment"
			case 3:
				// Result arrives AFTER this prompt was built, BEFORE the finish action.
				require.True(t, localWork.Load())
				require.NotContains(t, req.GetPrompt(), "PAYMENT_EVIDENCE")
				releaseChild()
				awaitBackgroundTerminal(t, loop.GetSubAgentManager(), nil)
			case 4:
				require.Contains(t, req.GetPrompt(), "PAYMENT_EVIDENCE")
				var dynamic string
				for _, chunk := range aicache.Split(req.GetPrompt()).Chunks {
					if chunk.Section == aicache.SectionDynamic {
						dynamic += chunk.Content
					}
				}
				require.Equal(t, 1, strings.Count(dynamic, "PAYMENT_EVIDENCE"), "delivery must survive provider cache splitting exactly once")
			default:
				t.Errorf("unexpected model request %d", step)
			}
			response := c.NewAIResponse()
			response.EmitOutputStream(bytes.NewBufferString("{\"@action\":\"" + action + "\"}"))
			response.Close()
			return response, nil
		}))
	inv := mock.NewMockInvoker(ctx)
	inv.SetConfig(cfg)
	original := aicommon.AIRuntimeInvokerGetter
	defer func() { aicommon.AIRuntimeInvokerGetter = original }()
	aicommon.AIRuntimeInvokerGetter = func(ctx context.Context, opts ...aicommon.ConfigOption) (aicommon.AITaskInvokeRuntime, error) {
		child := mock.NewMockInvoker(ctx)
		child.SetConfig(aicommon.NewConfig(ctx, opts...))
		return child, nil
	}
	var err error
	loop, err = NewReActLoop("background-integration", inv,
		WithAllowToolCall(false), WithAllowRAG(false), WithAllowAIForge(false), WithAllowPlanAndExec(false), WithAllowUserInteract(false),
		WithRegisterLoopAction("require_tool", "unused", nil, nil, nil),
		WithDisableLoopPerception(true), WithDisablePeriodicVerification(true), WithDisableIncreaseIteration(true),
		WithRegisterLoopAction("dispatch_test", "dispatch", nil, nil, func(l *ReActLoop, _ *aicommon.Action, op *LoopActionHandlerOperator) {
			_, err := l.SubmitSubAgents(op.GetTask(), []SubAgentJob{{Identifier: "payment", Goal: "payment"}},
				SubAgentOptions{TimelineMode: SubAgentTimelineFork, LoopBuilder: blockingSubAgentBuilder(started, release, "PAYMENT_EVIDENCE")}, "integration")
			require.NoError(t, err)
			op.Continue()
		}),
		WithRegisterLoopAction("read_deployment", "independent local tool", nil, nil, func(_ *ReActLoop, _ *aicommon.Action, op *LoopActionHandlerOperator) {
			select {
			case <-started:
			case <-ctx.Done():
				t.Error("child failed to start")
			}
			select {
			case <-release:
				t.Error("parent waited for child before local work")
			default:
			}
			localWork.Store(true)
			op.Continue()
		}),
	)
	require.NoError(t, err)
	require.NoError(t, loop.Execute("integration-parent", ctx, "investigate payment and deployment"))
	require.EqualValues(t, 4, calls.Load())
	require.True(t, localWork.Load())
}

func TestBackgroundSubAgents_NewResultAllowsNewAnswer(t *testing.T) {
	loop, _ := backgroundSubAgentFixture(t, 1)
	action := aicommon.NewSimpleAction("directly_answer", nil)
	noteDirectlyAnswerDeliveredWithoutTodoDelta(loop, action)
	require.Error(t, RejectDuplicateDirectlyAnswerWithoutTodoDelta(loop, action))
	loop.subAgentModelSeen = 1
	require.NoError(t, RejectDuplicateDirectlyAnswerWithoutTodoDelta(loop, action))
	noteDirectlyAnswerDeliveredWithoutTodoDelta(loop, action)
	require.Error(t, RejectDuplicateDirectlyAnswerWithoutTodoDelta(loop, action))
}

func TestBackgroundSubAgents_QueuedConfigSnapshotAndIdentity(t *testing.T) {
	loop, task := backgroundSubAgentFixture(t, 1)
	cfg := loop.GetConfig().(*aicommon.Config)
	cfg.SessionPromptState.SetSessionEvidence("evidence-at-dispatch")
	cfg.SessionPromptState.SetVerificationTodo("parent-only-todo")
	cfg.GetTimeline().PushText(cfg.AcquireId(), "parent-history-at-dispatch")
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	type observed struct{ evidence, taskID, forkID, history, todo string }
	seen := make(chan observed, 2)
	builder := managedLoopBuilder(func(p *PreparedSubAgent) (*ReActLoop, error) {
		child := NewMinimalReActLoop(p.Invoker.GetConfig(), p.Invoker)
		WithInitTask(func(_ *ReActLoop, task aicommon.AIStatefulTask, op *InitTaskOperator) {
			childCfg := p.Invoker.GetConfig().(*aicommon.Config)
			seen <- observed{
				evidence: childCfg.SessionPromptState.GetSessionEvidence(), taskID: task.GetId(), forkID: p.Timeline.Fork().TaskIndex,
				history: childCfg.GetTimeline().Dump(), todo: childCfg.SessionPromptState.GetVerificationTodo(),
			}
			childCfg.GetTimeline().PushText(childCfg.AcquireId(), "child-private-history")
			childCfg.SessionPromptState.SetVerificationTodo("child-only-todo")
			select {
			case <-release:
			case <-task.GetContext().Done():
			}
			task.SetResult("done")
			op.Done()
		})(child)
		return child, nil
	})
	opts := SubAgentOptions{TimelineMode: SubAgentTimelineFork, LoopBuilder: builder}
	first, err := loop.SubmitSubAgents(task, []SubAgentJob{{Identifier: "first"}}, opts, "first")
	require.NoError(t, err)
	firstSeen := <-seen
	require.Equal(t, first.Jobs[0].ID, firstSeen.taskID)
	require.Equal(t, firstSeen.taskID, firstSeen.forkID)
	second, err := loop.SubmitSubAgents(task, []SubAgentJob{{Identifier: "queued"}}, opts, "second")
	require.NoError(t, err)
	cfg.SessionPromptState.SetSessionEvidence("new-parent-evidence-after-dispatch")
	cfg.GetTimeline().PushText(cfg.AcquireId(), "new-parent-history-after-dispatch")
	once.Do(func() { close(release) })
	secondSeen := <-seen
	require.Equal(t, second.Jobs[0].ID, secondSeen.taskID)
	require.Equal(t, secondSeen.taskID, secondSeen.forkID)
	require.Equal(t, "evidence-at-dispatch", secondSeen.evidence)
	require.Contains(t, secondSeen.history, "parent-history-at-dispatch")
	require.NotContains(t, secondSeen.history, "new-parent-history-after-dispatch")
	require.NotContains(t, secondSeen.history, "child-private-history")
	require.Empty(t, secondSeen.todo)
	awaitBackgroundTerminal(t, loop.GetSubAgentManager(), nil)
	require.NotContains(t, cfg.GetTimeline().Dump(), "child-private-history")
	require.Equal(t, "parent-only-todo", cfg.SessionPromptState.GetVerificationTodo())
}

func TestBackgroundSubAgents_CancelAdmissionBeforeContextSignal(t *testing.T) {
	// Model the exact interleaving: Cancel committed cancelling under the lock,
	// but has not invoked the cancel function when the queued worker gets a slot.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := newSubAgentManager(ctx, 1)
	entry := &managedSubAgent{snapshot: SubAgentSnapshot{ID: "queued", State: "cancelling"}, cancel: cancel}
	m.jobs["queued"] = entry
	m.order = []string{"queued"}
	m.wg.Add(1)
	m.run(entry, ctx, &PreparedSubAgent{}, SubAgentOptions{}, nil, nil, NewProgressRegistry())
	snapshots, err := m.Inspect(nil)
	require.NoError(t, err)
	require.Equal(t, "cancelled", snapshots[0].State)
	require.Nil(t, snapshots[0].StartedAt, "runtime must never be armed")
}

func TestBackgroundSubAgents_CleanTimelineStillInheritsSessionContext(t *testing.T) {
	loop, task := backgroundSubAgentFixture(t, 1)
	cfg := loop.GetConfig().(*aicommon.Config)
	cfg.GetTimeline().PushText(cfg.AcquireId(), "parent-timeline-only")
	cfg.SessionPromptState.SetSessionEvidence("shared-at-dispatch-evidence")
	cfg.SessionPromptState.SetVerificationTodo("parent-todo")
	_, err := cfg.AppendUserInputHistory("original-user-question", time.Now())
	require.NoError(t, err)
	type contextView struct{ history, evidence, todo, previousInput string }
	seen := make(chan contextView, 1)
	builder := managedLoopBuilder(func(p *PreparedSubAgent) (*ReActLoop, error) {
		childCfg := p.Invoker.GetConfig().(*aicommon.Config)
		seen <- contextView{childCfg.GetTimeline().Dump(), childCfg.SessionPromptState.GetSessionEvidence(),
			childCfg.SessionPromptState.GetVerificationTodo(), childCfg.SessionPromptState.GetPrevSessionUserInput()}
		child := NewMinimalReActLoop(childCfg, p.Invoker)
		WithInitTask(func(_ *ReActLoop, _ aicommon.AIStatefulTask, op *InitTaskOperator) { op.Done() })(child)
		return child, nil
	})
	_, err = loop.SubmitSubAgents(task, []SubAgentJob{{Identifier: "clean"}}, SubAgentOptions{TimelineMode: SubAgentTimelineClean, LoopBuilder: builder}, "clean")
	require.NoError(t, err)
	awaitBackgroundTerminal(t, loop.GetSubAgentManager(), nil)
	view := <-seen
	require.NotContains(t, view.history, "parent-timeline-only")
	require.Equal(t, "shared-at-dispatch-evidence", view.evidence)
	require.Empty(t, view.todo)
	require.Equal(t, "original-user-question", view.previousInput)
}

func TestBackgroundSubAgents_ObservationBudgetResetsOnWork(t *testing.T) {
	loop, task := backgroundSubAgentFixture(t, 1)
	for i := 0; i < 50; i++ {
		require.NoError(t, loop.recordSubAgentControl(SubAgentWaitAction, newLoopActionHandlerOperator(task), nil))
		work := newLoopActionHandlerOperator(task)
		work.MarkToolExecuted()
		require.NoError(t, loop.recordSubAgentControl("require_tool", work, nil))
	}
	for i := 0; i < maxSubAgentControlRounds; i++ {
		require.NoError(t, loop.recordSubAgentControl(SubAgentInspectAction, newLoopActionHandlerOperator(task), nil))
	}
	require.NoError(t, loop.recordSubAgentControl(SubAgentCancelAction, newLoopActionHandlerOperator(task), nil))
	require.Error(t, loop.recordSubAgentControl(SubAgentInspectAction, newLoopActionHandlerOperator(task), nil))
}

func TestBackgroundSubAgents_UserExitCancelsOwnedWork(t *testing.T) {
	loop, task := backgroundSubAgentFixture(t, 1)
	started := make(chan string, 1)
	release := make(chan struct{})
	_, err := loop.SubmitSubAgents(task, []SubAgentJob{{Identifier: "worker"}},
		SubAgentOptions{TimelineMode: SubAgentTimelineClean, LoopBuilder: blockingSubAgentBuilder(started, release, "unused")}, "start")
	require.NoError(t, err)
	<-started
	op := newLoopActionHandlerOperator(task)
	op.ExitForUser()
	require.Empty(t, loop.admitSubAgentExit(op))
	items, err := loop.GetSubAgentManager().Inspect(nil)
	require.NoError(t, err)
	require.Equal(t, "cancelled", items[0].State)
}

func TestBackgroundSubAgents_DeliveryCommitDoesNotConsumeLaterResult(t *testing.T) {
	loop, _ := backgroundSubAgentFixture(t, 1)
	m := newSubAgentManager(context.Background(), 1)
	loop.subAgentManager = m
	for _, id := range []string{"first", "late"} {
		entry := &managedSubAgent{snapshot: SubAgentSnapshot{ID: id, State: "running"}, cancel: func() {}}
		m.jobs[id] = entry
		m.order = append(m.order, id)
	}
	m.settle(m.jobs["first"], &SubAgentResult{Record: TimelineRecord{Status: "completed", Result: "first-result"}}, nil)
	input, revision := loop.prepareSubAgentPrompt()
	require.Contains(t, input, "first-result")
	m.settle(m.jobs["late"], &SubAgentResult{Record: TimelineRecord{Status: "completed", Result: "later-result"}}, nil)
	loop.commitSubAgentPrompt(revision)
	loop.subAgentModelSeen = revision
	next, nextRevision := loop.prepareSubAgentPrompt()
	require.Contains(t, next, "later-result")
	require.NotContains(t, next, "first-result")
	require.Greater(t, nextRevision, revision)
	require.NotEmpty(t, loop.SubAgentFinishBlockReason())
}

func TestBackgroundSubAgents_InternalSynchronousSearchDoesNotDeadlock(t *testing.T) {
	for _, concurrency := range []int{1, 2} {
		t.Run(fmt.Sprint(concurrency), func(t *testing.T) {
			loop, task := backgroundSubAgentFixture(t, concurrency)
			search := managedLoopBuilder(func(p *PreparedSubAgent) (*ReActLoop, error) {
				child := NewMinimalReActLoop(p.Invoker.GetConfig(), p.Invoker)
				WithInitTask(func(_ *ReActLoop, task aicommon.AIStatefulTask, op *InitTaskOperator) {
					task.SetResult("search-result")
					op.Done()
				})(child)
				return child, nil
			})
			category := managedLoopBuilder(func(p *PreparedSubAgent) (*ReActLoop, error) {
				child := NewMinimalReActLoop(p.Invoker.GetConfig(), p.Invoker)
				WithInitTask(func(_ *ReActLoop, task aicommon.AIStatefulTask, op *InitTaskOperator) {
					results := DispatchSubAgents(p.Invoker, task, []SubAgentJob{{Identifier: "search"}},
						SubAgentOptions{TimelineMode: SubAgentTimelineClean, LoopBuilder: search, ExecuteConcurrency: 1})
					if len(results) == 1 {
						task.SetResult(results[0].Record.Result)
					}
					op.Done()
				})(child)
				return child, nil
			})
			_, err := loop.SubmitSubAgents(task, []SubAgentJob{{Identifier: "category-a"}, {Identifier: "category-b"}},
				SubAgentOptions{TimelineMode: SubAgentTimelineClean, LoopBuilder: category}, "categories")
			require.NoError(t, err)
			for _, item := range awaitBackgroundTerminal(t, loop.GetSubAgentManager(), nil) {
				require.Equal(t, "completed", item.State)
				raw, err := os.ReadFile(item.ResultReference)
				require.NoError(t, err)
				require.Equal(t, "search-result", string(raw))
			}
		})
	}
}

func TestBackgroundSubAgents_ExecutionTimeoutIsIndependentOfWait(t *testing.T) {
	loop, task := backgroundSubAgentFixture(t, 1)
	started := make(chan string, 1)
	release := make(chan struct{})
	_, err := loop.SubmitSubAgents(task, []SubAgentJob{{Identifier: "timeout", Timeout: 20 * time.Millisecond}},
		SubAgentOptions{TimelineMode: SubAgentTimelineClean, LoopBuilder: blockingSubAgentBuilder(started, release, "unused")}, "timeout")
	require.NoError(t, err)
	<-started
	m := loop.GetSubAgentManager()
	first, err := m.Wait(task.GetContext(), nil, time.Millisecond, 0)
	require.NoError(t, err)
	require.True(t, first.TimedOut)
	items := awaitBackgroundTerminal(t, m, nil)
	require.Equal(t, "timed_out", items[0].State)
}

func TestBackgroundSubAgents_QueuedChildDoesNotRestoreOldToolPolicy(t *testing.T) {
	loop, task := backgroundSubAgentFixture(t, 1)
	cfg := loop.config.(*aicommon.Config)
	manager := cfg.AiToolManager
	require.NoError(t, aicommon.WithDisallowMCPServers(false)(cfg))
	started, release := make(chan string, 1), make(chan struct{})
	_, err := loop.SubmitSubAgents(task, []SubAgentJob{{Identifier: "blocker"}},
		SubAgentOptions{TimelineMode: SubAgentTimelineClean, LoopBuilder: blockingSubAgentBuilder(started, release, "done")}, "blocker")
	require.NoError(t, err)
	<-started
	observed := make(chan bool, 1)
	builder := managedLoopBuilder(func(p *PreparedSubAgent) (*ReActLoop, error) {
		childCfg := p.Invoker.GetConfig().(*aicommon.Config)
		observed <- childCfg.AiToolManager == manager && childCfg.DisallowMCPServers && manager.DisallowMCPServers()
		child := NewMinimalReActLoop(p.Invoker.GetConfig(), p.Invoker)
		WithInitTask(func(_ *ReActLoop, task aicommon.AIStatefulTask, op *InitTaskOperator) { op.Done() })(child)
		return child, nil
	})
	_, err = loop.SubmitSubAgents(task, []SubAgentJob{{Identifier: "queued"}},
		SubAgentOptions{TimelineMode: SubAgentTimelineClean, LoopBuilder: builder}, "queued")
	require.NoError(t, err)
	manager.SetDisallowMCPServers(true)
	close(release)
	awaitBackgroundTerminal(t, loop.GetSubAgentManager(), nil)
	require.True(t, <-observed, "queued child must retain the live shared authority")
	require.True(t, manager.DisallowMCPServers())
}

func TestBackgroundSubAgents_CleanupPanicStillSettlesAndReleasesSlot(t *testing.T) {
	loop, task := backgroundSubAgentFixture(t, 1)
	started := make(chan string, 1)
	builder := managedLoopBuilder(func(p *PreparedSubAgent) (*ReActLoop, error) {
		child := NewMinimalReActLoop(p.Invoker.GetConfig(), p.Invoker)
		WithInitTask(func(_ *ReActLoop, task aicommon.AIStatefulTask, op *InitTaskOperator) {
			task.SetAsyncDeferCallback(func(error) { panic("cleanup callback failed") })
			started <- task.GetId()
			<-task.GetContext().Done()
			op.Done()
		})(child)
		return child, nil
	})
	_, err := loop.SubmitSubAgents(task, []SubAgentJob{{Identifier: "panic"}},
		SubAgentOptions{TimelineMode: SubAgentTimelineClean, LoopBuilder: builder}, "panic")
	require.NoError(t, err)
	<-started
	m := loop.GetSubAgentManager()
	_, err = m.Cancel(nil)
	require.NoError(t, err)
	items := awaitBackgroundTerminal(t, m, nil)
	require.Equal(t, "cancelled", items[0].State)
	require.Contains(t, items[0].Error, "cleanup callback failed")
	release := make(chan struct{})
	close(release)
	receipt, err := loop.SubmitSubAgents(task, []SubAgentJob{{Identifier: "next"}},
		SubAgentOptions{TimelineMode: SubAgentTimelineClean, LoopBuilder: blockingSubAgentBuilder(started, release, "next-result")}, "next")
	require.NoError(t, err)
	items = awaitBackgroundTerminal(t, m, []string{receipt.Jobs[0].ID})
	require.Equal(t, "completed", items[0].State)
}
