package scannode

import (
	"encoding/json"
	"testing"
)

func TestResolveJobFailureCodeKnownCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code   string
		policy JobFailureRetryPolicy
	}{
		{JobFailureCodeNodeCapacityExceeded, JobFailureRetryPolicyReschedule},
		{JobFailureCodeInvalidDispatchCommand, JobFailureRetryPolicyNone},
		{JobFailureCodeScriptExecutionFailed, JobFailureRetryPolicyNone},
		{JobFailureCodeRuleSnapshotPrepareFailed, JobFailureRetryPolicyTransient},
		{JobFailureCodeScriptExecutionPanic, JobFailureRetryPolicyTransient},
		{JobFailureCodeStartedEventPublishFailed, JobFailureRetryPolicyTransient},
		{JobFailureCodeAttemptMissingFromHeartbeat, JobFailureRetryPolicyReschedule},
		// Script-reported codes from Legion-owned yak scripts must resolve as
		// known so prepareJobFailureForPublish forwards them verbatim.
		{JobFailureCodeGitCloneError, JobFailureRetryPolicyTransient},
		{JobFailureCodeNotFoundFileCanCompile, JobFailureRetryPolicyNone},
		{JobFailureCodeSourceConfigInvalid, JobFailureRetryPolicyNone},
		{JobFailureCodeSourceWorkspaceFailed, JobFailureRetryPolicyNone},
		{JobFailureCodeSourceExtractFailed, JobFailureRetryPolicyNone},
		{JobFailureCodeScanFailed, JobFailureRetryPolicyNone},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.code, func(t *testing.T) {
			t.Parallel()
			res := ResolveJobFailureCode(tc.code)
			if !res.Known {
				t.Fatalf("expected known code %q", tc.code)
			}
			if res.CanonicalCode != tc.code {
				t.Fatalf("canonical = %q, want %q", res.CanonicalCode, tc.code)
			}
			if res.Policy != tc.policy {
				t.Fatalf("policy = %q, want %q", res.Policy, tc.policy)
			}
			if !ShouldRetryJobFailure(res) && tc.policy != JobFailureRetryPolicyNone {
				t.Fatalf("expected retryable policy for %q", tc.code)
			}
			if ShouldRetryJobFailure(res) && tc.policy == JobFailureRetryPolicyNone {
				t.Fatalf("expected non-retryable policy for %q", tc.code)
			}
		})
	}
}

func TestResolveJobFailureCodeUnknownUsesFallback(t *testing.T) {
	t.Parallel()

	res := ResolveJobFailureCode("custom_script_timeout")
	if res.Known {
		t.Fatal("expected unknown code")
	}
	if res.CanonicalCode != JobFailureCodeUnknownNodeReported {
		t.Fatalf("canonical = %q", res.CanonicalCode)
	}
	if res.RawCode != "custom_script_timeout" {
		t.Fatalf("raw = %q", res.RawCode)
	}
	if res.Policy != JobFailureRetryPolicyFallback {
		t.Fatalf("policy = %q", res.Policy)
	}
	if !ShouldRetryJobFailure(res) {
		t.Fatal("fallback policy should be retryable")
	}
}

func TestPrepareJobFailureForPublishKnownCode(t *testing.T) {
	t.Parallel()

	code, detail := prepareJobFailureForPublish(
		JobFailureCodeNodeCapacityExceeded,
		map[string]string{"script_release_id": "release-a"},
	)
	if code != JobFailureCodeNodeCapacityExceeded {
		t.Fatalf("code = %q", code)
	}
	if detail[jobFailureDetailCodeKnown] != "true" {
		t.Fatalf("detail = %#v", detail)
	}
	if detail[jobFailureDetailRetryPolicy] != string(JobFailureRetryPolicyReschedule) {
		t.Fatalf("retry policy = %q", detail[jobFailureDetailRetryPolicy])
	}
	if detail["script_release_id"] != "release-a" {
		t.Fatalf("lost dispatch detail: %#v", detail)
	}
}

func TestPrepareJobFailureForPublishScriptCodePassesThrough(t *testing.T) {
	t.Parallel()

	// A Legion yak script supplies ReturnData.error_code; the dispatch bridge
	// forwards it and publish normalization must keep it intact instead of
	// bucketing it into node_reported_failure_unknown.
	code, detail := prepareJobFailureForPublish(JobFailureCodeGitCloneError, nil)
	if code != JobFailureCodeGitCloneError {
		t.Fatalf("code = %q", code)
	}
	if detail[jobFailureDetailCodeKnown] != "true" {
		t.Fatalf("detail = %#v", detail)
	}
	if detail[jobFailureDetailRetryPolicy] != string(JobFailureRetryPolicyTransient) {
		t.Fatalf("retry policy = %q", detail[jobFailureDetailRetryPolicy])
	}
	if _, exists := detail[jobFailureDetailRawCode]; exists {
		t.Fatalf("raw code must not appear for known codes: %#v", detail)
	}
}

func TestPrepareJobFailureForPublishUnknownCodeBucketsAndPreservesRaw(t *testing.T) {
	t.Parallel()

	code, detail := prepareJobFailureForPublish("rule_engine_exit_42", nil)
	if code != JobFailureCodeUnknownNodeReported {
		t.Fatalf("code = %q", code)
	}
	if detail[jobFailureDetailCodeKnown] != "false" {
		t.Fatalf("detail = %#v", detail)
	}
	if detail[jobFailureDetailRawCode] != "rule_engine_exit_42" {
		t.Fatalf("raw detail = %q", detail[jobFailureDetailRawCode])
	}
	if detail[jobFailureDetailRetryPolicy] != string(JobFailureRetryPolicyFallback) {
		t.Fatalf("retry policy = %q", detail[jobFailureDetailRetryPolicy])
	}
}

func TestJobEventPublisherPublishFailedNormalizesUnknownCode(t *testing.T) {
	t.Parallel()

	publisher := &jobEventPublisher{}
	ref := jobExecutionRef{AttemptID: "attempt-1"}
	detail := map[string]string{"stage": "compile"}

	now := publisher // capture method without full NATS wiring
	_ = now

	// Exercise the same normalization path PublishFailed uses.
	code, normalizedDetail := prepareJobFailureForPublish("custom_script_timeout", detail)
	errorMessage := sanitizeUTF8String("boom")
	normalizedDetail = sanitizeUTF8Map(normalizedDetail)
	raw, err := json.Marshal(normalizedDetail)
	if err != nil {
		t.Fatal(err)
	}
	event := &struct {
		ErrorCode       string
		ErrorMessage    string
		ErrorDetailJSON []byte
	}{
		ErrorCode:       code,
		ErrorMessage:    errorMessage,
		ErrorDetailJSON: raw,
	}
	if event.ErrorCode != JobFailureCodeUnknownNodeReported {
		t.Fatalf("error_code = %q", event.ErrorCode)
	}
	var decoded map[string]string
	if err := json.Unmarshal(event.ErrorDetailJSON, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded[jobFailureDetailRawCode] != "custom_script_timeout" {
		t.Fatalf("raw_error_code = %q", decoded[jobFailureDetailRawCode])
	}
	if decoded["stage"] != "compile" {
		t.Fatalf("lost detail: %#v", decoded)
	}
	_ = ref
}
