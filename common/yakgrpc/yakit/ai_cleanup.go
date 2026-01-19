package yakit

import (
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func isSQLite(db *gorm.DB) bool {
	if db == nil || db.Dialect() == nil {
		return false
	}
	return strings.Contains(strings.ToLower(db.Dialect().GetName()), "sqlite")
}

func DropAIAgentRuntimeTable(db *gorm.DB) error {
	if db == nil {
		return utils.Errorf("db is nil")
	}
	db.DropTableIfExists(&schema.AIAgentRuntime{})
	if isSQLite(db) {
		db.Exec(`DELETE FROM sqlite_sequence WHERE name='ai_agent_runtimes';`)
	}
	return db.AutoMigrate(&schema.AIAgentRuntime{}).Error
}

func DropAIEventTables(db *gorm.DB) error {
	if db == nil {
		return utils.Errorf("db is nil")
	}
	db.DropTableIfExists(&schema.AiProcessAndAiEvent{})
	db.DropTableIfExists(&schema.AiOutputEvent{})
	if isSQLite(db) {
		db.Exec(`DELETE FROM sqlite_sequence WHERE name='ai_output_events';`)
	}
	return db.AutoMigrate(&schema.AiOutputEvent{}, &schema.AiProcessAndAiEvent{}).Error
}
