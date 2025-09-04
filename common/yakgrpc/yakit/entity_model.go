package yakit

import (
	"fmt"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/utils/dot"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
)

func CreateEntityBaseInfo(db *gorm.DB, entityBase *schema.EntityBaseInfo) error {
	return db.Create(entityBase).Error
}

func DeleteEntityBaseInfo(db *gorm.DB, id int64) error {
	return db.Unscoped().Delete(&schema.EntityBaseInfo{}, id).Error
}

func FilterEntities(db *gorm.DB, entityFilter *ypb.EntityFilter) *gorm.DB {
	if entityFilter == nil {
		return db
	}
	db = db.Model(&schema.ERModelEntity{})
	db = bizhelper.ExactQueryUInt64ArrayOr(db, "id", entityFilter.IDs)
	db = bizhelper.ExactQueryString(db, "entity_base_index", entityFilter.BaseIndex)
	if entityFilter.BaseID > 0 {
		db = bizhelper.ExactQueryInt64(db, "entity_base_id", int64(entityFilter.BaseID))
	}
	db = bizhelper.ExactQueryStringArrayOr(db, "entity_name", entityFilter.Names)
	db = bizhelper.ExactQueryStringArrayOr(db, "entity_type", entityFilter.Types)
	db = bizhelper.ExactOrQueryStringArrayOr(db, "hidden_index", entityFilter.HiddenIndex)
	return db
}

func QueryEntities(db *gorm.DB, entityFilter *ypb.EntityFilter) ([]*schema.ERModelEntity, error) {
	db = db.Model(&schema.ERModelEntity{})
	db = FilterEntities(db, entityFilter)
	var entities []*schema.ERModelEntity
	err := db.Find(&entities).Error
	return entities, err
}

func QueryEntitiesPaging(db *gorm.DB, entityFilter *ypb.EntityFilter, paging *ypb.Paging) (*bizhelper.Paginator, []*schema.ERModelEntity, error) {
	db = db.Model(&schema.ERModelEntity{})
	db = FilterEntities(db, entityFilter)
	db = bizhelper.OrderByPaging(db, paging)
	ret := make([]*schema.ERModelEntity, 0)
	pag, db := bizhelper.YakitPagingQuery(db, paging, &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}
	return pag, ret, nil
}

func UpdateEntity(db *gorm.DB, id uint, entity *schema.ERModelEntity) error {
	return db.Model(&schema.ERModelEntity{}).Where("id = ?", id).Updates(entity).Error
}

func CreateEntity(db *gorm.DB, entity *schema.ERModelEntity) error {
	return db.Create(entity).Error
}

func GetEntityByID(db *gorm.DB, id uint) (*schema.ERModelEntity, error) {
	var entity schema.ERModelEntity
	db = db.Model(&schema.ERModelEntity{})
	if err := db.Where("id = ?", id).First(&entity).Error; err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return nil, utils.Errorf("entity not found")
		}
		return nil, err
	}
	return &entity, nil
}

func GetEntityByIndex(db *gorm.DB, index string) (*schema.ERModelEntity, error) {
	var entity schema.ERModelEntity
	db = db.Model(&schema.ERModelEntity{})
	if err := db.Where("hidden_index = ?", index).First(&entity).Error; err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return nil, utils.Errorf("entity not found")
		}
		return nil, err
	}
	return &entity, nil
}

type AttributeFilter struct {
	Name       []string
	Value      []string
	UniqueOnly bool
	EntityID   []uint
}

//func FilterAttributes(db *gorm.DB, filter *AttributeFilter) *gorm.DB {
//	if filter == nil {
//		return db
//	}
//	db = db.Model(&schema.ERModelAttribute{})
//	db = bizhelper.ExactQueryStringArrayOr(db, "attribute_name", filter.Name)
//	db = bizhelper.ExactQueryStringArrayOr(db, "attribute_value", filter.Value)
//	db = bizhelper.ExactQueryUIntArrayOr(db, "entity_id", filter.EntityID)
//	if filter.UniqueOnly {
//		db = db.Where("unique_identifier = ?", true)
//	}
//	return db
//}

//func QueryAttributes(db *gorm.DB, filter *AttributeFilter) ([]*schema.ERModelAttribute, error) {
//	db = db.Model(&schema.ERModelAttribute{})
//	db = FilterAttributes(db, filter)
//	var attributes []*schema.ERModelAttribute
//	err := db.Find(&attributes).Error
//	return attributes, err
//}
//
//func UniqueAttributesIndexRelationship(db *gorm.DB, name string, values string) ([]uint, bool) {
//	db = FilterAttributes(db, &AttributeFilter{
//		Name:       []string{name},
//		Value:      []string{values},
//		UniqueOnly: true,
//	})
//	var attribute = make([]*schema.ERModelAttribute, 0)
//	err := db.Find(&attribute).Error
//	if err != nil {
//		return nil, false
//	}
//	return lo.Map(attribute, func(item *schema.ERModelAttribute, _ int) uint {
//		return item.ID
//	}), true
//}
//
//func UniqueAttributesIndexEntity(db *gorm.DB, name string, values string) ([]uint, bool) {
//	db = FilterAttributes(db, &AttributeFilter{
//		Name:       []string{name},
//		Value:      []string{values},
//		UniqueOnly: true,
//	})
//	var attribute = make([]*schema.ERModelAttribute, 0)
//	err := db.Find(&attribute).Error
//	if err != nil {
//		return nil, false
//	}
//	return lo.Map(attribute, func(item *schema.ERModelAttribute, _ int) uint {
//		return item.ID
//	}), true
//}

//func CreateAttribute(db *gorm.DB, attribute *schema.ERModelAttribute) error {
//	return db.Create(attribute).Error
//}

func AddRelationship(db *gorm.DB, sourceIndex, targetIndex, baseIndex, RelationshipType, decisionRationale string, attrs map[string]any) error {
	Relationship := schema.ERModelRelationship{
		SourceEntityIndex: sourceIndex,
		TargetEntityIndex: targetIndex,
		RelationshipType:  RelationshipType,
		DecisionRationale: decisionRationale,
		Attributes:        attrs,
		EntityBaseIndex:   baseIndex,
	}
	Relationship.Hash = Relationship.CalcHash()
	return utils.GormTransaction(db, func(tx *gorm.DB) error {
		findErr := tx.Where("hash = ?", Relationship.Hash).First(&Relationship).Error
		if findErr == nil {
			return nil // 事务成功结束
		}
		if gorm.IsRecordNotFoundError(findErr) {
			return tx.Create(&Relationship).Error
		}
		return findErr
	})
}

func GetOutgoingRelationships(db *gorm.DB, entity *schema.ERModelEntity) ([]*schema.ERModelRelationship, error) {
	var relationships []*schema.ERModelRelationship
	if err := db.Model(&schema.ERModelRelationship{}).Where("source_entity_index = ?", entity.HiddenIndex).Find(&relationships).Error; err != nil {
		return nil, err
	}
	return relationships, nil
}

func GetIncomingRelationships(db *gorm.DB, entity *schema.ERModelEntity) ([]*schema.ERModelRelationship, error) {
	var relationships []*schema.ERModelRelationship
	if err := db.Model(&schema.ERModelRelationship{}).Where("target_entity_index = ?", entity.HiddenIndex).Find(&relationships).Error; err != nil {
		return nil, err
	}
	return relationships, nil
}

func FilterRelationships(db *gorm.DB, relationshipFilter *ypb.RelationshipFilter) *gorm.DB {
	db = db.Model(&schema.ERModelRelationship{})
	if relationshipFilter == nil {
		return db
	}
	if relationshipFilter.BaseID > 0 {
		db = bizhelper.ExactQueryInt64(db, "entity_base_id", int64(relationshipFilter.BaseID))
	}
	db = bizhelper.ExactQueryString(db, "entity_base_index", relationshipFilter.BaseIndex)
	db = bizhelper.ExactQueryUInt64ArrayOr(db, "id", relationshipFilter.IDs)
	db = bizhelper.ExactQueryMultipleStringArrayOr(db, []string{"source_entity_index", "target_entity_index"}, relationshipFilter.AboutEntityIndex)
	db = bizhelper.ExactQueryStringArrayOr(db, "source_entity_index", relationshipFilter.SourceEntityIndex)
	db = bizhelper.ExactQueryStringArrayOr(db, "target_entity_index", relationshipFilter.TargetEntityIndex)
	db = bizhelper.ExactQueryStringArrayOr(db, "relationship_type", relationshipFilter.Types)
	return db
}

func QueryRelationshipPaging(db *gorm.DB, entityFilter *ypb.RelationshipFilter, paging *ypb.Paging) (*bizhelper.Paginator, []*schema.ERModelRelationship, error) {
	db = FilterRelationships(db, entityFilter)
	db = bizhelper.OrderByPaging(db, paging)
	ret := make([]*schema.ERModelRelationship, 0)
	pag, db := bizhelper.YakitPagingQuery(db, paging, &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}
	return pag, ret, nil
}

// RemoveRelationship 删除两个实体之间的永久关系。
func RemoveRelationship(db *gorm.DB, sourceID, targetID uint, RelationshipType string) error {
	result := db.Where("source_entity_id = ? AND target_entity_id = ? AND relationship_type = ?",
		sourceID, targetID, RelationshipType).Delete(&schema.ERModelRelationship{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return utils.Errorf("Relationship not found to remove")
	}
	return nil
}

func QueryEntityWithDepth(db *gorm.DB, entityFilter *ypb.EntityFilter, maxDepth int) (*ERModel, error) {
	entities, err := QueryEntities(db, entityFilter)
	if err != nil {
		return nil, err
	}
	if len(entities) == 0 {
		return nil, utils.Errorf("Entity not found")
	}

	return EntityRelationshipFind(db, entities, maxDepth)
}

func EntityRelationshipFind(db *gorm.DB, startList []*schema.ERModelEntity, maxDepth int) (*ERModel, error) {
	allEntities := make([]*schema.ERModelEntity, 0)
	allRelationships := make([]*schema.ERModelRelationship, 0)

	appendEntity := func(entityList ...*schema.ERModelEntity) {
		allEntities = append(allEntities, entityList...)
	}

	appendRelationship := func(RelationshipList ...*schema.ERModelRelationship) {
		allRelationships = append(allRelationships, RelationshipList...)
	}
	type queueItem struct {
		index string
		depth int
		e     *schema.ERModelEntity
	}

	var queue []*queueItem
	var visited = make(map[string]bool)
	for _, startEntity := range startList {
		queue = append(queue, &queueItem{
			index: startEntity.HiddenIndex,
			depth: 0,
			e:     startEntity,
		},
		)
		appendEntity(startEntity)
		visited[startEntity.HiddenIndex] = true
	}

	visitedRelationships := map[uint]bool{}

	head := 0
	for head < len(queue) {
		currentItem := queue[head]
		head++
		currentEntity := currentItem.e
		if maxDepth > 0 && currentItem.depth >= maxDepth {
			continue
		}
		// 准备要遍历的关系列表
		RelationshipsToExplore := make([]*schema.ERModelRelationship, 0)

		// query outgoing and incoming relationships
		if outgoings, err := GetOutgoingRelationships(db, currentEntity); err == nil {
			RelationshipsToExplore = append(RelationshipsToExplore, outgoings...)
		} else {
			log.Errorf("query outgoing failed: %v", err)
		}

		if incomings, err := GetIncomingRelationships(db, currentEntity); err == nil {
			RelationshipsToExplore = append(RelationshipsToExplore, incomings...)
		} else {
			log.Errorf("query incoming failed: %v", err)
		}

		for _, Relationship := range RelationshipsToExplore {
			if visitedRelationships[Relationship.ID] {
				continue
			}
			visitedRelationships[Relationship.ID] = true
			appendRelationship(Relationship)
			var neighborIndex string
			if Relationship.SourceEntityIndex == currentItem.index {
				neighborIndex = Relationship.TargetEntityIndex
			} else {
				neighborIndex = Relationship.SourceEntityIndex
			}
			if !visited[neighborIndex] {
				neighbor, err := GetEntityByIndex(db, neighborIndex)
				if err != nil {
					return nil, err
				}
				visited[neighborIndex] = true
				appendEntity(neighbor)
				queue = append(queue, &queueItem{index: neighborIndex, depth: currentItem.depth + 1, e: neighbor})
			}
		}
	}

	return &ERModel{
		Entities:      allEntities,
		Relationships: allRelationships,
	}, nil
}

type ERModel struct {
	Entities      []*schema.ERModelEntity
	Relationships []*schema.ERModelRelationship
}

func (model *ERModel) Dump() string {
	var sb strings.Builder
	sb.WriteString("Entities:\n")
	for _, entity := range model.Entities {
		sb.WriteString(fmt.Sprintf("- ID: %d\n", entity.ID))
		sb.WriteString(fmt.Sprintf("  EntityName: %s\n", entity.EntityName))
		sb.WriteString(fmt.Sprintf("  EntityType: %s\n", entity.EntityType))
		if entity.Description != "" {
			sb.WriteString(fmt.Sprintf("  Description: %s\n", utils.ShrinkString(entity.Description, 100)))
		}
		if entity.Rationale != "" {
			sb.WriteString(fmt.Sprintf("  Rationale: %s\n", utils.ShrinkString(entity.Rationale, 100)))
		}
		if len(entity.Attributes) > 0 {
			sb.WriteString("  Attributes:\n")
			for key, value := range entity.Attributes {
				sb.WriteString(fmt.Sprintf("    - %s: %s\n", key, utils.ShrinkString(value, 100)))
			}
		}
	}
	sb.WriteString("Relationships:\n")
	for _, relationship := range model.Relationships {
		sb.WriteString(fmt.Sprintf("- Source: %s\n", relationship.SourceEntityIndex))
		sb.WriteString(fmt.Sprintf("  Target: %s\n", relationship.TargetEntityIndex))
		sb.WriteString(fmt.Sprintf("  Type: %s\n", relationship.RelationshipType))
		if relationship.DecisionRationale != "" {
			sb.WriteString(fmt.Sprintf("  Rationale: %s\n", utils.ShrinkString(relationship.DecisionRationale, 100)))
		}
		if len(relationship.Attributes) > 0 {
			sb.WriteString("  Attributes:\n")
			for key, value := range relationship.Attributes {
				sb.WriteString(fmt.Sprintf("    - %s: %s\n", key, utils.ShrinkString(value, 100)))
			}
		}
	}
	return sb.String()
}

func (model *ERModel) Dot() *dot.Graph {
	G := dot.New()
	G.MakeDirected()

	rMap := make(map[string]int)

	for _, entity := range model.Entities {
		n := G.AddNode(entity.EntityName)
		for key, value := range entity.Attributes {
			G.NodeAttribute(n, key, utils.InterfaceToString(value))
		}
		rMap[entity.HiddenIndex] = n
	}

	for _, Relationship := range model.Relationships {
		sid, ok := rMap[Relationship.SourceEntityIndex]
		tid, ok2 := rMap[Relationship.TargetEntityIndex]
		if !ok || !ok2 {
			continue
		}
		G.AddEdge(sid, tid, Relationship.RelationshipType)
	}
	return G
}
