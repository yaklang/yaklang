package aid

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func TestPlanExecutionEvidenceActionsShareSessionStore(t *testing.T) {
	ctx := context.Background()
	invoker := &evidencePlanInvoker{MockInvoker: mock.NewMockInvoker(ctx)}
	task := mock.NewMockStatefulTask(ctx, "pe-evidence-test", "execute planned task")
	invoker.SetCurrentTask(task)

	// Keep this test independent of loopinfra package initialization: local
	// placeholders satisfy the registry actions enabled by this mock runtime,
	// while outputEvidenceAction models the option added by PE task execution.
	loop, err := reactloops.NewReActLoop(
		"pe-evidence-test",
		invoker,
		reactloops.WithRegisterLoopAction(schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL, "test placeholder", nil, nil, nil),
		reactloops.WithRegisterLoopAction(schema.AI_REACT_LOOP_ACTION_QUERY_MCP_SERVERS, "test placeholder", nil, nil, nil),
		reactloops.WithRegisterLoopAction(schema.AI_REACT_LOOP_ACTION_QUERY_MCP_TOOLS, "test placeholder", nil, nil, nil),
		reactloops.WithAllowRAG(false),
		reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false),
		reactloops.WithAllowUserInteract(false),
		outputEvidenceAction(nil),
	)
	require.NoError(t, err)
	loop.SetCurrentTask(task)

	_, err = loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_SAVE_EVIDENCE)
	require.NoError(t, err, "PE loops must retain the core save_evidence action")
	compatHandler, err := loop.GetActionHandler("output_evidence")
	require.NoError(t, err)

	action := aicommon.NewSimpleAction("output_evidence", aitool.InvokeParams{
		planEvidenceFieldName: "PE task confirmed that the generated artifact passes its regression test.",
	})
	require.NoError(t, compatHandler.ActionVerifier(loop, action))
	op := reactloops.NewActionHandlerOperator(task)
	compatHandler.ActionHandler(loop, action, op)
	require.True(t, op.IsContinued())
	require.Len(t, invoker.receipts, 1)
	require.NotContains(t, invoker.receipts[0], "generated artifact passes")
	cfg := invoker.GetConfig().(*mock.MockedAIConfig).EvidenceState
	require.Contains(t, aicommon.BuildPromptFrozenOpenMaterials(cfg).TimelineOpen, "generated artifact passes")
	cfg.GetTimeline().FreezeAll()
	require.Contains(t, aicommon.BuildPromptFrozenOpenMaterials(cfg).SessionEvidenceSemiDynamic, "generated artifact passes")
	require.Contains(t, invoker.GetConfig().GetSessionEvidenceRendered(), "generated artifact passes its regression test")
}

type evidencePlanInvoker struct {
	*mock.MockInvoker
	receipts []string
}

func (i *evidencePlanInvoker) AddToTimeline(entry, content string) {
	i.receipts = append(i.receipts, entry+":"+content)
}
