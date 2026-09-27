package reactloops

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestEvidenceSubAgents_ContextModeMixedBatch(t *testing.T) {
	loop, task := backgroundSubAgentFixture(t, 1)
	cfg := loop.GetConfig().(*aicommon.Config)
	require.NoError(t, aicommon.WithPersistentSessionId("parent-context-policy-test")(cfg))
	cfg.GetTimeline().PushText(cfg.AcquireId(), "PARENT_TIMELINE_SENTINEL")
	cfg.ApplySessionEvidenceOps([]aicommon.EvidenceOperation{{Op: "add", ID: "context-fact", Content: "PARENT_EVIDENCE_SENTINEL"}})
	cfg.SessionPromptState.SetVerificationTodo("PARENT_TODO_SENTINEL")
	_, err := cfg.AppendUserInputHistory("PARENT_USER_SENTINEL", time.Now())
	require.NoError(t, err)
	require.NoError(t, aicommon.WithPlanPrompt("PARENT_PLAN_SENTINEL")(cfg))
	cfg.GetOrCreateFrozenBlockPartitionProducer().AppendNewPartition("plan_facts", "plan", "PARENT_FROZEN_SENTINEL", 1)
	cfg.ContextProviderManager.Register("host", aicommon.FileContentContextProvider("HOST_CONTEXT_SENTINEL"))
	end := cfg.ContextProviderManager.BeginTaskContext("attachment", aicommon.FileContentContextProvider("PARENT_ATTACHMENT_SENTINEL"))
	defer end()
	type observation struct {
		mode, history, evidence, user, todo, plan, session, frozen, providers, input string
		sameTools, memoryDisabled                                                    bool
	}
	seen := make(chan observation, 2)
	builder := managedLoopBuilder(func(p *PreparedSubAgent) (*ReActLoop, error) {
		c := p.Invoker.GetConfig().(*aicommon.Config)
		var frozen strings.Builder
		for _, partition := range c.GetOrCreateFrozenBlockPartitionProducer().ProducePartitions() {
			frozen.WriteString(partition.Content)
		}
		seen <- observation{mode: p.Job.ContextMode, history: c.GetTimeline().Dump(), evidence: c.GetSessionEvidenceRendered(),
			user: c.SessionPromptState.GetPrevSessionUserInput(), todo: c.SessionPromptState.GetVerificationTodo(), plan: c.PlanPrompt,
			session: c.PersistentSessionId, frozen: frozen.String(), providers: c.ContextProviderManager.Execute(c, nil), input: p.Task.GetUserInput(),
			sameTools: c.AiToolManager == cfg.AiToolManager, memoryDisabled: c.DisableMemoryTriage}
		c.AppendReportedRisk(&schema.Risk{Url: "https://example.com/" + p.Job.ContextMode, RiskType: "xss", Title: p.Job.ContextMode})
		child := NewMinimalReActLoop(c, p.Invoker)
		WithInitTask(func(_ *ReActLoop, _ aicommon.AIStatefulTask, op *InitTaskOperator) { op.Done() })(child)
		return child, nil
	})
	receipt, err := loop.SubmitSubAgents(task, []SubAgentJob{
		{Identifier: "inherited", ContextMode: SubAgentContextFork, Goal: "EXPLICIT_GOAL", ResultContract: "EXPLICIT_CONTRACT"},
		{Identifier: "independent", ContextMode: SubAgentContextTaskOnly, Goal: "EXPLICIT_GOAL", ResultContract: "EXPLICIT_CONTRACT"},
	}, SubAgentOptions{LoopBuilder: builder}, "mixed")
	require.NoError(t, err)
	require.Equal(t, SubAgentContextFork, receipt.Jobs[0].ContextMode)
	require.Equal(t, SubAgentContextTaskOnly, receipt.Jobs[1].ContextMode)
	awaitBackgroundTerminal(t, loop.GetSubAgentManager(), nil)
	for range 2 {
		o := <-seen
		require.True(t, o.sameTools)
		require.Contains(t, o.providers, "HOST_CONTEXT_SENTINEL")
		require.Contains(t, o.input, "EXPLICIT_GOAL")
		require.Contains(t, o.input, "EXPLICIT_CONTRACT")
		require.Empty(t, o.todo)
		if o.mode == SubAgentContextFork {
			require.Contains(t, o.history, "PARENT_TIMELINE_SENTINEL")
			require.Contains(t, o.evidence, "PARENT_EVIDENCE_SENTINEL")
			require.Equal(t, "PARENT_USER_SENTINEL", o.user)
			require.Contains(t, o.frozen, "PARENT_FROZEN_SENTINEL")
			require.Contains(t, o.providers, "PARENT_ATTACHMENT_SENTINEL")
		} else {
			require.NotContains(t, o.history, "PARENT_TIMELINE_SENTINEL")
			require.Empty(t, o.evidence)
			require.Empty(t, o.user)
			require.Empty(t, o.plan)
			require.Empty(t, o.session)
			require.Empty(t, o.frozen)
			require.NotContains(t, o.providers, "PARENT_ATTACHMENT_SENTINEL")
			require.True(t, o.memoryDisabled)
		}
	}
	require.Contains(t, cfg.GetReportedRisksRendered(), "task_only")
	require.Contains(t, cfg.GetTimeline().Dump(), "PARENT_TIMELINE_SENTINEL")
	require.Contains(t, cfg.GetSessionEvidenceRendered(), "PARENT_EVIDENCE_SENTINEL")
	require.Equal(t, "PARENT_PLAN_SENTINEL", cfg.PlanPrompt)
}

func TestEvidenceSubAgents_QueuedConfigSnapshotAndIdentity(t *testing.T) {
	loop, task := backgroundSubAgentFixture(t, 1)
	cfg := loop.GetConfig().(*aicommon.Config)
	cfg.ApplySessionEvidenceOps([]aicommon.EvidenceOperation{{Op: "add", ID: "context-fact", Content: "evidence-at-dispatch"}})
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
				evidence: childCfg.GetSessionEvidenceRendered(), taskID: task.GetId(), forkID: p.Timeline.Fork().TaskIndex,
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
	cfg.ApplySessionEvidenceOps([]aicommon.EvidenceOperation{{Op: "add", ID: "context-fact", Content: "new-parent-evidence-after-dispatch"}})
	cfg.GetTimeline().PushText(cfg.AcquireId(), "new-parent-history-after-dispatch")
	once.Do(func() { close(release) })
	secondSeen := <-seen
	require.Equal(t, second.Jobs[0].ID, secondSeen.taskID)
	require.Equal(t, secondSeen.taskID, secondSeen.forkID)
	require.Contains(t, secondSeen.evidence, "evidence-at-dispatch")
	require.Contains(t, secondSeen.history, "parent-history-at-dispatch")
	require.NotContains(t, secondSeen.history, "new-parent-history-after-dispatch")
	require.NotContains(t, secondSeen.history, "child-private-history")
	require.Empty(t, secondSeen.todo)
	awaitBackgroundTerminal(t, loop.GetSubAgentManager(), nil)
	require.NotContains(t, cfg.GetTimeline().Dump(), "child-private-history")
	require.Equal(t, "parent-only-todo", cfg.SessionPromptState.GetVerificationTodo())
}

func TestEvidenceSubAgents_CleanTimelineStillInheritsSessionContext(t *testing.T) {
	loop, task := backgroundSubAgentFixture(t, 1)
	cfg := loop.GetConfig().(*aicommon.Config)
	cfg.GetTimeline().PushText(cfg.AcquireId(), "parent-timeline-only")
	cfg.ApplySessionEvidenceOps([]aicommon.EvidenceOperation{{Op: "add", ID: "context-fact", Content: "shared-at-dispatch-evidence"}})
	cfg.SessionPromptState.SetVerificationTodo("parent-todo")
	_, err := cfg.AppendUserInputHistory("original-user-question", time.Now())
	require.NoError(t, err)
	type contextView struct{ history, evidence, todo, previousInput string }
	seen := make(chan contextView, 1)
	builder := managedLoopBuilder(func(p *PreparedSubAgent) (*ReActLoop, error) {
		childCfg := p.Invoker.GetConfig().(*aicommon.Config)
		seen <- contextView{childCfg.GetTimeline().Dump(), childCfg.GetSessionEvidenceRendered(),
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
	require.Contains(t, view.evidence, "shared-at-dispatch-evidence")
	require.Empty(t, view.todo)
	require.Equal(t, "original-user-question", view.previousInput)
}
