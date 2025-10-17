package schema

import "github.com/jinzhu/gorm"

const (
	AI_Call_Tool  = "call_tool"
	AI_Task_Index = "task_index"
)

type AiProcess struct {
	gorm.Model
	ProcessType string           `json:"process_type"`
	ProcessId   string           `json:"process_id" `
	Events      []*AiOutputEvent `gorm:"many2many:ai_processes_and_events;"`
}
