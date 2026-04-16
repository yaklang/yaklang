package schema

import "github.com/jinzhu/gorm"

// AISession stores basic metadata for an AI chat session.
type AISession struct {
	gorm.Model

	SessionID        string `json:"session_id" gorm:"unique_index;not null"`
	Title            string `json:"title" gorm:"type:text"`
	TitleInitialized bool   `json:"title_initialized" gorm:"index;default:false"`

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
	CoordinatorID string `json:"coordinator_id" gorm:"unique_index;not null"`

	// TaskTree stores the serialized plan/execution tree (typically JSON).
	TaskTree string `json:"task_tree" gorm:"type:text"`

	// TaskProgress stores the serialized progress/state for pause & resume.
	TaskProgress string `json:"task_progress" gorm:"type:text"`
}

func (a *AISessionPlanAndExec) TableName() string {
	return "ai_session_plan_and_execs_v1"
}
