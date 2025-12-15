package yakit

import (
	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func AssociateAIEventToProcess(db *gorm.DB, eventId string, processIds []string) error {
	return utils.GormTransactionReturnDb(db, func(tx *gorm.DB) {
		for _, processId := range processIds {
			assoc := &schema.AiProcessAndAiEvent{
				ProcessesId: processId,
				EventId:     eventId,
			}
			if innerDb := tx.Model(assoc).Create(assoc); innerDb.Error != nil {
				log.Errorf("create AI event to AI Process failed: %v", innerDb.Error)
				return
			}
		}
	}).Error
}

func CreateOrUpdateAIOutputEvent(db *gorm.DB, event *schema.AiOutputEvent) error {
	db = db.Model(event)
	if event.EventUUID == "" {
		event.EventUUID = uuid.NewString()
	}
	if event.IsStream {
		return SaveStreamAIEvent(db, event)
	}
	return saveAIEvent(db, event)
}

func saveAIEvent(db *gorm.DB, event *schema.AiOutputEvent) error {
	db = db.Model(event)
	if db := db.Save(event); db.Error != nil {
		return db.Error
	}
	return AssociateAIEventToProcess(db, event.EventUUID, event.ProcessesId)
}

func SaveStreamAIEvent(outDb *gorm.DB, event *schema.AiOutputEvent) error {
	// Use FirstOrCreate pattern without transaction to avoid "database is locked" errors
	var existingEvent schema.AiOutputEvent
	if err := outDb.Where("event_uuid = ?", event.EventUUID).First(&existingEvent).Error; err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return saveAIEvent(outDb, event)
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
	db = bizhelper.ExactQueryStringArrayOr(db, "task_index", filter.GetTaskIndex())
	db = bizhelper.ExactQueryStringArrayOr(db, "task_uuid", filter.GetTaskUUID())
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

func QueryAIEventIDByProcessID(db *gorm.DB, processID string) ([]string, error) {
	var assoc []schema.AiProcessAndAiEvent
	if db = db.Model(&schema.AiProcessAndAiEvent{}).Where("processes_id = ?", processID).Find(&assoc); db.Error != nil {
		return nil, db.Error
	}
	var eventIDs []string
	for _, a := range assoc {
		eventIDs = append(eventIDs, a.EventId)
	}
	return eventIDs, nil
}
