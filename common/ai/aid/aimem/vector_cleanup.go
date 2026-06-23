package aimem

import (
	"context"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
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

		backend, err := getOrCreateHNSWBackend(hnswBackends, db, sessionID)
		if err == nil {
			for _, memoryID := range payload.memoryIDs {
				_ = backend.Delete(memoryID)
			}
			if len(payload.memoryIDs) > 0 {
				if err := backend.SaveGraph(); err != nil {
					log.Warnf("AIMemory HNSW save skipped: %v", err)
				}
			}
		} else {
			log.Warnf("AIMemory HNSW delete skipped: %v", err)
		}

		if len(payload.docIDs) == 0 {
			continue
		}

		store, ok, err := getOrCreateRAGStore(ragStores, ragExists, db, sessionID)
		if err != nil {
			log.Warnf("AIMemory RAG delete skipped: %v", err)
			continue
		}
		if !ok {
			continue
		}
		if err := store.Delete(payload.docIDs...); err != nil {
			log.Warnf("AIMemory RAG delete docs skipped: %v", err)
		}
	}
	return nil
}

func uniqueNonEmptyStrings(in []string) []string {
	if len(in) <= 1 {
		return in
	}
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
	backend, err := NewAIMemoryHNSWBackend(
		WithHNSWSessionID(sessionID),
		WithHNSWDatabase(db),
		WithHNSWAutoSave(false),
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
	store, err := vectorstore.LoadCollection(db, collectionName, vectorstore.WithEmbeddingClient(rag.NewEmptyMockEmbedding()))
	if err != nil {
		return nil, false, err
	}
	cache[collectionName] = store
	exists[collectionName] = true
	return store, true, nil
}
