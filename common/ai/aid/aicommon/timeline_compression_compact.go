package aicommon

import "encoding/json"

// Validate evidence upserts before treating them as compressible promoted
// items. Malformed evidence stays exact; the business replay ignores it.
func validCompressionEvidence(op *PromotableTimelineItem) bool {
	var item EvidenceItem
	return json.Unmarshal([]byte(op.Payload), &item) == nil && item.ID == op.Key
}
