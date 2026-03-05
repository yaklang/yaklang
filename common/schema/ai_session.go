package schema

import "github.com/jinzhu/gorm"

// AISession stores basic metadata for an AI chat session.
type AISession struct {
	gorm.Model

	SessionID        string `json:"session_id" gorm:"unique_index;not null"`
	Title            string `json:"title" gorm:"type:text"`
	TitleInitialized bool   `json:"title_initialized" gorm:"index;default:false"`
}

func (a *AISession) TableName() string {
	return "ai_sessions_v1"
}

