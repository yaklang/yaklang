package scannode

import "strings"

// JobFailureRetryPolicy tells the Legion scheduler how to react to a terminal
// JobFailed.error_code. Platform code should call ResolveJobFailureCode and
// branch on Policy; unknown node-reported codes always use Fallback.
type JobFailureRetryPolicy string

const (
	JobFailureRetryPolicyNone       JobFailureRetryPolicy = "none"
	JobFailureRetryPolicyImmediate  JobFailureRetryPolicy = "immediate"
	JobFailureRetryPolicyReschedule JobFailureRetryPolicy = "reschedule"
	JobFailureRetryPolicyTransient  JobFailureRetryPolicy = "transient"
	JobFailureRetryPolicyFallback   JobFailureRetryPolicy = "fallback"
)

const (
	// Node-reported JobFailed.error_code values emitted by scannode.
	JobFailureCodeNodeCapacityExceeded    = "node_capacity_exceeded"
	JobFailureCodeInvalidDispatchCommand  = "invalid_dispatch_command"
	JobFailureCodeScriptExecutionFailed   = "script_execution_failed"
	JobFailureCodeRuleSnapshotPrepareFailed = "rule_snapshot_prepare_failed"
	JobFailureCodeScriptExecutionPanic    = "script_execution_panic"
	JobFailureCodeStartedEventPublishFailed = "started_event_publish_failed"

	// JobFailureCodeUnknownNodeReported is the canonical bucket for unrecognized
	// node-reported codes when callers choose to normalize before persistence.
	JobFailureCodeUnknownNodeReported = "node_reported_failure_unknown"

	// Script-reported failure codes supplied by Legion-owned yak scripts via
	// ReturnData.error_code. They must stay registered here, otherwise
	// prepareJobFailureForPublish buckets them as unknown and Legion loses the
	// fine-grained failure_code its console catalog translates.
	JobFailureCodeGitCloneError          = "gitCloneError"
	JobFailureCodeNotFoundFileCanCompile = "notFoundFileCanCompile"
	JobFailureCodeSourceConfigInvalid    = "sourceConfigInvalid"
	JobFailureCodeSourceWorkspaceFailed  = "sourceWorkspaceFailed"
	JobFailureCodeSourceExtractFailed    = "sourceExtractFailed"
	JobFailureCodeScanFailed             = "scan_failed"

	// Platform-inferred codes: never emitted by scannode, documented here so
	// Legion and yaklang share one registry for retry decisions.
	JobFailureCodeAttemptMissingFromHeartbeat = "attempt_missing_from_heartbeat"
)

const (
	jobFailureDetailRetryPolicy = "failure_retry_policy"
	jobFailureDetailCodeKnown   = "failure_code_known"
	jobFailureDetailRawCode     = "raw_error_code"
)

type jobFailureCodeSpec struct {
	policy JobFailureRetryPolicy
}

// jobFailureCodeRegistry is the canonical map shared by scannode (emit) and
// Legion (validate / retry). Add new node-reported codes here before use.
//
// Script-reported codes carry policies for the error_detail metadata only;
// the Legion scheduler makes retry decisions from its dispatch contract plus
// its own non-retryable code set. gitCloneError is transient because remote
// Git hosts (e.g. github.com) regularly drop connections mid-clone; the other
// script codes describe deterministic input or environment problems.
var jobFailureCodeRegistry = map[string]jobFailureCodeSpec{
	JobFailureCodeNodeCapacityExceeded:      {policy: JobFailureRetryPolicyReschedule},
	JobFailureCodeInvalidDispatchCommand:    {policy: JobFailureRetryPolicyNone},
	JobFailureCodeScriptExecutionFailed:     {policy: JobFailureRetryPolicyNone},
	JobFailureCodeRuleSnapshotPrepareFailed: {policy: JobFailureRetryPolicyTransient},
	JobFailureCodeScriptExecutionPanic:      {policy: JobFailureRetryPolicyTransient},
	JobFailureCodeStartedEventPublishFailed: {policy: JobFailureRetryPolicyTransient},
	JobFailureCodeAttemptMissingFromHeartbeat: {policy: JobFailureRetryPolicyReschedule},

	JobFailureCodeGitCloneError:          {policy: JobFailureRetryPolicyTransient},
	JobFailureCodeNotFoundFileCanCompile: {policy: JobFailureRetryPolicyNone},
	JobFailureCodeSourceConfigInvalid:    {policy: JobFailureRetryPolicyNone},
	JobFailureCodeSourceWorkspaceFailed:  {policy: JobFailureRetryPolicyNone},
	JobFailureCodeSourceExtractFailed:    {policy: JobFailureRetryPolicyNone},
	JobFailureCodeScanFailed:             {policy: JobFailureRetryPolicyNone},
}

// JobFailureResolution is the output of ResolveJobFailureCode.
type JobFailureResolution struct {
	// CanonicalCode is the normalized registry code. For unknown inputs it equals
	// JobFailureCodeUnknownNodeReported while RawCode preserves the original.
	CanonicalCode string
	RawCode       string
	Policy        JobFailureRetryPolicy
	Known         bool
}

// ResolveJobFailureCode maps a JobFailed.error_code (node-reported or
// platform-inferred) to a retry policy. Unknown codes use Fallback so the
// platform never has to guess.
func ResolveJobFailureCode(code string) JobFailureResolution {
	raw := strings.TrimSpace(code)
	if raw == "" {
		return JobFailureResolution{
			CanonicalCode: JobFailureCodeUnknownNodeReported,
			RawCode:       raw,
			Policy:        JobFailureRetryPolicyFallback,
			Known:         false,
		}
	}
	if spec, ok := jobFailureCodeRegistry[raw]; ok {
		return JobFailureResolution{
			CanonicalCode: raw,
			RawCode:       raw,
			Policy:        spec.policy,
			Known:         true,
		}
	}
	return JobFailureResolution{
		CanonicalCode: JobFailureCodeUnknownNodeReported,
		RawCode:       raw,
		Policy:        JobFailureRetryPolicyFallback,
		Known:         false,
	}
}

// ShouldRetryJobFailure reports whether the scheduler should attempt another
// delivery for this failure resolution.
func ShouldRetryJobFailure(res JobFailureResolution) bool {
	switch res.Policy {
	case JobFailureRetryPolicyNone:
		return false
	default:
		return true
	}
}

// prepareJobFailureForPublish normalizes node failure events before they leave
// scannode. Known codes are published as-is with retry metadata; unknown codes
// are bucketed under JobFailureCodeUnknownNodeReported while raw_error_code keeps
// script/custom payloads for operators.
func prepareJobFailureForPublish(errorCode string, detail map[string]string) (string, map[string]string) {
	res := ResolveJobFailureCode(errorCode)
	if detail == nil {
		detail = make(map[string]string)
	} else {
		cloned := make(map[string]string, len(detail)+3)
		for key, value := range detail {
			cloned[key] = value
		}
		detail = cloned
	}
	detail[jobFailureDetailRetryPolicy] = string(res.Policy)
	if res.Known {
		detail[jobFailureDetailCodeKnown] = "true"
		return res.CanonicalCode, detail
	}
	detail[jobFailureDetailCodeKnown] = "false"
	if res.RawCode != "" && res.RawCode != JobFailureCodeUnknownNodeReported {
		detail[jobFailureDetailRawCode] = res.RawCode
	}
	return res.CanonicalCode, detail
}
