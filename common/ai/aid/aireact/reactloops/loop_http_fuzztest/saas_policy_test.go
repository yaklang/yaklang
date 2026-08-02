package loop_http_fuzztest

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
)

type boundedHTTPFuzzTestSink struct {
	target string
	assets []aicommon.AssetResult
}

func (s *boundedHTTPFuzzTestSink) AuthorizedTargetURL() string {
	return s.target
}

func (s *boundedHTTPFuzzTestSink) SubmitRisk(context.Context, *schema.Risk) (aicommon.ResultReceipt, error) {
	return aicommon.ResultReceipt{ResultID: "risk-1"}, nil
}

func (s *boundedHTTPFuzzTestSink) SubmitAsset(_ context.Context, asset aicommon.AssetResult) (aicommon.ResultReceipt, error) {
	s.assets = append(s.assets, asset)
	return aicommon.ResultReceipt{ResultID: "asset-1"}, nil
}

func newBoundedHTTPFuzzTestLoop(t *testing.T, target string) (*reactloops.ReActLoop, *boundedHTTPFuzzTestSink) {
	t.Helper()
	sink := &boundedHTTPFuzzTestSink{target: target}
	config := aicommon.NewConfig(context.Background(), aicommon.WithResultSink(sink))
	loop := reactloops.NewMinimalReActLoop(config, nil)
	loop.Set("current_request", "GET /api/search?q=1 HTTP/1.1\r\nHost: example.test\r\n\r\n")
	loop.Set("is_https", "true")
	return loop, sink
}

func TestNormalizeBoundedSaaSHTTPFuzzTarget(t *testing.T) {
	got, err := normalizeBoundedSaaSHTTPFuzzTarget("https://Example.test:443/api/search?q=1")
	if err != nil {
		t.Fatalf("normalize target: %v", err)
	}
	if got != "https://example.test/api/search?q=1" {
		t.Fatalf("unexpected normalized target: %q", got)
	}
	root, err := normalizeBoundedSaaSHTTPFuzzTarget("https://Example.test")
	if err != nil || root != "https://example.test/" {
		t.Fatalf("unexpected normalized root target: %q err=%v", root, err)
	}

	for _, target := range []string{
		"file:///tmp/request",
		"https://user:secret@example.test/api",
		"https://example.test/api#fragment",
		"https://example.test/api\nHost: other.test",
	} {
		if _, err := normalizeBoundedSaaSHTTPFuzzTarget(target); err == nil {
			t.Fatalf("expected target to be rejected: %q", target)
		}
	}
}

func TestBoundedSaaSHTTPFuzzActionAllowlist(t *testing.T) {
	for _, action := range []string{"fuzz_get_params", "fuzz_header", "generate_risk", "finish"} {
		if !boundedSaaSHTTPFuzzActionAllowed(action) {
			t.Fatalf("expected action %q to be allowed", action)
		}
	}
	for _, action := range []string{"set_http_request", "modify_http_request", "fuzz_path", "fuzz_body", "fuzz_upload", "fuzz_cookie", "generate_and_send_packet", "read_file"} {
		if boundedSaaSHTTPFuzzActionAllowed(action) {
			t.Fatalf("expected action %q to be rejected", action)
		}
	}
}

func TestValidateBoundedSaaSActions(t *testing.T) {
	loop, _ := newBoundedHTTPFuzzTestLoop(t, "https://example.test/api/search?q=1")
	if err := validateBoundedSaaSGetParamsAction(loop, "q", []string{"", "boundary-1"}, false); err != nil {
		t.Fatalf("expected bounded query action to pass: %v", err)
	}
	if err := validateBoundedSaaSHeaderAction(loop, "User-Agent", []string{"IRify-Bounded-Test"}); err != nil {
		t.Fatalf("expected bounded header action to pass: %v", err)
	}

	for name, err := range map[string]error{
		"raw query":      validateBoundedSaaSGetParamsAction(loop, "q", []string{"1"}, true),
		"unsafe name":    validateBoundedSaaSGetParamsAction(loop, "q&next", []string{"1"}, false),
		"fuzztag":        validateBoundedSaaSGetParamsAction(loop, "q", []string{"{{int(1-100)}}"}, false),
		"too many":       validateBoundedSaaSGetParamsAction(loop, "q", []string{"1", "2", "3", "4"}, false),
		"host header":    validateBoundedSaaSHeaderAction(loop, "Host", []string{"other.test"}),
		"control header": validateBoundedSaaSHeaderAction(loop, "User-Agent", []string{"ok\r\nHost: other.test"}),
	} {
		if err == nil {
			t.Fatalf("expected %s to be rejected", name)
		}
	}
}

func TestPinBoundedSaaSHeaderActionOverridesModelParameters(t *testing.T) {
	action, err := aicommon.ExtractAction(
		`{"@action":"fuzz_header","header_name":"Host","header_values":["other.test"]}`,
		"fuzz_header",
	)
	if err != nil {
		t.Fatalf("extract action: %v", err)
	}
	pinBoundedSaaSHeaderAction(action)
	if got := action.GetString("header_name"); got != boundedSaaSHeaderName {
		t.Fatalf("unexpected pinned header name: %q", got)
	}
	gotValues := action.GetStringSlice("header_values")
	if len(gotValues) != len(boundedSaaSHeaderValues) || gotValues[0] != "en-US" || gotValues[1] != "zh-CN" {
		t.Fatalf("unexpected pinned header values: %#v", gotValues)
	}

	loop, _ := newBoundedHTTPFuzzTestLoop(t, "https://example.test/api/search?q=1")
	if err := validateBoundedSaaSHeaderAction(loop, action.GetString("header_name"), gotValues); err != nil {
		t.Fatalf("pinned action must satisfy bounded policy: %v", err)
	}
}

func TestValidateBoundedSaaSHTTPFuzzRequestRejectsTargetChanges(t *testing.T) {
	loop, _ := newBoundedHTTPFuzzTestLoop(t, "https://example.test/api/search?q=1")
	if err := validateBoundedSaaSHTTPFuzzRequest(loop); err != nil {
		t.Fatalf("expected exact request to pass: %v", err)
	}

	loop.Set("current_request", "GET /api/search?q=1 HTTP/1.1\r\nHost: other.test\r\n\r\n")
	if err := validateBoundedSaaSHTTPFuzzRequest(loop); err == nil {
		t.Fatal("expected changed Host to be rejected")
	}
}

func TestClaimBoundedSaaSHTTPFuzzBatchRejectsSecondBatch(t *testing.T) {
	loop, _ := newBoundedHTTPFuzzTestLoop(t, "https://example.test/api/search?q=1")
	if err := claimBoundedSaaSHTTPFuzzBatch(loop); err != nil {
		t.Fatalf("claim first batch: %v", err)
	}
	if err := claimBoundedSaaSHTTPFuzzBatch(loop); err == nil {
		t.Fatal("expected second batch to be rejected")
	}
}

func TestBoundedSaaSHTTPFuzzCompletionGate(t *testing.T) {
	loop, _ := newBoundedHTTPFuzzTestLoop(t, "https://example.test/api/search?q=1")
	if err := validateBoundedSaaSHTTPFuzzCompleted(loop); err == nil {
		t.Fatal("expected finish and risk publication to be blocked before a successful batch")
	}
	if err := boundedSaaSHTTPFuzzFinishAction.ActionVerifier(loop, nil); err == nil {
		t.Fatal("expected the SaaS finish action to reject zero-request completion")
	}

	markBoundedSaaSHTTPFuzzCompleted(loop)
	if err := validateBoundedSaaSHTTPFuzzCompleted(loop); err != nil {
		t.Fatalf("expected completion gate to open after successful evidence publication: %v", err)
	}
	if err := boundedSaaSHTTPFuzzFinishAction.ActionVerifier(loop, nil); err != nil {
		t.Fatalf("expected finish after a successful batch: %v", err)
	}
}

func TestBoundedSaaSRiskTargetUsesServerAuthorization(t *testing.T) {
	loop, _ := newBoundedHTTPFuzzTestLoop(t, "https://example.test/api/search?q=1")
	got, err := validateBoundedSaaSRiskTarget(loop, "")
	if err != nil || got != "https://example.test/api/search?q=1" {
		t.Fatalf("unexpected authorized risk target: %q err=%v", got, err)
	}
	if _, err := validateBoundedSaaSRiskTarget(loop, "https://other.test/api"); err == nil {
		t.Fatal("expected cross-target risk to be rejected")
	}
}

func TestPublishBoundedSaaSHTTPFuzzAsset(t *testing.T) {
	loop, sink := newBoundedHTTPFuzzTestLoop(t, "https://example.test/api/search?q=1")
	err := publishBoundedSaaSHTTPFuzzAsset(loop, "fuzz_get_params", &loopHTTPFuzzOverviewStats{
		TotalRequests:       3,
		SuccessfulResponses: 3,
	}, 200)
	if err != nil {
		t.Fatalf("publish asset: %v", err)
	}
	if len(sink.assets) != 1 {
		t.Fatalf("expected one asset, got %d", len(sink.assets))
	}
	asset := sink.assets[0]
	if asset.Target != sink.target || asset.Kind != "http_endpoint" || !strings.Contains(string(asset.Payload), `"request_count":3`) {
		t.Fatalf("unexpected asset: %#v payload=%s", asset, asset.Payload)
	}
	var payload boundedSaaSHTTPFuzzAssetPayload
	if err := json.Unmarshal(asset.Payload, &payload); err != nil {
		t.Fatalf("decode asset payload: %v", err)
	}
	if payload.Scheme != "https" || payload.Host != "example.test" || payload.Port != "443" ||
		payload.HTTPURL != sink.target || payload.Method != "GET" || payload.HTTPStatusCode != 200 {
		t.Fatalf("unexpected structured endpoint payload: %#v", payload)
	}
}

func TestValidateBoundedSaaSHTTPFuzzOutcomeRejectsFalseGreen(t *testing.T) {
	loop, _ := newBoundedHTTPFuzzTestLoop(t, "https://example.test/api/search?q=1")
	if err := validateBoundedSaaSHTTPFuzzOutcome(loop, &loopHTTPFuzzOverviewStats{}); err == nil {
		t.Fatal("expected an all-failed bounded batch to fail")
	}
	if err := validateBoundedSaaSHTTPFuzzOutcome(loop, &loopHTTPFuzzOverviewStats{SuccessfulResponses: 1}); err != nil {
		t.Fatalf("expected a successful bounded batch to pass: %v", err)
	}
}
