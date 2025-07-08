package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
)

func CreateAIEvent(db *gorm.DB, event *schema.AiOutputEvent) error {
	db = db.Model(event)
	if db := db.Save(event); db.Error != nil {
		return db.Error
	}
	return nil
}
