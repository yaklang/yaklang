package loop_risk_enrich

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
)

func riskInitHarness(t *testing.T) (*reactloops.ReActLoop, *mock.MockInvoker) {
	t.Helper()
	db := riskTestDB(t)
	invoker := mock.NewMockInvoker(context.Background())
	cfg := invoker.GetConfig().(*mock.MockedAIConfig)
	cfg.BaseCheckpointableStorage = aicommon.NewCheckpointableStorageWithDB("risk-init-test", db)
	return reactloops.NewMinimalReActLoop(cfg, invoker), invoker
}

func riskTask(loop *reactloops.ReActLoop, attached ...*aicommon.AttachedResource) aicommon.AIStatefulTask {
	task := aicommon.NewStatefulTaskBase("risk-init-task", "补全当前风险证据", context.Background(), loop.GetEmitter(), true)
	task.SetAttachedDatas(attached)
	loop.SetCurrentTask(task)
	return task
}

func TestRiskInitLocksAttachedObjectAndRuntimeEnvironment(t *testing.T) {
	loop, invoker := riskInitHarness(t)
	db := projectDB(invoker)
	r := &schema.Risk{Hash: "init-risk", Host: "example.test", RuntimeId: "tool-one", AISessionID: "session-a"}
	require.NoError(t, db.Create(r).Error)
	require.NoError(t, db.Create(&schema.AISession{SessionID: "session-a", RelatedRuntimeIDS: `["tool-one","tool-two"]`}).Error)
	require.NoError(t, db.Create(&schema.HTTPFlow{Url: "https://example.test/a", RuntimeId: "tool-two"}).Error)
	require.NoError(t, db.Create(&schema.HTTPFlow{Url: "https://example.test/a", RuntimeId: "outside"}).Error)
	attached := aicommon.NewAttachedResource(aicommon.AttachedResourceTypeRiskID, aicommon.AttachedResourceKeyID, strconv.FormatUint(uint64(r.ID), 10))
	require.NoError(t, bootstrapRiskEnvironment(loop, invoker, riskTask(loop, attached)))
	env, err := environment(loop)
	require.NoError(t, err)
	require.Equal(t, int64(r.ID), env.RiskID)
	require.Equal(t, []string{"tool-one", "tool-two"}, env.RuntimeIDs)
	require.Contains(t, loop.Get("risk_enrich_context"), "Risk #")
	require.Contains(t, loop.Get("risk_enrich_inventory"), "Total HTTP flows: 1")
	_, _, err = loadBoundRisk(loop, invoker)
	require.NoError(t, err)
	// Newly added session runtimes must not silently expand this task's scope.
	require.NoError(t, db.Model(&schema.AISession{}).Where("session_id = ?", "session-a").
		UpdateColumn("related_runtime_ids", `["tool-one","tool-two","tool-three"]`).Error)
	lateFlow := &schema.HTTPFlow{Url: "https://example.test/a?late=1", RuntimeId: "tool-three", Request: "GET /a?late=1"}
	require.NoError(t, db.Create(lateFlow).Error)
	hits, _, _, err := searchFlows(db, r, env, searchCriteria{URLContains: "/a"})
	require.NoError(t, err)
	require.Len(t, hits, 1)
	require.ErrorContains(t, attachFlow(db, env, int(lateFlow.ID)), "boundary")

	// A subsequent task without a risk attachment must not silently reuse it.
	err = bootstrapRiskEnvironment(loop, invoker, riskTask(loop))
	require.ErrorContains(t, err, "attached resource")
	_, err = environment(loop)
	require.Error(t, err)
}

func TestRiskInitRejectsMissingMalformedAndConflictingAttachments(t *testing.T) {
	loop, invoker := riskInitHarness(t)
	r := &schema.Risk{Hash: "r1", Host: "example.test", RuntimeId: "tool-one"}
	require.NoError(t, projectDB(invoker).Create(r).Error)
	for _, input := range [][]*aicommon.AttachedResource{
		nil,
		{aicommon.NewAttachedResource("risk_id", "id", "not-an-id")},
		{aicommon.NewAttachedResource("risk_id", "id", "99999999")},
		{aicommon.NewAttachedResource("risk_id", "id", strconv.FormatUint(uint64(r.ID), 10)), aicommon.NewAttachedResource("risk_id", "id", "2")},
	} {
		err := bootstrapRiskEnvironment(loop, invoker, riskTask(loop, input...))
		require.Error(t, err)
		_, err = environment(loop)
		require.Error(t, err)
	}
}
