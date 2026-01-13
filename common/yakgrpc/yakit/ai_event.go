package yakit

import (
	"context"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yak/cartesian"
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
	db = bizhelper.ExactQueryStringArrayOr(db, "node_id", filter.GetNodeId())
	db = bizhelper.ExactQueryString(db, "session_id", filter.GetSessionID())
	return db
}

func YieldAIEvent(ctx context.Context, db *gorm.DB, filter *ypb.AIEventFilter) chan *schema.AiOutputEvent {
	outCh := make(chan *schema.AiOutputEvent)

	chunkSlice := func(slice []string, chunkSize int) [][]string {
		var chunks [][]string
		for i := 0; i < len(slice); i += chunkSize {
			end := i + chunkSize
			if end > len(slice) {
				end = len(slice)
			}
			chunks = append(chunks, slice[i:end])
		}
		return chunks
	}

	go func() {
		defer close(outCh)

		type filterChunk struct {
			Field  string
			Chunks [][]string
		}
		var allFilterChunks []filterChunk

		if len(filter.GetEventUUIDS()) > 0 {
			allFilterChunks = append(allFilterChunks, filterChunk{"event_uuid", chunkSlice(filter.GetEventUUIDS(), 10)})
		}
		if len(filter.GetCoordinatorId()) > 0 {
			allFilterChunks = append(allFilterChunks, filterChunk{"coordinator_id", chunkSlice(filter.GetCoordinatorId(), 10)})
		}
		if len(filter.GetEventType()) > 0 {
			allFilterChunks = append(allFilterChunks, filterChunk{"type", chunkSlice(filter.GetEventType(), 10)})
		}
		if len(filter.GetTaskIndex()) > 0 {
			allFilterChunks = append(allFilterChunks, filterChunk{"task_index", chunkSlice(filter.GetTaskIndex(), 10)})
		}
		if len(filter.GetTaskUUID()) > 0 {
			allFilterChunks = append(allFilterChunks, filterChunk{"task_uuid", chunkSlice(filter.GetTaskUUID(), 10)})
		}
		if len(filter.GetNodeId()) > 0 {
			allFilterChunks = append(allFilterChunks, filterChunk{"node_id", chunkSlice(filter.GetNodeId(), 10)})
		}

		var sets [][][]string
		for _, fc := range allFilterChunks {
			sets = append(sets, fc.Chunks)
		}

		baseDB := db.Model(&schema.AiOutputEvent{})
		baseDB = bizhelper.ExactQueryString(baseDB, "session_id", filter.GetSessionID())

		handler := func(combination [][]string) error {
			currentDB := baseDB
			for i, chunkValues := range combination {
				field := allFilterChunks[i].Field
				currentDB = bizhelper.ExactQueryStringArrayOr(currentDB, field, chunkValues)
			}

			// Execute query for this combination
			ch := bizhelper.YieldModel[*schema.AiOutputEvent](ctx, currentDB)
			for item := range ch {
				select {
				case outCh <- item:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		}
		var err error
		if len(sets) == 0 {
			err = handler(nil)
		} else {
			err = cartesian.ProductExContext(ctx, sets, handler)
		}
		if err != nil {
			log.Errorf("yield AI event failed: %v", err)
			return
		}
	}()

	return outCh
}

func QueryAIEventPaging(db *gorm.DB, filter *ypb.AIEventFilter, paging *ypb.Paging) (*bizhelper.Paginator, []*schema.AiOutputEvent, error) {
	db = FilterEvent(db, filter)
	db = bizhelper.OrderByPaging(db, paging)

	var events []*schema.AiOutputEvent
	paginator, db := bizhelper.YakitPagingQuery(db, paging, &events)
	if db.Error != nil {
		return nil, nil, db.Error
	}
	return paginator, events, nil
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

// DeleteAllAIEvent deletes all AI events from the database
func DeleteAllAIEvent(db *gorm.DB) error {
	return utils.GormTransaction(db, func(tx *gorm.DB) error {
		// First, delete all associations
		if err := tx.Model(&schema.AiProcessAndAiEvent{}).Delete(&schema.AiProcessAndAiEvent{}).Error; err != nil {
			log.Errorf("delete AI event associations failed: %v", err)
			return err
		}
		// Then, delete all events
		if err := tx.Model(&schema.AiOutputEvent{}).Delete(&schema.AiOutputEvent{}).Error; err != nil {
			log.Errorf("delete all AI events failed: %v", err)
			return err
		}
		return nil
	})
}

// YieldAllAIEvent yields all AI events from the database
func YieldAllAIEvent(db *gorm.DB, ctx context.Context) chan *schema.AiOutputEvent {
	db = db.Model(&schema.AiOutputEvent{})
	return bizhelper.YieldModel[*schema.AiOutputEvent](ctx, db)
}
