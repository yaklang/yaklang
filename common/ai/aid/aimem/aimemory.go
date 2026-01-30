package aimem

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed memory_triage.txt
var memoryTriagePrompt string

//go:embed corepact_principle.txt
var corepactPrinciplesPrompt string

func Session2MemoryName(sessionId string) string {
	return fmt.Sprintf("ai-memory-%s", sessionId)
}

func newAIMemory(sessionId string, requireInvoker bool, opts ...Option) (*AIMemoryTriage, error) {
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
	ragCheckingOpts := append([]rag.RAGSystemConfigOption{rag.WithDB(db)}, ragOpts...)

	// 检查 embedding 服务可用性，如果不可用，记录警告但继续
	var system *rag.RAGSystem
	var embeddingAvailable bool
	var err error

	ragCheckingStart := time.Now()
	embeddingAvailable = rag.CheckConfigEmbeddingAvailable(ragCheckingOpts...)
	//  检查是否有默认的嵌入模型可用
	if embeddingAvailable {
		system, err = rag.GetRagSystem(name, ragCheckingOpts...)
		if err != nil {
			log.Warnf("failed to create RAG collection, semantic search will be unavailable: %v", err)
			system = nil
			embeddingAvailable = false
		}
	}
	if du := time.Since(ragCheckingStart); du > 500*time.Millisecond {
		log.Warnf("[AI-Memory(%v)] checking RAG system embedding availability took %v, it's abnormal case.", name, du)
	}

	// 创建HNSW后端
	hnswBackendStart := time.Now()
	hnswBackend, err := NewAIMemoryHNSWBackend(WithHNSWSessionID(sessionId), WithHNSWDatabase(db))
	if du := time.Since(hnswBackendStart); du > 500*time.Millisecond {
		log.Warnf("[AI-Memory(%v)] creating HNSW backend took %v, it's abnormal case.", name, du)
	}
	if err != nil {
		return nil, utils.Errorf("create HNSW backend failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	triage := &AIMemoryTriage{
		ctx:                ctx,
		cancel:             cancel,
		rag:                system,
		invoker:            config.invoker,
		contextProvider:    config.contextProvider,
		sessionID:          sessionId,
		hnswBackend:        hnswBackend,
		db:                 db,
		keywordMatcher:     NewKeywordMatcher(), // 初始化关键词匹配器
		embeddingAvailable: embeddingAvailable,
	}

	if requireInvoker && triage.invoker == nil && config.autoLightReActInvoker {
		lightOpts := []aicommon.ConfigOption{
			aicommon.WithMemoryTriage(triage),
			aicommon.WithEnableSelfReflection(false),
			aicommon.WithDisallowMCPServers(true),
			aicommon.WithDisableSessionTitleGeneration(true),
		}
		lightOpts = append(lightOpts, config.lightReActOptions...)
		invoker, err := aicommon.LightAIRuntimeInvokerGetter(triage.ctx, lightOpts...)
		if err != nil {
			return nil, utils.Errorf("create light react invoker for ai-memory trigger failed: %v", err)
		}
		triage.invoker = invoker
	}

	if requireInvoker && triage.invoker == nil {
		return nil, utils.Error("aicommon invoker in memory is need, cannot be empty.")
	}

	return triage, nil
}

// NewAIMemory 创建AI记忆管理系统
func NewAIMemory(sessionId string, opts ...Option) (*AIMemoryTriage, error) {
	return newAIMemory(sessionId, true, opts...)
}

// NewAIMemoryForQuery 创建用于查询的 AI 记忆实例（不强制要求 invoker）
func NewAIMemoryForQuery(sessionId string, opts ...Option) (*AIMemoryTriage, error) {
	return newAIMemory(sessionId, false, opts...)
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
