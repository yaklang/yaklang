package aimem

import (
	"time"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// MemoryStore saves already extracted memories in a namespace. It does not
// extract, judge or deduplicate memories with an AI model.
type MemoryStore struct {
	sessionID          string
	db                 *gorm.DB
	rag                *rag.RAGSystem
	hnswBackend        *AIMemoryHNSWBackend
	embeddingAvailable bool
	emitter            *aicommon.Emitter
}

// NewMemoryStore reuses the existing database, TTL and index infrastructure.
// Invoker options are ignored: persistence never creates an AI runtime.
func NewMemoryStore(namespace string, opts ...Option) (*MemoryStore, error) {
	config := &Config{embeddingAvailabilityCheck: rag.CheckConfigEmbeddingAvailable}
	for _, opt := range opts {
		opt(config)
	}
	return newMemoryStore(namespace, config)
}

func newMemoryStore(namespace string, config *Config) (*MemoryStore, error) {
	if namespace == "" {
		return nil, utils.Error("memory namespace is required")
	}
	if config.database == nil {
		config.database = consts.GetGormProjectDatabase()
	}
	db := config.database
	if db == nil {
		return nil, utils.Error("database connection is nil")
	}
	name := Session2MemoryName(namespace)
	ragOpts := append([]rag.RAGSystemConfigOption{rag.WithDB(db)}, config.ragOptions...)
	var system *rag.RAGSystem
	checkStart := time.Now()
	embeddingAvailable := config.embeddingAvailabilityCheck(ragOpts...)
	if elapsed := time.Since(checkStart); elapsed > 500*time.Millisecond {
		log.Warnf("[AI-Memory(%v)] checking embedding availability took %v", name, elapsed)
	}
	if embeddingAvailable {
		collectionStart := time.Now()
		var err error
		system, err = rag.GetRagSystem(name, ragOpts...)
		if elapsed := time.Since(collectionStart); elapsed > 500*time.Millisecond {
			log.Warnf("[AI-Memory(%v)] loading RAG system took %v", name, elapsed)
		}
		if err != nil {
			log.Warnf("failed to create RAG collection, semantic search will be unavailable: %v", err)
			system, embeddingAvailable = nil, false
		}
	}
	indexStart := time.Now()
	backend, err := NewAIMemoryHNSWBackend(WithHNSWSessionID(namespace), WithHNSWDatabase(db))
	if elapsed := time.Since(indexStart); elapsed > 500*time.Millisecond {
		log.Warnf("[AI-Memory(%v)] creating HNSW backend took %v, it's abnormal case.", name, elapsed)
	}
	if err != nil {
		return nil, utils.Errorf("create HNSW backend failed: %v", err)
	}
	return &MemoryStore{
		sessionID: namespace, db: db, rag: system, hnswBackend: backend,
		embeddingAvailable: embeddingAvailable, emitter: config.emitter,
	}, nil
}

// Close flushes the score index without closing the shared database or RAG system.
func (s *MemoryStore) Close() error {
	if s.hnswBackend != nil {
		return s.hnswBackend.Close()
	}
	return nil
}
