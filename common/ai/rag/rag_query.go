package rag

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag/enhancesearch"
	"github.com/yaklang/yaklang/common/utils"
)

// RAG 搜索结果类型常量
const (
	RAGResultTypeMessage   = "message"
	RAGResultTypeMidResult = "mid_result"
	RAGResultTypeResult    = "result"
	RAGResultTypeError     = "error"
)

// RAGQueryConfig RAG查询配置
type RAGQueryConfig struct {
	Ctx                  context.Context
	Limit                int
	CollectionName       string
	CollectionNumLimit   int
	CollectionScoreLimit float64
	EnhancePlan          string
	Filter               func(key string, getDoc func() *Document) bool
	Concurrent           int
	MsgCallBack          func(*RAGSearchResult)
}

const (
	EnhancePlanHypotheticalAnswer          = "hypothetical_answer"
	EnhancePlanHypotheticalAnswerWithSplit = "hypothetical_answer_with_split"
	EnhancePlanSplitQuery                  = "split_query"
	EnhancePlanGeneralizeQuery             = "generalize_query"
)

// RAGQueryOption RAG查询选项
type RAGQueryOption func(*RAGQueryConfig)

// WithRAGLimit 设置查询结果限制
func WithRAGLimit(limit int) RAGQueryOption {
	return func(config *RAGQueryConfig) {
		config.Limit = limit
	}
}

// WithRAGCollectionName 指定搜索的集合名称
func WithRAGCollectionName(collectionName string) RAGQueryOption {
	return func(config *RAGQueryConfig) {
		config.CollectionName = collectionName
	}
}

// WithRAGCollectionScoreLimit 设置集合搜索分数阈值
func WithRAGCollectionScoreLimit(scoreLimit float64) RAGQueryOption {
	return func(config *RAGQueryConfig) {
		config.CollectionScoreLimit = scoreLimit
	}
}

// WithRAGCollectionLimit 设置搜索的集合数量限制
func WithRAGCollectionLimit(collectionLimit int) RAGQueryOption {
	return func(config *RAGQueryConfig) {
		config.CollectionNumLimit = collectionLimit
	}
}

// WithRAGEnhance 启用或禁用增强搜索
func WithRAGEnhance(enhancePlan string) RAGQueryOption {
	return func(config *RAGQueryConfig) {
		config.EnhancePlan = enhancePlan
	}
}

// WithRAGFilter 设置文档过滤器
func WithRAGFilter(filter func(key string, getDoc func() *Document) bool) RAGQueryOption {
	return func(config *RAGQueryConfig) {
		config.Filter = filter
	}
}

// WithRAGMsgCallBack 设置消息回调函数
func WithRAGMsgCallBack(msgCallBack func(*RAGSearchResult)) RAGQueryOption {
	return func(config *RAGQueryConfig) {
		config.MsgCallBack = msgCallBack
	}
}

// WithRAGCtx 设置上下文
func WithRAGCtx(ctx context.Context) RAGQueryOption {
	return func(config *RAGQueryConfig) {
		config.Ctx = ctx
	}
}

// WithRAGConcurrent 设置并发数
func WithRAGConcurrent(concurrent int) RAGQueryOption {
	return func(config *RAGQueryConfig) {
		config.Concurrent = concurrent
	}
}

// NewRAGQueryConfig 创建新的RAG查询配置
func NewRAGQueryConfig(opts ...RAGQueryOption) *RAGQueryConfig {
	config := &RAGQueryConfig{
		Limit:                10,
		Filter:               nil,
		MsgCallBack:          nil,
		CollectionNumLimit:   5,
		CollectionScoreLimit: 0.3,
		EnhancePlan:          "hypothetical_answer",
		Ctx:                  context.Background(),
	}
	for _, opt := range opts {
		opt(config)
	}
	return config
}

// RAGSearchResult RAG搜索结果
type RAGSearchResult struct {
	Message   string      `json:"message"`
	Data      interface{} `json:"data"`
	Type      string      `json:"type"`      // message, mid_result, result
	Score     float64     `json:"score"`     // 相似度分数
	Source    string      `json:"source"`    // 结果来源（集合名称）
	Timestamp int64       `json:"timestamp"` // 时间戳
}

// Query 在RAG系统中搜索多个集合
// 这个函数直接在RAG级别进行查询，不依赖于知识库结构
func Query(db *gorm.DB, query string, opts ...RAGQueryOption) (chan *RAGSearchResult, error) {
	return _query(db, query, "1", opts...)
}

// _query 内部查询函数，用于对一些增强搜索的递归调用
func _query(db *gorm.DB, query string, queryId string, opts ...RAGQueryOption) (chan *RAGSearchResult, error) {
	config := NewRAGQueryConfig(opts...)
	ctx := config.Ctx
	resultCh := make(chan *RAGSearchResult)

	sendRaw := func(msg *RAGSearchResult) {
		if config.MsgCallBack != nil {
			config.MsgCallBack(msg)
		}
		select {
		case resultCh <- msg:
		case <-ctx.Done():
			return
		}
	}

	sendMsg := func(msg string) {
		msgResult := &RAGSearchResult{
			Message:   fmt.Sprintf("[%s] %s", queryId, msg),
			Type:      RAGResultTypeMessage,
			Timestamp: time.Now().UnixMilli(),
		}
		sendRaw(msgResult)
	}

	sendMidResult := func(doc *Document, score float64, source string) {
		msgResult := &RAGSearchResult{
			Message:   fmt.Sprintf("[%s] 找到文档: %s", queryId, doc.ID),
			Data:      doc,
			Type:      RAGResultTypeMidResult,
			Score:     score,
			Source:    source,
			Timestamp: time.Now().UnixMilli(),
		}
		sendRaw(msgResult)
	}

	sendResult := func(doc *Document, score float64, source string) {
		msgResult := &RAGSearchResult{
			Message:   fmt.Sprintf("[%s] 最终结果: %s", queryId, doc.ID),
			Data:      doc,
			Type:      RAGResultTypeResult,
			Score:     score,
			Source:    source,
			Timestamp: time.Now().UnixMilli(),
		}
		sendRaw(msgResult)
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				sendMsg(fmt.Sprintf("查询过程中发生错误: %v", r))
			}
			close(resultCh)
		}()

		var searchQuery string = query

		// 如果启用增强搜索，生成假设文档
		if config.EnhancePlan != "" {
			switch config.EnhancePlan {
			case EnhancePlanHypotheticalAnswer:
				sendMsg("开始生成假设文档")
				if enhance, err := enhancesearch.HypotheticalAnswer(config.Ctx, query); err != nil {
					sendMsg(fmt.Sprintf("生成假设文档失败，使用原始查询: %v", err))
				} else {
					searchQuery = enhance
					displayEnhance := enhance
					if len(displayEnhance) > 100 {
						displayEnhance = displayEnhance[:100] + "..."
					}
					sendMsg(fmt.Sprintf("假设文档生成完成: %s", displayEnhance))
				}
			case EnhancePlanHypotheticalAnswerWithSplit:
				sendMsg("开始生成假设文档")
				if enhance, err := enhancesearch.HypotheticalAnswer(config.Ctx, query); err != nil {
					sendMsg(fmt.Sprintf("生成假设文档失败，使用原始查询: %v", err))
				} else {
					searchQuery = enhance
					displayEnhance := enhance
					if len(displayEnhance) > 100 {
						displayEnhance = displayEnhance[:100] + "..."
					}
					sendMsg(fmt.Sprintf("假设文档生成完成: %s", displayEnhance))
				}
				res, err := _query(db, searchQuery, queryId+"-"+strconv.Itoa(1), append(opts, WithRAGEnhance(EnhancePlanSplitQuery))...)
				if err != nil {
					sendMsg(fmt.Sprintf("查询失败: %v", err))
				} else {
					for result := range res {
						sendRaw(result)
					}
				}
			case EnhancePlanSplitQuery:
				sendMsg("开始拆分查询")
				if enhanceSentences, err := enhancesearch.SplitQuery(config.Ctx, query); err != nil {
					sendMsg(fmt.Sprintf("拆分查询失败，使用原始查询: %v", err))
				} else {
					swg := utils.NewSizedWaitGroup(config.Concurrent)
					for i, enhanceSentence := range enhanceSentences {
						swg.Add(1)
						go func() {
							defer swg.Done()
							res, err := _query(db, enhanceSentence, queryId+"-"+strconv.Itoa(i+1), append(opts, WithRAGEnhance(""))...)
							if err != nil {
								sendMsg(fmt.Sprintf("查询失败: %v", err))
							} else {
								for result := range res {
									sendRaw(result)
								}
							}
						}()
					}
					swg.Wait()
				}
			case EnhancePlanGeneralizeQuery:
				sendMsg("开始泛化增强查询")
				if enhance, err := enhancesearch.GeneralizeQuery(config.Ctx, query); err != nil {
					sendMsg(fmt.Sprintf("增强搜索失败，使用原始查询: %v", err))
				} else {
					searchQuery = enhance
					// 限制显示的增强查询长度
					displayEnhance := enhance
					if len(enhance) > 100 {
						displayEnhance = enhance[:100] + "..."
					}
					sendMsg(fmt.Sprintf("增强查询生成完成: %s", displayEnhance))
				}
			}
		}

		var targetCollections []string

		// 确定要搜索的集合
		if config.CollectionName != "" {
			// 指定了集合名称，只搜索该集合
			targetCollections = []string{config.CollectionName}
			sendMsg(fmt.Sprintf("指定搜索集合: %s", config.CollectionName))
		} else {
			// 自动发现相关集合
			sendMsg(fmt.Sprintf("未指定集合名称，将搜索最相关的 %d 个集合", config.CollectionNumLimit))
			collectionResults, err := QueryCollection(db, query)
			if err != nil {
				sendMsg(fmt.Sprintf("搜索集合失败: %v", err))
				return
			}

			sendMsg(fmt.Sprintf("共发现 %d 个相关集合", len(collectionResults)))

			// 根据分数阈值和数量限制筛选集合
			var filteredCollections []*SearchResult
			for _, result := range collectionResults {
				if result.Score >= config.CollectionScoreLimit {
					filteredCollections = append(filteredCollections, result)
				}
			}

			// 限制集合数量
			if len(filteredCollections) > config.CollectionNumLimit {
				filteredCollections = filteredCollections[:config.CollectionNumLimit]
			}

			// 提取集合名称
			for _, result := range filteredCollections {
				if collectionName, ok := result.Document.Metadata["collection_name"].(string); ok {
					targetCollections = append(targetCollections, collectionName)
					sendMsg(fmt.Sprintf("选择集合: %s (相似度: %.3f)", collectionName, result.Score))
				}
			}
		}

		if len(targetCollections) == 0 {
			sendMsg("没有找到符合条件的集合")
			return
		}

		sendMsg(fmt.Sprintf("开始在 %d 个集合中搜索，查询限制: %d", len(targetCollections), config.Limit))

		// 收集所有结果
		type ScoredResult struct {
			Document *Document
			Score    float64
			Source   string
		}

		var allResults []ScoredResult

		// 在每个集合中搜索
		for _, collectionName := range targetCollections {
			sendMsg(fmt.Sprintf("正在搜索集合: %s", collectionName))

			// 加载集合对应的RAG系统
			ragSystem, err := LoadCollection(db, collectionName)
			if err != nil {
				sendMsg(fmt.Sprintf("加载集合 %s 失败: %v", collectionName, err))
				continue
			}

			// 在该集合中执行搜索
			searchResults, err := ragSystem.QueryWithFilter(searchQuery, 1, config.Limit+5, func(key string, getDoc func() *Document) bool {
				if key == DocumentTypeCollectionInfo {
					return false
				}
				if config.Filter != nil {
					return config.Filter(key, getDoc)
				}
				return true
			})
			if err != nil {
				sendMsg(fmt.Sprintf("在集合 %s 中搜索失败: %v", collectionName, err))
				continue
			}

			sendMsg(fmt.Sprintf("在集合 %s 中找到 %d 个结果", collectionName, len(searchResults)))

			// 收集结果并标记来源
			for _, result := range searchResults {
				allResults = append(allResults, ScoredResult{
					Document: &result.Document,
					Score:    result.Score,
					Source:   collectionName,
				})

				// 发送中间结果
				sendMidResult(&result.Document, result.Score, collectionName)
			}
		}

		// 按分数排序所有结果
		sort.Slice(allResults, func(i, j int) bool {
			return allResults[i].Score > allResults[j].Score
		})

		sendMsg(fmt.Sprintf("共收集到 %d 个候选结果", len(allResults)))

		// 限制最终结果数量
		finalCount := config.Limit
		if len(allResults) < finalCount {
			finalCount = len(allResults)
		}

		// 发送最终结果
		for i := 0; i < finalCount; i++ {
			result := allResults[i]
			sendResult(result.Document, result.Score, result.Source)
		}

		sendMsg(fmt.Sprintf("查询完成，返回 %d 个最佳结果", finalCount))
	}()

	return resultCh, nil
}

// SimpleQuery 简化的RAG查询接口，直接返回结果
func SimpleQuery(db *gorm.DB, query string, limit int, opts ...RAGQueryOption) ([]*SearchResult, error) {
	// 添加限制选项
	options := append(opts, WithRAGLimit(limit), WithRAGEnhance(EnhancePlanHypotheticalAnswer))

	resultCh, err := Query(db, query, options...)
	if err != nil {
		return nil, err
	}

	var results []*SearchResult
	for result := range resultCh {
		if result.Type == RAGResultTypeResult && result.Data != nil {
			if doc, ok := result.Data.(*Document); ok {
				results = append(results, &SearchResult{
					Document: *doc,
					Score:    result.Score,
				})
			}
		}
	}

	return results, nil
}
