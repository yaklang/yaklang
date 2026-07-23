package scannode

import (
	"encoding/json"
	"testing"

	"github.com/yaklang/yaklang/common/spec"
)

func TestBuildScriptBaseParams(t *testing.T) {
	t.Parallel()

	params := buildScriptBaseParams("http://127.0.0.1:8080/webhook", "runtime-1")
	if len(params) != 4 {
		t.Fatalf("unexpected params length: %d", len(params))
	}
	if params[0] != "--yakit-webhook" || params[1] != "http://127.0.0.1:8080/webhook" {
		t.Fatalf("unexpected webhook params: %#v", params)
	}
	if params[2] != "--runtime_id" || params[3] != "runtime-1" {
		t.Fatalf("unexpected runtime params: %#v", params)
	}
}

func TestBuildScriptBaseParamsWithoutRuntimeID(t *testing.T) {
	t.Parallel()

	params := buildScriptBaseParams("http://127.0.0.1:8080/webhook", "")
	if len(params) != 2 {
		t.Fatalf("unexpected params length: %d", len(params))
	}
	if params[0] != "--yakit-webhook" || params[1] != "http://127.0.0.1:8080/webhook" {
		t.Fatalf("unexpected webhook params: %#v", params)
	}
}

func TestClassifyUploadError(t *testing.T) {
	tests := []struct {
		input    string
		wantCode string
	}{
		{"context deadline exceeded during artifact-ticket fetch", "ticket_timeout"},
		{"artifact-ticket request timeout", "ticket_timeout"},
		{"expiredtoken: sts token expired", "sts_expired"},
		{"accessdenied: insufficient permission", "sts_expired"},
		{"PutObject failed: connection reset", "put_failed"},
		{"CompleteMultipartUpload: internal error", "multipart_failed"},
		{"NewMultipartUpload: multipart init failed", "multipart_failed"},
		{"unknown network error", "put_failed"},
	}
	for _, tt := range tests {
		got := classifyUploadError(tt.input)
		if got != tt.wantCode {
			t.Errorf("classifyUploadError(%q) = %q, want %q", tt.input, got, tt.wantCode)
		}
	}
}

func TestUploadFailedEventSerialization(t *testing.T) {
	event := &spec.SSAArtifactUploadFailedEvent{
		ErrorCode:     "sts_expired",
		ErrorMessage:  "sts token expired",
		UploadedBytes: 12000,
	}
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var decoded spec.SSAArtifactUploadFailedEvent
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if decoded.ErrorCode != "sts_expired" {
		t.Errorf("ErrorCode = %q, want sts_expired", decoded.ErrorCode)
	}
	if decoded.ErrorMessage != "sts token expired" {
		t.Errorf("ErrorMessage = %q, want sts token expired", decoded.ErrorMessage)
	}
	if decoded.UploadedBytes != 12000 {
		t.Errorf("UploadedBytes = %d, want 12000", decoded.UploadedBytes)
	}
}
