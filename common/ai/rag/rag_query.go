package rag

import (
	"context"
	"fmt"
	"io"
	"regexp"
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
	G.GraphAttribute("splines", "true") // 使用true而不是ortho，避免兼容性问题
	G.GraphAttribute("concentrate", "true")
	G.GraphAttribute("nodesep", "0.3")
	G.GraphAttribute("ranksep", "0.8") // 增加层级间距，确保TB效果明显
	G.GraphAttribute("compound", "true")
	G.GraphAttribute("clusterrank", "local")
	G.GraphAttribute("packmode", "cluster")
	// 强制TB布局的额外设置
	G.GraphAttribute("ordering", "out")
	G.GraphAttribute("newrank", "true")

	// 核心策略：使用UUID作为节点的唯一标识符
	// UUID -> 实体映射
	entityMap := make(map[string]*schema.ERModelEntity)
	// UUID -> 节点ID映射（用于DOT图中的节点引用）
	nodeMap := make(map[string]int)

	// 建立实体映射（UUID -> 实体）
	for _, entity := range e.Entities {
		entityMap[entity.Uuid] = entity
	}

	// 辅助函数：判断字符串是否像 UUID
	isUUIDish := func(s string) bool {
		match, _ := regexp.MatchString("^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$", s)
		return match
	}

	// 辅助函数：从实体中选择一个合适的显示名称
	pickDisplayName := func(entity *schema.ERModelEntity) string {
		// 优先使用 label 属性
		if label, ok := entity.Attributes["label"].(string); ok && label != "" && !isUUIDish(label) {
			return label
		}
		// 其次使用 title 属性
		if title, ok := entity.Attributes["title"].(string); ok && title != "" && !isUUIDish(title) {
			return title
		}
		// 再次使用 section_title 属性
		if sectionTitle, ok := entity.Attributes["section_title"].(string); ok && sectionTitle != "" && !isUUIDish(sectionTitle) {
			return sectionTitle
		}
		// 再次使用 english_title 属性
		if englishTitle, ok := entity.Attributes["english_title"].(string); ok && englishTitle != "" && !isUUIDish(englishTitle) {
			return englishTitle
		}
		// 再次使用 standard_number 属性
		if standardNumber, ok := entity.Attributes["standard_number"].(string); ok && standardNumber != "" && !isUUIDish(standardNumber) {
			return standardNumber
		}
		// 再次使用 name 属性
		if name, ok := entity.Attributes["name"].(string); ok && name != "" && !isUUIDish(name) {
			return name
		}
		// 如果EntityName也是UUID格式，使用EntityType
		if isUUIDish(entity.EntityName) {
			return entity.EntityType
		}
		// 最后使用 EntityName
		return entity.EntityName
	}

	// 按实体类型分子图
	subgraphMap := make(map[string]*dot.Graph)
	subgraphNodes := make(map[string][]string) // 子图类型 -> UUID列表

	getSubgraph := func(entityType string) *dot.Graph {
		if entityType == "" {
			entityType = "UNKNOWN"
		}
		if sg, exists := subgraphMap[entityType]; exists {
			return sg
		}
		sg := G.CreateSubGraph(fmt.Sprintf("cluster_%s", entityType))
		sg.GraphAttribute("label", entityType)
		sg.GraphAttribute("style", "filled")
		sg.GraphAttribute("fillcolor", "lightgray")
		// 为子图也设置TB布局
		sg.GraphAttribute("rankdir", "TB")
		sg.GraphAttribute("rank", "same") // 子图内节点尽量保持在同一层级
		subgraphMap[entityType] = sg
		return sg
	}

	// 需要查询缺失节点的函数
	ensureNodeExists := func(uuid string) bool {
		if _, exists := entityMap[uuid]; exists {
			return true
		}

		// 如果UUID找不到对应的实体，尝试从数据库加载
		db := consts.GetGormProfileDatabase()
		if db != nil {
			loadedEntity, err := yakit.GetEntityByIndex(db, uuid)
			if err == nil && loadedEntity != nil {
				entityMap[uuid] = loadedEntity
				log.Infof("动态加载实体: %s (%s)", uuid, loadedEntity.EntityType)
				return true
			}
		}
		log.Warnf("无法找到UUID对应的实体: %s", uuid)
		return false
	}

	// 创建节点 - 使用UUID作为节点标识符
	for _, entity := range e.Entities {
		uuid := entity.Uuid
		sg := getSubgraph(entity.EntityType)

		// 显示名称作为label，直接传给AddNode，避免重复设置
		display := pickDisplayName(entity)
		nodeID := sg.AddNode(display)
		nodeMap[uuid] = nodeID

		// 不再额外设置label，因为AddNode已经设置了

		// 记录到子图节点列表
		subgraphNodes[entity.EntityType] = append(subgraphNodes[entity.EntityType], uuid)

		// 暂时禁用额外属性添加，专注解决重复label问题
		// TODO: 重新设计属性添加逻辑
		/*
			importantAttrs := []string{"content", "term", "english_term", "standard_number"}
			attrCount := 0
			for _, key := range importantAttrs {
				if attrCount >= 1 {
					break
				}
				// 严格过滤可能导致重复label的属性
				keyLower := strings.ToLower(strings.TrimSpace(key))
				if keyLower == "label" || keyLower == "title" || keyLower == "section_title" ||
					keyLower == "english_title" || keyLower == "name" {
					continue
				}
				if value, ok := entity.Attributes[key]; ok {
					strValue := utils.InterfaceToString(value)
					if strValue != "" && len(strValue) < 80 && !isUUIDish(strValue) {
						sg.NodeAttribute(nodeID, key, strValue)
						attrCount++
					}
				}
			}
		*/
	}

	// 暂时禁用 same rank 功能，避免索引越界问题
	// TODO: 需要重新设计nodeID管理机制
	/*
		for entityType, uuids := range subgraphNodes {
			sg := subgraphMap[entityType]
			nodesPerRow := 4
			for i := 0; i < len(uuids); i += nodesPerRow {
				end := i + nodesPerRow
				if end > len(uuids) {
					end = len(uuids)
				}
				if end-i >= 2 {
					var nodeIDs []int
					for _, uuid := range uuids[i:end] {
						if nodeID, exists := nodeMap[uuid]; exists {
							nodeIDs = append(nodeIDs, nodeID)
						}
					}
					if len(nodeIDs) >= 2 {
						sg.MakeSameRank(nodeIDs[0], nodeIDs[1], nodeIDs[2:]...)
					}
				}
			}
		}
	*/

	// 添加关系边 - 使用UUID确保连接稳定性
	for _, rel := range e.Relationships {
		sourceUUID := rel.SourceTemporaryName
		targetUUID := rel.TargetTemporaryName

		// 确保源节点和目标节点都存在
		if !ensureNodeExists(sourceUUID) || !ensureNodeExists(targetUUID) {
			log.Warnf("跳过关系：找不到节点 %s -> %s (%s)", sourceUUID, targetUUID, rel.RelationshipType)
			continue
		}

		// 如果是新加载的节点，需要创建对应的DOT节点
		if _, exists := nodeMap[sourceUUID]; !exists {
			entity := entityMap[sourceUUID]
			sg := getSubgraph(entity.EntityType)
			display := pickDisplayName(entity)
			nodeID := sg.AddNode(display) // 直接使用display作为label，不再额外设置
			nodeMap[sourceUUID] = nodeID
		}

		if _, exists := nodeMap[targetUUID]; !exists {
			entity := entityMap[targetUUID]
			sg := getSubgraph(entity.EntityType)
			display := pickDisplayName(entity)
			nodeID := sg.AddNode(display) // 直接使用display作为label，不再额外设置
			nodeMap[targetUUID] = nodeID
		}

		// 添加边
		G.AddEdge(nodeMap[sourceUUID], nodeMap[targetUUID], rel.RelationshipType)
	}

	return G
}

// RAGQueryConfig RAG查询配置
type RAGQueryConfig struct {
	Ctx                  context.Context
	Limit                int // 单次子查询的结果限制。
	CollectionNumLimit   int
	CollectionNames      []string
	CollectionScoreLimit float64
	EnhancePlan          []string // 默认开启 HyDE 、 泛化查询 、拆分查询
	Filter               func(key string, getDoc func() *Document) bool
	Concurrent           int
	MsgCallBack          func(*RAGSearchResult)
	OnSubQueryStart      func(method string, query string)
	OnQueryFinish        func([]*ScoredResult)
	OnStatus             func(label string, value string)
	OnlyResults          bool // 仅返回最终结果，忽略中间结果和消息

	// On Stream Reader
	OnLogReader func(reader io.Reader)

	RAGSimilarityThreshold   float64 // RAG相似度限制
	EveryQueryResultCallback func(result *ScoredResult)
	RAGQueryType             []string

	LoadConfig []SQLiteVectorStoreHNSWOption
}

const (
	BasicPlan                              = "basic" // 空字符串表示不使用任何增强计划
	EnhancePlanHypotheticalAnswer          = "hypothetical_answer"
	EnhancePlanHypotheticalAnswerWithSplit = "hypothetical_answer_with_split"
	EnhancePlanSplitQuery                  = "split_query"
	EnhancePlanGeneralizeQuery             = "generalize_query"
	EnhancePlanExactKeywordSearch          = "exact_keyword_search"
)

func MethodVerboseName(i string) string {
	switch i {
	case EnhancePlanHypotheticalAnswer:
		return "HyDE"
	case EnhancePlanSplitQuery:
		return "拆分查询"
	case EnhancePlanGeneralizeQuery:
		return "泛化查询"
	case EnhancePlanExactKeywordSearch:
		return "泛化关键字"
	default:
		return "基础回答"
	}
}

// RAGQueryOption RAG查询选项
type RAGQueryOption func(*RAGQueryConfig)

// WithRAGLimit 设置查询结果限制
func WithRAGLimit(limit int) RAGQueryOption {
	return func(config *RAGQueryConfig) {
		config.Limit = limit
	}
}

func WithRAGDocumentType(documentType ...string) RAGQueryOption {
	return func(config *RAGQueryConfig) {
		if len(documentType) > 0 {
			config.RAGQueryType = documentType
		}
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
func WithRAGEnhance(enhancePlan ...string) RAGQueryOption {
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

func WithRAGLogReader(f func(reader io.Reader)) RAGQueryOption {
	return func(config *RAGQueryConfig) {
		config.OnLogReader = f
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

func WithRAGOnlyResults(onlyResults bool) RAGQueryOption {
	return func(config *RAGQueryConfig) {
		config.OnlyResults = onlyResults
	}
}

func WithRAGSimilarityThreshold(threshold float64) RAGQueryOption {
	return func(config *RAGQueryConfig) {
		config.RAGSimilarityThreshold = threshold
	}
}

func WithEveryQueryResultCallback(callback func(result *ScoredResult)) RAGQueryOption {
	return func(config *RAGQueryConfig) {
		config.EveryQueryResultCallback = callback
	}
}

func WithRAGOnQueryFinish(callback func([]*ScoredResult)) RAGQueryOption {
	return func(config *RAGQueryConfig) {
		config.OnQueryFinish = callback
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
		EnhancePlan:          []string{EnhancePlanHypotheticalAnswer, EnhancePlanGeneralizeQuery, EnhancePlanSplitQuery},
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

func (R RAGSearchResult) GetContent() string {
	return utils.InterfaceToString(R.Data)
}

func (R RAGSearchResult) GetSource() string {
	return R.Source
}

func (R RAGSearchResult) GetScore() float64 {
	return R.Score
}

type ScoredResult struct {
	Index       int64
	QueryMethod string
	QueryOrigin string
	Document    *Document
	Score       float64
	Source      string
}

func (s *ScoredResult) GetTitle() string {
	title, _ := s.Document.Metadata.GetTitle()
	return title
}

func (s *ScoredResult) GetType() string {
	return string(s.Document.Type)
}

func (s *ScoredResult) GetContent() string {
	return s.Document.Content
}

func (s *ScoredResult) GetSource() string {
	return s.Source
}

func (s *ScoredResult) GetScoreMethod() string {
	return s.QueryMethod
}

func (s *ScoredResult) GetScore() float64 {
	return s.Score
}

func (s *ScoredResult) GetUUID() string {
	return s.Document.ID
}

func QueryYakitProfile(query string, opts ...RAGQueryOption) (<-chan *RAGSearchResult, error) {
	return Query(consts.GetGormProfileDatabase(), query, opts...)
}

// Query 在RAG系统中搜索多个集合
// 这个函数直接在RAG级别进行查询，不依赖于知识库结构
func Query(db *gorm.DB, query string, opts ...RAGQueryOption) (<-chan *RAGSearchResult, error) {
	return _query(db, query, "1", opts...)
}

// _query 内部查询函数，用于对一些增强搜索的递归调用
func _query(db *gorm.DB, query string, queryId string, opts ...RAGQueryOption) (<-chan *RAGSearchResult, error) {
	config := NewRAGQueryConfig(opts...)
	ctx := config.Ctx
	resultCh := chanx.NewUnlimitedChan[*RAGSearchResult](ctx, 10)

	sendRaw := func(msg *RAGSearchResult) {
		if config.MsgCallBack != nil {
			config.MsgCallBack(msg)
		}
		if config.OnlyResults && msg.Type != RAGResultTypeResult {
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
		resultCh.SafeFeed(msg)
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

	sendERMAnalysisResult := func(ermResult *yakit.ERModel) {
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
		r, err := LoadCollectionEx(db, name, utils.InterfaceToSliceInterface(config.LoadConfig)...)
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

	chans := chanx.NewUnlimitedChan[*subQuery](ctx, 10)
	status("STATUS", "开始创建子查询（强化）")
	wg := new(sync.WaitGroup)

	startSubQuery(BasicPlan, query) // 基础查询
	chans.FeedBlock(&subQuery{
		Method: BasicPlan,
		Query:  query,
	})

	if utils.StringArrayContains(config.EnhancePlan, EnhancePlanHypotheticalAnswer) {
		wg.Add(1)
		go func() {
			method := EnhancePlanHypotheticalAnswer
			defer func() {
				log.Infof("end to sub query, method: %s, query: %s", method, query)
				wg.Done()
			}()
			log.Infof("start to create sub query for enhance plan: %s", config.EnhancePlan)
			start := time.Now()
			result, err := enhancesearch.HypotheticalAnswer(ctx, query)
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
	}

	if utils.StringArrayContains(config.EnhancePlan, EnhancePlanGeneralizeQuery) {
		wg.Add(1)
		go func() {
			method := EnhancePlanGeneralizeQuery
			defer func() {
				log.Infof("end to sub query, method: %s, query: %s", method, query)
				wg.Done()
			}()
			log.Infof("start to create sub query for enhance plan: %s", config.EnhancePlan)
			start := time.Now()
			results, err := enhancesearch.GeneralizeQuery(ctx, query)
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
	}

	if utils.StringArrayContains(config.EnhancePlan, EnhancePlanSplitQuery) {
		wg.Add(1)
		go func() {
			method := EnhancePlanSplitQuery
			defer func() {
				log.Infof("end to sub query, method: %s, query: %s", method, query)
				wg.Done()
			}()
			log.Infof("start to create sub query for enhance plan: %s", config.EnhancePlan)
			start := time.Now()
			results, err := enhancesearch.SplitQuery(ctx, query)
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
	}

	if utils.StringArrayContains(config.EnhancePlan, EnhancePlanExactKeywordSearch) {
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
			results, err := enhancesearch.ExtractKeywords(ctx, query)
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
	}

	go func() {
		wg.Wait()
		log.Info("end to create sub query")
		chans.Close()
	}()

	go func() {
		defer func() {
			resultCh.Close()
		}()
		// 收集所有结果

		var offset int64 = 0
		var allResults []*ScoredResult
		var storeMutex sync.Mutex
		var enhanceSubQuery int64 = 0
		var resultRecorder = map[string]struct{}{}

		var nodesRecorder = make(map[string]struct{})

		storeResults := func(source string, result SearchResult, query *subQuery) (int64, bool) {
			res := &ScoredResult{
				QueryMethod: query.Method,
				QueryOrigin: query.Query,
				Document:    &result.Document,
				Score:       result.Score,
				Source:      source,
			}

			if config.EveryQueryResultCallback != nil {
				config.EveryQueryResultCallback(res)
			}

			storeMutex.Lock()
			defer storeMutex.Unlock()

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

			_, exist := resultRecorder[result.Document.ID]
			if !exist {
				resultRecorder[result.Document.ID] = struct{}{}
			}

			offset += 1
			res.Index = offset
			allResults = append(allResults, res)
			return offset, exist
		}

		var ragQueryCostSum float64 = 0
		var ragAtomicQueryCount int64 = 0
		var queryCostMutex sync.Mutex
		updateQueryAvgCost := func(cost float64) {
			queryCostMutex.Lock()
			defer queryCostMutex.Unlock()
			ragQueryCostSum += cost
			ragAtomicQueryCount++
			avgCost := 0.0
			if ragAtomicQueryCount > 0 {
				avgCost = ragQueryCostSum / float64(ragAtomicQueryCount)
			}
			status("RAG原子查询平均用时", fmt.Sprintf("%.2fs", avgCost))
		}

		for subquery := range chans.OutputChannel() {
			enhanceSubQuery++
			status("强化查询", fmt.Sprint(enhanceSubQuery))

			logReader, logWriter := utils.NewPipe()
			if config.OnLogReader != nil {
				go func() {
					defer func() {
						if err := recover(); err != nil {
							log.Warnf("[OnLogReader] panic: %v", err)
						}
					}()
					config.OnLogReader(logReader)
				}()
			} else {
				go func() {
					io.Copy(io.Discard, logReader)
				}()
			}

			logWriter.WriteString(fmt.Sprintf("[增强方案:%v]：", MethodVerboseName(subquery.Method)))
			logWriter.WriteString(fmt.Sprintf("%v", subquery.Query))

			currentSearchCount := int64(0)
			var queryWg sync.WaitGroup
			for _, ragSystem := range cols { // 一个子查询的不同集合查询至少是可以并行的，
				queryWg.Add(1)
				go func() {
					defer queryWg.Done()
					// 在该集合中执行搜索
					log.Infof("start to query %v with subquery: %v", ragSystem.Name, utils.ShrinkString(subquery.Query, 100))
					queryStart := time.Now()

					if subquery.ExactSearch {
						searchResultsChan, err := ragSystem.FuzzRawSearch(ctx, subquery.Query, config.Limit)
						if err != nil {
							log.Infof("start to keyword query [%v] failed: %v", ragSystem.Name, err)
							return
						}
						for result := range searchResultsChan {
							atomic.AddInt64(&currentSearchCount, 1)
							idx, exist := storeResults(ragSystem.Name, result, subquery)
							if exist {
								continue
							}
							sendMidResult(idx, subquery.Method, subquery.Query, &result.Document, result.Score, ragSystem.Name)
						}
					} else {
						searchResults, err := ragSystem.QueryWithFilter(subquery.Query, 1, config.Limit, func(key string, getDoc func() *Document) bool {
							if key == DocumentTypeCollectionInfo {
								return false
							}

							if len(config.RAGQueryType) > 0 && !utils.StringArrayContains(config.RAGQueryType, string(getDoc().Type)) {
								return false

							}

							if config.Filter != nil {
								return config.Filter(key, getDoc)
							}
							return true
						})
						if err != nil {
							log.Infof("start to query ragsystem[%v] failed: %v", ragSystem.Name, err)
							return
						}

						if searchResults != nil {
							log.Infof("query ragsystem[%v] with subquery: %v got %d results", ragSystem.Name, utils.ShrinkString(subquery.Query, 100), len(searchResults))
						} else {
							log.Infof("query ragsystem[%v] with subquery: %v got 0 result", ragSystem.Name, utils.ShrinkString(subquery.Query, 100))
						}

						// 只要没报错，就记录时间
						updateQueryAvgCost(time.Since(queryStart).Seconds())
						for _, result := range searchResults {
							if config.RAGSimilarityThreshold > 0 && result.Score < config.RAGSimilarityThreshold { // rag query 相似度过滤
								continue
							}
							atomic.AddInt64(&currentSearchCount, 1)
							idx, exist := storeResults(ragSystem.Name, result, subquery)
							if exist {
								continue
							}
							sendMidResult(idx, subquery.Method, subquery.Query, &result.Document, result.Score, ragSystem.Name)
						}
					}

				}()
			}
			queryWg.Wait()
			logWriter.WriteString("\n\n查询完成，结果数：" + fmt.Sprint(currentSearchCount) + "\n")
			logWriter.Close()
			if currentSearchCount > 0 {
				status(subquery.Method+"结果数", fmt.Sprint(currentSearchCount))
			}
		}

		if config.OnQueryFinish != nil {
			config.OnQueryFinish(allResults)
		}

		sortedResults := utils.RRFRankWithDefaultK[*ScoredResult](allResults)

		sendMsg(fmt.Sprintf("共收集到 %d 个候选结果", len(allResults)))

		// 限制最终结果数量
		finalCount := config.Limit
		if len(sortedResults) < finalCount {
			finalCount = len(sortedResults)
		}

		// 发送最终结果
		for i := 0; i < finalCount; i++ {
			result := sortedResults[i]
			sendResult(result.Index, result.QueryMethod, result.QueryOrigin, result.Document, result.Score, result.Source)
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
			ermResult, err := yakit.EntityRelationshipFind(db, entities, 4)
			if err != nil {
				return
			}
			// 发送 ERM 分析结果
			sendERMAnalysisResult(ermResult)
			// 生成并发送 Dot 图
			sendMsg(fmt.Sprintf("生成知识图 Dot 图，共 %d 个实体，%d 个关系", len(ermResult.Entities), len(ermResult.Relationships)))
			dotGraph := ermResult.Dot()
			sendDotGraphResult(dotGraph)

			sendMsg(fmt.Sprintf("ERM 分析完成，生成 %d 个实体和 %d 个关系的知识图", len(ermResult.Entities), len(ermResult.Relationships)))
		}
		sendMsg(fmt.Sprintf("查询完成，返回 %d 个最佳结果", finalCount))
	}()
	return resultCh.OutputChannel(), nil
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
