package aimem

import (
	"context"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/utils"
)

// Config AIMemoryTriage的配置
type Config struct {
	invoker         aicommon.AIInvokeRuntime
	contextProvider func() (string, error)
	ragOptions      []any
	database        *gorm.DB
}

// Option AIMemoryTriage的配置选项
type Option func(config *Config)

// AIMemoryTriage AI记忆管理系统

type AIMemoryTriage struct {
	ctx    context.Context
	cancel context.CancelFunc

	rag             *rag.RAGSystem
	invoker         aicommon.AIInvokeRuntime
	contextProvider func() (string, error)
	sessionID       string
	db              *gorm.DB

	// HNSW后端用于ScoreVector搜索
	hnswBackend *AIMemoryHNSWBackend

	// 关键词匹配器 - 支持中英文混合
	keywordMatcher *KeywordMatcher

	// embedding 服务可用标志
	embeddingAvailable bool
}

func (a *AIMemoryTriage) SetInvoker(invoker aicommon.AIInvokeRuntime) {
	a.invoker = invoker
}

// WithContextProvider 设置上下文提供者
func WithContextProvider(i func() (string, error)) Option {
	return func(config *Config) {
		config.contextProvider = i
	}
}

// WithInvoker 设置AI调用运行时
func WithInvoker(invoker aicommon.AIInvokeRuntime) Option {
	return func(config *Config) {
		config.invoker = invoker
	}
}

// WithRAGOptions 设置RAG选项（主要用于测试时注入mock embedding）
func WithRAGOptions(opts ...any) Option {
	return func(config *Config) {
		config.ragOptions = append(config.ragOptions, opts...)
	}
}

// WithDatabase 设置GORM数据库连接
func WithDatabase(db *gorm.DB) Option {
	return func(config *Config) {
		config.database = db
	}
}

func (r *AIMemoryTriage) SafeGetDB() *gorm.DB {
	if r.db == nil {
		return consts.GetGormProjectDatabase()
	}
	return r.db
}

func (r *AIMemoryTriage) GetDB() *gorm.DB {
	return r.db
}

// MockMemoryTriage 这是一个简单的mock，用于不需要使用triage的测试
type MockMemoryTriage struct {
	invoker aicommon.AIInvokeRuntime

	overSearch bool
}

func (m *MockMemoryTriage) SetOverSearch(over bool) {
	m.overSearch = over
}

func (m *MockMemoryTriage) SetInvoker(invoker aicommon.AIInvokeRuntime) {
	m.invoker = invoker
}

func (m *MockMemoryTriage) AddRawText(text string) ([]*aicommon.MemoryEntity, error) {
	entity := &aicommon.MemoryEntity{
		Id:                 "mock-id",
		CreatedAt:          time.Now(),
		Content:            text,
		Tags:               []string{"mock-tag"},
		C_Score:            0.5,
		O_Score:            0.5,
		R_Score:            0.5,
		E_Score:            0.5,
		P_Score:            0.5,
		A_Score:            0.5,
		T_Score:            0.5,
		CorePactVector:     []float32{0.1, 0.2, 0.3},
		PotentialQuestions: []string{"What is mock?", "How to use mock?"},
	}
	return []*aicommon.MemoryEntity{entity}, nil
}

func (m *MockMemoryTriage) SaveMemoryEntities(entities ...*aicommon.MemoryEntity) error {
	return nil
}

func (m *MockMemoryTriage) Close() error {
	return nil
}

func (m *MockMemoryTriage) SearchBySemantics(query string, limit int) ([]*aicommon.SearchResult, error) {
	entity := &aicommon.MemoryEntity{
		Id:                 "mock-id",
		CreatedAt:          time.Now(),
		Content:            "This is a mock memory entity related to " + query,
		Tags:               []string{"mock-tag"},
		C_Score:            0.5,
		O_Score:            0.5,
		R_Score:            0.5,
		E_Score:            0.5,
		P_Score:            0.5,
		A_Score:            0.5,
		T_Score:            0.5,
		CorePactVector:     []float32{0.1, 0.2, 0.3},
		PotentialQuestions: []string{"What is mock?", "How to use mock?"},
	}
	result := &aicommon.SearchResult{
		Entity: entity,
		Score:  0.9,
	}
	return []*aicommon.SearchResult{result}, nil
}

func (m *MockMemoryTriage) GetSessionID() string {
	return ""
}

func (m *MockMemoryTriage) SearchByTags(tags []string, matchAll bool, limit int) ([]*aicommon.MemoryEntity, error) {
	entity := &aicommon.MemoryEntity{
		Id:                 "mock-id",
		CreatedAt:          time.Now(),
		Content:            "This is a mock memory entity with tags",
		Tags:               tags,
		C_Score:            0.5,
		O_Score:            0.5,
		R_Score:            0.5,
		E_Score:            0.5,
		P_Score:            0.5,
		A_Score:            0.5,
		T_Score:            0.5,
		CorePactVector:     []float32{0.1, 0.2, 0.3},
		PotentialQuestions: []string{"What is mock?", "How to use mock?"},
	}
	return []*aicommon.MemoryEntity{entity}, nil
}

func (m *MockMemoryTriage) HandleMemory(i any) error {
	// Mock实现：简单记录输入但不实际处理
	return nil
}

func (m *MockMemoryTriage) SearchMemory(origin any, bytesLimit int) (*aicommon.SearchMemoryResult, error) {
	// Mock实现：返回一个简单的搜索结果
	entity := &aicommon.MemoryEntity{
		Id:                 "mock-search-id",
		CreatedAt:          time.Now(),
		Content:            "Mock search result for: " + utils.InterfaceToString(origin),
		Tags:               []string{"mock-search"},
		C_Score:            0.8,
		O_Score:            0.7,
		R_Score:            0.9,
		E_Score:            0.6,
		P_Score:            0.7,
		A_Score:            0.8,
		T_Score:            0.9,
		CorePactVector:     []float32{0.8, 0.7, 0.9},
		PotentialQuestions: []string{"What is this search about?"},
	}

	content := entity.Content
	return &aicommon.SearchMemoryResult{
		Memories:      []*aicommon.MemoryEntity{entity},
		TotalContent:  content,
		ContentBytes:  len([]byte(content)),
		SearchSummary: "Mock search completed",
	}, nil
}

func (m *MockMemoryTriage) SearchMemoryWithoutAI(origin any, bytesLimit int) (*aicommon.SearchMemoryResult, error) {
	// Mock实现：无AI版本，直接基于关键词匹配
	entity := &aicommon.MemoryEntity{
		Id:                 "mock-search-no-ai-id",
		CreatedAt:          time.Now(),
		Content:            "Mock keyword search result for: " + utils.InterfaceToString(origin),
		Tags:               []string{"mock-search", "keyword-only"},
		C_Score:            0.6,
		O_Score:            0.6,
		R_Score:            0.7,
		E_Score:            0.5,
		P_Score:            0.6,
		A_Score:            0.6,
		T_Score:            0.7,
		CorePactVector:     []float32{0.6, 0.6, 0.7},
		PotentialQuestions: []string{"What keywords matched in this search?"},
	}

	var results []*aicommon.MemoryEntity
	results = append(results, entity)

	if m.overSearch {
		for i := 0; i < 300; i++ {
			results = append(results, &aicommon.MemoryEntity{
				Id:                 "mock-search-no-ai-id-" + utils.InterfaceToString(i),
				CreatedAt:          time.Now(),
				Content:            "Mock keyword search result for: " + utils.InterfaceToString(origin) + " #" + utils.InterfaceToString(i),
				Tags:               []string{"mock-search", "keyword-only"},
				C_Score:            0.6,
				O_Score:            0.6,
				R_Score:            0.7,
				E_Score:            0.5,
				P_Score:            0.6,
				A_Score:            0.6,
				T_Score:            0.7,
				CorePactVector:     []float32{0.6, 0.6, 0.7},
				PotentialQuestions: []string{"What keywords matched in this search?"},
			})
		}
	}

	content := entity.Content
	return &aicommon.SearchMemoryResult{
		Memories:      results,
		TotalContent:  content,
		ContentBytes:  len([]byte(content)),
		SearchSummary: "Mock keyword-based search completed (without AI)",
	}, nil
}

func NewMockMemoryTriage() *MockMemoryTriage {
	return &MockMemoryTriage{}
}
