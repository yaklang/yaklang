package aimem

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed memory_triage.txt
var memoryTriagePrompt string

//go:embed corepact_principle.txt
var corepactPrinciplesPrompt string

// NewAIMemory 创建AI记忆管理系统
func NewAIMemory(sessionId string, opts ...Option) (*AIMemoryTriage, error) {
	if sessionId == "" {
		return nil, utils.Errorf("sessionId is required")
	}

	// 应用配置选项
	config := &Config{}
	for _, opt := range opts {
		opt(config)
	}

	name := fmt.Sprintf("ai-memory-%s", sessionId)
	if config.database == nil {
		config.database = consts.GetGormProjectDatabase()
	}
	db := config.database

	// 使用配置中的RAG选项
	ragOpts := config.ragOptions

	// 尝试加载现有collection，如果不存在则创建新的
	system, err := rag.LoadCollection(db, name, ragOpts...)
	if err != nil {
		log.Infof("collection not found, creating new one: %s", name)
		system, err = rag.CreateCollection(db, name, fmt.Sprintf("AI Memory for session %s", sessionId), ragOpts...)
		if err != nil {
			return nil, utils.Errorf("create collection failed: %v", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	// 创建HNSW后端
	hnswBackend, err := NewAIMemoryHNSWBackend(WithHNSWSessionID(sessionId), WithHNSWDatabase(db))
	if err != nil {
		return nil, utils.Errorf("create HNSW backend failed: %v", err)
	}

	triage := &AIMemoryTriage{
		ctx:             ctx,
		cancel:          cancel,
		rag:             system,
		invoker:         config.invoker,
		contextProvider: config.contextProvider,
		sessionID:       sessionId,
		hnswBackend:     hnswBackend,
		db:              db,
	}

	if triage.invoker == nil {
		return nil, utils.Error("aicommon invoker in memory is need, cannot be empty.")
	}

	return triage, nil
}

// GetSessionID 获取当前会话ID
func (r *AIMemoryTriage) GetSessionID() string {
	return r.sessionID
}

// GetHNSWStats 获取HNSW索引统计信息
func (r *AIMemoryTriage) GetHNSWStats() map[string]interface{} {
	if r.hnswBackend == nil {
		return map[string]interface{}{
			"error": "HNSW backend not initialized",
		}
	}
	return r.hnswBackend.GetStats()
}

// RebuildHNSWIndex 重建HNSW索引
func (r *AIMemoryTriage) RebuildHNSWIndex() error {
	if r.hnswBackend == nil {
		return utils.Errorf("HNSW backend not initialized")
	}
	return r.hnswBackend.RebuildIndex()
}

// Close 关闭资源
func (r *AIMemoryTriage) Close() error {
	if r.cancel != nil {
		r.cancel()
	}
	if r.hnswBackend != nil {
		if err := r.hnswBackend.Close(); err != nil {
			log.Errorf("close HNSW backend failed: %v", err)
		}
	}
	return nil
}
