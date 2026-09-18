package reactloops

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

func TestBackgroundSubAgents_ContextModeMixedBatch(t *testing.T) {
	loop, task := backgroundSubAgentFixture(t, 1)
	cfg := loop.GetConfig().(*aicommon.Config)
	require.NoError(t, aicommon.WithPersistentSessionId("parent-context-policy-test")(cfg))
	cfg.GetTimeline().PushText(cfg.AcquireId(), "PARENT_TIMELINE_SENTINEL")
	cfg.SessionPromptState.SetSessionEvidence("PARENT_EVIDENCE_SENTINEL")
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
		seen <- observation{mode: p.Job.ContextMode, history: c.GetTimeline().Dump(), evidence: c.SessionPromptState.GetSessionEvidence(),
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
			require.Equal(t, "PARENT_EVIDENCE_SENTINEL", o.evidence)
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
	require.Equal(t, "PARENT_EVIDENCE_SENTINEL", cfg.SessionPromptState.GetSessionEvidence())
	require.Equal(t, "PARENT_PLAN_SENTINEL", cfg.PlanPrompt)
}

func TestBackgroundSubAgents_InvalidContextModeDoesNotStartJobs(t *testing.T) {
	loop, task := backgroundSubAgentFixture(t, 1)
	_, err := loop.SubmitSubAgents(task, []SubAgentJob{{ContextMode: "clean"}}, SubAgentOptions{}, "bad-mode")
	require.ErrorContains(t, err, "context_mode")
	require.Nil(t, loop.GetSubAgentManager())
}
