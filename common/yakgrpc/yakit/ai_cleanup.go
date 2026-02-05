package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func DropAIAgentRuntimeTable(db *gorm.DB) error {
	if db == nil {
		return utils.Errorf("db is nil")
	}
	return schema.DropRecreateTable(db, &schema.AIAgentRuntime{})
}

func DropAIEventTables(db *gorm.DB) error {
	if db == nil {
		return utils.Errorf("db is nil")
	}
	if err := schema.DropRecreateTable(db, &schema.AiProcessAndAiEvent{}); err != nil {
		return err
	}
	return schema.DropRecreateTable(db, &schema.AiOutputEvent{})
}
