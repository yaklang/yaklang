package aimem

import (
	"context"
	"strings"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// DeleteMemoryVectorArtifacts removes HNSW graph nodes and RAG documents for the given entities.
func DeleteMemoryVectorArtifacts(ctx context.Context, db *gorm.DB, entities []schema.AIMemoryEntity) error {
	if len(entities) == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if db == nil {
		return nil
	}

	type sessionPayload struct {
		memoryIDs []string
		docIDs    []string
	}
	bySession := make(map[string]*sessionPayload, 8)
	for i := range entities {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		sessionID := strings.TrimSpace(entities[i].SessionID)
		if sessionID == "" {
			continue
		}
		payload, ok := bySession[sessionID]
		if !ok {
			payload = &sessionPayload{}
			bySession[sessionID] = payload
		}
		if entities[i].MemoryID != "" {
			payload.memoryIDs = append(payload.memoryIDs, entities[i].MemoryID)
		}
		if ids := entities[i].DocumentQuestionHashIDs(); len(ids) > 0 {
			payload.docIDs = append(payload.docIDs, ids...)
		}
	}

	hnswBackends := make(map[string]*AIMemoryHNSWBackend, len(bySession))
	ragStores := make(map[string]*vectorstore.SQLiteVectorStoreHNSW, len(bySession))
	ragExists := make(map[string]bool, len(bySession))

	for sessionID, payload := range bySession {
		payload.memoryIDs = uniqueNonEmptyStrings(payload.memoryIDs)
		payload.docIDs = uniqueNonEmptyStrings(payload.docIDs)

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Preserve entity rows if either artifact store fails, so a later cleanup
		// can retry with the same document IDs instead of losing that mapping.
		if len(payload.docIDs) > 0 {
			store, ok, err := getOrCreateRAGStore(ragStores, ragExists, db, sessionID)
			if err != nil {
				return err
			}
			if ok {
				if err := store.Delete(payload.docIDs...); err != nil {
					return err
				}
			}
		}
		backend, err := getOrCreateHNSWBackend(hnswBackends, db, sessionID)
		if err != nil {
			return err
		}
		if err := backend.deleteAndSave(ctx, payload.memoryIDs, false); err != nil {
			return err
		}
	}
	return nil
}

func uniqueNonEmptyStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func getOrCreateHNSWBackend(cache map[string]*AIMemoryHNSWBackend, db *gorm.DB, sessionID string) (*AIMemoryHNSWBackend, error) {
	if backend := cache[sessionID]; backend != nil {
		return backend, nil
	}
	// Midterm archive sessions use independent HNSW collection tables.
	midtermMode := strings.HasPrefix(sessionID, MidtermSessionPrefix)
	backend, err := NewAIMemoryHNSWBackend(
		WithHNSWSessionID(sessionID),
		WithHNSWDatabase(db),
		WithHNSWAutoSave(false),
		WithHNSWMidtermMode(midtermMode),
	)
	if err != nil {
		return nil, err
	}
	cache[sessionID] = backend
	return backend, nil
}

func getOrCreateRAGStore(
	cache map[string]*vectorstore.SQLiteVectorStoreHNSW,
	exists map[string]bool,
	db *gorm.DB,
	sessionID string,
) (*vectorstore.SQLiteVectorStoreHNSW, bool, error) {
	collectionName := Session2MemoryName(sessionID)
	if store := cache[collectionName]; store != nil {
		return store, true, nil
	}
	if ok, known := exists[collectionName]; known && !ok {
		return nil, false, nil
	}
	if !vectorstore.HasCollection(db, collectionName) {
		exists[collectionName] = false
		return nil, false, nil
	}
	store, err := vectorstore.LoadCollection(db, collectionName,
		vectorstore.WithEmbeddingClient(rag.NewEmptyMockEmbedding()),
		// Maintenance must not rebuild a large corrupt RAG or erase its surviving
		// documents as a side effect of deleting a small memory batch.
		vectorstore.WithTryRebuildHNSWIndex(false),
		vectorstore.WithAutoDeleteCorruptedRAG(false))
	if err != nil {
		return nil, false, err
	}
	cache[collectionName] = store
	exists[collectionName] = true
	return store, true, nil
}

// BatchCleanupMemories 批量物理删除指定 session 内的记忆及其所有索引数据。
//
// 针对单 session 攒批场景优化：HNSW 只加载一次 → 批量 Delete → 只 SaveGraph 一次；
// RAG 批量收集 docIDs 一次性删除；DB 一条 DELETE WHERE memory_id IN (...)。
//
// 适用于 TTL 过期清理和低价值淘汰等自动清理场景。
func BatchCleanupMemories(ctx context.Context, db *gorm.DB, sessionID string, memoryIDs []string) error {
	if len(memoryIDs) == 0 || db == nil || sessionID == "" {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	memoryIDs = uniqueNonEmptyStrings(memoryIDs)
	if len(memoryIDs) == 0 {
		return nil
	}

	// Bound parameters and entity metadata even for explicit bulk callers.
	for start := 0; start < len(memoryIDs); start += 100 {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := cleanupMemoryBatch(ctx, db, sessionID, memoryIDs[start:min(start+100, len(memoryIDs))]); err != nil {
			return err
		}
	}
	return nil
}

func cleanupMemoryBatch(ctx context.Context, db *gorm.DB, sessionID string, memoryIDs []string) error {
	midtermMode := strings.HasPrefix(sessionID, MidtermSessionPrefix)
	entityTable := "ai_memory_entities_v1"
	if midtermMode {
		entityTable = "ai_midterm_archive_entities_v1"
	}
	var entities []schema.AIMemoryEntity
	if err := db.Table(entityTable).Select("memory_id, session_id, potential_questions").
		Where("memory_id IN (?) AND session_id = ?", memoryIDs, sessionID).
		Find(&entities).Error; err != nil {
		return utils.Wrap(err, "query entities for cleanup")
	}
	if len(entities) == 0 {
		return nil
	}
	var docIDs []string
	for i := range entities {
		docIDs = append(docIDs, entities[i].DocumentQuestionHashIDs()...)
	}
	docIDs = uniqueNonEmptyStrings(docIDs)
	if len(docIDs) > 0 {
		store, ok, err := getOrCreateRAGStore(make(map[string]*vectorstore.SQLiteVectorStoreHNSW), make(map[string]bool), db, sessionID)
		if err != nil {
			return utils.Wrap(err, "load memory RAG for cleanup")
		}
		if ok {
			if err := store.Delete(docIDs...); err != nil {
				return utils.Wrap(err, "delete memory RAG documents")
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	backend, err := NewAIMemoryHNSWBackend(
		WithHNSWSessionID(sessionID), WithHNSWDatabase(db),
		WithHNSWAutoSave(false), WithHNSWMidtermMode(midtermMode))
	if err != nil {
		return err
	}
	if err := backend.deleteAndSave(ctx, memoryIDs, true); err != nil {
		return err
	}
	log.Debugf("cleaned up %d memories for session %s", len(entities), sessionID)
	return nil
}
