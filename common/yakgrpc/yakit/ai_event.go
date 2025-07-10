package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func CreateAIEvent(db *gorm.DB, event *schema.AiOutputEvent) error {
	db = db.Model(event)
	if event.IsStream {
		return SaveStreamAIEvent(db, event)
	}
	if db := db.Save(event); db.Error != nil {
		return db.Error
	}
	return nil
}

func SaveStreamAIEvent(outDb *gorm.DB, event *schema.AiOutputEvent) error {
	return utils.GormTransaction(outDb, func(tx *gorm.DB) error {
		var existingEvent schema.AiOutputEvent
		if innerDB := tx.Where("event_uuid = ?", event.EventUUID).FirstOrInit(&existingEvent); innerDB.Error != nil {
			return innerDB.Error
		}
		if existingEvent.EventUUID == "" {
			// If the event does not exist, create a new one
			event.ID = existingEvent.ID
			if tx = tx.Save(event); tx.Error != nil {
				return tx.Error
			}
		} else {
			existingEvent.StreamDelta = append(existingEvent.StreamDelta, event.StreamDelta...)
			if tx = tx.Save(&existingEvent); tx.Error != nil {
				return tx.Error
			}
		}
		return nil
	})
}
