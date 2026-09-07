package schema

import (
	"time"

	"github.com/yaklang/gorm"
)

const (
	AIReActScheduleStatusActive    = "active"
	AIReActScheduleStatusPaused    = "paused"
	AIReActScheduleStatusCompleted = "completed"

	AIReActScheduleTargetContinueSession = "continue_session"
	AIReActScheduleTargetNewSession      = "new_session_per_run"
)

// AIReActSchedule stores a project-scoped recurring ReAct task definition.
// ReAct output remains in the existing AI session/event tables; this row owns
// only the durable schedule and its next-fire cursor.
type AIReActSchedule struct {
	gorm.Model

	UUID            string `json:"uuid" gorm:"column:uuid;unique_index;not null"`
	Name            string `json:"name" gorm:"type:text;not null"`
	Status          string `json:"status" gorm:"index;type:varchar(32);not null"`
	TargetMode      string `json:"target_mode" gorm:"type:varchar(32);not null"`
	TargetSessionID string `json:"target_session_id" gorm:"index;type:varchar(128)"`
	// CreatedFromSessionID records where the user created the schedule. It is
	// informational for isolated runs; only TargetSessionID owns lifecycle.
	CreatedFromSessionID string `json:"created_from_session_id" gorm:"index;type:varchar(128)"`

	// OriginalRequest is the user's scheduling utterance. Prompt is the durable,
	// model-normalized work instruction executed on every occurrence.
	OriginalRequest       string `json:"original_request" gorm:"type:text"`
	Prompt                string `json:"prompt" gorm:"type:text;not null"`
	StartParams           string `json:"start_params" gorm:"type:text"`
	AttachedResourceInfos string `json:"attached_resource_infos" gorm:"type:text"`
	FocusModeLoop         string `json:"focus_mode_loop" gorm:"type:varchar(256)"`

	RRule    string    `json:"rrule" gorm:"type:text;not null"`
	Timezone string    `json:"timezone" gorm:"type:varchar(128);not null"`
	StartAt  time.Time `json:"start_at" gorm:"index"`

	NextRunAt *time.Time `json:"next_run_at" gorm:"index"`
	LastRunAt *time.Time `json:"last_run_at" gorm:"index"`

	MisfireGraceSeconds int64      `json:"misfire_grace_seconds" gorm:"default:300"`
	MaxRuntimeSeconds   int64      `json:"max_runtime_seconds" gorm:"default:7200"`
	PauseReason         string     `json:"pause_reason" gorm:"type:text"`
	LastError           string     `json:"last_error" gorm:"type:text"`
	LastOutcome         string     `json:"last_outcome" gorm:"type:varchar(32)"`
	LastSkipReason      string     `json:"last_skip_reason" gorm:"type:varchar(128)"`
	LastStartedAt       *time.Time `json:"last_started_at" gorm:"index"`
	LastFinishedAt      *time.Time `json:"last_finished_at" gorm:"index"`
}

func (a *AIReActSchedule) TableName() string {
	return "ai_react_schedules_v1"
}
