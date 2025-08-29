package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
)

func CreateEntityBaseInfo(db *gorm.DB, entityBase *schema.EntityBaseInfo) error {
	return db.Create(entityBase).Error
}

func DeleteEntityBaseInfo(db *gorm.DB, id int64) error {
	return db.Unscoped().Delete(&schema.EntityBaseInfo{}, id).Error
}

type EntityFilter struct {
	Name         []string
	Type         []string
	ID           []uint
	EntityBaseID []uint
}

func FilterEntities(db *gorm.DB, entityFilter *EntityFilter) *gorm.DB {
	if entityFilter == nil {
		return db
	}
	db = db.Model(&schema.ERModelEntity{})
	db = bizhelper.ExactQueryUIntArrayOr(db, "id", entityFilter.ID)
	db = bizhelper.ExactQueryUIntArrayOr(db, "entity_base_id", entityFilter.EntityBaseID)
	db = bizhelper.ExactQueryStringArrayOr(db, "entity_name", entityFilter.Name)
	db = bizhelper.ExactQueryStringArrayOr(db, "entity_type", entityFilter.Type)

	return db
}

func QueryEntities(db *gorm.DB, entityFilter *EntityFilter) ([]*schema.ERModelEntity, error) {
	db = db.Model(&schema.ERModelEntity{})
	db = FilterEntities(db, entityFilter)
	var entities []*schema.ERModelEntity
	err := db.Find(&entities).Error
	return entities, err
}

func UpdateEntity(db *gorm.DB, id uint, entity *schema.ERModelEntity) error {
	return db.Model(&schema.ERModelEntity{}).Where("id = ?", id).Updates(entity).Error
}

func CreateEntity(db *gorm.DB, entity *schema.ERModelEntity) error {
	return db.Create(entity).Error
}

func GetEntityByID(db *gorm.DB, id uint) (*schema.ERModelEntity, error) {
	var entity schema.ERModelEntity
	db = db.Model(&schema.ERModelEntity{}).Preload("Attributes").Preload("IncomingRelationships").Preload("OutgoingRelationships")
	if err := db.Where("id = ?", id).First(&entity).Error; err != nil {
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

func AddRelationship(db *gorm.DB, sourceID, targetID uint, RelationshipType, decisionRationale string, attrs map[string]any) error {
	Relationship := schema.ERModelRelationship{
		SourceEntityID:    sourceID,
		TargetEntityID:    targetID,
		RelationshipType:  RelationshipType,
		DecisionRationale: decisionRationale,
		Attributes:        attrs,
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
