package scannode

import "strings"

const (
	legionCommandStream = "LEGION_COMMANDS"
	legionHIDSPrefix    = "legion.hids"

	legionCommandDispatch          = "job.dispatch"
	legionCommandCancel            = "job.cancel"
	legionCommandCapabilityApply   = "capability.apply"
	legionCommandHIDSResponseActionExecute = "hids.response_action.execute"
	legionCommandSSARuleSyncExport = "ssa.rule_sync.export"

	legionEventClaimed           = "job.claimed"
	legionEventStarted           = "job.started"
	legionEventProgress          = "job.progressed"
	legionEventAsset             = "job.asset"
	legionEventRisk              = "job.risk"
	legionEventReport            = "job.report"
	legionEventArtifactReady     = "job.artifact_ready"
	legionEventSucceeded         = "job.succeeded"
	legionEventFailed            = "job.failed"
	legionEventCancelled         = "job.cancelled"
	legionEventCapabilityStatus  = "capability.status"
	legionEventCapabilityAlert   = "capability.alert"
	legionEventCapabilityFailed  = "capability.failed"
	legionEventHIDSObservation   = "hids.observation"
	legionEventHIDSResponseActionResult = "hids.response_action.result"
	legionEventSSARuleSyncReady  = "ssa.rule_sync.ready"
	legionEventSSARuleSyncFailed = "ssa.rule_sync.failed"

	legionAssetKindTCPOpenPort        = "tcp_open_port"
	legionAssetKindServiceFingerprint = "service_fingerprint"
	legionArtifactKindSSAResultV1     = "ssa_result_v1"

	legionRiskKindVulnerability = "vulnerability"
	legionRiskKindWeakPassword  = "weak_password"
	legionRiskKindSecurityRisk  = "security_risk"

	legionReportKindScan = "scan_report"
)

func commandSubjectWildcard(base string) string {
	return trimSubject(base) + ".>"
}

func jobEventSubject(prefix, eventType string) string {
	return trimSubject(prefix) + "." + strings.TrimPrefix(eventType, ".")
}

func capabilityEventSubject(prefix, eventType string) string {
	normalizedEventType := strings.TrimPrefix(strings.TrimSpace(eventType), ".")
	if normalizedEventType == legionEventHIDSObservation {
		return legionHIDSPrefix + ".observation"
	}
	if normalizedEventType == legionEventHIDSResponseActionResult {
		return legionHIDSPrefix + ".response_action.result"
	}
	return jobEventSubject(prefix, normalizedEventType)
}

func trimSubject(value string) string {
	return strings.TrimSuffix(strings.TrimSpace(value), ".")
}
