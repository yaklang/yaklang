package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
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
	// Use FirstOrCreate pattern without transaction to avoid "database is locked" errors
	var existingEvent schema.AiOutputEvent
	if err := outDb.Where("event_uuid = ?", event.EventUUID).First(&existingEvent).Error; err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return outDb.Create(event).Error
		}
		return err
	}

	// Event exists, append StreamDelta
	existingEvent.StreamDelta = append(existingEvent.StreamDelta, event.StreamDelta...)
	// Use Save to update the event, which handles []byte fields correctly
	// Without transaction, this minimizes lock time and reduces "database is locked" errors
	return outDb.Save(&existingEvent).Error
}

func FilterEvent(db *gorm.DB, filter *ypb.AIEventFilter) *gorm.DB {
	db = db.Model(&schema.AiOutputEvent{})
	db = bizhelper.ExactQueryStringArrayOr(db, "event_uuid", filter.GetEventUUIDS())
	db = bizhelper.ExactQueryStringArrayOr(db, "coordinator_id", filter.GetCoordinatorId())
	db = bizhelper.ExactQueryStringArrayOr(db, "type", filter.GetEventType())
	return db
}

func QueryAIEvent(db *gorm.DB, filter *ypb.AIEventFilter) ([]*schema.AiOutputEvent, error) {
	var event []*schema.AiOutputEvent
	db = FilterEvent(db, filter)
	if db = db.Find(&event); db.Error != nil {
		return nil, db.Error
	}
	return event, nil
}

func GetRandomAIMaterials(db *gorm.DB, limit int) ([]*schema.AIYakTool, []*schema.KnowledgeBaseEntry, []*schema.AIForge, error) {
	var tool []*schema.AIYakTool
	var kbEntries []*schema.KnowledgeBaseEntry
	var forges []*schema.AIForge

	_, err := bizhelper.RandomQuery(db.Model(&schema.AIYakTool{}), limit, &tool)
	if err != nil {
		return nil, nil, nil, err
	}

	_, err = bizhelper.RandomQuery(db.Model(&schema.KnowledgeBaseEntry{}), limit, &kbEntries)
	if err != nil {
		return nil, nil, nil, err
	}

	_, err = bizhelper.RandomQuery(db.Model(&schema.AIForge{}), limit, &forges)
	if err != nil {
		return nil, nil, nil, err
	}

	return tool, kbEntries, forges, nil
}
