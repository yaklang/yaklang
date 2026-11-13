package schema

import "github.com/yaklang/gorm"

// AISessionTaskInput stores the resources attached by one accepted AI input.
// The session ID is a logical lookup key only; this table deliberately has no
// foreign-key or association relationship with AISession.
type AISessionTaskInput struct {
	gorm.Model

	SessionID       string `json:"session_id" gorm:"column:session_id;index;not null"`
	TaskID          string `json:"task_id" gorm:"column:task_id;index;not null"`
	TaskUUID        string `json:"task_uuid" gorm:"column:task_uuid;unique_index;not null"`
	UserInputUUID   string `json:"user_input_uuid" gorm:"column:user_input_uuid;index"`
	UserInput       string `json:"user_input" gorm:"column:user_input;type:text"`
	InputSource     string `json:"input_source" gorm:"column:input_source;index"`
	FocusModeLoop   string `json:"focus_mode_loop" gorm:"column:focus_mode_loop"`
	ScheduleUUID    string `json:"schedule_uuid" gorm:"column:schedule_uuid"`
	ScheduleName    string `json:"schedule_name" gorm:"column:schedule_name"`
	ScheduledAt     string `json:"scheduled_at" gorm:"column:scheduled_at"`
	ScheduleTrigger string `json:"schedule_trigger" gorm:"column:schedule_trigger"`
	Resources       string `json:"resources" gorm:"column:resources;type:text"`
}

func (a *AISessionTaskInput) TableName() string {
	return "ai_session_task_inputs_v2"
}
