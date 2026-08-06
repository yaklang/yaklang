package aicommon

import (
	"errors"
	"testing"

	"github.com/yaklang/yaklang/common/schema"
)

type recordingToolRiskRuntime struct {
	target     string
	capability string
	params     map[string]any
	err        error
}

func (r *recordingToolRiskRuntime) AuthorizedTarget() string { return r.target }

func (r *recordingToolRiskRuntime) Execute(capability string, params map[string]any) (map[string]any, error) {
	r.capability = capability
	r.params = params
	return map[string]any{"result_id": "risk-1"}, r.err
}

func TestSubmitToolRiskToPlatformSinkMapsStructuredRisk(t *testing.T) {
	runtime := &recordingToolRiskRuntime{target: "https://example.test/audit"}
	err := submitToolRiskToPlatformSink(runtime, &schema.Risk{
		Title:          "Reflected XSS",
		RiskType:       "xss",
		Severity:       "high",
		Parameter:      "q",
		Payload:        "<script>alert(1)</script>",
		Description:    "q is reflected without encoding",
		Solution:       "apply contextual output encoding",
		Details:        `{"summary":"confirmed reflection"}`,
		QuotedRequest:  "GET /audit?q=payload HTTP/1.1",
		QuotedResponse: "HTTP/1.1 200 OK",
	})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.capability != "result.risk" {
		t.Fatalf("unexpected capability: %q", runtime.capability)
	}
	if runtime.params["target"] != runtime.target || runtime.params["verified"] != true {
		t.Fatalf("unexpected result scope: %#v", runtime.params)
	}
	if runtime.params["description"] != "q is reflected without encoding" || runtime.params["solution"] != "apply contextual output encoding" {
		t.Fatalf("missing structured fields: %#v", runtime.params)
	}
}

func TestSubmitToolRiskToPlatformSinkPropagatesFailure(t *testing.T) {
	runtime := &recordingToolRiskRuntime{
		target: "https://example.test/audit",
		err:    errors.New("publish failed"),
	}
	err := submitToolRiskToPlatformSink(runtime, &schema.Risk{Title: "XSS", RiskType: "xss"})
	if err == nil || !errors.Is(err, runtime.err) {
		t.Fatalf("expected wrapped publish failure, got %v", err)
	}
}
