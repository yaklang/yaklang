package schema

import (
	"time"

	"gorm.io/gorm"
)

// AISession stores basic metadata for an AI chat session.
type AISession struct {
	gorm.Model

	SessionID        string    `json:"session_id" gorm:"uniqueIndex;not null"`
	Title            string    `json:"title" gorm:"type:text"`
	TitleInitialized bool      `json:"title_initialized" gorm:"index;default:false"`
	StartParams      string    `json:"start_params" gorm:"column:start_params;type:text"`
	LastUsedAt       time.Time `json:"last_used_at" gorm:"column:last_used_at;index"`

	// Source identifies who started the session (e.g. ide, cli); indexed for filtering.
	Source string `json:"source" gorm:"index;type:varchar(128)"`

	// IMSource stores structured IM metadata JSON when Source == "im"
	// (platform / chatType / chatTitle / senderName / threadID), empty otherwise.
	// Populated best-effort by the IM engine via UpdateAISessionIMMeta; AutoMigrate
	// adds the column automatically. Not indexed (filtering stays on Source).
	IMSource string `json:"im_source" gorm:"column:im_source;type:text"`

	// RelatedRuntimeIDS stores a JSON-encoded string array of related runtime UUIDs.
	RelatedRuntimeIDS string `json:"related_runtime_ids" gorm:"column:related_runtime_ids;type:text"`
}

func (a *AISession) TableName() string {
	return "ai_sessions_v1"
}

// AISessionPlanAndExec stores PlanAndExec execution state for a session.
type AISessionPlanAndExec struct {
	gorm.Model

	SessionID     string `json:"session_id" gorm:"index;not null"`
	CoordinatorID string `json:"coordinator_id" gorm:"uniqueIndex;not null"`

	// TaskTree stores the serialized plan/execution tree (typically JSON).
	TaskTree string `json:"task_tree" gorm:"type:text"`

	// TaskProgress stores the serialized progress/state for pause & resume.
	TaskProgress string `json:"task_progress" gorm:"type:text"`
}

func (a *AISessionPlanAndExec) TableName() string {
	return "ai_session_plan_and_execs_v1"
}
