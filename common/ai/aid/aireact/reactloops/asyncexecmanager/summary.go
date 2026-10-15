package asyncexecmanager

import "time"

type Summary struct {
	ID           string     `json:"execute_id"`
	Metadata     Metadata   `json:"metadata"`
	State        State      `json:"state"`
	CreatedAt    time.Time  `json:"created_at"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	EndedAt      *time.Time `json:"ended_at,omitempty"`
	Error        string     `json:"error,omitempty"`
	CancelReason string     `json:"cancel_reason,omitempty"`
	Revision     uint64     `json:"revision"`
}

func cloneSummary(summary Summary) Summary {
	summary.Metadata = cloneMetadata(summary.Metadata)
	if summary.StartedAt != nil {
		started := *summary.StartedAt
		summary.StartedAt = &started
	}
	if summary.EndedAt != nil {
		ended := *summary.EndedAt
		summary.EndedAt = &ended
	}
	return summary
}
