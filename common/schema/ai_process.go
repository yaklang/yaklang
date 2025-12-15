package schema

import "github.com/jinzhu/gorm"

const (
	AI_Call_Tool  = "call_tool"
	AI_Task_Index = "task_index"
)

type AiProcess struct {
	gorm.Model
	ProcessType string `json:"process_type" gorm:"index"`
	ProcessId   string `json:"process_id" gorm:"index"`
}

func (a *AiProcess) TableName() string {
	return "ai_processes_v1"
}

type AiProcessAndAiEvent struct {
	ProcessesId string `json:"processes_id" gorm:"index"`
	EventId     string `json:"event_id" gorm:"index"`
}

func (a *AiProcessAndAiEvent) TableName() string {
	return "ai_processes_and_ai_events_v1"
}
