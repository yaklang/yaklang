package entityrepos

import (
	"bytes"
	"context"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
)

func (r *EntityRepository) YieldEntities(ctx context.Context) chan *schema.ERModelEntity {
	db := r.db.Model(&schema.ERModelEntity{})
	db = bizhelper.ExactQueryString(db, "repository_uuid", r.info.Uuid)
	return bizhelper.YieldModel[*schema.ERModelEntity](ctx, db)
}

func (r *EntityRepository) YieldRelationships(ctx context.Context) chan *schema.ERModelRelationship {
	db := r.db.Model(&schema.ERModelRelationship{})
	db = bizhelper.ExactQueryString(db, "repository_uuid", r.info.Uuid)
	return bizhelper.YieldModel[*schema.ERModelRelationship](ctx, db)
}

// GetRelationshipsByEntityUUID 获取指定实体相关的所有关系
func (r *EntityRepository) GetRelationshipsByEntityUUID(ctx context.Context, entityUUID string) []*schema.ERModelRelationship {
	var relationships []*schema.ERModelRelationship
	db := r.db.Model(&schema.ERModelRelationship{})
	db = db.Where("repository_uuid = ? AND (source_entity_index = ? OR target_entity_index = ?)", r.info.Uuid, entityUUID, entityUUID)
	db.Find(&relationships)
	return relationships
}

// GetEntityByUUID 根据UUID获取实体
func (r *EntityRepository) GetEntityByUUID(entityUUID string) (*schema.ERModelEntity, error) {
	var entity schema.ERModelEntity
	err := r.db.Model(&schema.ERModelEntity{}).Where("repository_uuid = ? AND uuid = ?", r.info.Uuid, entityUUID).First(&entity).Error
	if err != nil {
		return nil, err
	}
	return &entity, nil
}

type HopBlock struct {
	Src          *schema.ERModelEntity
	Relationship *schema.ERModelRelationship
	Next         *HopBlock
	IsEnd        bool
	Dst          *schema.ERModelEntity
}

type KHopPath struct {
	K    int
	Hops *HopBlock
}

func (p *KHopPath) GetRelatedEntityUUIDs() []string {
	if p == nil {
		return nil
	}
	k := p.Hops
	if k == nil {
		return nil
	}

	var uuids []string
	current := k
	for current != nil {
		if current.Src != nil {
			uuids = append(uuids, current.Src.Uuid)
		}
		if current.IsEnd && current.Dst != nil {
			uuids = append(uuids, current.Dst.Uuid)
		}
		current = current.Next
	}
	return uuids
}

// use *schema.ERModelEntity and *schema.ERModelRelationship to represent entities and relationships
// their ToRAGContent() methods can be used to convert to RAGContent
func (p *KHopPath) String() string {
	if p == nil {
		return ""
	}

	k := p.Hops
	if k == nil {
		return ""
	}
	var buf bytes.Buffer
	var current = k
	for current != nil {
		if k.Src == nil || k.Relationship == nil {
			return ""
		}
		buf.WriteString(current.Src.ToRAGContent())
		buf.WriteString(" --[")
		buf.WriteString(current.Relationship.RelationshipType)
		if current.Relationship.RelationshipTypeVerbose != "" {
			buf.WriteString(" (" + current.Relationship.RelationshipTypeVerbose + ")")
		}
		buf.WriteString("]--> ")

		if current.IsEnd {
			if ret := current.Dst.ToRAGContent(); ret != "" {
				buf.WriteString(ret)
			}
			break
		}
		current = current.Next
	}
	return buf.String()
}

type KHopConfig struct {
	K    int // k=0表示返回所有路径，k>0表示返回k-hop路径，k>=2
	KMin int // default 2
}

type KHopQueryOption func(*KHopConfig)

// WithKHopK 设置k-hop的跳数，k>=2时返回k-hop路径，k=0返回所有路径
func WithKHopK(k int) KHopQueryOption {
	return func(config *KHopConfig) {
		if k < 0 {
			k = 0
		}
		config.K = k
	}
}

// WithKHopKMin 设置最小路径长度
func WithKHopKMin(kMin int) KHopQueryOption {
	return func(config *KHopConfig) {
		if kMin < 2 {
			kMin = 2
		}
		config.KMin = kMin
	}
}

func (r *EntityRepository) YieldKHop(ctx context.Context, opts ...KHopQueryOption) chan *KHopPath {
	config := &KHopConfig{K: 0, KMin: 2} // 默认k=0，返回所有路径
	for _, opt := range opts {
		opt(config)
	}

	var ch = make(chan *KHopPath, 100) // 缓冲通道避免阻塞

	go func() {
		defer close(ch)

		// 找到所有可能的路径片段
		r.findAllPathSegments(ctx, config.K, config.KMin, ch)
	}()

	return ch
}

// findAllPathSegments 找到所有可能的路径片段
func (r *EntityRepository) findAllPathSegments(ctx context.Context, k int, kMin int, resultCh chan<- *KHopPath) {
	// 首先找到图中所有的路径
	allPaths := r.findAllPaths(ctx)
	if len(allPaths) == 0 {
		return
	}

	// 从每条路径中提取k-hop子路径
	for _, path := range allPaths {
		select {
		case <-ctx.Done():
			return
		default:
		}

		r.extractKHopSegments(ctx, path, k, kMin, resultCh)
	}
}

// findAllPaths 使用DFS找到图中所有的路径
func (r *EntityRepository) findAllPaths(ctx context.Context) []*HopBlock {
	var allPaths []*HopBlock
	entityMap := make(map[string]*schema.ERModelEntity)
	relationshipMap := make(map[string]map[string][]*schema.ERModelRelationship) // source -> target -> relationships

	// 构建实体映射
	for entity := range r.YieldEntities(ctx) {
		entityMap[entity.Uuid] = entity
	}

	// 构建关系映射
	for rel := range r.YieldRelationships(ctx) {
		log.Infof("Building relationship: %s -> %s", rel.SourceEntityIndex, rel.TargetEntityIndex)

		if relationshipMap[rel.SourceEntityIndex] == nil {
			relationshipMap[rel.SourceEntityIndex] = make(map[string][]*schema.ERModelRelationship)
		}
		relationshipMap[rel.SourceEntityIndex][rel.TargetEntityIndex] = append(
			relationshipMap[rel.SourceEntityIndex][rel.TargetEntityIndex], rel)

		// 反向关系也添加（因为关系可能是双向的）
		if relationshipMap[rel.TargetEntityIndex] == nil {
			relationshipMap[rel.TargetEntityIndex] = make(map[string][]*schema.ERModelRelationship)
		}
		relationshipMap[rel.TargetEntityIndex][rel.SourceEntityIndex] = append(
			relationshipMap[rel.TargetEntityIndex][rel.SourceEntityIndex], rel)
	}

	// 调试信息
	log.Infof("Entity map size: %d", len(entityMap))
	log.Infof("Relationship map size: %d", len(relationshipMap))
	for source, targets := range relationshipMap {
		log.Infof("Entity %s has %d targets", source, len(targets))
		for target := range targets {
			log.Infof("  %s -> %s", source, target)
		}
	}

	// 从每个实体开始DFS
	for _, entity := range entityMap {
		log.Infof("Starting DFS from entity: %s", entity.EntityName)
		visited := make(map[string]bool) // 每个起始节点都有自己的visited集合
		r.dfsFindPaths(ctx, entity, entityMap, relationshipMap, visited, &allPaths, &HopBlock{
			Src:   entity,
			IsEnd: true,
		})
	}

	log.Infof("Total paths found: %d", len(allPaths))
	return allPaths
}

// dfsFindPaths 深度优先搜索找到所有路径
func (r *EntityRepository) dfsFindPaths(ctx context.Context, currentEntity *schema.ERModelEntity,
	entityMap map[string]*schema.ERModelEntity,
	relationshipMap map[string]map[string][]*schema.ERModelRelationship,
	visited map[string]bool,
	allPaths *[]*HopBlock,
	currentPath *HopBlock) {

	select {
	case <-ctx.Done():
		return
	default:
	}

	// 将当前实体标记为已访问
	visited[currentEntity.Uuid] = true

	// 查找当前实体的邻居
	hasUnvisitedNeighbors := false
	if neighbors, exists := relationshipMap[currentEntity.Uuid]; exists {
		log.Infof("Exploring %d neighbors for entity: %s", len(neighbors), currentEntity.EntityName)
		for neighborUUID, relationships := range neighbors {
			if visited[neighborUUID] {
				log.Infof("Skipping visited neighbor: %s", neighborUUID)
				continue
			}

			hasUnvisitedNeighbors = true
			neighborEntity := entityMap[neighborUUID]
			if neighborEntity == nil {
				log.Infof("Neighbor entity not found: %s", neighborUUID)
				continue
			}

			// 为每个关系创建新的路径节点
			for _, rel := range relationships {
				log.Infof("Creating path extension: %s -> %s", currentEntity.EntityName, neighborEntity.EntityName)

				newHop := &HopBlock{
					Src:          neighborEntity,
					Relationship: rel,
					IsEnd:        true,
				}

				// 将新节点添加到当前路径末尾
				lastHop := currentPath
				for lastHop.Next != nil {
					lastHop = lastHop.Next
				}
				lastHop.Next = newHop
				lastHop.IsEnd = false

				// 递归探索
				r.dfsFindPaths(ctx, neighborEntity, entityMap, relationshipMap, visited, allPaths, currentPath)

				// 回溯：移除刚添加的节点
				lastHop.Next = nil
				lastHop.IsEnd = true
			}
		}
	}

	// 如果没有未访问的邻居，这是一个路径的末尾，将当前路径添加到结果集
	if !hasUnvisitedNeighbors {
		log.Infof("Reached path end for entity: %s, path length: %d", currentEntity.EntityName, r.getPathLength(currentPath))
		pathCopy := r.copyHopBlock(currentPath)
		*allPaths = append(*allPaths, pathCopy)
	}

	// 回溯：标记当前实体为未访问
	visited[currentEntity.Uuid] = false
}

// extractKHopSegments 从路径中提取k-hop子路径
func (r *EntityRepository) extractKHopSegments(ctx context.Context, path *HopBlock, k int, kMin int, resultCh chan<- *KHopPath) {
	// 将路径转换为切片，方便处理
	pathSlice := r.hopBlockToSlice(path)
	log.Infof("Processing path with %d elements, k=%d, kMin=%d", len(pathSlice), k, kMin)
	log.Infof("Path content: %s", r.pathToString(path))

	if len(pathSlice) < kMin { // 使用KMin作为最小长度要求
		log.Infof("Path too short: %d < %d", len(pathSlice), kMin)
		return
	}

	// 计算路径中的实体数量（跳数 = 实体数 - 1）
	pathLength := len(pathSlice)

	if k == 0 {
		// 返回所有长度>=KMin的子路径
		log.Infof("Extracting all subpaths >= %d from path of length %d", kMin, pathLength)
		for subK := kMin; subK <= pathLength; subK++ {
			r.extractSubPaths(pathSlice, subK, resultCh)
		}
	} else if k >= kMin {
		// 返回指定长度k的子路径
		log.Infof("Extracting subpaths of length %d from path of length %d", k, pathLength)
		r.extractSubPaths(pathSlice, k, resultCh)
	} else {
		log.Infof("Skipping: k=%d < kMin=%d", k, kMin)
	}
}

// extractSubPaths 从路径切片中提取指定长度的子路径
func (r *EntityRepository) extractSubPaths(pathSlice []*HopBlock, k int, resultCh chan<- *KHopPath) {
	if len(pathSlice) < k {
		log.Infof("Path slice too short: %d < %d", len(pathSlice), k)
		return
	}

	log.Infof("Extracting %d subpaths of length %d from path of length %d", len(pathSlice)-k+1, k, len(pathSlice))

	// 滑动窗口提取子路径
	for i := 0; i <= len(pathSlice)-k; i++ {
		subPath := pathSlice[i : i+k]

		// 构建子路径的HopBlock链表
		subHopBlock := r.buildHopBlockFromSlice(subPath)
		if subHopBlock == nil {
			log.Infof("Failed to build hop block for subpath %d", i)
			continue
		}

		log.Infof("Built subpath %d: %s", i, r.pathToString(subHopBlock))

		// 发送结果
		select {
		case resultCh <- &KHopPath{
			K:    k - 1, // k个实体构成k-1跳
			Hops: subHopBlock,
		}:
			log.Infof("Sent subpath %d to result channel", i)
		default:
			// 通道已满，跳过
			log.Infof("Channel full, skipping subpath %d", i)
			return
		}
	}
}

// hopBlockToSlice 将HopBlock链表转换为切片
func (r *EntityRepository) hopBlockToSlice(hop *HopBlock) []*HopBlock {
	var result []*HopBlock
	current := hop
	for current != nil {
		result = append(result, current)
		current = current.Next
	}
	return result
}

// buildHopBlockFromSlice 从切片构建HopBlock链表
func (r *EntityRepository) buildHopBlockFromSlice(slice []*HopBlock) *HopBlock {
	if len(slice) == 0 {
		return nil
	}

	// 复制第一个节点
	head := r.copyHopBlock(slice[0])
	head.Next = nil
	head.IsEnd = false

	current := head
	for i := 1; i < len(slice); i++ {
		copied := r.copyHopBlock(slice[i])
		copied.Next = nil
		if i == len(slice)-1 {
			copied.IsEnd = true
		} else {
			copied.IsEnd = false
		}
		current.Next = copied
		current = copied
	}

	return head
}

// copyHopBlock 复制整个HopBlock路径链
func (r *EntityRepository) copyHopBlock(hop *HopBlock) *HopBlock {
	if hop == nil {
		return nil
	}

	// 复制第一个节点
	head := &HopBlock{
		Src:          hop.Src,
		Relationship: hop.Relationship,
		Next:         nil,
		IsEnd:        false, // 暂时设为false
		Dst:          hop.Dst,
	}

	// 如果原始路径只有一个节点
	if hop.Next == nil {
		head.IsEnd = hop.IsEnd
		return head
	}

	// 复制剩余的路径
	current := head
	original := hop.Next
	for original != nil {
		newNode := &HopBlock{
			Src:          original.Src,
			Relationship: original.Relationship,
			Next:         nil,
			IsEnd:        false,
			Dst:          original.Dst,
		}
		current.Next = newNode
		current = newNode
		original = original.Next
	}

	// 设置最后一个节点的IsEnd
	current.IsEnd = true
	return head
}

// getPathLength 获取路径的长度（实体数量）
func (r *EntityRepository) getPathLength(hop *HopBlock) int {
	if hop == nil {
		return 0
	}

	count := 0
	current := hop
	for current != nil {
		if current.Src != nil {
			count++
		}
		current = current.Next
	}
	return count
}

// pathToString 将路径转换为字符串表示（用于调试）
func (r *EntityRepository) pathToString(hop *HopBlock) string {
	if hop == nil {
		return "nil"
	}

	result := ""
	current := hop
	for current != nil {
		if current.Src != nil {
			if result != "" {
				result += " -> "
			}
			result += current.Src.EntityName
		}
		current = current.Next
	}
	return result
}
