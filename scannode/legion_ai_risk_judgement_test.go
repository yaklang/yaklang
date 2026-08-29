package scannode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"testing"
	"time"

	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

const testAIRiskJudgementResultKind = "ai_risk_judgement_v1"

func TestLegionServerFocusRuntimePublishesScopedRiskJudgement(t *testing.T) {
	runtime, _, publisher := newTestRiskJudgementRuntime(t, validAIRiskJudgementResultContext())

	receipt, err := runtime.Execute(serverFocusCapabilitySubmitRiskJudgementV1, validRiskJudgementParams("risk-1"))
	if err != nil {
		t.Fatalf("submit risk judgement: %v", err)
	}
	if receipt["result_id"] == "" || receipt["dedupe_key"] == "" || receipt["backend_id"] != "job-1" {
		t.Fatalf("unexpected judgement receipt: %#v", receipt)
	}
	if len(publisher.reports) != 1 || len(publisher.reportKinds) != 1 {
		t.Fatalf("expected one incremental judgement report, got %#v", publisher)
	}
	if publisher.reportKinds[0] != testAIRiskJudgementResultKind {
		t.Fatalf("unexpected judgement result kind: %q", publisher.reportKinds[0])
	}
	if publisher.lifecycleRef != (jobExecutionRef{
		CommandID: "bind-1",
		JobID:     "job-1",
		SubtaskID: "subtask-1",
		AttemptID: "attempt-1",
	}) {
		t.Fatalf("unexpected judgement execution ref: %#v", publisher.lifecycleRef)
	}

	var payload aiFocusRiskJudgement
	if err := json.Unmarshal(publisher.reports[0], &payload); err != nil {
		t.Fatalf("decode judgement payload: %v", err)
	}
	if payload.RiskID != "risk-1" || payload.Verdict != "confirmed_vuln" || payload.Confidence != 0.93 {
		t.Fatalf("unexpected judgement payload: %#v", payload)
	}
	if payload.OwnerUserID != "user-1" || payload.ProductKey != "ssa" || payload.ProjectID != "project-1" ||
		payload.SourceSnapshotID != "snapshot-1" || payload.SourceSHA256 != strings.Repeat("a", 64) {
		t.Fatalf("locked scope identity was not published: %#v", payload)
	}
	if payload.TaskRunID != "task-run-1" || payload.TaskRunItemID != "task-item-1" ||
		payload.SessionID != "session-1" || payload.TurnID != "turn-1" {
		t.Fatalf("locked runtime identity was not published: %#v", payload)
	}
	if payload.ScopeSHA256 != "529215ea4782d304c7cd9f1d7b65219161d669d2a6a6d410aa131d3d41f63c26" ||
		payload.AllowedRiskIDsSHA256 != testAllowedRiskIDsSHA256([]string{"risk-2", "risk-1", "risk-1"}) ||
		payload.RequiredResultCount != 2 ||
		strings.Join(payload.AllowedRiskIDs, ",") != "risk-1,risk-2" {
		t.Fatalf("locked risk scope hash was not published: %#v", payload)
	}
	if len(payload.EvidenceRefs) != 3 {
		t.Fatalf("unexpected evidence refs: %#v", payload.EvidenceRefs)
	}
	if publisher.reportIDs[0] != receipt["result_id"] || payload.DedupeKey != receipt["dedupe_key"] {
		t.Fatalf("receipt does not match deterministic publication: receipt=%#v payload=%#v", receipt, payload)
	}
	var payloadFields map[string]any
	if err := json.Unmarshal(publisher.reports[0], &payloadFields); err != nil {
		t.Fatalf("decode judgement payload fields: %v", err)
	}
	expectedFields := []string{
		"schema_version", "focus_run_id", "focus_release_id",
		"task_run_id", "task_run_item_id", "session_id", "turn_id",
		"owner_user_id", "product_key", "project_id",
		"source_snapshot_id", "source_sha256",
		"allowed_risk_ids", "allowed_risk_ids_sha256", "required_result_count",
		"scope_sha256", "risk_id", "verdict", "confidence", "reason",
		"fix_suggestion", "evidence_refs", "dedupe_key",
	}
	if len(payloadFields) != len(expectedFields) {
		t.Fatalf("judgement payload fields = %#v, want exactly %#v", payloadFields, expectedFields)
	}
	for _, field := range expectedFields {
		if _, ok := payloadFields[field]; !ok {
			t.Fatalf("judgement payload omitted %q: %#v", field, payloadFields)
		}
	}
}

func TestLegionServerFocusRuntimeRejectsOutOfScopeRiskJudgement(t *testing.T) {
	runtime, _, publisher := newTestRiskJudgementRuntime(t, validAIRiskJudgementResultContext())

	if _, err := runtime.Execute(serverFocusCapabilitySubmitRiskJudgementV1, validRiskJudgementParams("risk-outside")); err == nil ||
		!strings.Contains(err.Error(), "outside the allowed risk scope") {
		t.Fatalf("expected out-of-scope rejection, got %v", err)
	}
	if len(publisher.reports) != 0 {
		t.Fatalf("out-of-scope judgement was published: %#v", publisher.reports)
	}
}

func TestLegionServerFocusRuntimeRejectsInvalidRiskJudgement(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{
			name: "verdict",
			mutate: func(params map[string]any) {
				params["verdict"] = "definitely_safe"
			},
			want: "verdict",
		},
		{
			name: "confidence",
			mutate: func(params map[string]any) {
				params["confidence"] = 1.1
			},
			want: "confidence",
		},
		{
			name: "missing confidence",
			mutate: func(params map[string]any) {
				delete(params, "confidence")
			},
			want: "confidence is required",
		},
		{
			name: "non numeric confidence",
			mutate: func(params map[string]any) {
				params["confidence"] = "0.93"
			},
			want: "confidence must be a number",
		},
		{
			name: "missing evidence",
			mutate: func(params map[string]any) {
				params["evidence_refs"] = []any{}
			},
			want: "evidence_refs",
		},
		{
			name: "invalid file line",
			mutate: func(params map[string]any) {
				params["evidence_refs"] = []any{map[string]any{
					"type":       "file_line",
					"file":       "../outside.go",
					"start_line": 0,
					"end_line":   0,
				}}
			},
			want: "file_line",
		},
		{
			name: "invalid dataflow",
			mutate: func(params map[string]any) {
				params["evidence_refs"] = []any{map[string]any{"type": "dataflow"}}
			},
			want: "dataflow_id",
		},
		{
			name: "invalid rule",
			mutate: func(params map[string]any) {
				params["evidence_refs"] = []any{map[string]any{"type": "rule"}}
			},
			want: "rule_id",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime, _, publisher := newTestRiskJudgementRuntime(t, validAIRiskJudgementResultContext())
			params := validRiskJudgementParams("risk-1")
			test.mutate(params)
			if _, err := runtime.Execute(serverFocusCapabilitySubmitRiskJudgementV1, params); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("submit error = %v, want %q", err, test.want)
			}
			if len(publisher.reports) != 0 {
				t.Fatalf("invalid judgement was published: %#v", publisher.reports)
			}
		})
	}
}

func TestLegionRiskJudgementPublicationIsIdempotentPerRisk(t *testing.T) {
	resultContext := validAIRiskJudgementResultContext()
	setTestRiskJudgementScopeRiskIDs(resultContext, []string{"risk-1"})
	runtime, sink, publisher := newTestRiskJudgementRuntime(t, resultContext)
	params := validRiskJudgementParams("risk-1")

	first, err := runtime.Execute(serverFocusCapabilitySubmitRiskJudgementV1, params)
	if err != nil {
		t.Fatalf("submit first judgement: %v", err)
	}
	second, err := runtime.Execute(serverFocusCapabilitySubmitRiskJudgementV1, params)
	if err != nil {
		t.Fatalf("submit duplicate judgement: %v", err)
	}
	if first["result_id"] != second["result_id"] || first["dedupe_key"] != second["dedupe_key"] {
		t.Fatalf("duplicate judgement identity changed: first=%#v second=%#v", first, second)
	}
	if len(publisher.reportIDs) != 2 || publisher.reportIDs[0] != publisher.reportIDs[1] {
		t.Fatalf("duplicate publication did not reuse event id: %#v", publisher.reportIDs)
	}

	if err := sink.Succeed(context.Background(), []byte(`{"status":"done"}`)); err != nil {
		t.Fatalf("finalize idempotent judgement: %v", err)
	}
	if publisher.succeeded != 1 || len(publisher.reports) != 3 {
		t.Fatalf("unexpected finalization publications: %#v", publisher)
	}
	var summary map[string]any
	if err := json.Unmarshal(publisher.reports[2], &summary); err != nil {
		t.Fatalf("decode focus summary: %v", err)
	}
	if summary["risk_judgement_count"] != float64(1) || len(summary["risk_judgement_result_ids"].([]any)) != 1 {
		t.Fatalf("duplicate judgement inflated summary: %#v", summary)
	}
}

func TestLegionRiskJudgementFinalizerRequiresConfiguredCount(t *testing.T) {
	resultContext := validAIRiskJudgementResultContext()
	resultContext.RiskJudgementScope.RequiredResultCount = 2
	runtime, sink, publisher := newTestRiskJudgementRuntime(t, resultContext)

	if err := sink.Succeed(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "required results") {
		t.Fatalf("expected required-kind finalizer rejection, got %v", err)
	}
	if _, err := runtime.Execute(serverFocusCapabilitySubmitRiskJudgementV1, validRiskJudgementParams("risk-1")); err != nil {
		t.Fatalf("submit first required judgement: %v", err)
	}
	if err := sink.Succeed(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "1 of 2") {
		t.Fatalf("expected required-count finalizer rejection, got %v", err)
	}
	if publisher.succeeded != 0 {
		t.Fatalf("finalizer published success before required count: %#v", publisher)
	}
	if _, err := runtime.Execute(serverFocusCapabilitySubmitRiskJudgementV1, validRiskJudgementParams("risk-2")); err != nil {
		t.Fatalf("submit second required judgement: %v", err)
	}
	if err := sink.Succeed(context.Background(), nil); err != nil {
		t.Fatalf("finalize complete judgement set: %v", err)
	}
	if publisher.succeeded != 1 {
		t.Fatalf("expected one success after complete judgement set, got %d", publisher.succeeded)
	}
}

func TestLegionRiskJudgementScopeFailsClosed(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*aiv1.AIFocusResultContext)
		want   string
	}{
		{
			name: "missing scope",
			mutate: func(resultContext *aiv1.AIFocusResultContext) {
				resultContext.RiskJudgementScope = nil
			},
			want: "scope is required",
		},
		{
			name: "missing owner",
			mutate: func(resultContext *aiv1.AIFocusResultContext) {
				resultContext.RiskJudgementScope.OwnerUserId = ""
			},
			want: "owner_user_id",
		},
		{
			name: "hash mismatch",
			mutate: func(resultContext *aiv1.AIFocusResultContext) {
				resultContext.RiskJudgementScope.AllowedRiskIdsSha256 = strings.Repeat("f", 64)
			},
			want: "allowed_risk_ids_sha256 mismatch",
		},
		{
			name: "required count exceeds scope",
			mutate: func(resultContext *aiv1.AIFocusResultContext) {
				resultContext.RiskJudgementScope.RequiredResultCount = 3
			},
			want: "required_result_count",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resultContext := validAIRiskJudgementResultContext()
			test.mutate(resultContext)
			publisher := &recordingAIFocusRiskPublisher{}
			rawSink, err := newLegionAIFocusResultSink(publisher, "bind-1", resultContext)
			if test.name == "missing scope" {
				if err != nil {
					t.Fatalf("legacy result context without judgement scope must remain valid: %v", err)
				}
				sink := rawSink.(*legionAIFocusResultSink)
				if err := sink.bindFocusExecutionContract(testLegionRiskJudgementExecutionContract(t)); err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("bind error = %v, want %q", err, test.want)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("new sink error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestLegionRiskJudgementContractRejectsWrongReportKind(t *testing.T) {
	publisher := &recordingAIFocusRiskPublisher{}
	rawSink, err := newLegionAIFocusResultSink(publisher, "bind-1", validAIRiskJudgementResultContext())
	if err != nil {
		t.Fatalf("new judgement result sink: %v", err)
	}
	sink := rawSink.(*legionAIFocusResultSink)
	if err := sink.bindFocusExecutionContract(
		testLegionRiskJudgementExecutionContractWithKind(t, "job_risk"),
	); err == nil || !strings.Contains(err.Error(), legionAIRiskJudgementReportKindV1) {
		t.Fatalf("bind error = %v, want exact judgement report kind rejection", err)
	}
}

func TestLegionRiskJudgementRuntimeAndProxyFenceOldBindings(t *testing.T) {
	runtime, _, publisher := newTestRiskJudgementRuntime(t, validAIRiskJudgementResultContext())
	runtime.deactivateFocusTurn(runtime.authorizedFocusReleaseID)
	if _, err := runtime.Execute(serverFocusCapabilitySubmitRiskJudgementV1, validRiskJudgementParams("risk-1")); err == nil ||
		!strings.Contains(err.Error(), "authorized Focus Turn") {
		t.Fatalf("expected inactive-turn fence, got %v", err)
	}
	if len(publisher.reports) != 0 {
		t.Fatalf("inactive runtime published judgement: %#v", publisher.reports)
	}

	oldSink := &blockingAIFocusResultSink{
		started:  make(chan struct{}),
		release:  make(chan struct{}),
		resultID: "old-attempt",
	}
	newSink := &immediateAIFocusResultSink{resultID: "new-attempt"}
	proxy := newAISessionResultSinkProxy(oldSink)
	judgement := aiFocusRiskJudgement{RiskID: "risk-1"}

	submitDone := make(chan aiFocusResultReceipt, 1)
	go func() {
		receipt, _ := proxy.SubmitRiskJudgement(context.Background(), testAIRiskJudgementResultKind, judgement)
		submitDone <- receipt
	}()
	<-oldSink.started

	setDone := make(chan struct{})
	go func() {
		proxy.Set(newSink)
		close(setDone)
	}()
	select {
	case <-setDone:
		t.Fatal("rebind completed while an old-attempt judgement was still in flight")
	case <-time.After(50 * time.Millisecond):
	}

	close(oldSink.release)
	if receipt := <-submitDone; receipt.ResultID != "old-attempt" {
		t.Fatalf("unexpected old-attempt judgement receipt: %#v", receipt)
	}
	select {
	case <-setDone:
	case <-time.After(time.Second):
		t.Fatal("rebind did not complete after the old-attempt judgement finished")
	}

	receipt, err := proxy.SubmitRiskJudgement(context.Background(), testAIRiskJudgementResultKind, judgement)
	if err != nil {
		t.Fatalf("submit rebound judgement: %v", err)
	}
	if receipt.ResultID != "new-attempt" {
		t.Fatalf("expected rebound judgement sink, got %#v", receipt)
	}
}

func (s *blockingAIFocusResultSink) SubmitRiskJudgement(
	context.Context,
	string,
	aiFocusRiskJudgement,
) (aiFocusResultReceipt, error) {
	close(s.started)
	<-s.release
	return aiFocusResultReceipt{ResultID: s.resultID}, nil
}

func (s *immediateAIFocusResultSink) SubmitRiskJudgement(
	context.Context,
	string,
	aiFocusRiskJudgement,
) (aiFocusResultReceipt, error) {
	return aiFocusResultReceipt{ResultID: s.resultID}, nil
}

func newTestRiskJudgementRuntime(
	t *testing.T,
	resultContext *aiv1.AIFocusResultContext,
) (*legionServerFocusRuntime, *legionAIFocusResultSink, *recordingAIFocusRiskPublisher) {
	t.Helper()
	publisher := &recordingAIFocusRiskPublisher{}
	rawSink, err := newLegionAIFocusResultSink(publisher, "bind-1", resultContext)
	if err != nil {
		t.Fatalf("new judgement result sink: %v", err)
	}
	sink := rawSink.(*legionAIFocusResultSink)
	runtimeValue, err := newLegionServerFocusRuntime(context.Background(), resultContext.GetTargetUrl(), sink)
	if err != nil {
		t.Fatalf("new judgement focus runtime: %v", err)
	}
	runtime := runtimeValue.(*legionServerFocusRuntime)
	runtime.authorizedFocusReleaseID = resultContext.GetFocusReleaseId()
	if err := runtime.activateFocusTurn(resultContext.GetFocusReleaseId(), testLegionRiskJudgementExecutionContract(t)); err != nil {
		t.Fatalf("activate judgement Focus Turn: %v", err)
	}
	t.Cleanup(func() { runtime.deactivateFocusTurn(resultContext.GetFocusReleaseId()) })
	return runtime, sink, publisher
}

func testLegionRiskJudgementExecutionContract(t *testing.T) *legionFocusExecutionContract {
	return testLegionRiskJudgementExecutionContractWithKind(t, testAIRiskJudgementResultKind)
}

func testLegionRiskJudgementExecutionContractWithKind(
	t *testing.T,
	kind string,
) *legionFocusExecutionContract {
	t.Helper()
	raw, err := json.Marshal(legionFocusExecutionContract{
		SchemaVersion: legionFocusExecutionContractSchemaV1,
		Stages:        []legionFocusExecutionStage{{Key: "risk_judgement"}},
		Capabilities:  []string{serverFocusCapabilitySubmitRiskJudgementV1},
		Results: []legionFocusExecutionResultContract{{
			Key:        "risk_judgements",
			Capability: serverFocusCapabilitySubmitRiskJudgementV1,
			Kind:       kind,
			Required:   true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := parseLegionFocusExecutionContract(string(raw))
	if err != nil {
		t.Fatalf("parse judgement execution contract: %v", err)
	}
	return contract
}

func validAIRiskJudgementResultContext() *aiv1.AIFocusResultContext {
	resultContext := validAIFocusResultContext()
	resultContext.FocusMode = "ssa_risk_ai_judgement"
	resultContext.FocusReleaseId = "ssa_risk_ai_judgement@1.0.0+abcdef123456"
	resultContext.TargetUrl = "https://workspace.invalid/snapshot-1/"
	riskIDs := []string{"risk-2", "risk-1", "risk-1"}
	resultContext.RiskJudgementScope = &aiv1.AIFocusRiskJudgementScope{
		OwnerUserId:          "user-1",
		ProductKey:           "ssa",
		ProjectId:            "project-1",
		SourceSnapshotId:     "snapshot-1",
		SourceSha256:         strings.Repeat("a", 64),
		AllowedRiskIds:       riskIDs,
		AllowedRiskIdsSha256: testAllowedRiskIDsSHA256(riskIDs),
		RequiredResultCount:  2,
		TaskRunId:            "task-run-1",
		TaskRunItemId:        "task-item-1",
		SessionId:            "session-1",
		TurnId:               "turn-1",
	}
	return resultContext
}

func setTestRiskJudgementScopeRiskIDs(resultContext *aiv1.AIFocusResultContext, riskIDs []string) {
	resultContext.RiskJudgementScope.AllowedRiskIds = append([]string(nil), riskIDs...)
	resultContext.RiskJudgementScope.AllowedRiskIdsSha256 = testAllowedRiskIDsSHA256(riskIDs)
	unique := make(map[string]struct{}, len(riskIDs))
	for _, riskID := range riskIDs {
		if riskID = strings.TrimSpace(riskID); riskID != "" {
			unique[riskID] = struct{}{}
		}
	}
	resultContext.RiskJudgementScope.RequiredResultCount = uint32(len(unique))
}

func validRiskJudgementParams(riskID string) map[string]any {
	return map[string]any{
		"risk_id":        riskID,
		"verdict":        "confirmed_vuln",
		"confidence":     0.93,
		"reason":         "Untrusted request data reaches a SQL execution sink.",
		"fix_suggestion": "Use a parameterized query and validate the identifier allowlist.",
		"evidence_refs": []any{
			map[string]any{"type": "dataflow", "dataflow_id": "flow-1"},
			map[string]any{"type": "file_line", "file": "pkg/query.go", "start_line": 21, "end_line": 24},
			map[string]any{"type": "rule", "rule_id": "CWE-89"},
		},
	}
}

func testAllowedRiskIDsSHA256(values []string) string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			set[value] = struct{}{}
		}
	}
	canonical := make([]string, 0, len(set))
	for value := range set {
		canonical = append(canonical, value)
	}
	sort.Strings(canonical)
	raw, _ := json.Marshal(canonical)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

var _ aiFocusResultSink = (*blockingAIFocusResultSink)(nil)
var _ aiFocusResultSink = (*immediateAIFocusResultSink)(nil)
