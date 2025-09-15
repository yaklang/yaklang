package rag

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag/enhancesearch"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// RAG 搜索结果类型常量
const (
	RAGResultTypeMessage   = "message"
	RAGResultEntity        = "entity"
	RAGResultTypeMidResult = "mid_result"
	RAGResultTypeResult    = "result"
	RAGResultTypeError     = "error"
)

// RAGQueryConfig RAG查询配置
type RAGQueryConfig struct {
	Ctx                  context.Context
	Limit                int
	CollectionNumLimit   int
	CollectionNames      []string
	CollectionScoreLimit float64
	EnhancePlan          string
	Filter               func(key string, getDoc func() *Document) bool
	Concurrent           int
	MsgCallBack          func(*RAGSearchResult)
	OnSubQueryStart      func(method string, query string)
	OnStatus             func(label string, value string)
}

const (
	EnhancePlanHypotheticalAnswer          = "hypothetical_answer"
	EnhancePlanHypotheticalAnswerWithSplit = "hypothetical_answer_with_split"
	EnhancePlanSplitQuery                  = "split_query"
	EnhancePlanGeneralizeQuery             = "generalize_query"
	EnhancePlanExactKeywordSearch          = "exact_keyword_search"
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
		config.CollectionNames = append(config.CollectionNames, collectionName)
	}
}

func WithRAGQueryStatus(i func(label string, i any, tags ...string)) RAGQueryOption {
	return func(c *RAGQueryConfig) {
		c.OnStatus = func(label string, value string) {
			if i == nil {
				return
			}
			i(label, value)
		}
	}
}

func WithRAGCollectionNames(collectionNames ...string) RAGQueryOption {
	return func(config *RAGQueryConfig) {
		config.CollectionNames = append(config.CollectionNames, collectionNames...)
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
	Message     string      `json:"message"`
	Data        interface{} `json:"data"`
	Type        string      `json:"type"`      // message, mid_result, result
	Score       float64     `json:"score"`     // 相似度分数
	Source      string      `json:"source"`    // 结果来源（集合名称）
	Timestamp   int64       `json:"timestamp"` // 时间戳
	QueryMethod string      `json:"query_method"`
	QueryOrigin string      `json:"query_origin"`
	Index       int64       `json:"index"`
}

func QueryYakitProfile(query string, opts ...RAGQueryOption) (chan *RAGSearchResult, error) {
	return Query(consts.GetGormProfileDatabase(), query, opts...)
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

	sendMidResult := func(idx int64, queryMethod string, query string, doc *Document, score float64, source string) {
		msgResult := &RAGSearchResult{
			Message:     fmt.Sprintf("[%s] 找到文档: %s", queryId, doc.ID),
			Data:        doc,
			Type:        RAGResultTypeMidResult,
			Score:       score,
			Source:      source,
			Timestamp:   time.Now().UnixMilli(),
			QueryMethod: queryMethod,
			QueryOrigin: query,
			Index:       idx,
		}
		sendRaw(msgResult)
	}

	sendEntityResult := func(i *schema.ERModelEntity) {
		msgResult := &RAGSearchResult{
			Message:   fmt.Sprintf("[%s] 找到知识实体: %s", queryId, i.Uuid),
			Data:      i,
			Type:      RAGResultEntity,
			Timestamp: time.Now().UnixMilli(),
		}
		sendRaw(msgResult)
	}

	sendResult := func(idx int64, queryMethod string, query string, doc *Document, score float64, source string) {
		msgResult := &RAGSearchResult{
			Message:     fmt.Sprintf("[%s] 最终结果: %s", queryId, doc.ID),
			Data:        doc,
			Type:        RAGResultTypeResult,
			Score:       score,
			Source:      source,
			Timestamp:   time.Now().UnixMilli(),
			Index:       idx,
			QueryMethod: queryMethod,
			QueryOrigin: query,
		}
		sendRaw(msgResult)
	}

	startSubQuery := func(method string, query string) {
		log.Infof("start to sub query, method: %s, query: %s", method, query)
		if config.OnSubQueryStart != nil {
			config.OnSubQueryStart(method, query)
		}
	}

	status := func(label string, value string) {
		if config.OnStatus != nil {
			config.OnStatus(label, value)
		}
	}

	status("STATUS", "初始化RAG查询配置")
	var cols []*RAGSystem
	start := time.Now()
	for _, name := range ListCollections(db) {
		log.Infof("start to load collection %v", name)
		r, err := LoadCollection(db, name)
		if err != nil {
			log.Warnf("load collection %s failed: %v", name, err)
			continue
		}
		cols = append(cols, r)
	}
	status("RAG预加载用时", fmt.Sprintf("%.2fs", time.Since(start).Seconds()))

	type subQuery struct {
		Method      string
		Query       string
		ExactSearch bool
	}

	chans := chanx.NewUnlimitedChan[*subQuery](config.Ctx, 10)
	status("STATUS", "开始创建子查询（强化）")
	wg := new(sync.WaitGroup)
	wg.Add(1)
	go func() {
		defer func() {
			log.Infof("end to sub query, method: %s, query: %s", queryId, query)
			wg.Done()
		}()
		log.Infof("start to create sub query for enhance plan: %s", config.EnhancePlan)
		start := time.Now()
		result, err := enhancesearch.HypotheticalAnswer(config.Ctx, query)
		if err != nil {
			log.Warnf("enhance [HypotheticalAnswer] query failed: %v", err)
			return
		}
		status("HyDE强化用时", fmt.Sprintf("%.2fs", time.Since(start).Seconds()))
		if result != "" {
			startSubQuery(EnhancePlanHypotheticalAnswer, result)
			chans.FeedBlock(&subQuery{
				Method: EnhancePlanHypotheticalAnswer,
				Query:  result,
			})
		}
	}()

	wg.Add(1)
	go func() {
		method := EnhancePlanGeneralizeQuery
		defer func() {
			log.Infof("end to sub query, method: %s, query: %s", method, query)
			wg.Done()
		}()
		log.Infof("start to create sub query for enhance plan: %s", config.EnhancePlan)
		start := time.Now()
		results, err := enhancesearch.GeneralizeQuery(config.Ctx, query)
		if err != nil {
			log.Warnf("enhance [GeneralizeQuery] query failed: %v", err)
			return
		}
		status("泛化查询用时", fmt.Sprintf("%.2fs", time.Since(start).Seconds()))
		for _, result := range results {
			if result != "" {
				startSubQuery(EnhancePlanGeneralizeQuery, result)
				chans.FeedBlock(&subQuery{
					Method: EnhancePlanGeneralizeQuery,
					Query:  result,
				})
			}
		}
	}()

	wg.Add(1)
	go func() {
		method := EnhancePlanSplitQuery
		defer func() {
			log.Infof("end to sub query, method: %s, query: %s", method, query)
			wg.Done()
		}()
		log.Infof("start to create sub query for enhance plan: %s", config.EnhancePlan)
		start := time.Now()
		results, err := enhancesearch.SplitQuery(config.Ctx, query)
		if err != nil {
			log.Warnf("enhance [GeneralizeQuery] query failed: %v", err)
			return
		}
		status("拆分子查询用时", fmt.Sprintf("%.2fs", time.Since(start).Seconds()))
		for _, result := range results {
			if result != "" {
				startSubQuery(EnhancePlanSplitQuery, result)
				chans.FeedBlock(&subQuery{
					Method: EnhancePlanSplitQuery,
					Query:  result,
				})
			}
		}
	}()

	wg.Add(1)
	go func() {
		method := EnhancePlanExactKeywordSearch
		defer func() {
			log.Infof("end to sub query, method: %s, query: %s", queryId, query)
			wg.Done()
		}()
		log.Infof("start to create sub query for enhance plan: %s", method)
		start := time.Now()
		// 直接使用原始查询作为精确关键词搜索
		results, err := enhancesearch.ExtractKeywords(config.Ctx, query)
		if err != nil {
			log.Warnf("enhance [ExtractKeywords] query failed: %v", err)
			return
		}
		status("关键词提取用时", fmt.Sprintf("%.2fs", time.Since(start).Seconds()))
		for _, result := range results {
			if result != "" {
				startSubQuery(method, result)
				chans.FeedBlock(&subQuery{
					Method:      method,
					Query:       result,
					ExactSearch: true,
				})
			}
		}
	}()

	go func() {
		wg.Wait()
		log.Info("end to create sub query")
		chans.Close()
	}()

	go func() {
		defer func() {
			close(resultCh)
		}()
		// 收集所有结果
		type ScoredResult struct {
			Index       int64
			QueryMethod string
			QueryOrigin string
			Document    *Document
			Score       float64
			Source      string
		}

		var offset int64 = 0
		var allResults []ScoredResult
		var enhanceSubQuery int64 = 0
		var ragQueryCostSum float64 = 0
		var ragAtomicQueryCount int64 = 0
		var resultRecorder = map[string]float64{}

		var nodesRecorder = make(map[string]struct{})

		for subquery := range chans.OutputChannel() {
			enhanceSubQuery++
			status("强化查询", fmt.Sprint(enhanceSubQuery))

			currentSearchCount := 0
			for _, ragSystem := range cols {
				// 在该集合中执行搜索
				log.Infof("start to query %v with subquery: %v", ragSystem.Name, utils.ShrinkString(subquery.Query, 100))
				queryStart := time.Now()

				if subquery.ExactSearch {
					status("[TODO]精确关键词搜索", "TODO")
					continue
				}

				searchResults, err := ragSystem.QueryWithFilter(subquery.Query, 1, config.Limit+5, func(key string, getDoc func() *Document) bool {
					if key == DocumentTypeCollectionInfo {
						return false
					}
					if config.Filter != nil {
						return config.Filter(key, getDoc)
					}
					return true
				})
				if err != nil {
					log.Infof("start to query ragsystem[%v] failed: %v", ragSystem.Name, err)
					continue
				}

				if len(searchResults) > 0 {
					cost := time.Since(queryStart).Seconds()
					ragQueryCostSum += cost
					ragAtomicQueryCount++
					avgCost := 0.0
					if ragAtomicQueryCount > 0 {
						avgCost = ragQueryCostSum / float64(ragAtomicQueryCount)
					}
					status("RAG原子查询平均用时", fmt.Sprintf("%.2fs", avgCost))
				}

				if searchResults != nil {
					log.Infof("query ragsystem[%v] with subquery: %v got %d results", ragSystem.Name, utils.ShrinkString(subquery.Query, 100), len(searchResults))
				} else {
					log.Infof("query ragsystem[%v] with subquery: %v got 0 result", ragSystem.Name, utils.ShrinkString(subquery.Query, 100))
				}

				// 收集结果并标记来源
				for _, result := range searchResults {
					docId := result.Document.ID
					if score, ok := resultRecorder[docId]; ok {
						if score < result.Score {
							resultRecorder[docId] = result.Score
						}
						continue
					}
					resultRecorder[docId] = result.Score

					currentSearchCount++
					idx := atomic.AddInt64(&offset, 1)
					allResults = append(allResults, ScoredResult{
						Index:       idx,
						QueryMethod: subquery.Method,
						QueryOrigin: subquery.Query,
						Document:    &result.Document,
						Score:       result.Score,
						Source:      ragSystem.Name,
					})
					// 发送中间结果
					sendMidResult(idx, subquery.Method, subquery.Query, &result.Document, result.Score, ragSystem.Name)

					// send nodes from erm
					if ret := result.Document.EntityUUID; ret != "" {
						if _, ok := nodesRecorder[ret]; !ok {
							nodesRecorder[ret] = struct{}{}
						}
					}
					if ret := result.Document.RelatedEntities; len(ret) > 0 {
						for _, id := range ret {
							if _, ok := nodesRecorder[id]; !ok {
								nodesRecorder[id] = struct{}{}
							}
						}
					}
				}
			}

			if currentSearchCount > 0 {
				status(subquery.Method+"结果数", fmt.Sprint(currentSearchCount))
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
			score := result.Score
			if storedScore, ok := resultRecorder[result.QueryMethod]; ok {
				if storedScore > score {
					score = storedScore
				}
			}
			sendResult(result.Index, result.QueryMethod, result.QueryOrigin, result.Document, score, result.Source)
		}

		status("RAG-to-Entity", fmt.Sprintf("关联到%d个知识实体", len(nodesRecorder)))
		for nodeId := range nodesRecorder {
			entity, err := yakit.GetEntityByIndex(db, nodeId)
			if err != nil {
				log.Error(err)
				continue
			}
			sendEntityResult(entity)
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
