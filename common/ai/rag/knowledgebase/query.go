package knowledgebase

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/enhancesearch"
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
	Filter               func(key string, docGetter func() *rag.Document, knowledgeBaseEntryGetter func() (*schema.KnowledgeBaseEntry, error)) bool
	MsgCallBack          func(*SearchKnowledgebaseResult)
}

type QueryOption func(*QueryConfig)

func WithLimit(limit int) QueryOption {
	return func(config *QueryConfig) {
		config.Limit = limit
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

type SearchKnowledgebaseResult struct {
	Message string
	Data    any
	Type    string
}

func (kb *KnowledgeBase) SearchKnowledgeEntriesWithEnhance(query string, opts ...QueryOption) (chan *SearchKnowledgebaseResult, error) {
	config := NewQueryConfig(opts...)
	ctx := config.Ctx
	resultCh := make(chan *SearchKnowledgebaseResult)
	sendMsg := func(msg string) {
		msgResult := &SearchKnowledgebaseResult{
			Message: msg,
			Type:    "message",
		}
		if config.MsgCallBack != nil {
			config.MsgCallBack(msgResult)
		}
		select {
		case resultCh <- msgResult:
		case <-ctx.Done():
			return
		}
	}
	sendResult := func(result *schema.KnowledgeBaseEntry) {
		msgResult := &SearchKnowledgebaseResult{
			Message: result.KnowledgeTitle,
			Data:    result,
			Type:    "result",
		}
		if config.MsgCallBack != nil {
			config.MsgCallBack(msgResult)
		}
		select {
		case resultCh <- msgResult:
		case <-ctx.Done():
			return
		}
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				sendMsg(fmt.Sprintf("生成假设文档时发生错误: %v", r))
			}
			close(resultCh)
		}()
		sendMsg("开始生成假设文档")
		enhance, err := enhancesearch.HypotheticalAnswer(config.Ctx, query)
		if err != nil {
			sendMsg(fmt.Sprintf("增强搜索失败: %v", err))
			return
		}
		sendMsg("假设文档: " + enhance)
		sendMsg(fmt.Sprintf("开始搜索, page: %d, limit: %d", 1, config.Limit+5))
		// 先通过RAG系统进行向量搜索
		searchResults, err := kb.ragSystem.QueryWithFilter(enhance, 1, config.Limit+5, func(key string, getDoc func() *rag.Document) bool {
			if config.Filter == nil {
				return true
			}
			return config.Filter(key, getDoc, func() (*schema.KnowledgeBaseEntry, error) {
				var entry schema.KnowledgeBaseEntry
				err := kb.db.Model(&schema.KnowledgeBaseEntry{}).Where("id = ?", key).First(&entry).Error
				if err != nil {
					return nil, err
				}
				return &entry, nil
			})
		})
		if err != nil {
			sendMsg(fmt.Sprintf("RAG搜索失败: %v", err))
			return
		}

		sendMsg(fmt.Sprintf("共搜索到: %d 个结果", len(searchResults)))
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
				sendResult(entry)
			}
		}
		sendMsg(fmt.Sprintf("共搜索到: %d 个文档", len(entries)))
	}()
	return resultCh, nil
}

// SearchKnowledgeEntriesWithScore 搜索知识条目并返回相似度分数
func (kb *KnowledgeBase) SearchKnowledgeEntriesWithScore(query string, limit int) ([]*KnowledgeEntryWithScore, error) {
	// 先通过RAG系统进行向量搜索
	searchResults, err := kb.ragSystem.QueryWithPage(query, 1, limit)
	if err != nil {
		return nil, utils.Errorf("RAG搜索失败: %v", err)
	}

	// 通过搜索结果中的文档ID查询对应的知识库条目，并保留分数
	var entriesWithScore []*KnowledgeEntryWithScore
	for _, result := range searchResults {
		// 文档ID就是知识库条目的ID
		entryID, err := strconv.ParseInt(result.Document.ID, 10, 64)
		if err != nil {
			// 如果ID解析失败，跳过这个结果
			continue
		}

		entry, err := yakit.GetKnowledgeBaseEntryById(kb.db, entryID)
		if err != nil {
			// 如果查询失败，跳过这个结果
			continue
		}

		entriesWithScore = append(entriesWithScore, &KnowledgeEntryWithScore{
			Entry: entry,
			Score: float64(result.Score),
		})
	}

	return entriesWithScore, nil
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
	config := NewQueryConfig(opts...)
	res, err := kb.SearchKnowledgeEntriesWithEnhance(query, opts...)
	if err != nil {
		return "", utils.Errorf("搜索失败: %v", err)
	}
	docs := make([]*schema.KnowledgeBaseEntry, 0)
	for result := range res {
		if config.MsgCallBack != nil {
			config.MsgCallBack(result)
		}
		if result.Type == "result" {
			if v, ok := result.Data.(*schema.KnowledgeBaseEntry); ok {
				docs = append(docs, v)
			}
		}
	}
	var docStrs []string
	for _, doc := range docs {
		docStrs = append(docStrs, fmt.Sprintf("知识标题: %s\n知识详情：%s", doc.KnowledgeTitle, doc.KnowledgeDetails))
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
		return "", utils.Errorf("查询失败: %v", err)
	}
	return answer.GetString("answer"), nil
}

func Query(db *gorm.DB, query string, opts ...QueryOption) (chan *SearchKnowledgebaseResult, error) {
	config := NewQueryConfig(opts...)
	ctx := config.Ctx
	resultCh := make(chan *SearchKnowledgebaseResult)
	sendMsg := func(msg string) {
		msgResult := &SearchKnowledgebaseResult{
			Message: msg,
			Type:    "message",
		}
		if config.MsgCallBack != nil {
			config.MsgCallBack(msgResult)
		}
		select {
		case resultCh <- msgResult:
		case <-ctx.Done():
			return
		}
	}
	sendMidResult := func(result *schema.KnowledgeBaseEntry) {
		msgResult := &SearchKnowledgebaseResult{
			Message: result.KnowledgeTitle,
			Data:    result,
			Type:    "mid_result",
		}
		if config.MsgCallBack != nil {
			config.MsgCallBack(msgResult)
		}
		select {
		case resultCh <- msgResult:
		case <-ctx.Done():
			return
		}
	}
	sendResult := func(result *schema.KnowledgeBaseEntry) {
		msgResult := &SearchKnowledgebaseResult{
			Message: result.KnowledgeTitle,
			Data:    result,
			Type:    "result",
		}
		if config.MsgCallBack != nil {
			config.MsgCallBack(msgResult)
		}
		select {
		case resultCh <- msgResult:
		case <-ctx.Done():
			return
		}
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				sendMsg(fmt.Sprintf("生成假设文档时发生错误: %v", r))
			}
			close(resultCh)
		}()
		sendMsg("开始生成假设文档")
		enhance, err := enhancesearch.HypotheticalAnswer(config.Ctx, query)
		if err != nil {
			sendMsg(fmt.Sprintf("增强搜索失败: %v", err))
			return
		}
		sendMsg("假设文档: " + enhance)

		var ids [][]any
		if config.CollectionName == "" {
			sendMsg(fmt.Sprintf("未指定集合名称，将搜索 %d 个相关集合进行搜索", config.CollectionNumLimit))
			docs, err := rag.QueryCollection(db, query)
			if err != nil {
				sendMsg(fmt.Sprintf("搜索集合失败: %v", err))
				return
			}
			sendMsg(fmt.Sprintf("共搜索到 %d 个集合", len(docs)))
			for _, doc := range docs {
				sendMsg(fmt.Sprintf("集合名称: %s", doc.Document.Metadata["collection_name"]))
				id := doc.Document.Metadata["collection_id"]
				ids = append(ids, []any{id, doc.Document.Metadata["collection_name"]})
			}
		}

		sendMsg(fmt.Sprintf("开始搜索, page: %d, limit: %d", 1, config.Limit+5))

		var allResults []*schema.KnowledgeBaseEntry
		for _, id := range ids {
			sendMsg(fmt.Sprintf("开始搜索集合: %s", id[1]))
			kb, err := LoadKnowledgeBaseByID(db, int64(id[0].(int)))
			if err != nil {
				sendMsg(fmt.Sprintf("加载知识库失败: %v", err))
				return
			}
			searchResultCh, err := kb.SearchKnowledgeEntriesWithEnhance(enhance, opts...)
			if err != nil {
				sendMsg(fmt.Sprintf("搜索集合失败: %v", err))
				return
			}
			for res := range searchResultCh {
				if res.Type == "result" {
					allResults = append(allResults, res.Data.(*schema.KnowledgeBaseEntry))
					sendMidResult(res.Data.(*schema.KnowledgeBaseEntry))
				}
			}
		}

		if len(allResults) > config.Limit {
			allResults = allResults[:config.Limit]
		}

		for _, result := range allResults {
			sendResult(result)
		}
		sendMsg(fmt.Sprintf("共搜索到: %d 个文档", len(allResults)))
	}()
	return resultCh, nil
}
