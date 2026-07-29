package yakit

import (
	"github.com/yaklang/yaklang/common/schema"
)

const (
	ServerPushType_HTTPFlowCommitted            = "httpflow/committed"
	ServerPushType_HTTPFlowCommittedSubscribe   = "httpflow/committed/subscribe"
	ServerPushType_HTTPFlowCommittedUnsubscribe = "httpflow/committed/unsubscribe"
)

// HTTPFlowCommittedEvent is the Phase 2 shadow contract. It intentionally
// carries no request or response packet; QueryHTTPFlows remains the data and
// recovery path while the frontend measures delivery and reconciliation.
type HTTPFlowCommittedEvent struct {
	Version           uint32 `json:"version"`
	ID                uint64 `json:"id"`
	Sequence          uint64 `json:"sequence,omitempty"`
	ProjectGeneration uint64 `json:"project_generation"`
	DatabaseIdentity  string `json:"database_identity"`
	CommittedAtUnixMs int64  `json:"committed_at_unix_ms"`
	HighWaterID       uint64 `json:"high_water_id"`
}

func BroadcastHTTPFlowCommitted(flow *schema.HTTPFlow) {
	if flow == nil || flow.ID == 0 || flow.SourceType != schema.HTTPFlow_SourceType_MITM || flow.RuntimeTiming == nil {
		return
	}
	record, ok := PublishHTTPFlowLiveCommitted(flow)
	if !ok {
		return
	}
	broadcastDataToSubscribersLazy(
		ServerPushType_HTTPFlowCommitted,
		ServerPushType_HTTPFlowCommitted,
		func() any {
			return &HTTPFlowCommittedEvent{
				Version:           1,
				ID:                uint64(record.Flow.ID),
				Sequence:          record.Sequence,
				ProjectGeneration: record.ProjectGeneration,
				DatabaseIdentity:  record.DatabaseIdentity,
				CommittedAtUnixMs: record.CommittedAtUnixMs,
				HighWaterID:       record.HighWaterID,
			}
		},
	)
}
