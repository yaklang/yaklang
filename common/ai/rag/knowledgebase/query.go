package knowledgebase

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type QueryConfig struct {
	Ctx                  context.Context
	Limit                int
	CollectionName       string
	CollectionNumLimit   int
	CollectionScoreLimit int
	EnableAISummary      bool
	Filter               func(key string, docGetter func() *rag.Document, knowledgeBaseEntryGetter func() (*schema.KnowledgeBaseEntry, error)) bool
	EnhancePlan          string
	MsgCallBack          func(*SearchKnowledgebaseResult)
}

type QueryOption func(*QueryConfig)

func WithLimit(limit int) QueryOption {
	return func(config *QueryConfig) {
		config.Limit = limit
	}
}

func WithEnhancePlan(enhancePlan string) QueryOption {
	return func(config *QueryConfig) {
		config.EnhancePlan = enhancePlan
	}
}

func WithEnableAISummary(enableAISummary bool) QueryOption {
	return func(config *QueryConfig) {
		config.EnableAISummary = enableAISummary
	}
}

func WithCollectionName(collectionName string) QueryOption {
	return func(config *QueryConfig) {
		config.CollectionName = collectionName
	}
}

func WithCollectionScoreLimit(collectionScoreLimit int) QueryOption {
	return func(config *QueryConfig) {
		config.CollectionScoreLimit = collectionScoreLimit
	}
}

func WithCollectionLimit(collectionLimit int) QueryOption {
	return func(config *QueryConfig) {
		config.CollectionNumLimit = collectionLimit
	}
}

func WithFilter(filter func(key string, docGetter func() *rag.Document, knowledgeBaseEntryGetter func() (*schema.KnowledgeBaseEntry, error)) bool) QueryOption {
	return func(config *QueryConfig) {
		config.Filter = filter
	}
}

func WithMsgCallBack(msgCallBack func(*SearchKnowledgebaseResult)) QueryOption {
	return func(config *QueryConfig) {
		config.MsgCallBack = msgCallBack
	}
}

func WithCtx(ctx context.Context) QueryOption {
	return func(config *QueryConfig) {
		config.Ctx = ctx
	}
}

func NewQueryConfig(opts ...QueryOption) *QueryConfig {
	config := &QueryConfig{
		Limit:              10,
		Filter:             nil,
		MsgCallBack:        nil,
		CollectionNumLimit: 5,
		Ctx:                context.Background(),
	}
	for _, opt := range opts {
		opt(config)
	}
	return config
}

// SearchKnowledgebaseResult 消息类型常量
const (
	SearchResultTypeMessage   = rag.RAGResultTypeMessage
	SearchResultTypeMidResult = rag.RAGResultTypeMidResult
	SearchResultTypeResult    = rag.RAGResultTypeResult
	SearchResultTypeError     = rag.RAGResultTypeError
	SearchResultTypeAISummary = "ai_summary"
)

type SearchKnowledgebaseResult struct {
	Message string
	Data    any
	Type    string
}

func (kb *KnowledgeBase) SearchKnowledgeEntriesWithEnhance(query string, opts ...QueryOption) (chan *SearchKnowledgebaseResult, error) {
	return Query(kb.db, query, append(opts, WithCollectionName(kb.name))...)
}

// SearchKnowledgeEntries 搜索知识条目，返回知识库条目对象
func (kb *KnowledgeBase) SearchKnowledgeEntries(query string, limit int) ([]*schema.KnowledgeBaseEntry, error) {
	// 先通过RAG系统进行向量搜索
	searchResults, err := kb.ragSystem.QueryWithPage(query, 1, limit+5)
	if err != nil {
		return nil, utils.Errorf("RAG搜索失败: %v", err)
	}

	// 通过搜索结果中的文档ID查询对应的知识库条目
	var entries []*schema.KnowledgeBaseEntry
	docIDs := make(map[string]bool)
	for _, result := range searchResults {
		var docID string
		if result.Document.Metadata["original_doc_id"] != nil {
			docID = result.Document.Metadata["original_doc_id"].(string)
		} else {
			docID = result.Document.ID
		}

		if docID != "" && !docIDs[docID] {
			docIDs[docID] = true
			// 文档ID就是知识库条目的ID
			entryID, err := strconv.ParseInt(docID, 10, 64)
			if err != nil {
				// 如果ID解析失败，跳过这个结果
				continue
			}

			entry, err := yakit.GetKnowledgeBaseEntryById(kb.db, entryID)
			if err != nil {
				// 如果查询失败，跳过这个结果
				continue
			}

			entries = append(entries, entry)
		}
	}

	if len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}

func (kb *KnowledgeBase) Query(query string, opts ...QueryOption) (string, error) {
	res, err := kb.SearchKnowledgeEntriesWithEnhance(query, append(opts, WithEnableAISummary(true))...)
	if err != nil {
		return "", utils.Errorf("搜索失败: %v", err)
	}
	for result := range res {
		if result.Type == SearchResultTypeAISummary {
			return result.Message, nil
		}
	}
	return "", errors.New("搜索失败: 未知原因")
}

func Query(db *gorm.DB, query string, opts ...QueryOption) (chan *SearchKnowledgebaseResult, error) {
	config := NewQueryConfig(opts...)
	resultCh := make(chan *SearchKnowledgebaseResult)

	// 构造RAG查询选项
	ragOpts := []rag.RAGQueryOption{
		rag.WithRAGLimit(config.Limit),
		rag.WithRAGEnhance(rag.EnhancePlanHypotheticalAnswer),
		rag.WithRAGCtx(config.Ctx),
	}

	// 如果有Filter配置，转换为RAG Filter
	if config.Filter != nil {
		ragFilter := func(key string, getDoc func() *rag.Document) bool {
			return config.Filter(key, getDoc, func() (*schema.KnowledgeBaseEntry, error) {
				var entry schema.KnowledgeBaseEntry
				err := db.Model(&schema.KnowledgeBaseEntry{}).Where("id = ?", key).First(&entry).Error
				if err != nil {
					return nil, err
				}
				return &entry, nil
			})
		}
		ragOpts = append(ragOpts, rag.WithRAGFilter(ragFilter))
	}

	var allResults []*SearchKnowledgebaseResult
	knowledgeBaseMsgCallback := func(kbResult *SearchKnowledgebaseResult) {
		// 调用原始回调
		if config.MsgCallBack != nil {
			config.MsgCallBack(kbResult)
		}

		// 发送到结果通道
		select {
		case resultCh <- kbResult:
		case <-config.Ctx.Done():
			return
		}
	}
	// 设置RAG消息回调，转换为知识库结果格式
	ragMsgCallback := func(ragResult *rag.RAGSearchResult) {
		kbResult := &SearchKnowledgebaseResult{
			Message: ragResult.Message,
			Type:    ragResult.Type,
		}

		// 对于result类型的消息，需要将Document转换为KnowledgeBaseEntry
		if ragResult.Type == SearchResultTypeResult && ragResult.Data != nil {
			if doc, ok := ragResult.Data.(*rag.Document); ok {
				// 从文档ID获取知识库条目
				var docID string
				if doc.Metadata["original_doc_id"] != nil {
					docID = doc.Metadata["original_doc_id"].(string)
				} else {
					docID = doc.ID
				}

				if entryID, err := strconv.ParseInt(docID, 10, 64); err == nil {
					if entry, err := yakit.GetKnowledgeBaseEntryById(db, entryID); err == nil {
						kbResult.Data = entry
						kbResult.Message = entry.KnowledgeTitle
					}
				}
			}
		}

		allResults = append(allResults, kbResult)

		knowledgeBaseMsgCallback(kbResult)
	}

	ragOpts = append(ragOpts, rag.WithRAGMsgCallBack(ragMsgCallback))

	// 调用rag.Query进行搜索
	ragResultCh, err := rag.Query(db, query, ragOpts...)
	if err != nil {
		return nil, err
	}

	// 启动协程处理RAG结果并转换为知识库结果
	go func() {
		defer close(resultCh)

		for range ragResultCh {
		}

		if config.EnableAISummary {
			var docStrs []string
			for _, doc := range allResults {
				if doc.Type == SearchResultTypeResult {
					if v, ok := doc.Data.(*schema.KnowledgeBaseEntry); ok {
						docStrs = append(docStrs, fmt.Sprintf("知识标题: %s\n知识详情：%s", v.KnowledgeTitle, v.KnowledgeDetails))
					}
				}
			}
			docStr := strings.Join(docStrs, "\n\n")
			prompt := `请根据以下知识库条目，回答问题：
知识库条目：
%s
问题：
%s
	`
			prompt = fmt.Sprintf(prompt, docStr, query)
			answer, err := Simpleliteforge.SimpleExecute(config.Ctx, prompt, []aitool.ToolOption{aitool.WithStringParam("answer")})
			if err != nil {
				knowledgeBaseMsgCallback(&SearchKnowledgebaseResult{
					Message: err.Error(),
					Type:    SearchResultTypeError,
					Data:    nil,
				})
			}
			answerStr := answer.GetString("answer")
			knowledgeBaseMsgCallback(&SearchKnowledgebaseResult{
				Message: answerStr,
				Type:    SearchResultTypeAISummary,
				Data:    nil,
			})
		}
	}()

	return resultCh, nil
}
