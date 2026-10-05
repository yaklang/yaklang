package aimem

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/schema"
)

// The existing project KV table is an internal outbox. No receipt, retry state,
// or storage ID is added to Timeline candidates, compression inputs or prompts.
type timelineMemoryReceipt struct {
	Namespace string                 `json:"namespace"`
	Entity    *aicommon.MemoryEntity `json:"entity"`
	Saved     bool                   `json:"saved"`
	Score     bool                   `json:"score"`
	Questions map[string]bool        `json:"questions,omitempty"`
	Complete  bool                   `json:"complete"`
}

// Bounded, context-aware stripes serialize retries and task notifications even
// through different adapters of the same backend. Database unique keys remain
// authoritative; process locks are not durable claims.
var timelineMemoryLocks = func() [64]chan struct{} {
	var locks [64]chan struct{}
	for i := range locks {
		locks[i] = make(chan struct{}, 1)
	}
	return locks
}()

func timelineMemoryGroup(namespace string, complete bool) string {
	return fmt.Sprintf("aimemory-timeline-v1/%x/%v", sha256.Sum256([]byte(namespace)), complete)
}

func timelineMemoryID(namespace string, entity *aicommon.MemoryEntity) string {
	copy := *entity
	copy.Id, copy.CreatedAt, copy.ExpiresAt = "", time.Time{}, nil
	raw, _ := json.Marshal(copy)
	return fmt.Sprintf("timeline-%x", sha256.Sum256(append([]byte(namespace+"\x00"), raw...)))
}

// PersistTimelineMemories first journals every valid candidate, then resumes
// incomplete receipts. Nil candidates mean recovery only. Each stage is durable
// and independently retriable; one failed entry never acknowledges another.
func (s *MemoryStore) PersistTimelineMemories(ctx context.Context, candidates []any) error {
	if s.db == nil || s.sessionID == "" {
		return fmt.Errorf("timeline memory persistence requires database and namespace")
	}
	hash := sha256.Sum256([]byte(s.sessionID))
	lock := timelineMemoryLocks[int(hash[0])%len(timelineMemoryLocks)]
	select {
	case lock <- struct{}{}:
		defer func() { <-lock }()
	case <-ctx.Done():
		return ctx.Err()
	}
	var failures []error
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(failures, err)...)
		}
		entity, err := MemoryEntityFromCompression(candidate)
		if err == nil {
			entity.Id = timelineMemoryID(s.sessionID, entity)
			entity.ExpiresAt = CalcExpiresAt(entity.T_Score, entity.CreatedAt)
			err = s.enqueueTimelineMemory(entity)
		}
		if err != nil {
			failures = append(failures, err)
		}
	}
	// Keyset pagination processes each pending entry once per attempt, including
	// when earlier entries fail. It neither loops forever nor loads the whole DB.
	var after uint
	for {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(failures, err)...)
		}
		var rows []schema.ProjectGeneralStorage
		if err := s.db.Where("\"group\" = ? AND id > ?", timelineMemoryGroup(s.sessionID, false), after).
			Order("id").Limit(32).Find(&rows).Error; err != nil {
			return errors.Join(append(failures, err)...)
		}
		for _, row := range rows {
			after = row.ID
			if err := s.resumeTimelineMemory(ctx, &row); err != nil {
				failures = append(failures, fmt.Errorf("persist timeline memory receipt %d: %w", row.ID, err))
			}
		}
		if len(rows) < 32 {
			break
		}
	}
	return errors.Join(failures...)
}

func (s *MemoryStore) enqueueTimelineMemory(entity *aicommon.MemoryEntity) error {
	key := strconv.Quote("aimemory-timeline-v1/" + entity.Id)
	var existing schema.ProjectGeneralStorage
	err := s.db.Where("key = ?", key).First(&existing).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	receipt := timelineMemoryReceipt{Namespace: s.sessionID, Entity: entity}
	// Adopt an exact pre-outbox entry (the previous implementation used random
	// UUIDs). Keep its ID, creation time and TTL; repair indexes without a second
	// entity write. This is exact matching, never semantic memory deduplication.
	var after uint
	for {
		var old []schema.AIMemoryEntity
		if err := s.db.Unscoped().Where("session_id = ? AND content = ? AND memory_id NOT LIKE ? AND id > ?",
			s.sessionID, entity.Content, "timeline-%", after).Order("id").Limit(32).Find(&old).Error; err != nil {
			return err
		}
		for _, row := range old {
			after = row.ID
			legacy := memoryEntityFromRow(row)
			if timelineMemoryID(s.sessionID, legacy) == entity.Id {
				receipt.Entity, receipt.Saved = legacy, true
				break
			}
		}
		if receipt.Saved || len(old) < 32 {
			break
		}
	}
	value, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	row := schema.ProjectGeneralStorage{Key: key, Value: strconv.Quote(string(value)), Group: timelineMemoryGroup(s.sessionID, false)}
	if err := s.db.Create(&row).Error; err != nil {
		// Another process may have inserted the identical outbox key.
		if s.db.Where("key = ?", key).First(&existing).Error == nil {
			return nil
		}
		return err
	}
	return nil
}

func (s *MemoryStore) checkpointTimelineMemory(row *schema.ProjectGeneralStorage, receipt *timelineMemoryReceipt) error {
	value, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	quoted := strconv.Quote(string(value))
	result := s.db.Model(&schema.ProjectGeneralStorage{}).Where("id = ? AND value = ?", row.ID, row.Value).
		Updates(map[string]any{"value": quoted,
			"group": timelineMemoryGroup(s.sessionID, receipt.Complete)})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("timeline memory receipt changed concurrently; retry")
	}
	row.Value = quoted
	return nil
}

func (s *MemoryStore) resumeTimelineMemory(ctx context.Context, row *schema.ProjectGeneralStorage) error {
	value, err := strconv.Unquote(row.Value)
	if err != nil {
		return fmt.Errorf("decode timeline memory receipt: %w", err)
	}
	var receipt timelineMemoryReceipt
	if err := json.Unmarshal([]byte(value), &receipt); err != nil {
		return fmt.Errorf("decode timeline memory receipt: %w", err)
	}
	entity := receipt.Entity
	if receipt.Namespace != s.sessionID || entity == nil || entity.Id == "" ||
		row.Key != strconv.Quote("aimemory-timeline-v1/"+timelineMemoryID(s.sessionID, entity)) {
		return fmt.Errorf("invalid timeline memory receipt identity")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if entity.ExpiresAt != nil && !entity.ExpiresAt.After(time.Now()) {
		receipt.Complete = true
		return s.checkpointTimelineMemory(row, &receipt)
	}
	var stored schema.AIMemoryEntity
	err = s.db.Unscoped().Where("memory_id = ?", entity.Id).First(&stored).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if receipt.Saved {
			// Cleanup or explicit deletion must not be reversed by replay.
			receipt.Complete = true
			return s.checkpointTimelineMemory(row, &receipt)
		}
		stored = memoryEntityRow(s.sessionID, entity)
		stored.CreatedAt, stored.ExpiresAt = entity.CreatedAt, entity.ExpiresAt
		if err := s.db.Create(&stored).Error; err != nil {
			if s.db.Unscoped().Where("memory_id = ?", entity.Id).First(&stored).Error != nil {
				return err
			}
		}
	} else if err != nil {
		return err
	}
	if stored.SessionID != s.sessionID || stored.Content != entity.Content {
		return fmt.Errorf("timeline memory entity identity conflict")
	}
	if stored.DeletedAt != nil || (stored.ExpiresAt != nil && !stored.ExpiresAt.After(time.Now())) {
		receipt.Complete = true
		return s.checkpointTimelineMemory(row, &receipt)
	}
	if !receipt.Saved {
		receipt.Saved = true
		if err := s.checkpointTimelineMemory(row, &receipt); err != nil {
			return err
		}
		if s.emitter != nil {
			s.emitter.EmitJSON(schema.EVENT_TYPE_MEMORY_SAVE, "memory-save", map[string]any{
				"memory_session_id": s.sessionID, "memory": entity})
		}
	}
	if !receipt.Score {
		if s.hnswBackend == nil {
			return fmt.Errorf("timeline memory score index is unavailable")
		}
		if err := s.hnswBackend.Add(entity); err != nil {
			return err
		}
		if err := s.hnswBackend.SaveGraph(); err != nil {
			return err
		}
		receipt.Score = true
		if err := s.checkpointTimelineMemory(row, &receipt); err != nil {
			return err
		}
	}
	if receipt.Questions == nil {
		receipt.Questions = make(map[string]bool)
	}
	var failures []error
	for _, question := range entity.PotentialQuestions {
		if err := ctx.Err(); err != nil {
			return err
		}
		docID := stored.QuestionHashID(question)
		if receipt.Questions[docID] {
			continue
		}
		if s.rag == nil {
			failures = append(failures, fmt.Errorf("timeline memory question index is unavailable"))
			break
		}
		if err := s.indexTimelineQuestion(ctx, entity.Id, docID, question); err != nil {
			failures = append(failures, err)
			continue
		}
		receipt.Questions[docID] = true
		if err := s.checkpointTimelineMemory(row, &receipt); err != nil {
			return err
		}
	}
	if len(failures) > 0 {
		return errors.Join(failures...)
	}
	receipt.Complete = true
	if err := s.checkpointTimelineMemory(row, &receipt); err != nil {
		return err
	}
	if s.emitter != nil {
		s.emitter.EmitJSON(schema.EVENT_TYPE_STRUCTURED, "memory-index", map[string]any{
			"memory_session_id": s.sessionID, "memory_id": entity.Id, "entity_saved": true,
			"index_requested": len(entity.PotentialQuestions), "index_succeeded": len(entity.PotentialQuestions),
			"index_failed": 0, "index_skipped": 0})
	}
	MaybeCleanup(s.db)
	return nil
}

func (s *MemoryStore) indexTimelineQuestion(ctx context.Context, memoryID, docID, question string) error {
	// A crash/failed graph flush may leave the document vector committed. Reuse
	// that vector rather than calling the embedding provider for the same text.
	doc, found, err := s.rag.VectorStore.Get(docID)
	if err != nil {
		return err
	}
	if found {
		if doc.Content != question || doc.Metadata["memory_id"] != memoryID || doc.Metadata["session_id"] != s.sessionID {
			return fmt.Errorf("timeline memory question identity conflict")
		}
		return s.rag.VectorStore.RestoreDocumentGraph(ctx, docID)
	} else {
		err = s.rag.Add(docID, question, rag.WithDocumentMetadataKeyValue("memory_id", memoryID),
			rag.WithDocumentMetadataKeyValue("question", question), rag.WithDocumentMetadataKeyValue("session_id", s.sessionID))
	}
	if err != nil {
		return err
	}
	return s.rag.VectorStore.PersistGraph(ctx)
}

func memoryEntityFromRow(row schema.AIMemoryEntity) *aicommon.MemoryEntity {
	return &aicommon.MemoryEntity{Id: row.MemoryID, CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt,
		Content: row.Content, Tags: []string(row.Tags), PotentialQuestions: []string(row.PotentialQuestions),
		C_Score: row.C_Score, O_Score: row.O_Score, R_Score: row.R_Score, E_Score: row.E_Score,
		P_Score: row.P_Score, A_Score: row.A_Score, T_Score: row.T_Score, CorePactVector: []float32(row.CorePactVector)}
}
