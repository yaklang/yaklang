package scannode

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	jobv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/job/v1"
)

type recordedAIFocusRisk struct {
	eventID   string
	ref       jobExecutionRef
	riskKind  string
	title     string
	target    string
	severity  string
	dedupeKey string
	raw       []byte
}

type recordedAIFocusAsset struct {
	eventID     string
	ref         jobExecutionRef
	assetKind   string
	title       string
	target      string
	identityKey string
	raw         []byte
}

type recordingAIFocusRiskPublisher struct {
	risks        []recordedAIFocusRisk
	assets       []recordedAIFocusAsset
	succeeded    int
	failed       int
	cancelled    int
	lifecycleRef jobExecutionRef
	reports      [][]byte
	reportIDs    []string
	order        []string
}

type blockingAIFocusResultSink struct {
	started  chan struct{}
	release  chan struct{}
	resultID string
}

func (s *blockingAIFocusResultSink) SubmitRisk(
	context.Context,
	*schema.Risk,
) (aicommon.ResultReceipt, error) {
	close(s.started)
	<-s.release
	return aicommon.ResultReceipt{ResultID: s.resultID}, nil
}

type immediateAIFocusResultSink struct {
	resultID string
}

func (s *immediateAIFocusResultSink) SubmitRisk(
	context.Context,
	*schema.Risk,
) (aicommon.ResultReceipt, error) {
	return aicommon.ResultReceipt{ResultID: s.resultID}, nil
}

func (p *recordingAIFocusRiskPublisher) PublishAssetWithEventID(
	_ context.Context,
	ref jobExecutionRef,
	eventID string,
	assetKind string,
	title string,
	target string,
	identityKey string,
	raw []byte,
) error {
	p.assets = append(p.assets, recordedAIFocusAsset{
		eventID:     eventID,
		ref:         ref,
		assetKind:   assetKind,
		title:       title,
		target:      target,
		identityKey: identityKey,
		raw:         append([]byte(nil), raw...),
	})
	return nil
}

func (p *recordingAIFocusRiskPublisher) PublishRiskWithEventID(
	_ context.Context,
	ref jobExecutionRef,
	eventID string,
	riskKind string,
	title string,
	target string,
	severity string,
	dedupeKey string,
	raw []byte,
) error {
	p.risks = append(p.risks, recordedAIFocusRisk{
		eventID:   eventID,
		ref:       ref,
		riskKind:  riskKind,
		title:     title,
		target:    target,
		severity:  severity,
		dedupeKey: dedupeKey,
		raw:       append([]byte(nil), raw...),
	})
	return nil
}

func (p *recordingAIFocusRiskPublisher) PublishSucceeded(
	_ context.Context,
	ref jobExecutionRef,
	_ any,
) error {
	p.succeeded++
	p.lifecycleRef = ref
	p.order = append(p.order, "succeeded")
	return nil
}

func (p *recordingAIFocusRiskPublisher) PublishReportWithEventID(
	_ context.Context,
	ref jobExecutionRef,
	eventID string,
	_ string,
	reportJSON []byte,
) error {
	p.lifecycleRef = ref
	p.reportIDs = append(p.reportIDs, eventID)
	p.reports = append(p.reports, append([]byte(nil), reportJSON...))
	p.order = append(p.order, "report")
	return nil
}

func (p *recordingAIFocusRiskPublisher) PublishFailed(
	_ context.Context,
	ref jobExecutionRef,
	_ string,
	_ string,
	_ map[string]string,
) error {
	p.failed++
	p.lifecycleRef = ref
	return nil
}

func (p *recordingAIFocusRiskPublisher) PublishCancelled(
	_ context.Context,
	ref jobExecutionRef,
	_ string,
) error {
	p.cancelled++
	p.lifecycleRef = ref
	return nil
}

func TestLegionAIFocusResultSinkPublishesRunScopedRisk(t *testing.T) {
	t.Parallel()

	publisher := &recordingAIFocusRiskPublisher{}
	sink, err := newLegionAIFocusResultSink(
		publisher,
		"bind-1",
		validAIFocusResultContext(),
	)
	if err != nil {
		t.Fatalf("new result sink: %v", err)
	}

	first := &schema.Risk{
		Hash:           "risk-1",
		Url:            "https://example.com/orders?id=2",
		Title:          "订单接口越权",
		RiskType:       "privilege-escalation",
		Severity:       "warning",
		Parameter:      "id",
		QuotedResponse: strings.Repeat("中", maxInlineFocusRiskFieldBytes/2),
	}
	receipt, err := sink.SubmitRisk(context.Background(), first)
	if err != nil {
		t.Fatalf("submit risk: %v", err)
	}
	if receipt.ResultID == "" || receipt.BackendID != "job-1" {
		t.Fatalf("unexpected receipt: %#v", receipt)
	}
	if len(publisher.risks) != 1 {
		t.Fatalf("expected one published risk, got %d", len(publisher.risks))
	}
	published := publisher.risks[0]
	if published.eventID != receipt.ResultID {
		t.Fatalf("expected receipt to expose platform event id, got %#v", published)
	}
	if published.ref != (jobExecutionRef{
		CommandID: "bind-1",
		JobID:     "job-1",
		SubtaskID: "subtask-1",
		AttemptID: "attempt-1",
	}) {
		t.Fatalf("unexpected job reference: %#v", published.ref)
	}
	if published.severity != "medium" {
		t.Fatalf("expected warning to normalize to medium, got %s", published.severity)
	}
	if published.dedupeKey == "" || published.dedupeKey != receipt.DedupeKey {
		t.Fatalf("unexpected dedupe key: %q", published.dedupeKey)
	}
	var payload schema.Risk
	if err := json.Unmarshal(published.raw, &payload); err != nil {
		t.Fatalf("unmarshal published risk: %v", err)
	}
	if len(payload.QuotedResponse) > maxInlineFocusRiskFieldBytes ||
		!utf8.ValidString(payload.QuotedResponse) {
		t.Fatalf("expected response evidence to be capped at a UTF-8 boundary, got %d bytes", len(payload.QuotedResponse))
	}

	second := *first
	second.Hash = "risk-2"
	second.Title = "同一漏洞的另一标题"
	second.QuotedResponse = ""
	secondReceipt, err := sink.SubmitRisk(context.Background(), &second)
	if err != nil {
		t.Fatalf("submit duplicate risk: %v", err)
	}
	if secondReceipt.DedupeKey != receipt.DedupeKey {
		t.Fatalf("expected stable dedupe key, got %s and %s", receipt.DedupeKey, secondReceipt.DedupeKey)
	}
	if secondReceipt.ResultID != receipt.ResultID {
		t.Fatalf(
			"expected retries to reuse one platform event id, got %s and %s",
			receipt.ResultID,
			secondReceipt.ResultID,
		)
	}
	if publisher.risks[1].eventID != publisher.risks[0].eventID {
		t.Fatalf(
			"expected duplicate risks to publish one event id, got %s and %s",
			publisher.risks[0].eventID,
			publisher.risks[1].eventID,
		)
	}
}

func TestAISessionResultSinkProxyFencesRebindAgainstInFlightSubmission(t *testing.T) {
	t.Parallel()

	oldSink := &blockingAIFocusResultSink{
		started:  make(chan struct{}),
		release:  make(chan struct{}),
		resultID: "old-attempt",
	}
	newSink := &immediateAIFocusResultSink{resultID: "new-attempt"}
	proxy := newAISessionResultSinkProxy(oldSink)

	submitDone := make(chan aicommon.ResultReceipt, 1)
	go func() {
		receipt, _ := proxy.SubmitRisk(context.Background(), &schema.Risk{})
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
		t.Fatal("rebind completed while an old-attempt submission was still in flight")
	case <-time.After(50 * time.Millisecond):
	}

	close(oldSink.release)
	if receipt := <-submitDone; receipt.ResultID != "old-attempt" {
		t.Fatalf("unexpected in-flight receipt: %#v", receipt)
	}
	select {
	case <-setDone:
	case <-time.After(time.Second):
		t.Fatal("rebind did not complete after the in-flight submission finished")
	}

	receipt, err := proxy.SubmitRisk(context.Background(), &schema.Risk{})
	if err != nil {
		t.Fatalf("submit after rebind: %v", err)
	}
	if receipt.ResultID != "new-attempt" {
		t.Fatalf("expected new sink after rebind, got %#v", receipt)
	}
}

func TestLegionAIFocusResultSinkPublishesIdempotentAsset(t *testing.T) {
	t.Parallel()

	publisher := &recordingAIFocusRiskPublisher{}
	sink, err := newLegionAIFocusResultSink(
		publisher,
		"bind-1",
		validAIFocusResultContext(),
	)
	if err != nil {
		t.Fatalf("new result sink: %v", err)
	}
	assetSink, ok := sink.(aicommon.AssetResultSink)
	if !ok {
		t.Fatalf("expected Legion result sink to support structured assets")
	}
	asset := aicommon.AssetResult{
		Kind:        "http_endpoint",
		Title:       "GET https://example.com/api/users",
		Target:      "https://example.com/api/users",
		IdentityKey: "http_endpoint:GET:https://example.com/api/users",
		Payload:     []byte(`{"method":"GET","status_code":200,"verified":true}`),
	}

	first, err := assetSink.SubmitAsset(context.Background(), asset)
	if err != nil {
		t.Fatalf("submit asset: %v", err)
	}
	second, err := assetSink.SubmitAsset(context.Background(), asset)
	if err != nil {
		t.Fatalf("resubmit asset: %v", err)
	}
	if first.ResultID == "" || second.ResultID != first.ResultID {
		t.Fatalf("expected a stable asset event id, first=%#v second=%#v", first, second)
	}
	if first.DedupeKey != asset.IdentityKey || first.BackendID != "job-1" {
		t.Fatalf("unexpected asset receipt: %#v", first)
	}
	if len(publisher.assets) != 2 {
		t.Fatalf("expected two idempotent publications, got %d", len(publisher.assets))
	}
	published := publisher.assets[0]
	if published.eventID != first.ResultID ||
		published.assetKind != asset.Kind ||
		published.target != asset.Target ||
		published.identityKey != asset.IdentityKey ||
		string(published.raw) != string(asset.Payload) {
		t.Fatalf("unexpected published asset: %#v", published)
	}

	reboundContext := validAIFocusResultContext()
	reboundContext.Job.AttemptId = "attempt-2"
	reboundPublisher := &recordingAIFocusRiskPublisher{}
	reboundSink, err := newLegionAIFocusResultSink(
		reboundPublisher,
		"bind-2",
		reboundContext,
	)
	if err != nil {
		t.Fatalf("new rebound result sink: %v", err)
	}
	reboundReceipt, err := reboundSink.(aicommon.AssetResultSink).
		SubmitAsset(context.Background(), asset)
	if err != nil {
		t.Fatalf("submit rebound asset: %v", err)
	}
	if reboundReceipt.ResultID != first.ResultID {
		t.Fatalf(
			"expected asset identity to remain stable across runtime rebinds, first=%s rebound=%s",
			first.ResultID,
			reboundReceipt.ResultID,
		)
	}
}

func TestValidateLegionAIFocusResultContextRejectsIncompleteIdentity(t *testing.T) {
	t.Parallel()

	resultContext := validAIFocusResultContext()
	resultContext.Job.AttemptId = ""
	_, err := validateLegionAIFocusResultContext("bind-1", resultContext)
	if err == nil || !strings.Contains(err.Error(), "attempt_id is required") {
		t.Fatalf("expected missing attempt validation error, got %v", err)
	}

	resultContext = validAIFocusResultContext()
	resultContext.SchemaVersion = "legion.focus-result.v2"
	_, err = validateLegionAIFocusResultContext("bind-1", resultContext)
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("expected schema validation error, got %v", err)
	}

	resultContext = validAIFocusResultContext()
	resultContext.TargetUrl = ""
	_, err = validateLegionAIFocusResultContext("bind-1", resultContext)
	if err == nil || !strings.Contains(err.Error(), "target_url is required") {
		t.Fatalf("expected target validation error, got %v", err)
	}
}

func TestLegionAIFocusResultSinkProvidesServerAuthorizedTarget(t *testing.T) {
	t.Parallel()

	sink, err := newLegionAIFocusResultSink(
		&recordingAIFocusRiskPublisher{},
		"bind-1",
		validAIFocusResultContext(),
	)
	if err != nil {
		t.Fatalf("new result sink: %v", err)
	}
	if target := aicommon.AuthorizedTargetURLFromConfig(&resultSinkConfigStub{sink: sink}); target != "https://example.com/health?q=1" {
		t.Fatalf("unexpected authorized target: %q", target)
	}
}

type resultSinkConfigStub struct {
	sink aicommon.ResultSink
}

func (s *resultSinkConfigStub) GetResultSink() aicommon.ResultSink { return s.sink }

func TestValidateAISessionBindCommandRequiresMatchingFocusRun(t *testing.T) {
	t.Parallel()

	command := validAISessionBindCommand()
	command.ResultContext = validAIFocusResultContext()
	command.Session.RunId = "different-run"

	err := validateAISessionBindCommand("node-ai", command)
	if err == nil || !strings.Contains(err.Error(), "run_id must match") {
		t.Fatalf("expected focus run mismatch, got %v", err)
	}
}

func TestAISessionResultSinkProxySwitchesRebindIdentityAndFinalizes(t *testing.T) {
	t.Parallel()

	firstPublisher := &recordingAIFocusRiskPublisher{}
	first, err := newLegionAIFocusResultSink(
		firstPublisher,
		"bind-1",
		validAIFocusResultContext(),
	)
	if err != nil {
		t.Fatalf("new first sink: %v", err)
	}
	secondContext := validAIFocusResultContext()
	secondContext.Job.AttemptId = "attempt-2"
	secondPublisher := &recordingAIFocusRiskPublisher{}
	second, err := newLegionAIFocusResultSink(
		secondPublisher,
		"bind-2",
		secondContext,
	)
	if err != nil {
		t.Fatalf("new second sink: %v", err)
	}

	proxy := newAISessionResultSinkProxy(first)
	proxy.Set(second)
	_, err = proxy.SubmitRisk(context.Background(), &schema.Risk{
		Hash:     "risk-rebound",
		Url:      "https://example.com",
		Title:    "Rebound result",
		RiskType: "info-exposure",
		Severity: "low",
	})
	if err != nil {
		t.Fatalf("submit rebound risk: %v", err)
	}
	if len(firstPublisher.risks) != 0 || len(secondPublisher.risks) != 1 {
		t.Fatalf(
			"expected only rebound sink to publish, first=%d second=%d",
			len(firstPublisher.risks),
			len(secondPublisher.risks),
		)
	}
	if secondPublisher.risks[0].ref.AttemptID != "attempt-2" {
		t.Fatalf("unexpected rebound attempt: %#v", secondPublisher.risks[0].ref)
	}
	if err := proxy.Succeed(context.Background(), []byte(`{"status":"done"}`)); err != nil {
		t.Fatalf("finalize rebound result: %v", err)
	}
	if secondPublisher.succeeded != 1 ||
		secondPublisher.lifecycleRef.AttemptID != "attempt-2" {
		t.Fatalf("unexpected lifecycle publication: %#v", secondPublisher)
	}
}

func TestLegionAIFocusResultSinkPublishesIdempotentSummaryBeforeSuccess(t *testing.T) {
	publisher := &recordingAIFocusRiskPublisher{}
	rawSink, err := newLegionAIFocusResultSink(publisher, "bind-1", validAIFocusResultContext())
	if err != nil {
		t.Fatalf("new result sink: %v", err)
	}
	sink := rawSink.(*legionAIFocusResultSink)
	asset := aicommon.AssetResult{
		Kind:        "http_endpoint",
		Title:       "HEAD https://example.com/health?q=1",
		Target:      "https://example.com/health?q=1",
		IdentityKey: "http_endpoint:HEAD:https://example.com/health?q=1",
		Payload:     []byte(`{"status_code":204}`),
	}
	for range 2 {
		if _, err := sink.SubmitAsset(context.Background(), asset); err != nil {
			t.Fatalf("submit asset: %v", err)
		}
	}
	risk := &schema.Risk{
		Url:      asset.Target,
		Title:    "Observed response",
		RiskType: "observation",
		Severity: "info",
	}
	for range 2 {
		if _, err := sink.SubmitRisk(context.Background(), risk); err != nil {
			t.Fatalf("submit risk: %v", err)
		}
	}
	if err := sink.Succeed(context.Background(), []byte(`{
		"focus_run_id":"client-spoof",
		"focus_mode":"client-spoof",
		"status":"done"
	}`)); err != nil {
		t.Fatalf("succeed: %v", err)
	}
	if len(publisher.reports) != 1 || len(publisher.reportIDs) != 1 {
		t.Fatalf("expected one summary report: %#v", publisher)
	}
	if len(publisher.order) != 2 || publisher.order[0] != "report" || publisher.order[1] != "succeeded" {
		t.Fatalf("unexpected publication order: %#v", publisher.order)
	}
	var summary map[string]any
	if err := json.Unmarshal(publisher.reports[0], &summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if summary["focus_run_id"] != "focus-run-1" || summary["focus_mode"] != "infosec_recon" {
		t.Fatalf("server identity was not preserved: %#v", summary)
	}
	if summary["asset_count"] != float64(1) || summary["risk_count"] != float64(1) {
		t.Fatalf("unexpected unique result counts: %#v", summary)
	}
	if summary["target_url"] != "https://example.com/health?q=1" {
		t.Fatalf("unexpected target in summary: %#v", summary)
	}
	if publisher.reportIDs[0] != focusSummaryReportEventID("job-1") {
		t.Fatalf("unexpected report event id: %q", publisher.reportIDs[0])
	}
}

func validAIFocusResultContext() *aiv1.AIFocusResultContext {
	return &aiv1.AIFocusResultContext{
		FocusRunId:    "focus-run-1",
		FocusMode:     "infosec_recon",
		SchemaVersion: legionAIFocusResultSchemaV1,
		ExecutionMode: "single_run",
		TargetUrl:     "https://example.com/health?q=1",
		Job: &jobv1.JobRef{
			JobId:     "job-1",
			SubtaskId: "subtask-1",
			AttemptId: "attempt-1",
		},
	}
}
