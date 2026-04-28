package yakit

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type AIEventRecoveryHistoryResult struct {
	BlockCount  int
	EventCount  int
	NextStartID int64
	HasMore     bool
}

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
	if event == nil {
		return nil
	}
	event.NormalizeRecoveryBlock()
	db = db.Model(event)

	// stream-finished is a structured event emitted by the AI emitter to mark the end of a stream.
	// It includes `event_writer_id` in JSON content; use it to force-flush and close the stream buffer
	// to reduce extra updates and memory retention.
	if event.NodeId == "stream-finished" {
		if streamWriterID := event.GetStreamEventWriterId(); streamWriterID != "" {
			globalStreamEventBuffer.finish(db, streamWriterID)
		}
	}

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
	// Stream events are extremely frequent (token-by-token). Coalesce updates in-memory and flush in batches
	// to reduce sqlite write-lock contention.
	return globalStreamEventBuffer.append(outDb, event)
}

func FilterEvent(db *gorm.DB, filter *ypb.AIEventFilter) *gorm.DB {
	db = db.Model(&schema.AiOutputEvent{})
	if filter == nil {
		return db
	}

	if !filter.GetUseOR() {
		if len(filter.GetEventUUIDS()) > 0 {
			db = db.Where("event_uuid IN (?)", filter.GetEventUUIDS())
		}
		db = bizhelper.ExactQueryStringArrayOr(db, "coordinator_id", filter.GetCoordinatorId())
		db = bizhelper.ExactQueryStringArrayOr(db, "type", filter.GetEventType())
		db = bizhelper.ExactQueryStringArrayOr(db, "task_index", filter.GetTaskIndex())
		db = bizhelper.ExactQueryStringArrayOr(db, "task_uuid", filter.GetTaskUUID())
		db = bizhelper.ExactQueryStringArrayOr(db, "node_id", filter.GetNodeId())
		db = bizhelper.ExactQueryString(db, "session_id", filter.GetSessionID())
		return db
	}

	var clauses []string
	var args []interface{}
	if len(filter.GetEventUUIDS()) > 0 {
		clauses = append(clauses, "(event_uuid IN (?))")
		args = append(args, filter.GetEventUUIDS())
	}
	if len(filter.GetCoordinatorId()) > 0 {
		clauses = append(clauses, "(coordinator_id IN (?))")
		args = append(args, filter.GetCoordinatorId())
	}
	if len(filter.GetEventType()) > 0 {
		clauses = append(clauses, "(`type` IN (?))")
		args = append(args, filter.GetEventType())
	}
	if len(filter.GetTaskIndex()) > 0 {
		clauses = append(clauses, "(task_index IN (?))")
		args = append(args, filter.GetTaskIndex())
	}
	if len(filter.GetTaskUUID()) > 0 {
		clauses = append(clauses, "(task_uuid IN (?))")
		args = append(args, filter.GetTaskUUID())
	}
	if len(filter.GetNodeId()) > 0 {
		clauses = append(clauses, "(node_id IN (?))")
		args = append(args, filter.GetNodeId())
	}
	if sessionID := filter.GetSessionID(); sessionID != "" {
		clauses = append(clauses, "(session_id = ?)")
		args = append(args, sessionID)
	}
	if len(clauses) == 0 {
		return db
	}
	db = db.Where(strings.Join(clauses, " OR "), args...)
	return db
}

func YieldAIEvent(ctx context.Context, db *gorm.DB, filter *ypb.AIEventFilter, opts ...bizhelper.YieldModelOpts) chan *schema.AiOutputEvent {
	db = FilterEvent(db, filter)

	return bizhelper.YieldModel[*schema.AiOutputEvent](ctx, db, opts...)
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

func filterRecoveryHistoryEventsBySession(db *gorm.DB, sessionID string) *gorm.DB {
	return FilterEvent(db, &ypb.AIEventFilter{SessionID: sessionID})
}

func YieldAIEventRecoveryHistory(ctx context.Context, db *gorm.DB, sessionID string, startID int64, limit int) (chan *schema.AiOutputEvent, *AIEventRecoveryHistoryResult, error) {
	if db == nil {
		return nil, nil, utils.Errorf("database is nil")
	}
	if sessionID == "" {
		return nil, nil, utils.Errorf("session_id is empty")
	}
	if limit <= 0 {
		return nil, nil, utils.Errorf("limit must be greater than 0")
	}

	outC := make(chan *schema.AiOutputEvent)
	result := &AIEventRecoveryHistoryResult{}

	go func() {
		defer close(outC)

		query := filterRecoveryHistoryEventsBySession(db, sessionID).Where("is_recovery_block = ?", true)
		if startID > 0 {
			query = query.Where("id < ?", startID)
		}

		for anchor := range bizhelper.YieldModel[*schema.AiOutputEvent](
			ctx,
			query.Order("id desc"),
			bizhelper.WithYieldModel_Limit(limit),
		) {
			if anchor == nil {
				continue
			}

			result.BlockCount++
			result.NextStartID = int64(anchor.ID)

			if anchor.RecoveryIndexID == "" {
				select {
				case <-ctx.Done():
					return
				case outC <- anchor:
					result.EventCount++
				}
				continue
			}

			blockQuery := filterRecoveryHistoryEventsBySession(db, sessionID).
				Where("recovery_index_id = ?", anchor.RecoveryIndexID).
				Order("id asc")

			for blockEvent := range bizhelper.YieldModel[*schema.AiOutputEvent](ctx, blockQuery) {
				if blockEvent == nil {
					continue
				}
				select {
				case <-ctx.Done():
					return
				case outC <- blockEvent:
					result.EventCount++
				}
			}
		}

		if result.NextStartID <= 0 {
			return
		}

		var count int64
		err := filterRecoveryHistoryEventsBySession(db, sessionID).
			Where("is_recovery_block = ?", true).
			Where("id < ?", result.NextStartID).
			Count(&count).Error
		if err != nil {
			log.Errorf("count recovery history anchors failed: %v", err)
			return
		}
		result.HasMore = count > 0
	}()

	return outC, result, nil
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

// DeleteAllAIEvent deletes all AI events from the database.
func DeleteAllAIEvent(db *gorm.DB) error {
	_, err := DeleteAllAIEventWithCount(db)
	return err
}

// DeleteAllAIEventWithCount deletes all AI events and returns deleted event count.
func DeleteAllAIEventWithCount(db *gorm.DB) (int64, error) {
	if db == nil {
		return 0, utils.Errorf("database is nil")
	}
	deletedEvents, err := countRowsIgnoreMissingTable(db, &schema.AiOutputEvent{})
	if err != nil {
		return 0, err
	}
	if err := schema.DropRecreateTable(db, &schema.AiProcessAndAiEvent{}); err != nil {
		log.Errorf("drop & recreate AI event associations failed: %v", err)
		return deletedEvents, err
	}
	if err = schema.DropRecreateTable(db, &schema.AiProcess{}); err != nil {
		log.Errorf("drop & recreate AI events failed: %v", err)
		return deletedEvents, err
	}
	if err := schema.DropRecreateTable(db, &schema.AiOutputEvent{}); err != nil {
		log.Errorf("drop & recreate AI events failed: %v", err)
		return deletedEvents, err
	}
	return deletedEvents, nil
}

// DeleteAIEventBySessionID deletes AI events under a session and their process associations.
func DeleteAIEventBySessionID(db *gorm.DB, sessionId string) (int64, error) {
	if sessionId == "" {
		return 0, utils.Errorf("sessionId is empty")
	}

	var deletedEvents int64
	err := utils.GormTransaction(db, func(tx *gorm.DB) error {
		// Delete associations in-DB to avoid loading huge UUID lists into memory.
		eventTable := tx.NewScope(&schema.AiOutputEvent{}).TableName()
		if r := tx.Model(&schema.AiProcessAndAiEvent{}).
			Where(fmt.Sprintf("event_id IN (SELECT event_uuid FROM %s WHERE session_id = ?)", eventTable), sessionId).
			Delete(&schema.AiProcessAndAiEvent{}); r.Error != nil {
			log.Errorf("delete AI event associations by session failed: %v", r.Error)
			return r.Error
		}

		r := tx.Model(&schema.AiOutputEvent{}).
			Where("session_id = ?", sessionId).
			Unscoped().
			Delete(&schema.AiOutputEvent{})
		if r.Error != nil {
			log.Errorf("delete AI events by session failed: %v", r.Error)
			return r.Error
		}
		deletedEvents = r.RowsAffected
		return nil
	})
	return deletedEvents, err
}

// YieldAllAIEvent yields all AI events from the database
func YieldAllAIEvent(db *gorm.DB, ctx context.Context) chan *schema.AiOutputEvent {
	db = db.Model(&schema.AiOutputEvent{})
	return bizhelper.YieldModel[*schema.AiOutputEvent](ctx, db)
}
