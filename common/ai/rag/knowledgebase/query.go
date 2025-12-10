package knowledgebase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
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
	Filter               func(key string, docGetter func() *vectorstore.Document, knowledgeBaseEntryGetter func() (*schema.KnowledgeBaseEntry, error)) bool
	EnhancePlan          string
	AIService            string
	AICallback           aicommon.AICallbackType
	MsgCallBack          func(*SearchKnowledgebaseResult)
}

type QueryOption func(*QueryConfig)

func WithAIService(aiService string) QueryOption {
	return func(config *QueryConfig) {
		config.AIService = aiService
	}
}

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

func WithFilter(filter func(key string, docGetter func() *vectorstore.Document, knowledgeBaseEntryGetter func() (*schema.KnowledgeBaseEntry, error)) bool) QueryOption {
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
	SearchResultTypeMessage   = vectorstore.RAGResultTypeMessage
	SearchResultTypeMidResult = vectorstore.RAGResultTypeMidResult
	SearchResultTypeResult    = vectorstore.RAGResultTypeResult
	SearchResultTypeError     = vectorstore.RAGResultTypeError
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
	searchResults, err := kb.vectorStore.QueryWithFilter(query, 1, limit+5, func(key string, getDoc func() *vectorstore.Document) bool {
		return getDoc().Type == schema.RAGDocumentType_Knowledge
	})
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
			entry, err := yakit.GetKnowledgeBaseEntryByHiddenIndex(kb.db, docID)
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
	res, err := kb.SearchKnowledgeEntriesWithEnhance(query, append(opts, WithEnableAISummary(true), WithCollectionName(kb.name))...)
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
	ragOpts := []vectorstore.CollectionQueryOption{
		vectorstore.WithRAGLimit(config.Limit),
		vectorstore.WithRAGEnhance(config.EnhancePlan),
		vectorstore.WithRAGCtx(config.Ctx),
	}

	// 如果有Filter配置，转换为RAG Filter
	if config.Filter != nil {
		ragFilter := func(key string, getDoc func() *vectorstore.Document) bool {
			if getDoc().Type != schema.RAGDocumentType_Knowledge {
				return false
			}
			return config.Filter(key, getDoc, func() (*schema.KnowledgeBaseEntry, error) {
				var entry schema.KnowledgeBaseEntry
				err := db.Model(&schema.KnowledgeBaseEntry{}).Where("id = ?", key).First(&entry).Error
				if err != nil {
					return nil, err
				}
				return &entry, nil
			})
		}
		ragOpts = append(ragOpts, vectorstore.WithRAGFilter(ragFilter))
	} else {
		ragOpts = append(ragOpts, vectorstore.WithRAGFilter(func(key string, getDoc func() *vectorstore.Document) bool {
			return getDoc().Type == schema.RAGDocumentType_Knowledge
		}))
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
	ragMsgCallback := func(ragResult *vectorstore.RAGSearchResult) {
		kbResult := &SearchKnowledgebaseResult{
			Message: ragResult.Message,
			Type:    ragResult.Type,
		}

		// 对于result类型的消息，需要将Document转换为KnowledgeBaseEntry
		if ragResult.Type == SearchResultTypeResult && ragResult.Data != nil {
			if doc, ok := ragResult.Data.(*vectorstore.Document); ok {
				// 从文档ID获取知识库条目
				var docID string
				if doc.Metadata["original_doc_id"] != nil {
					docID = doc.Metadata["original_doc_id"].(string)
				} else {
					docID = doc.ID
				}

				if entry, err := yakit.GetKnowledgeBaseEntryByHiddenIndex(db, docID); err == nil {
					kbResult.Data = entry
					kbResult.Message = entry.KnowledgeTitle
				}
			}
		}

		allResults = append(allResults, kbResult)

		knowledgeBaseMsgCallback(kbResult)
	}

	ragOpts = append(ragOpts, vectorstore.WithRAGMsgCallBack(ragMsgCallback))

	if config.CollectionName != "" {
		ragOpts = append(ragOpts, vectorstore.WithRAGQueryCollectionNames(config.CollectionName))
	} else {
		kbNames, err := yakit.GetKnowledgeBaseNameList(db)
		if err != nil {
			return nil, utils.Errorf("get knowledge base name list failed: %v", err)
		}
		ragOpts = append(ragOpts, vectorstore.WithRAGQueryCollectionNames(kbNames...))
	}

	// 调用rag.Query进行搜索
	ragResultCh, err := vectorstore.Query(db, query, ragOpts...)
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
			prompt := `你是一名严谨的知识助手。请仅基于下方“知识库条目”的内容回答“用户问题”，不要引入外部知识或主观推断；若无法从条目中得到答案，请直接回复“未在知识库中找到相关信息”。

作答要求：
- 优先给出简明结论，其后用要点说明依据；
- 多条目信息需去重、合并，避免冲突；如存在冲突，说明取舍依据；
- 严禁编造、兜圈或输出与问题无关的内容；

知识库条目：
%s

用户问题：
%s
	`
			prompt = fmt.Sprintf(prompt, docStr, query)
			knowledgeBaseMsgCallback(&SearchKnowledgebaseResult{
				Message: prompt,
				Type:    SearchResultTypeMessage,
				Data:    nil,
			})
			var aiCommonOptions []aicommon.ConfigOption

			if config.AICallback != nil {
				aiCommonOptions = append(aiCommonOptions, aicommon.WithAICallback(config.AICallback))
			} else if config.AIService != "" {
				aiCommonOptions = append(aiCommonOptions, aicommon.WithAIServiceName(config.AIService))
			}

			answer, err := Simpleliteforge.SimpleExecuteWithOptions(config.Ctx, prompt, []aitool.ToolOption{aitool.WithStringParam("answer")}, aiCommonOptions...)
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
