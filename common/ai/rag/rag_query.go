package rag

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
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
	"github.com/yaklang/yaklang/common/utils/dot"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// RAG 搜索结果类型常量
const (
	RAGResultTypeMessage   = "message"
	RAGResultEntity        = "entity"
	RAGResultTypeMidResult = "mid_result"
	RAGResultTypeResult    = "result"
	RAGResultTypeError     = "error"
	RAGResultTypeERM       = "erm_analysis"
	RAGResultTypeDotGraph  = "dot_graph"
)

// SimpleERMAnalysisResult 简化的 ERM 分析结果结构体，避免导入循环
type SimpleERMAnalysisResult struct {
	Entities      []*schema.ERModelEntity `json:"entities"`
	Relationships []*SimpleRelationship   `json:"relationships"`
	OriginalData  []byte                  `json:"original_data"`
}

// SimpleRelationship 简化的关系结构体
type SimpleRelationship struct {
	SourceTemporaryName     string `json:"source_temporary_name"`
	TargetTemporaryName     string `json:"target_temporary_name"`
	RelationshipType        string `json:"relationship_type"`
	RelationshipTypeVerbose string `json:"relationship_type_verbose"`
	DecorationAttributes    string `json:"decoration_attributes"`
}

// GenerateDotGraph 生成 Dot 图 (默认从上到下布局)
func (e *SimpleERMAnalysisResult) GenerateDotGraph() *dot.Graph {
	return e.GenerateDotGraphWithDirection("TB")
}

// GenerateDotGraphWithDirection 生成指定方向的 Dot 图
// 支持的方向：
// - "TB": 从上到下 (Top to Bottom)
// - "BT": 从下到上 (Bottom to Top)
// - "LR": 从左到右 (Left to Right)
// - "RL": 从右到左 (Right to Left)
func (e *SimpleERMAnalysisResult) GenerateDotGraphWithDirection(direction string) *dot.Graph {
	G := dot.New()
	G.MakeDirected()
	// 设置布局方向，默认从上到下
	if direction == "" {
		direction = "TB"
	}
	G.GraphAttribute("rankdir", direction)
	// 优化全局布局
	G.GraphAttribute("splines", "true")
	G.GraphAttribute("concentrate", "true")
	G.GraphAttribute("nodesep", "0.5")
	G.GraphAttribute("ranksep", "1.0 equally")
	G.GraphAttribute("compound", "true")

	// 用于生成唯一名称的计数器
	nameCounter := 1
	nameMap := make(map[string]string)

	// 清理名称，确保是有效的 dot 标识符
	cleanName := func(name string) string {
		if name == "" {
			// 如果名称为空，使用 a1, a2, a3... 格式
			cleaned := fmt.Sprintf("a%d", nameCounter)
			nameCounter++
			return cleaned
		}

		// 清理特殊字符，只保留字母、数字、下划线
		cleaned := ""
		for _, r := range name {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
				cleaned += string(r)
			} else {
				cleaned += "_"
			}
		}

		// 确保以字母开头
		if len(cleaned) == 0 || (cleaned[0] >= '0' && cleaned[0] <= '9') {
			cleaned = fmt.Sprintf("a%d_%s", nameCounter, cleaned)
			nameCounter++
		}

		return cleaned
	}

	// 获取或创建唯一名称
	getUniqueName := func(originalName string) string {
		cleaned := cleanName(originalName)
		if _, exists := nameMap[cleaned]; exists {
			// 如果名称冲突，添加后缀
			counter := 1
			for {
				newName := fmt.Sprintf("%s_%d", cleaned, counter)
				if _, exists := nameMap[newName]; !exists {
					nameMap[newName] = originalName
					return newName
				}
				counter++
			}
		} else {
			nameMap[cleaned] = originalName
			return cleaned
		}
	}

	// UUID-like 检测
	uuidLike := regexp.MustCompile(`(?i)[0-9a-f]{8}[_-][0-9a-f]{4}[_-][0-9a-f]{4}[_-][0-9a-f]{4}[_-][0-9a-f]{12}`)
	isUUIDish := func(s string) bool {
		// 直接 UUID 或者前缀+UUID
		if uuidLike.MatchString(s) {
			return true
		}
		// 类似 a12_xxx_uuid 这种
		return strings.Count(s, "_") >= 4 && uuidLike.MatchString(strings.TrimPrefix(s, strings.SplitN(s, "_", 2)[0]+"_"))
	}

	// 选择友好展示名称
	pickDisplayName := func(ent *schema.ERModelEntity) string {
		candidates := []string{
			utils.InterfaceToString(ent.Attributes["label"]),
			utils.InterfaceToString(ent.Attributes["title"]),
			utils.InterfaceToString(ent.Attributes["section_title"]),
			utils.InterfaceToString(ent.Attributes["english_title"]),
			utils.InterfaceToString(ent.Attributes["standard_number"]),
			utils.InterfaceToString(ent.Attributes["name"]),
			utils.InterfaceToString(ent.Attributes["qualified_name"]),
			utils.InterfaceToString(ent.Attributes["qualifiedName"]),
		}
		for _, v := range candidates {
			v = strings.TrimSpace(v)
			if v != "" {
				return utils.ShrinkString(v, 120)
			}
		}
		return ent.EntityName
	}

	// 按实体类型分子图，并收集节点用于 same-rank 分组
	subgraphMap := make(map[string]*dot.Graph)
	subgraphNodes := make(map[*dot.Graph][]int)
	getSubgraph := func(label string) *dot.Graph {
		if label == "" {
			label = "UNKNOWN"
		}
		if sg, ok := subgraphMap[label]; ok {
			return sg
		}
		sg := G.CreateSubGraph(label)
		sg.GraphAttribute("label", label)
		subgraphMap[label] = sg
		return sg
	}

	for _, entity := range e.Entities {
		nodeName := getUniqueName(entity.EntityName)
		sg := getSubgraph(entity.EntityType)
		n := sg.AddNode(nodeName)
		subgraphNodes[sg] = append(subgraphNodes[sg], n)

		// 合理的可读 label（避免 UUID）
		display := pickDisplayName(entity)
		if isUUIDish(display) {
			// 若仍像 UUID，退回 entity type
			display = entity.EntityType
		}
		sg.NodeAttribute(n, "label", display)

		for key, value := range entity.Attributes {
			// 将数组/切片安全地规整为一个字符串
			values := utils.InterfaceToStringSlice(value)
			var attrVal string
			if len(values) > 1 {
				attrVal = strings.Join(values, "; ")
			} else if len(values) == 1 {
				attrVal = values[0]
			} else {
				attrVal = utils.InterfaceToString(value)
			}
			// 避免覆盖我们设置的友好 label
			if strings.EqualFold(key, "label") {
				continue
			}
			sg.NodeAttribute(n, key, attrVal)
		}
	}

	// same-rank：每行尽量 10 个
	for sg, nodes := range subgraphNodes {
		for i := 0; i < len(nodes); i += 10 {
			end := i + 10
			if end > len(nodes) {
				end = len(nodes)
			}
			if end-i >= 2 {
				n1 := nodes[i]
				n2 := nodes[i+1]
				others := nodes[i+2 : end]
				sg.MakeSameRank(n1, n2, others...)
			}
		}
	}

	for _, relationship := range e.Relationships {
		sourceName := getUniqueName(relationship.SourceTemporaryName)
		targetName := getUniqueName(relationship.TargetTemporaryName)
		G.AddEdgeByLabel(sourceName, targetName, relationship.RelationshipType)
	}

	return G
}

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

	sendERMAnalysisResult := func(ermResult *SimpleERMAnalysisResult) {
		msgResult := &RAGSearchResult{
			Message:   fmt.Sprintf("[%s] ERM 分析结果: %d 个实体, %d 个关系", queryId, len(ermResult.Entities), len(ermResult.Relationships)),
			Data:      ermResult,
			Type:      RAGResultTypeERM,
			Timestamp: time.Now().UnixMilli(),
		}
		sendRaw(msgResult)
	}

	sendDotGraphResult := func(dotGraph *dot.Graph) {
		dotString := dotGraph.GenerateDOTString()
		msgResult := &RAGSearchResult{
			Message:   fmt.Sprintf("[%s] 知识图 Dot 图", queryId),
			Data:      dotString,
			Type:      RAGResultTypeDotGraph,
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

		// 收集所有实体
		var entities []*schema.ERModelEntity
		entityMap := make(map[string]*schema.ERModelEntity)

		for nodeId := range nodesRecorder {
			entity, err := yakit.GetEntityByIndex(db, nodeId)
			if err != nil {
				log.Error(err)
				continue
			}
			entities = append(entities, entity)
			entityMap[entity.Uuid] = entity
			sendEntityResult(entity)
		}

		// 生成 ERM 分析结果
		if len(entities) > 0 {
			sendMsg(fmt.Sprintf("开始生成 ERM 分析结果，共 %d 个实体", len(entities)))

			// 收集实体之间的关系（BFS 最多 4 层）
			var relationships []*SimpleRelationship
			relationshipSet := make(map[string]bool) // 避免重复关系

			// 建立便捷函数：确保实体已加载
			ensureEntity := func(uuid string) *schema.ERModelEntity {
				if e, ok := entityMap[uuid]; ok {
					return e
				}
				ent, err := yakit.GetEntityByIndex(db, uuid)
				if err != nil || ent == nil {
					log.Warnf("load entity by uuid[%s] failed: %v", uuid, err)
					return nil
				}
				entityMap[uuid] = ent
				entities = append(entities, ent)
				return ent
			}

			type qItem struct {
				uuid  string
				depth int
			}
			var queue []qItem
			visited := make(map[string]bool)
			const maxDepth = 4

			for _, e := range entities {
				queue = append(queue, qItem{uuid: e.Uuid, depth: 0})
			}

			for len(queue) > 0 {
				item := queue[0]
				queue = queue[1:]
				if visited[item.uuid] {
					continue
				}
				visited[item.uuid] = true
				if item.depth >= maxDepth {
					continue
				}

				cur := ensureEntity(item.uuid)
				if cur == nil {
					continue
				}

				// 传出关系
				if outgoingRels, err := yakit.GetOutgoingRelationships(db, cur); err == nil {
					for _, rel := range outgoingRels {
						sid := rel.SourceEntityIndex
						tid := rel.TargetEntityIndex
						src := ensureEntity(sid)
						tgt := ensureEntity(tid)
						if src == nil || tgt == nil {
							continue
						}
						relKey := fmt.Sprintf("%s-%s-%s", sid, tid, rel.RelationshipType)
						if !relationshipSet[relKey] {
							relationshipSet[relKey] = true
							relationships = append(relationships, &SimpleRelationship{
								SourceTemporaryName:     sid,
								TargetTemporaryName:     tid,
								RelationshipType:        rel.RelationshipType,
								RelationshipTypeVerbose: rel.RelationshipTypeVerbose,
								DecorationAttributes:    fmt.Sprintf("source:%s,target:%s", src.EntityName, tgt.EntityName),
							})
						}
						if !visited[tid] {
							queue = append(queue, qItem{uuid: tid, depth: item.depth + 1})
						}
					}
				} else {
					log.Errorf("获取实体 %s 的传出关系失败: %v", cur.Uuid, err)
				}

				// 传入关系
				if incomingRels, err := yakit.GetIncomingRelationships(db, cur); err == nil {
					for _, rel := range incomingRels {
						sid := rel.SourceEntityIndex
						tid := rel.TargetEntityIndex
						src := ensureEntity(sid)
						tgt := ensureEntity(tid)
						if src == nil || tgt == nil {
							continue
						}
						relKey := fmt.Sprintf("%s-%s-%s", sid, tid, rel.RelationshipType)
						if !relationshipSet[relKey] {
							relationshipSet[relKey] = true
							relationships = append(relationships, &SimpleRelationship{
								SourceTemporaryName:     sid,
								TargetTemporaryName:     tid,
								RelationshipType:        rel.RelationshipType,
								RelationshipTypeVerbose: rel.RelationshipTypeVerbose,
								DecorationAttributes:    fmt.Sprintf("source:%s,target:%s", src.EntityName, tgt.EntityName),
							})
						}
						if !visited[sid] {
							queue = append(queue, qItem{uuid: sid, depth: item.depth + 1})
						}
					}
				} else {
					log.Errorf("获取实体 %s 的传入关系失败: %v", cur.Uuid, err)
				}
			}

			// 创建 ERMAnalysisResult
			ermResult := &SimpleERMAnalysisResult{
				Entities:      entities,
				Relationships: relationships,
				OriginalData:  []byte(fmt.Sprintf("Query: %s", query)),
			}

			// 发送 ERM 分析结果
			sendERMAnalysisResult(ermResult)

			// 生成并发送 Dot 图
			sendMsg(fmt.Sprintf("生成知识图 Dot 图，共 %d 个实体，%d 个关系", len(entities), len(relationships)))
			dotGraph := ermResult.GenerateDotGraph()
			sendDotGraphResult(dotGraph)

			sendMsg(fmt.Sprintf("ERM 分析完成，生成 %d 个实体和 %d 个关系的知识图", len(entities), len(relationships)))
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
