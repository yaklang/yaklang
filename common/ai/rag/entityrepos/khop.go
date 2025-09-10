package entityrepos

import (
	"bytes"
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (r *EntityRepository) YieldEntities(ctx context.Context, filter *ypb.EntityFilter) chan *schema.ERModelEntity {
	if filter == nil {
		filter = &ypb.EntityFilter{}
	}
	filter.BaseIndex = r.info.Uuid
	return yakit.YieldEntities(ctx, r.db, filter)
}

func (r *EntityRepository) YieldRelationships(ctx context.Context, filter *ypb.RelationshipFilter) chan *schema.ERModelRelationship {
	if filter == nil {
		filter = &ypb.RelationshipFilter{}
	}
	filter.BaseIndex = r.info.Uuid
	return yakit.YieldRelationships(ctx, r.db, filter)
}

// GetRelationshipsByEntityUUID 获取指定实体相关的所有关系
func (r *EntityRepository) GetRelationshipsByEntityUUID(ctx context.Context, entityUUID string) []*schema.ERModelRelationship {
	var relationships []*schema.ERModelRelationship

	err := utils.GormTransaction(r.db, func(tx *gorm.DB) error {
		db := tx.Model(&schema.ERModelRelationship{})
		log.Debugf("Querying relationships for entity %s, table name: %s", entityUUID, db.NewScope(&schema.ERModelRelationship{}).TableName())
		db = db.Where("repository_uuid = ? AND (source_entity_index = ? OR target_entity_index = ?)", r.info.Uuid, entityUUID, entityUUID)
		return db.Find(&relationships).Error
	})

	if err != nil {
		log.Debugf("Error querying relationships: %v", err)
	}
	log.Debugf("Found %d relationships for entity %s", len(relationships), entityUUID)
	return relationships
}

// GetEntityByUUID 根据UUID获取实体
func (r *EntityRepository) GetEntityByUUID(entityUUID string) (*schema.ERModelEntity, error) {
	var entity schema.ERModelEntity

	err := utils.GormTransaction(r.db, func(tx *gorm.DB) error {
		db := tx.Model(&schema.ERModelEntity{})
		log.Debugf("Querying entity %s, table name: %s", entityUUID, db.NewScope(&schema.ERModelEntity{}).TableName())
		return db.Where("repository_uuid = ? AND uuid = ?", r.info.Uuid, entityUUID).First(&entity).Error
	})

	if err != nil {
		log.Debugf("Error querying entity %s: %v", entityUUID, err)
		return nil, err
	}
	log.Debugf("Found entity %s: %s", entityUUID, entity.EntityName)
	return &entity, nil
}

type HopBlock struct {
	Src          *schema.ERModelEntity
	Relationship *schema.ERModelRelationship
	Next         *HopBlock
	IsEnd        bool
	Dst          *schema.ERModelEntity
}

func (h *HopBlock) Hash() string {
	if h == nil {
		return ""
	}
	var parts []string
	current := h
	for current != nil {
		if current.Src != nil {
			parts = append(parts, current.Src.Uuid)
		}
		if current.Relationship != nil {
			parts = append(parts, current.Relationship.Uuid)
		}
		if current.IsEnd && current.Dst != nil {
			parts = append(parts, current.Dst.Uuid)
		}
		current = current.Next
	}
	return utils.CalcSha1(parts)
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

func (p *KHopPath) GetRelatedUUIDs() []string {
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
		uuids = append(uuids, current.Relationship.Uuid)
		current = current.Next
	}
	return uuids
}

func (p *KHopPath) Hash() string {
	uuidList := p.GetRelatedUUIDs()
	return utils.CalcSha1(uuidList)
}

func (p *KHopPath) ToRAGContent() string {
	return p.String()
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

	// 收集路径中的所有实体
	var entities []*schema.ERModelEntity
	var relationships []*schema.ERModelRelationship

	current := k
	for current != nil {
		if current.Src != nil {
			entities = append(entities, current.Src)
		}
		if current.Relationship != nil {
			relationships = append(relationships, current.Relationship)
		}
		if current.IsEnd && current.Dst != nil {
			entities = append(entities, current.Dst)
		}
		current = current.Next
	}

	// 构建字符串
	for i, entity := range entities {
		if i > 0 {
			if i-1 < len(relationships) {
				rel := relationships[i-1]
				buf.WriteString(" --[")
				buf.WriteString(rel.RelationshipType)
				if rel.RelationshipTypeVerbose != "" {
					buf.WriteString(" (" + rel.RelationshipTypeVerbose + ")")
				}
				buf.WriteString("]--> ")
			}
		}
		buf.WriteString(entity.ToRAGContent())
	}

	return buf.String()
}

type KHopConfig struct {
	K    int // k=0表示返回所有路径，k>0表示返回k-hop路径，k>=2
	KMin int // default 2 (minimum 2-hop paths)
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

// WithKHopKMin 设置最小路径长度，最小值为2
func WithKHopKMin(kMin int) KHopQueryOption {
	return func(config *KHopConfig) {
		if kMin < 2 {
			kMin = 2
		}
		config.KMin = kMin
	}
}

func NewKHopConfig(options ...any) *KHopConfig {
	config := &KHopConfig{K: 0, KMin: 2} // 默认k=0，KMin=2，返回所有>=2-hop路径
	for _, opt := range options {
		if optFunc, ok := opt.(KHopQueryOption); ok {
			optFunc(config)
		}
	}
	return config
}

// todo:  yield k-hop with entity filter or rag search
func (r *EntityRepository) YieldKHop(ctx context.Context, opts ...any) <-chan *KHopPath {
	config := NewKHopConfig(opts...)

	var channel = chanx.NewUnlimitedChan[*KHopPath](ctx, 1000)

	var input = chanx.NewUnlimitedChan[string](ctx, 1000)
	go func() {
		defer input.Close()
		for relationship := range r.YieldRelationships(ctx, nil) {
			input.SafeFeed(relationship.SourceEntityIndex)
		}
	}()

	go func() {
		defer channel.Close()

		// 找到所有可能的路径片段
		r.findAllPathSegments(ctx, config.K, config.KMin, channel, input.OutputChannel())
	}()

	var hashMap = make(map[string]bool) // filter duplicate paths
	var result = chanx.NewUnlimitedChan[*KHopPath](ctx, 1000)
	go func() {
		defer result.Close()

		for path := range channel.OutputChannel() {
			if !hashMap[path.Hash()] {
				hashMap[path.Hash()] = true
				result.SafeFeed(path)
			} else {
				log.Debug("find exist sub path: %s", path.String())
			}
		}
	}()
	return result.OutputChannel()
}

// findAllPathSegments 找到所有可能的路径片段
func (r *EntityRepository) findAllPathSegments(ctx context.Context, k int, kMin int, resultCh *chanx.UnlimitedChan[*KHopPath], startUUID <-chan string) {
	// 首先找到图中所有的路径
	allPaths := r.findAllPaths(ctx, startUUID)
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

// findAllPaths 使用DFS找到图中所有的路径（优化版本：直接在关系遍历中处理，只从源实体开始）
func (r *EntityRepository) findAllPaths(ctx context.Context, startEntityChannel <-chan string) []*HopBlock {
	var allPaths []*HopBlock

	// 使用set来避免重复处理源实体（有向图）
	processedEntities := make(map[string]bool)

	// 直接在关系遍历中处理源实体（有向图）
	for startUUID := range startEntityChannel {
		select {
		case <-ctx.Done():
			log.Debugf("Context cancelled during path finding")
			return allPaths
		default:
		}

		// 只处理源实体（有向图中只需从有出边的实体开始）
		if !processedEntities[startUUID] {
			processedEntities[startUUID] = true

			log.Debugf("Starting DFS from source entity: %s", startUUID)
			visited := make(map[string]bool)

			// 按需加载起始实体
			startEntity, err := r.GetEntityByUUID(startUUID)
			if err != nil || startEntity == nil {
				log.Debugf("Failed to load entity %s: %v", startUUID, err)
				continue
			}

			// 创建一个空的初始路径，第一个实体会在DFS中添加
			r.dfsFindPathsOptimized(ctx, startEntity, visited, &allPaths, nil)
		}
	}

	log.Debugf("Processed %d entities, found %d paths total", len(processedEntities), len(allPaths))
	return allPaths
}

// dfsFindPathsOptimized 优化的DFS算法，按需加载数据
// 对于有向图，只沿着出边遍历，并使用路径级别的去重来避免环
func (r *EntityRepository) dfsFindPathsOptimized(ctx context.Context, currentEntity *schema.ERModelEntity,
	visited map[string]bool, allPaths *[]*HopBlock, currentPath *HopBlock) {

	select {
	case <-ctx.Done():
		return
	default:
	}

	// 如果当前路径为空，说明这是路径的起点
	if currentPath == nil {
		currentPath = &HopBlock{
			Src:   currentEntity,
			IsEnd: true,
		}
	}

	if currentEntity.Uuid == "6179511a-0c78-4228-896c-1d0e9ef53af7" {
		log.Debugf("Skipping path %s (already in current path)", currentEntity.Uuid)
	}

	// 查找当前实体的出边关系（有向图）
	hasUnvisitedNeighbors := false
	relationships := r.GetRelationshipsByEntityUUID(ctx, currentEntity.Uuid)

	log.Debugf("Exploring %d relationships for entity: %s", len(relationships), currentEntity.EntityName)

	// 只处理从当前节点出发的出边（有向图）
	for _, rel := range relationships {
		// 只处理当前节点作为源节点的出边
		if rel.SourceEntityIndex != currentEntity.Uuid {
			continue
		}

		neighborUUID := rel.TargetEntityIndex

		// 检查是否已经在当前路径中（路径级别去重，避免环）
		if r.isEntityInPath(currentPath, neighborUUID) {
			log.Debugf("Skipping neighbor %s (already in current path)", neighborUUID)
			continue
		}

		// 按需加载邻居实体
		neighborEntity, err := r.GetEntityByUUID(neighborUUID)
		if err != nil || neighborEntity == nil {
			log.Debugf("Failed to load neighbor entity %s: %v", neighborUUID, err)
			continue
		}

		hasUnvisitedNeighbors = true

		log.Debugf("Creating path extension: %s -> %s", currentEntity.EntityName, neighborEntity.EntityName)

		newHop := &HopBlock{
			Src:          currentEntity,
			Relationship: rel,
			Dst:          neighborEntity,
			IsEnd:        true,
		}

		// 将新节点添加到当前路径末尾
		lastHop := currentPath
		for lastHop.Next != nil {
			lastHop = lastHop.Next
		}

		// 如果最后一个节点没有关系（说明这是路径的第一个关系），替换它
		if lastHop.Relationship == nil {
			lastHop.Relationship = rel
			lastHop.Dst = neighborEntity
			lastHop.IsEnd = false
		} else {
			// 否则添加新的节点
			lastHop.Next = newHop
			lastHop.IsEnd = false
		}

		// 递归探索（不传递visited，因为我们使用路径级别去重）
		r.dfsFindPathsOptimized(ctx, neighborEntity, visited, allPaths, currentPath)

		// 回溯：恢复路径状态
		if lastHop.Relationship == rel {
			// 如果我们替换了关系，清空它
			lastHop.Relationship = nil
			lastHop.Dst = nil
			lastHop.IsEnd = true
		} else {
			// 如果我们添加了新节点，移除它
			lastHop.Next = nil
			lastHop.IsEnd = true
		}
	}

	// 如果没有未访问的邻居，这是一个路径的末尾，将当前路径添加到结果集
	if !hasUnvisitedNeighbors {
		log.Debugf("Reached path end for entity: %s, path length: %d", currentEntity.EntityName, r.getPathLength(currentPath))
		pathCopy := r.copyHopBlock(currentPath)
		*allPaths = append(*allPaths, pathCopy)
	}
}

// isEntityInPath 检查实体是否已经在当前路径中
func (r *EntityRepository) isEntityInPath(path *HopBlock, entityUUID string) bool {
	current := path
	for current != nil {
		if current.Src != nil && current.Src.Uuid == entityUUID {
			return true
		}
		if current.IsEnd && current.Dst != nil && current.Dst.Uuid == entityUUID {
			return true
		}
		current = current.Next
	}
	return false
}

// extractKHopSegments 从路径中提取k-hop子路径
func (r *EntityRepository) extractKHopSegments(ctx context.Context, path *HopBlock, k int, kMin int, resultCh *chanx.UnlimitedChan[*KHopPath]) {
	// 将路径转换为切片，方便处理
	pathSlice := r.hopBlockToSlice(path)
	log.Debugf("Processing path with %d elements, k=%d, kMin=%d", len(pathSlice), k, kMin)
	log.Debugf("Path content: %s", r.pathToString(path))

	if len(pathSlice) < kMin { // 使用KMin作为最小长度要求
		log.Debugf("Path too short: %d < %d", len(pathSlice), kMin)
		return
	}

	// 计算路径中的实体数量（跳数 = 实体数 - 1）
	pathLength := len(pathSlice)

	if k == 0 {
		// 返回所有长度>=KMin的子路径
		log.Debugf("Extracting all subpaths >= %d from path of length %d", kMin, pathLength)
		for subK := kMin; subK <= pathLength; subK++ {
			r.extractSubPaths(pathSlice, subK, resultCh)
		}
	} else if k >= kMin {
		// 返回指定跳数k的子路径（k跳对应k+1个实体）
		entityCount := k + 1
		log.Debugf("Extracting subpaths of length %d (K=%d hops) from path of length %d", entityCount, k, pathLength)
		r.extractSubPaths(pathSlice, entityCount, resultCh)
	} else {
		log.Debugf("Skipping: k=%d < kMin=%d", k, kMin)
	}
}

// extractSubPaths 从路径切片中提取指定长度的子路径
func (r *EntityRepository) extractSubPaths(pathSlice []*HopBlock, k int, resultCh *chanx.UnlimitedChan[*KHopPath]) {
	if len(pathSlice) < k {
		log.Debugf("Path slice too short: %d < %d", len(pathSlice), k)
		return
	}

	log.Debugf("Extracting %d subpaths of length %d from path of length %d", len(pathSlice)-k+1, k, len(pathSlice))

	// 滑动窗口提取子路径
	for i := 0; i <= len(pathSlice)-k; i++ {
		subPath := pathSlice[i : i+k]

		// 构建子路径的HopBlock链表
		subHopBlock := r.buildHopBlockFromSlice(subPath)
		if subHopBlock == nil {
			log.Debugf("Failed to build hop block for subpath %d", i)
			continue
		}

		log.Debugf("Built subpath %d: %s", i, r.pathToString(subHopBlock))

		// 计算路径的跳数（K值）
		pathK := k - 1 // k个实体构成k-1跳

		// 如果K=1（1-hop），则跳过不返回（根据用户要求）
		if pathK == 1 {
			log.Debugf("Skipping 1-hop path for subpath %d", i)
			continue
		}

		// 发送结果
		khopPath := &KHopPath{
			K:    pathK,
			Hops: subHopBlock,
		}
		log.Debugf("Sending path: K=%d, path elements=%d, String=%s", pathK, len(subPath), khopPath.String())

		resultCh.SafeFeed(khopPath)
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

	log.Debugf("Building HopBlock from slice with %d elements", len(slice))
	for i, hop := range slice {
		srcName := "nil"
		dstName := "nil"
		if hop.Src != nil {
			srcName = hop.Src.EntityName
		}
		if hop.Dst != nil {
			dstName = hop.Dst.EntityName
		}
		log.Debugf("Slice[%d]: Src=%s, Dst=%s, IsEnd=%v", i, srcName, dstName, hop.IsEnd)
	}

	// 对于K-hop路径，slice长度应该是K+1（K+1个实体）
	// 我们需要构建K个HopBlock，每个HopBlock包含Src、Relationship和Dst
	// 最后一个HopBlock的IsEnd=true

	var head *HopBlock
	var current *HopBlock

	for i := 0; i < len(slice)-1; i++ {
		newHop := &HopBlock{
			Src:          slice[i].Src,
			Relationship: slice[i].Relationship,
			Next:         nil,
			IsEnd:        false,
			Dst:          slice[i+1].Src, // 下一个实体的Src作为当前Dst
		}

		if head == nil {
			head = newHop
			current = head
		} else {
			current.Next = newHop
			current = newHop
		}
	}

	// 设置最后一个HopBlock为终结节点，并设置其Dst为原始slice最后一个元素的Src
	if current != nil {
		current.IsEnd = true
		if len(slice) > 1 {
			current.Dst = slice[len(slice)-1].Src
		}
	}

	log.Debugf("Built HopBlock chain with length: %d", r.getPathLength(head))
	return head
}

// copyHopBlock 复制整个HopBlock路径链
func (r *EntityRepository) copyHopBlock(hop *HopBlock) *HopBlock {
	if hop == nil {
		return nil
	}

	// 如果只有一个HopBlock
	if hop.Next == nil {
		return &HopBlock{
			Src:          hop.Src,
			Relationship: hop.Relationship,
			Next:         nil,
			IsEnd:        hop.IsEnd,
			Dst:          hop.Dst,
		}
	}

	// 复制第一个HopBlock
	head := &HopBlock{
		Src:          hop.Src,
		Relationship: hop.Relationship,
		Next:         nil,
		IsEnd:        false, // 第一个HopBlock不是终结节点
		Dst:          hop.Dst,
	}

	// 复制剩余的HopBlock
	current := head
	original := hop.Next
	for original != nil {
		newNode := &HopBlock{
			Src:          original.Src,
			Relationship: original.Relationship,
			Next:         nil,
			IsEnd:        false, // 暂时设为false
			Dst:          original.Dst,
		}

		current.Next = newNode
		current = newNode
		original = original.Next
	}

	// 设置最后一个HopBlock为终结节点
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
		// 如果是终结节点且Dst不为nil，计算Dst
		if current.IsEnd && current.Dst != nil && current.Next == nil {
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

func (r *EntityRepository) AddKHopToVectorIndex(kHop *KHopPath) error {
	r.ragMutex.Lock()
	defer r.ragMutex.Unlock()

	metadata := map[string]any{
		schema.META_Base_Index: r.info.Uuid,
		META_K:                 kHop.K,
	}

	var opts []rag.DocumentOption

	opts = append(opts, rag.WithDocumentRawMetadata(metadata),
		rag.WithDocumentType(schema.RAGDocumentType_KHop),
		rag.WithDocumentRelatedEntities(kHop.GetRelatedEntityUUIDs()...),
	)
	documentID := fmt.Sprintf("%s_khop", uuid.NewString())
	content := kHop.ToRAGContent()
	return r.GetRAGSystem().Add(documentID, content, opts...)
}

const (
	META_K = "k"
)
