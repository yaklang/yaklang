package scannode

import (
	"encoding/json"
	"strings"
)

const (
	capabilityStatusPendingApply   = "pending_apply"
	capabilityStatusFailed         = "failed"
	capabilityNormalizedDetailKey  = "_platform_status"
	capabilityUnknownReportedValue = "unknown"
)

func normalizeCapabilityEventStatus(status string, detail []byte) (string, []byte) {
	reported := strings.ToLower(strings.TrimSpace(status))
	switch reported {
	case capabilityStatusPendingApply, capabilityStatusStored, capabilityStatusRunning, capabilityStatusFailed:
		return reported, cloneBytes(detail)
	case "stopped":
		return capabilityStatusStored, annotateNormalizedCapabilityStatus(detail, reported, capabilityStatusStored)
	case "degraded":
		return capabilityStatusRunning, annotateNormalizedCapabilityStatus(detail, reported, capabilityStatusRunning)
	case "":
		return capabilityStatusStored, annotateNormalizedCapabilityStatus(detail, capabilityUnknownReportedValue, capabilityStatusStored)
	default:
		return capabilityStatusStored, annotateNormalizedCapabilityStatus(detail, reported, capabilityStatusStored)
	}
}

func annotateNormalizedCapabilityStatus(detail []byte, reported string, normalized string) []byte {
	if strings.TrimSpace(reported) == "" || reported == normalized {
		return cloneBytes(detail)
	}

	document := map[string]any{}
	if len(detail) > 0 {
		if err := json.Unmarshal(detail, &document); err != nil {
			document["raw_detail_json"] = string(detail)
		}
	}
	document[capabilityNormalizedDetailKey] = map[string]any{
		"reported":   reported,
		"normalized": normalized,
	}

	raw, err := json.Marshal(document)
	if err != nil {
		return cloneBytes(detail)
	}
	return raw
}

func normalizeCapabilityApplyResult(result CapabilityApplyResult) CapabilityApplyResult {
	normalizedStatus, normalizedDetail := normalizeCapabilityEventStatus(
		result.Status,
		result.StatusDetailJSON,
	)
	result.Status = normalizedStatus
	result.StatusDetailJSON = normalizedDetail
	return result
}
