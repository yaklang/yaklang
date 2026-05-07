package schema

import "github.com/jinzhu/gorm"

// AIReActThinkingChunk stores raw deltas of model "thinking" / reason streams from
// the main ReAct loop AI transaction (one row per flushed chunk).
// Logical merge scope is PersistentSessionId + LoopName when session id is set; TaskId/RuntimeId
// identify the producing task/runtime for audit. Rows are merged by created_at (then id).
type AIReActThinkingChunk struct {
	gorm.Model

	PersistentSessionId string `json:"persistent_session_id" gorm:"index:idx_ai_react_thinking_ps_loop"`
	TaskId              string `json:"task_id" gorm:"index:idx_ai_react_thinking_task"`
	RuntimeId           string `json:"runtime_id" gorm:"index:idx_ai_react_thinking_runtime"`
	LoopName            string `json:"loop_name" gorm:"index"`
	ByteLen             int    `json:"byte_len"`
	Content             string `json:"content" gorm:"type:text"`
}

func (AIReActThinkingChunk) TableName() string {
	return "ai_re_act_thinking_chunks"
}
