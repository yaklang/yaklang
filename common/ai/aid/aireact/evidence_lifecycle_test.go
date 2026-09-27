package aireact

import (
	"context"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"strings"
	"testing"
)

// Real action -> Config -> Timeline -> complete loop prompt. No model call is
// needed: save_evidence writes directly, and prompt construction is read-only.
func TestEvidenceActionToOpenFreezeSemiLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	react, err := NewTestReAct(aicommon.WithContext(ctx), aicommon.WithAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		t.Error("saving evidence must not call a model")
		return nil, nil
	}))
	require.NoError(t, err)
	cfg := react.config
	cfg.GetTimeline().SetTimelineBucketByteSize(-1)
	loop, err := reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_DEFAULT, react)
	require.NoError(t, err)
	task := aicommon.NewStatefulTaskBase("evidence-lifecycle", "verify evidence", ctx, cfg.GetEmitter())
	loop.SetCurrentTask(task)
	handler, err := loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_SAVE_EVIDENCE)
	require.NoError(t, err)
	save := func(body string) {
		action := aicommon.NewSimpleAction(schema.AI_REACT_LOOP_ACTION_SAVE_EVIDENCE, aitool.InvokeParams{"evidence_id": "fact", "evidence_content": body})
		require.NoError(t, handler.ActionVerifier(loop, action))
		op := reactloops.NewActionHandlerOperator(task)
		handler.ActionHandler(loop, action, op)
		require.True(t, op.IsContinued())
	}
	render := func() string {
		result, err := react.promptManager.AssembleLoopPrompt(nil, &reactloops.LoopPromptAssemblyInput{Nonce: "evidence-test", UserQuery: "continue"})
		require.NoError(t, err)
		return result.Prompt
	}
	react.AddToTimeline("note", "BEFORE_SAVE")
	save("UNIQUE_FACT_V1")
	prompt := render()
	require.Equal(t, 1, strings.Count(prompt, "UNIQUE_FACT_V1"), "receipt must not duplicate the delta body")
	require.Contains(t, prompt, "[UPSERT]")
	require.NotContains(t, prompt, "<|SESSION_EVIDENCE_")
	save("UNIQUE_FACT_V1")
	require.Equal(t, 1, strings.Count(render(), "UNIQUE_FACT_V1"), "retry cannot create a duplicate delta")
	react.AddToTimeline("note", "BETWEEN_SAVES")
	save("UNIQUE_FACT_V2")
	prompt = render()
	require.Less(t, strings.Index(prompt, "UNIQUE_FACT_V1"), strings.Index(prompt, "BETWEEN_SAVES"))
	require.Less(t, strings.Index(prompt, "BETWEEN_SAVES"), strings.Index(prompt, "UNIQUE_FACT_V2"))
	cfg.GetTimeline().FreezeAll()
	prompt = render()
	require.NotContains(t, prompt, "UNIQUE_FACT_V1")
	require.Equal(t, 1, strings.Count(prompt, "UNIQUE_FACT_V2"))
	require.Contains(t, aicommon.BuildPromptFrozenOpenMaterials(cfg).SessionEvidenceSemiDynamic, "UNIQUE_FACT_V2")
	cfg.ApplySessionEvidenceOps([]aicommon.EvidenceOperation{{Op: "delete", ID: "fact"}})
	pending := aicommon.BuildPromptFrozenOpenMaterials(cfg)
	require.Contains(t, pending.TimelineOpen, "[TOMBSTONE]")
	require.Contains(t, pending.SessionEvidenceSemiDynamic, "UNIQUE_FACT_V2")
	cfg.GetTimeline().FreezeAll()
	require.NotContains(t, render(), "UNIQUE_FACT_V2")
	require.Empty(t, cfg.GetSessionEvidenceRendered())
}
