package aimem

import (
	"context"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

var memoryTriagePrompt = promptloader.MustLoad("ai/aid/aimem/memory_triage.txt")

var corepactPrinciplesPrompt = promptloader.MustLoad("ai/aid/aimem/corepact_principle.txt")

var memoryTriageInstruction = promptloader.MustLoad("ai/aid/aimem/memory_triage_instruction.txt") + "\n" + corepactPrinciplesPrompt

func Session2MemoryName(sessionId string) string {
	return fmt.Sprintf("ai-memory-%s", sessionId)
}

func newAIMemory(sessionId string, requireInvoker bool, opts ...Option) (*AIMemoryTriage, error) {
	if sessionId == "" {
		return nil, utils.Errorf("sessionId is required")
	}

	// 应用配置选项
	config := &Config{embeddingAvailabilityCheck: rag.CheckConfigEmbeddingAvailable}
	for _, opt := range opts {
		opt(config)
	}

	store, err := newMemoryStore(sessionId, config)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	triage := &AIMemoryTriage{
		ctx:                ctx,
		cancel:             cancel,
		rag:                store.rag,
		invoker:            config.invoker,
		contextProvider:    config.contextProvider,
		sessionID:          sessionId,
		hnswBackend:        store.hnswBackend,
		db:                 store.db,
		keywordMatcher:     NewKeywordMatcher(), // 初始化关键词匹配器
		embeddingAvailable: store.embeddingAvailable,
	}

	if requireInvoker && triage.invoker == nil && config.autoReActInvoker {
		lightOpts := []aicommon.ConfigOption{
			aicommon.WithMemoryTriage(triage),
			aicommon.WithDisallowMCPServers(true),
			aicommon.WithDisableSessionTitleGeneration(true),
		}
		lightOpts = append(lightOpts, config.reActOptions...)
		invoker, err := aicommon.AIRuntimeInvokerGetter(triage.ctx, lightOpts...)
		if err != nil {
			return nil, utils.Errorf("create react invoker for ai-memory trigger failed: %v", err)
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
