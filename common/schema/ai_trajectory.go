package schema

import (
	"time"

	"github.com/yaklang/gorm"
)

const (
	AITrajectoryTerminal            = "terminal"
	AITrajectoryCompression         = "compression"
	AITrajectoryEvaluationPending   = "pending"
	AITrajectoryEvaluationRunning   = "running"
	AITrajectoryEvaluationCompleted = "completed"
	AITrajectoryEvaluationFailed    = "failed"
	AITrajectoryEvaluationSkipped   = "skipped"
)

// AITrajectory is a session's durable training-data segment. A persistent
// session has one terminal segment per task and one immutable segment per
// committed compression, independent of loop nesting and model configuration.
type AITrajectory struct {
	gorm.Model
	PersistentSessionID string    `gorm:"unique_index:idx_ai_trajectory_session_key;index;not null" json:"persistent_session_id"`
	TrajectoryKey       string    `gorm:"unique_index:idx_ai_trajectory_session_key;not null" json:"trajectory_key"`
	Kind                string    `gorm:"index;not null" json:"kind"`
	RuntimeID           string    `json:"runtime_id"`
	TaskID              string    `json:"task_id"`
	UserQuery           string    `gorm:"type:text" json:"user_query"`
	Status              string    `json:"status"`
	CapturedAt          time.Time `gorm:"index" json:"captured_at"`
	// Immutable decision-prompt/response strings can be collected without dumping
	// the live Timeline. Compression segments preserve the earlier context.
	Prompt   string `gorm:"type:text" json:"prompt"`
	Response string `gorm:"type:text" json:"response"`
	// CompressionJSON retains the request boundary, retained range and token counts.
	CompressionJSON  string     `gorm:"type:text" json:"compression_json,omitempty"`
	ContentHash      string     `json:"content_hash"`
	EvaluationStatus string     `gorm:"index" json:"evaluation_status"`
	ValueScore       *int       `json:"value_score,omitempty"`
	EvaluationReason string     `gorm:"type:text" json:"evaluation_reason"`
	EvaluationModel  string     `json:"evaluation_model"`
	EvaluationError  string     `gorm:"type:text" json:"evaluation_error"`
	EvaluatedAt      *time.Time `json:"evaluated_at,omitempty"`
}
