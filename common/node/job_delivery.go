package node

// PendingJobDelivery describes durable completion delivery, never resumable work.
type PendingJobDelivery struct {
	AttemptID       string `json:"attempt_id"`
	JobID           string `json:"job_id"`
	SubtaskID       string `json:"subtask_id"`
	CommandID       string `json:"command_id"`
	NodeSessionID   string `json:"node_session_id"`
	TerminalEventID string `json:"terminal_event_id"`
}

type JobDeliveryReceipt struct {
	AttemptID       string `json:"attempt_id"`
	TerminalEventID string `json:"terminal_event_id"`
	Disposition     string `json:"disposition"`
}

type JobDeliveryProvider interface {
	PendingJobDeliveries() []PendingJobDelivery
	AcceptJobDeliveryRecovery(SessionState) error
	ApplyJobDeliveryReceipts([]JobDeliveryReceipt) error
}
