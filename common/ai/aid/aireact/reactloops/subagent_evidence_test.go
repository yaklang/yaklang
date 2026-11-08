package reactloops

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

type evidenceChild struct {
	commands chan aicommon.EvidenceOperation
	saved    chan error
	ready    chan *ReActLoop
	release  chan struct{}
}

type evidenceCompressionFailureInvoker struct {
	aicommon.AIInvokeRuntime
}

func (i *evidenceCompressionFailureInvoker) AssembleLoopPrompt(tools []*aitool.Tool, input *aicommon.LoopPromptAssemblyInput) (*aicommon.LoopPromptAssemblyResult, error) {
	if input.PromptMode == aicommon.LoopPromptModeCompressContext {
		return nil, fmt.Errorf("intentional compression assembly failure")
	}
	return i.AIInvokeRuntime.AssembleLoopPrompt(tools, input)
}

func newEvidenceChild(t *testing.T) *evidenceChild {
	t.Helper()
	c := &evidenceChild{make(chan aicommon.EvidenceOperation), make(chan error, 1), make(chan *ReActLoop, 1), make(chan struct{})}
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(c.release) }) })
	return c
}

func saveEvidenceAction(loop *ReActLoop, task aicommon.AIStatefulTask, evidence aicommon.EvidenceOperation) error {
	raw, err := json.Marshal(map[string]any{"@action": "save_evidence", "evidence_id": evidence.ID, "evidence_content": evidence.Content})
	if err != nil {
		return err
	}
	action, err := aicommon.ExtractAction(string(raw), schema.AI_REACT_LOOP_ACTION_SAVE_EVIDENCE)
	if err != nil {
		return err
	}
	if err := loopAction_SaveEvidence.ActionVerifier(loop, action); err != nil {
		return err
	}
	op := NewActionHandlerOperator(task)
	loopAction_SaveEvidence.ActionHandler(loop, action, op)
	if _, err := op.IsTerminated(); err != nil {
		return err
	}
	if !op.IsContinued() {
		return fmt.Errorf("save_evidence did not continue: %s", op.GetFeedback().String())
	}
	return nil
}

func (c *evidenceChild) builder(p *PreparedSubAgent) (*ReActLoop, error) {
	loop := NewMinimalReActLoop(p.Invoker.GetConfig(), p.Invoker)
	WithInitTask(func(loop *ReActLoop, task aicommon.AIStatefulTask, op *InitTaskOperator) {
		cfg := loop.GetConfig().(*aicommon.Config)
		cfg.GetTimeline().PushText(cfg.AcquireId(), "CHILD_PRIVATE_TRACE")
		cfg.ApplySessionEvidenceOps([]aicommon.EvidenceOperation{{Op: "add", ID: "private", Content: "CHILD_PRIVATE_EVIDENCE"}})
		c.ready <- loop
		for {
			select {
			case evidence := <-c.commands:
				c.saved <- saveEvidenceAction(loop, task, evidence)
			case <-c.release:
				task.SetResult("child completed")
				op.Done()
				return
			case <-task.GetContext().Done():
				op.Done()
				return
			}
		}
	})(loop)
	return loop, nil
}

func (c *evidenceChild) save(t *testing.T, id, content string) {
	t.Helper()
	select {
	case c.commands <- aicommon.EvidenceOperation{ID: id, Content: content}:
	case <-time.After(3 * time.Second):
		t.Fatal("child did not accept save_evidence")
	}
	select {
	case err := <-c.saved:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("child did not finish save_evidence")
	}
}

func (c *evidenceChild) await(t *testing.T) *ReActLoop {
	t.Helper()
	select {
	case loop := <-c.ready:
		return loop
	case <-time.After(3 * time.Second):
		t.Fatal("child did not start")
		return nil
	}
}

func TestBackgroundSubAgents_ExplicitEvidenceSharesOnlySelectedFindings(t *testing.T) {
	for _, mode := range []string{SubAgentContextFork, SubAgentContextTaskOnly} {
		t.Run(mode, func(t *testing.T) {
			parent, task := backgroundSubAgentFixture(t, 1)
			cfg := parent.GetConfig().(*aicommon.Config)
			cfg.GetTimeline().SetTimelineBucketByteSize(-1)
			_, err := SaveSessionEvidence(cfg, "parent", "PARENT_AT_DISPATCH")
			require.NoError(t, err)
			child := newEvidenceChild(t)
			receipt, err := parent.SubmitSubAgents(task, []SubAgentJob{{Identifier: "investigator", Goal: "investigate", ContextMode: mode}}, SubAgentOptions{LoopBuilder: managedLoopBuilder(child.builder)}, "selected")
			require.NoError(t, err)
			childLoop := child.await(t)
			require.True(t, childLoop.IsSubAgent(), "custom child builders use the common sub-agent identity and evidence channel")
			_, err = SaveSessionEvidence(cfg, "parent", "NEW_PARENT_FACT")
			require.NoError(t, err)
			_, err = SaveSessionEvidence(cfg, "unrelated", "UNRELATED_PARENT_FACT")
			require.NoError(t, err)

			// No final result exists: explicit evidence is independently useful.
			type observation struct {
				value *SubAgentObservation
				err   error
			}
			waited := make(chan observation, 1)
			manager := parent.GetSubAgentManager()
			go func() {
				value, err := manager.Wait(task.GetContext(), nil, time.Second, 0, 0)
				waited <- observation{value, err}
			}()
			child.save(t, "shared", "SHARED_DISCOVERY_V1")
			observed := <-waited
			require.NoError(t, observed.err)
			require.Equal(t, "evidence_available", observed.value.Reason)
			require.False(t, observed.value.TimedOut)
			require.False(t, observed.value.Jobs[0].terminal())
			require.Equal(t, "shared", observed.value.Jobs[0].LastEvidenceID)
			require.NotEmpty(t, parent.SubAgentFinishBlockReason(), "sharing evidence does not complete a child")

			view := cfg.GetSessionEvidenceRendered()
			require.Contains(t, view, "SHARED_DISCOVERY_V1")
			require.Contains(t, view, "NEW_PARENT_FACT")
			require.Contains(t, view, "UNRELATED_PARENT_FACT")
			require.NotContains(t, view, "CHILD_PRIVATE_EVIDENCE")
			require.NotContains(t, cfg.GetTimeline().Dump(), "CHILD_PRIVATE_TRACE")
			require.NotContains(t, childLoop.GetConfig().GetSessionEvidenceRendered(), "NEW_PARENT_FACT", "publication is not a shared mutable child snapshot")

			prompt, err := parent.generateLoopPrompt("evidence-nonce", task.GetUserInput(), "", nil, "", NewActionHandlerOperator(task))
			require.NoError(t, err)
			require.Equal(t, 1, strings.Count(prompt, "SHARED_DISCOVERY_V1"))
			require.NotContains(t, prompt, "CHILD_PRIVATE_TRACE")
			require.NotContains(t, prompt, "CHILD_PRIVATE_EVIDENCE")
			require.Equal(t, observed.value.Jobs[0].EvidenceRevision, parent.SubAgentModelSeenEvidenceRevision())

			beforeID := cfg.GetTimeline().GetMaxID()
			child.save(t, "shared", "SHARED_DISCOVERY_V1")
			require.Equal(t, beforeID, cfg.GetTimeline().GetMaxID(), "duplicate publication must not grow the parent journal")
			observationAfterRead, err := manager.Wait(task.GetContext(), nil, time.Millisecond, 0, parent.SubAgentModelSeenEvidenceRevision())
			require.NoError(t, err)
			require.True(t, observationAfterRead.TimedOut, "already-read evidence and retries must not wake the parent repeatedly")

			// Evidence follows normal Timeline freeze/promotion, rather than a
			// second copy of the body in child-result notifications.
			cfg.GetTimeline().FreezeAll()
			sealed := aicommon.BuildPromptFrozenOpenMaterials(cfg)
			require.Contains(t, sealed.SessionEvidenceSemiDynamic, "SHARED_DISCOVERY_V1")
			require.NotContains(t, sealed.TimelineOpen, "SHARED_DISCOVERY_V1")
			child.save(t, "shared", "SHARED_DISCOVERY_V2")
			updated := aicommon.BuildPromptFrozenOpenMaterials(cfg)
			require.Equal(t, sealed.SessionEvidenceSemiDynamic, updated.SessionEvidenceSemiDynamic)
			require.Contains(t, updated.TimelineOpen, "SHARED_DISCOVERY_V2")
			cfg.GetTimeline().FreezeAll()
			promoted := aicommon.BuildPromptFrozenOpenMaterials(cfg)
			require.Contains(t, promoted.SessionEvidenceSemiDynamic, "SHARED_DISCOVERY_V2")
			require.NotContains(t, promoted.SessionEvidenceSemiDynamic, "SHARED_DISCOVERY_V1")
			jobs, err := manager.Inspect([]string{receipt.Jobs[0].ID})
			require.NoError(t, err)
			require.Equal(t, uint64(2), jobs[0].EvidenceRevision)
		})
	}
}

func TestBackgroundSubAgents_EvidenceDuringPromptAssemblyIsNotLost(t *testing.T) {
	parent, task := backgroundSubAgentFixture(t, 1)
	child := newEvidenceChild(t)
	_, err := parent.SubmitSubAgents(task, []SubAgentJob{{Identifier: "late", Goal: "investigate"}}, SubAgentOptions{LoopBuilder: managedLoopBuilder(child.builder)}, "late")
	require.NoError(t, err)
	child.await(t)
	input := parent.captureLoopPromptBuildInput("nonce", task.GetUserInput(), "", nil, "", NewActionHandlerOperator(task))
	child.save(t, "late-fact", "EVIDENCE_AFTER_INPUT_CAPTURE")
	_, err = parent.generateLoopPromptFromInput(input)
	require.NoError(t, err)
	manager := parent.GetSubAgentManager()
	observation, err := manager.Wait(task.GetContext(), nil, time.Millisecond, 0, parent.SubAgentModelSeenEvidenceRevision())
	require.NoError(t, err)
	require.Equal(t, "evidence_available", observation.Reason)
	_, err = parent.generateLoopPrompt("next", task.GetUserInput(), "", nil, "", NewActionHandlerOperator(task))
	require.NoError(t, err)
	observation, err = manager.Wait(task.GetContext(), nil, time.Millisecond, 0, parent.SubAgentModelSeenEvidenceRevision())
	require.NoError(t, err)
	require.True(t, observation.TimedOut)
}

func TestBackgroundSubAgents_EvidenceWaitRespectsSelectedJobs(t *testing.T) {
	parent, task := backgroundSubAgentFixture(t, 2)
	children := map[string]*evidenceChild{"a": newEvidenceChild(t), "b": newEvidenceChild(t)}
	builder := managedLoopBuilder(func(p *PreparedSubAgent) (*ReActLoop, error) { return children[p.Job.Identifier].builder(p) })
	receipt, err := parent.SubmitSubAgents(task, []SubAgentJob{{Identifier: "a", Goal: "A"}, {Identifier: "b", Goal: "B"}}, SubAgentOptions{LoopBuilder: builder}, "selection")
	require.NoError(t, err)
	children["a"].await(t)
	children["b"].await(t)
	children["a"].save(t, "fact-a", "FINDING_FROM_A")
	manager := parent.GetSubAgentManager()
	observed, err := manager.Wait(task.GetContext(), []string{receipt.Jobs[1].ID}, time.Millisecond, 0, 0)
	require.NoError(t, err)
	require.True(t, observed.TimedOut)
	observed, err = manager.Wait(task.GetContext(), []string{receipt.Jobs[0].ID}, time.Second, 0, 0)
	require.NoError(t, err)
	require.Equal(t, "evidence_available", observed.Reason)
}

func TestBackgroundSubAgents_ClosedOwnerRejectsLateEvidence(t *testing.T) {
	parent, task := backgroundSubAgentFixture(t, 1)
	child := newEvidenceChild(t)
	_, err := parent.SubmitSubAgents(task, []SubAgentJob{{Identifier: "cancelled", Goal: "investigate"}}, SubAgentOptions{LoopBuilder: managedLoopBuilder(child.builder)}, "close")
	require.NoError(t, err)
	childLoop := child.await(t)
	publisher := childLoop.saveEvidencePublisher
	require.NotNil(t, publisher)
	parent.shutdownSubAgents()
	err = publisher(aicommon.EvidenceOperation{ID: "late", Content: "MUST_NOT_REACH_CLOSED_PARENT"})
	require.Error(t, err)
	require.NotContains(t, parent.GetConfig().GetSessionEvidenceRendered(), "MUST_NOT_REACH_CLOSED_PARENT")
}

func TestDispatchSubAgents_ExplicitEvidenceUsesCommonParentJournal(t *testing.T) {
	parent, task := backgroundSubAgentFixture(t, 1)
	child := newEvidenceChild(t)
	results := make(chan []*SubAgentResult, 1)
	go func() {
		results <- DispatchSubAgents(parent.GetInvoker(), task, []SubAgentJob{{Identifier: "sync", Goal: "investigate"}}, SubAgentOptions{ParentLoop: parent, LoopBuilder: managedLoopBuilder(child.builder)})
	}()
	child.await(t)
	child.save(t, "", "SYNC_EXPLICIT_FINDING")
	view := parent.GetConfig().GetSessionEvidenceRendered()
	require.Contains(t, view, "SYNC_EXPLICIT_FINDING")
	require.NotContains(t, view, "CHILD_PRIVATE_EVIDENCE")
	canonical, err := aicommon.BuildSessionEvidenceUpsert("", "SYNC_EXPLICIT_FINDING")
	require.NoError(t, err)
	require.Contains(t, view, "[id: "+canonical.ID+"]")
	// Cancellation releases the deliberately blocked synchronous child.
	task.Cancel()
	select {
	case got := <-results:
		require.Len(t, got, 1)
	case <-time.After(3 * time.Second):
		t.Fatal("synchronous child did not stop")
	}
}

func TestBackgroundSubAgents_EvidenceSurvivesNormalPromptCompressionFallback(t *testing.T) {
	parent, task := backgroundSubAgentFixture(t, 1)
	child := newEvidenceChild(t)
	_, err := parent.SubmitSubAgents(task, []SubAgentJob{{Identifier: "fallback", Goal: "investigate"}}, SubAgentOptions{LoopBuilder: managedLoopBuilder(child.builder)}, "fallback")
	require.NoError(t, err)
	child.await(t)
	child.save(t, "fallback-fact", "FINDING_IN_NORMAL_FALLBACK")
	parent.invoker = &evidenceCompressionFailureInvoker{parent.GetInvoker()}
	cfg := parent.GetConfig().(*aicommon.Config)
	require.NoError(t, aicommon.WithTimelineContentLimit(1)(cfg))
	input := parent.captureLoopPromptBuildInput("fallback", task.GetUserInput(), "", nil, "", NewActionHandlerOperator(task))
	result, err := parent.generateLoopPromptFromInput(input)
	require.NoError(t, err)
	require.NotEqual(t, aicommon.LoopPromptModeCompressContext, result.Mode, "failed compression must return the successful normal prompt")
	require.Contains(t, result.Prompt, "FINDING_IN_NORMAL_FALLBACK")
	observed, err := parent.GetSubAgentManager().Wait(task.GetContext(), nil, time.Millisecond, 0, parent.SubAgentModelSeenEvidenceRevision())
	require.NoError(t, err)
	require.True(t, observed.TimedOut, "fallback must acknowledge the evidence actually supplied to the model")
}

func TestBackgroundSubAgents_ConcurrentEvidenceKeepsEveryFinding(t *testing.T) {
	parent, task := backgroundSubAgentFixture(t, 3)
	children := map[string]*evidenceChild{"a": newEvidenceChild(t), "b": newEvidenceChild(t), "c": newEvidenceChild(t)}
	builder := managedLoopBuilder(func(p *PreparedSubAgent) (*ReActLoop, error) { return children[p.Job.Identifier].builder(p) })
	_, err := parent.SubmitSubAgents(task, []SubAgentJob{{Identifier: "a", Goal: "A"}, {Identifier: "b", Goal: "B"}, {Identifier: "c", Goal: "C"}}, SubAgentOptions{LoopBuilder: builder}, "concurrent")
	require.NoError(t, err)
	for _, child := range children {
		child.await(t)
	}
	var writers sync.WaitGroup
	for id, child := range children {
		writers.Add(1)
		go func(id string, child *evidenceChild) {
			defer writers.Done()
			select {
			case child.commands <- aicommon.EvidenceOperation{ID: "fact-" + id, Content: "CONCURRENT_FINDING_" + id}:
			case <-task.GetContext().Done():
			}
		}(id, child)
	}
	writers.Wait()
	for _, child := range children {
		select {
		case err := <-child.saved:
			require.NoError(t, err)
		case <-time.After(3 * time.Second):
			t.Fatal("concurrent save did not complete")
		}
	}
	view := parent.GetConfig().GetSessionEvidenceRendered()
	for id := range children {
		require.Contains(t, view, "CONCURRENT_FINDING_"+id)
	}
	require.NotContains(t, view, "CHILD_PRIVATE_EVIDENCE")
}

func TestBackgroundSubAgents_CompressionDoesNotAcknowledgeBusinessEvidence(t *testing.T) {
	parent, task := backgroundSubAgentFixture(t, 1)
	child := newEvidenceChild(t)
	_, err := parent.SubmitSubAgents(task, []SubAgentJob{{Identifier: "compression", Goal: "investigate"}}, SubAgentOptions{LoopBuilder: managedLoopBuilder(child.builder)}, "compression")
	require.NoError(t, err)
	child.await(t)
	child.save(t, "compression-fact", "BUSINESS_FINDING_AFTER_COMPRESSION")
	cfg := parent.GetConfig().(*aicommon.Config)
	require.NoError(t, aicommon.WithTimelineContentLimit(1)(cfg))
	input := parent.captureLoopPromptBuildInput("compress", task.GetUserInput(), "", nil, "", NewActionHandlerOperator(task))
	result, err := parent.generateLoopPromptFromInput(input)
	require.NoError(t, err)
	require.Equal(t, aicommon.LoopPromptModeCompressContext, result.Mode)
	observed, err := parent.GetSubAgentManager().Wait(task.GetContext(), nil, time.Millisecond, 0, parent.SubAgentModelSeenEvidenceRevision())
	require.NoError(t, err)
	require.Equal(t, "evidence_available", observed.Reason)
	require.NoError(t, aicommon.WithTimelineContentLimit(300000)(cfg))
	_, err = parent.generateLoopPrompt("business", task.GetUserInput(), "", nil, "", NewActionHandlerOperator(task))
	require.NoError(t, err)
	observed, err = parent.GetSubAgentManager().Wait(task.GetContext(), nil, time.Millisecond, 0, parent.SubAgentModelSeenEvidenceRevision())
	require.NoError(t, err)
	require.True(t, observed.TimedOut)
}

func TestBackgroundSubAgents_InternalHelperCanForwardExplicitEvidence(t *testing.T) {
	parent, task := backgroundSubAgentFixture(t, 1)
	helper := newEvidenceChild(t)
	outer := managedLoopBuilder(func(p *PreparedSubAgent) (*ReActLoop, error) {
		loop := NewMinimalReActLoop(p.Invoker.GetConfig(), p.Invoker)
		WithInitTask(func(loop *ReActLoop, task aicommon.AIStatefulTask, op *InitTaskOperator) {
			DispatchSubAgents(p.Invoker, task, []SubAgentJob{{Identifier: "helper", Goal: "find evidence"}}, SubAgentOptions{ParentLoop: loop, LoopBuilder: managedLoopBuilder(helper.builder)})
			op.Done()
		})(loop)
		return loop, nil
	})
	_, err := parent.SubmitSubAgents(task, []SubAgentJob{{Identifier: "outer", Goal: "investigate"}}, SubAgentOptions{LoopBuilder: outer}, "nested")
	require.NoError(t, err)
	helper.await(t)
	helper.save(t, "nested-fact", "FINDING_FROM_INTERNAL_HELPER")
	require.Contains(t, parent.GetConfig().GetSessionEvidenceRendered(), "FINDING_FROM_INTERNAL_HELPER")
	require.NotContains(t, parent.GetConfig().GetSessionEvidenceRendered(), "CHILD_PRIVATE_EVIDENCE")
	observed, err := parent.GetSubAgentManager().Wait(task.GetContext(), nil, time.Millisecond, 0, 0)
	require.NoError(t, err)
	require.Equal(t, "evidence_available", observed.Reason)
}
