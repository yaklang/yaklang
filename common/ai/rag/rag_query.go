package rag

import (
	"fmt"
	"regexp"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
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

type RAGQueryConfig = vectorstore.CollectionQueryConfig

const (
	BasicPlan                              = "basic" // 空字符串表示不使用任何增强计划
	EnhancePlanHypotheticalAnswer          = "hypothetical_answer"
	EnhancePlanHypotheticalAnswerWithSplit = "hypothetical_answer_with_split"
	EnhancePlanSplitQuery                  = "split_query"
	EnhancePlanGeneralizeQuery             = "generalize_query"
	EnhancePlanExactKeywordSearch          = "exact_keyword_search"
)

var MethodVerboseName = vectorstore.MethodVerboseName

type RAGQueryOption = vectorstore.CollectionQueryOption

var NewRAGQueryConfig = vectorstore.NewRAGQueryConfig

type RAGSearchResult = vectorstore.RAGSearchResult

type ScoredResult = vectorstore.ScoredResult

func Query(db *gorm.DB, query string, opts ...RAGSystemConfigOption) (<-chan *RAGSearchResult, error) {
	vectorstoreOptions := NewRAGSystemConfig(opts...).ConvertToRAGQueryOptions()
	return vectorstore.Query(db, query, vectorstoreOptions...)
}

func SimpleQuery(db *gorm.DB, query string, limit int, opts ...RAGSystemConfigOption) ([]*vectorstore.SearchResult, error) {
	vectorstoreOptions := NewRAGSystemConfig(opts...).ConvertToRAGQueryOptions()
	return vectorstore.SimpleQuery(db, query, limit, vectorstoreOptions...)
}

func QueryYakitProfile(query string, opts ...RAGSystemConfigOption) (<-chan *RAGSearchResult, error) {
	return Query(consts.GetGormProfileDatabase(), query, opts...)
}
