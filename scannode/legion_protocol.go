package scannode

import "strings"

const (
	legionCommandStream = "LEGION_COMMANDS"

	legionCommandDispatch        = "job.dispatch"
	legionCommandCancel          = "job.cancel"
	legionCommandCapabilityApply = "capability.apply"

	legionEventClaimed          = "job.claimed"
	legionEventStarted          = "job.started"
	legionEventProgress         = "job.progressed"
	legionEventAsset            = "job.asset"
	legionEventRisk             = "job.risk"
	legionEventReport           = "job.report"
	legionEventSucceeded        = "job.succeeded"
	legionEventFailed           = "job.failed"
	legionEventCancelled        = "job.cancelled"
	legionEventCapabilityStatus = "capability.status"
	legionEventCapabilityAlert  = "capability.alert"
	legionEventCapabilityFailed = "capability.failed"

	legionAssetKindTCPOpenPort        = "tcp_open_port"
	legionAssetKindServiceFingerprint = "service_fingerprint"

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

func trimSubject(value string) string {
	return strings.TrimSuffix(strings.TrimSpace(value), ".")
}
