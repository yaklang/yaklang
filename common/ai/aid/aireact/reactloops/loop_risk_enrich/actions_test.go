package loop_risk_enrich

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	_ "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loopinfra"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func runRiskAction(t *testing.T, loop *reactloops.ReActLoop, name, args string) (string, error) {
	t.Helper()
	handler, err := loop.GetActionHandler(name)
	require.NoError(t, err)
	rawAction := fmt.Sprintf(`{"@action":%q}`, name)
	if args != "" {
		rawAction = fmt.Sprintf(`{"@action":%q,%s}`, name, args)
	}
	modelAction, err := aicommon.ExtractAction(rawAction, name)
	require.NoError(t, err)
	op := reactloops.NewActionHandlerOperator(loop.GetCurrentTask())
	handler.ActionHandler(loop, modelAction, op)
	_, err = op.IsTerminated()
	return op.GetFeedback().String(), err
}

func TestRiskActionsStayBoundAndRequireEvidenceBeforeWrite(t *testing.T) {
	_, invoker := riskInitHarness(t)
	loop, err := reactloops.NewReActLoop(schema.AI_REACT_LOOP_ACTION_RISK_ENRICH, invoker,
		reactloops.WithAllowRAG(false), reactloops.WithAllowToolCall(false),
		reactloops.WithAllowAIForge(false), reactloops.WithAllowPlanAndExec(false), reactloops.WithAllowUserInteract(false),
		surveyAction(invoker), searchFlowAction(invoker), searchPortAction(invoker),
		inspectFlowAction(invoker), recordAssessmentAction(), attachFlowAction(invoker), fillPortAction(invoker))
	require.NoError(t, err)
	for _, name := range []string{"survey_risk_evidence", "search_risk_flows", "search_risk_ports", "inspect_risk_flow", "assess_risk_flow", "attach_risk_flow", "fill_risk_port"} {
		handler, err := loop.GetActionHandler(name)
		require.NoError(t, err)
		actionSchema := aitool.NewWithoutCallback(name, handler.Options...).ToJSONSchemaString()
		require.NotContains(t, actionSchema, `"risk_id"`, name)
		require.NotContains(t, actionSchema, `"runtime_id"`, name)
		require.NotContains(t, actionSchema, `"session_id"`, name)
	}
	db := projectDB(invoker)
	r := &schema.Risk{Hash: "action-risk", Host: "example.test", RuntimeId: "runtime-a", Description: "Login endpoint reflects marker"}
	require.NoError(t, db.Create(r).Error)
	f := &schema.HTTPFlow{Url: "https://example.test/login", RuntimeId: r.RuntimeId, Request: "GET /login?marker=x HTTP/1.1", Response: "HTTP/1.1 200 OK\r\n\r\nmarker=x"}
	require.NoError(t, db.Create(f).Error)
	other := &schema.HTTPFlow{Url: "https://example.test/login", RuntimeId: "outside", Request: "GET /login?marker=x"}
	require.NoError(t, db.Create(other).Error)
	port := &schema.Port{Host: "example.test", Port: 8443, RuntimeId: r.RuntimeId, ServiceType: "https"}
	require.NoError(t, db.Create(port).Error)
	attachment := aicommon.NewAttachedResource(aicommon.AttachedResourceTypeRiskID, aicommon.AttachedResourceKeyID, strconv.FormatUint(uint64(r.ID), 10))
	readonly := aicommon.NewStatefulTaskBase("risk-query", "只展示关联流量，不要写入", context.Background(), loop.GetEmitter(), true)
	readonly.SetAttachedDatas([]*aicommon.AttachedResource{attachment})
	loop.SetCurrentTask(readonly)
	require.NoError(t, bootstrapRiskEnvironment(loop, invoker, readonly))
	response, err := runRiskAction(t, loop, "survey_risk_evidence", "")
	require.NoError(t, err)
	require.Contains(t, response, "Total HTTP flows: 1")
	response, err = runRiskAction(t, loop, "search_risk_flows", `"risk_id":999999,"url_contains":"/login","request_contains":"marker=x"`)
	require.NoError(t, err)
	require.Contains(t, response, fmt.Sprintf("flow_id=%d", f.ID))
	require.NotContains(t, response, fmt.Sprintf("flow_id=%d", other.ID))
	require.Contains(t, searchHistoryPrompt(loop), "marker=x")
	response, err = runRiskAction(t, loop, "search_risk_ports", `"risk_id":999999,"port":8443`)
	require.NoError(t, err)
	require.Contains(t, response, fmt.Sprintf("port_id=%d", port.ID))
	_, err = runRiskAction(t, loop, "fill_risk_port", fmt.Sprintf(`"port_id":%d`, port.ID))
	require.ErrorContains(t, err, "user did not request")
	args := fmt.Sprintf(`"flow_id":%d`, f.ID)
	_, err = runRiskAction(t, loop, "attach_risk_flow", args)
	require.ErrorContains(t, err, "user did not request")
	_, err = runRiskAction(t, loop, "inspect_risk_flow", fmt.Sprintf(`"flow_id":%d`, other.ID))
	require.ErrorContains(t, err, "boundary")

	writeTask := aicommon.NewStatefulTaskBase("risk-write", "请补全当前风险的证据", context.Background(), loop.GetEmitter(), true)
	writeTask.SetAttachedDatas([]*aicommon.AttachedResource{attachment})
	loop.SetCurrentTask(writeTask)
	require.NoError(t, bootstrapRiskEnvironment(loop, invoker, writeTask))
	_, err = runRiskAction(t, loop, "attach_risk_flow", args)
	require.ErrorContains(t, err, "inspect and mark")
	_, err = runRiskAction(t, loop, "assess_risk_flow", fmt.Sprintf(`"flow_id":%d,"verdict":"supports","reason":"proof"`, f.ID))
	require.ErrorContains(t, err, "inspect_risk_flow")
	response, err = runRiskAction(t, loop, "inspect_risk_flow", args)
	require.NoError(t, err)
	require.Contains(t, response, "marker=x")
	_, err = runRiskAction(t, loop, "assess_risk_flow", fmt.Sprintf(`"flow_id":%d,"verdict":"uncertain","reason":"response is ambiguous"`, f.ID))
	require.NoError(t, err)
	_, err = runRiskAction(t, loop, "attach_risk_flow", args)
	require.ErrorContains(t, err, "inspect and mark")
	_, err = runRiskAction(t, loop, "assess_risk_flow", fmt.Sprintf(`"flow_id":%d,"verdict":"supports","reason":"request marker appears unescaped in response"`, f.ID))
	require.NoError(t, err)
	_, err = runRiskAction(t, loop, "attach_risk_flow", args)
	require.NoError(t, err)
	updated, err := loadRisk(db, int(r.ID))
	require.NoError(t, err)
	require.Len(t, updated.PacketPairs, 1)
	require.Equal(t, int64(f.ID), updated.PacketPairs[0].HTTPFlowId)
	_, err = runRiskAction(t, loop, "fill_risk_port", fmt.Sprintf(`"port_id":%d`, port.ID))
	require.NoError(t, err)
	updated, err = loadRisk(db, int(r.ID))
	require.NoError(t, err)
	require.Equal(t, port.Port, updated.Port)
}

func TestRiskLoopFailsBeforeModelCallWithoutAttachedRisk(t *testing.T) {
	_, invoker := riskInitHarness(t)
	loop, err := reactloops.CreateLoopByName(schema.AI_REACT_LOOP_ACTION_RISK_ENRICH, invoker,
		reactloops.WithAllowRAG(false), reactloops.WithAllowUserInteract(false))
	require.NoError(t, err)
	task := aicommon.NewStatefulTaskBase("risk-missing-attachment", "补全风险 123", context.Background(), loop.GetEmitter(), true)
	err = loop.ExecuteWithExistedTask(task)
	require.ErrorContains(t, err, "attached resource of type risk_id")
}
