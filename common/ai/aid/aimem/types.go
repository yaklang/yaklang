package aimem

import (
	"bytes"
	"context"
	"fmt"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/utils"
)

// MemoryEntity 表示一个记忆条目
type MemoryEntity struct {
	Id        string
	CreatedAt time.Time
	// 尽量保留原文，适当增加一点点内容的 Content，不准超过1000字，作为记忆来说可用
	Content string
	Tags    []string // 已有 TAG，

	// 7 dims - C.O.R.E. P.A.C.T. Framework (all normalized to 0.0-1.0)
	C_Score float64 // Connectivity Score 这个记忆与其他记忆如何关联？这是一个一次性事实，几乎与其他事实没有什么关联程度
	O_Score float64 // Origin Score 记忆与信息来源确定性，这个来源从哪里来？到底有多少可信度？
	R_Score float64 // Relevance Score 这个信息对用户的目的有多关键？无关紧要？锦上添花？还是成败在此一举？
	E_Score float64 // Emotion Score 用户在表达这个信息时的情绪如何？越低越消极，消极评分时一般伴随信息源不可信
	P_Score float64 // Preference Score 个人偏好对齐评分，这个行为或者问题是否绑定了用户个人风格，品味？
	A_Score float64 // Actionability Score 可操作性评分，是否可以从学习中改进未来行为？
	T_Score float64 // Temporality Score 时效评分，核心问题：这个记忆应该如何被保留？配合时间搜索

	CorePactVector []float32

	// designed for rag searching
	PotentialQuestions []string
}

func (r *MemoryEntity) String() string {
	var buf bytes.Buffer
	buf.WriteString("MemoryEntity{\n")
	buf.WriteString("  ID: " + r.Id + "\n")
	buf.WriteString("  Content: " + r.Content + "\n")
	buf.WriteString("  Tags: ")
	for i, tag := range r.Tags {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(tag)
	}
	buf.WriteString("\n")
	buf.WriteString("  C.O.R.E. P.A.C.T. Scores:\n")
	buf.WriteString(fmt.Sprintf("    C=%.2f, O=%.2f, R=%.2f, E=%.2f, P=%.2f, A=%.2f, T=%.2f\n",
		r.C_Score, r.O_Score, r.R_Score, r.E_Score, r.P_Score, r.A_Score, r.T_Score))
	buf.WriteString("  Potential Questions:\n")
	for _, question := range r.PotentialQuestions {
		buf.WriteString("    - " + question + "\n")
	}
	buf.WriteString("}")
	return buf.String()
}

// SearchResult 搜索结果
type SearchResult struct {
	Entity *MemoryEntity
	Score  float64
}

// ScoreFilter 评分过滤器，用于按C.O.R.E. P.A.C.T.评分搜索
type ScoreFilter struct {
	C_Min, C_Max float64
	O_Min, O_Max float64
	R_Min, R_Max float64
	E_Min, E_Max float64
	P_Min, P_Max float64
	A_Min, A_Max float64
	T_Min, T_Max float64
}

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

type MemoryTriage interface {
	// AddRawText 添加原始文本，返回提取的记忆实体
	AddRawText(text string) ([]*MemoryEntity, error)
	// SaveMemoryEntities 保存记忆条目到数据库
	SaveMemoryEntities(entities ...*MemoryEntity) error

	SearchBySemantics(query string, limit int) ([]*SearchResult, error)

	SearchByTags(tags []string, matchAll bool, limit int) ([]*MemoryEntity, error)

	// HandleMemory 智能处理输入内容，自动构造记忆、去重并保存
	HandleMemory(i any) error

	// SearchMemory 根据输入内容搜索相关记忆，限制总内容字节数
	SearchMemory(origin any, bytesLimit int) (*SearchMemoryResult, error)

	Close() error
}

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
}

func (m *MockMemoryTriage) AddRawText(text string) ([]*MemoryEntity, error) {
	entity := &MemoryEntity{
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
	return []*MemoryEntity{entity}, nil
}

func (m *MockMemoryTriage) SaveMemoryEntities(entities ...*MemoryEntity) error {
	return nil
}

func (m *MockMemoryTriage) Close() error {
	return nil
}

func (m *MockMemoryTriage) SearchBySemantics(query string, limit int) ([]*SearchResult, error) {
	entity := &MemoryEntity{
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
	result := &SearchResult{
		Entity: entity,
		Score:  0.9,
	}
	return []*SearchResult{result}, nil
}

func (m *MockMemoryTriage) SearchByTags(tags []string, matchAll bool, limit int) ([]*MemoryEntity, error) {
	entity := &MemoryEntity{
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
	return []*MemoryEntity{entity}, nil
}

func (m *MockMemoryTriage) HandleMemory(i any) error {
	// Mock实现：简单记录输入但不实际处理
	return nil
}

func (m *MockMemoryTriage) SearchMemory(origin any, bytesLimit int) (*SearchMemoryResult, error) {
	// Mock实现：返回一个简单的搜索结果
	entity := &MemoryEntity{
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
	return &SearchMemoryResult{
		Memories:      []*MemoryEntity{entity},
		TotalContent:  content,
		ContentBytes:  len([]byte(content)),
		SearchSummary: "Mock search completed",
	}, nil
}

func NewMockMemoryTriage() *MockMemoryTriage {
	return &MockMemoryTriage{}
}
