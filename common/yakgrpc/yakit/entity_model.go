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
	db = db.Model(&schema.ERModelEntity{}).Preload("Attributes")
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
	if err := db.First(&entity, id).Error; err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return nil, utils.Errorf("entity not found")
		}
		return nil, err
	}
	return &entity, nil
}

type AttributeFilter struct {
	Name         []string
	Value        []string
	UniqueOnly   bool
	EntityID     []uint
	EntityBaseID []uint
}

func FilterAttributes(db *gorm.DB, filter *AttributeFilter) *gorm.DB {
	if filter == nil {
		return db
	}
	db = db.Model(&schema.ERModelAttribute{})
	db = bizhelper.ExactQueryStringArrayOr(db, "attribute_name", filter.Name)
	db = bizhelper.ExactQueryStringArrayOr(db, "attribute_value", filter.Value)
	db = bizhelper.ExactQueryUIntArrayOr(db, "entity_id", filter.EntityID)
	db = bizhelper.ExactQueryUIntArrayOr(db, "entity_base_id", filter.EntityBaseID)
	if filter.UniqueOnly {
		db = db.Where("unique_identifier = ?", true)
	}
	return db
}

func QueryAttributes(db *gorm.DB, filter *AttributeFilter) ([]*schema.ERModelAttribute, error) {
	db = db.Model(&schema.ERModelAttribute{})
	db = FilterAttributes(db, filter)
	var attributes []*schema.ERModelAttribute
	err := db.Find(&attributes).Error
	return attributes, err
}

func UniqueAttributesIndexEntity(db *gorm.DB, entityBaseId uint, name string, values string) (uint, bool) {
	db = FilterAttributes(db, &AttributeFilter{
		Name:         []string{name},
		Value:        []string{values},
		UniqueOnly:   true,
		EntityBaseID: []uint{entityBaseId},
	})
	var attribute = schema.ERModelAttribute{}
	err := db.Preload("Entity").First(&attribute).Error
	if err != nil {
		return 0, false
	}
	return attribute.EntityID, true
}

func CreateAttribute(db *gorm.DB, attribute *schema.ERModelAttribute) error {
	return db.Create(attribute).Error
}

func AddRelation(db *gorm.DB, sourceID, targetID uint, relationType, decisionRationale string) error {
	relation := schema.ERModelRelation{
		SourceEntityID:    sourceID,
		TargetEntityID:    targetID,
		RelationType:      relationType,
		DecisionRationale: decisionRationale,
	}
	relation.Hash = relation.CalcHash()
	return utils.GormTransaction(db, func(tx *gorm.DB) error {
		findErr := tx.Where("hash = ?", relation.Hash).First(&relation).Error
		if findErr == nil {
			return nil // 事务成功结束
		}
		if gorm.IsRecordNotFoundError(findErr) {
			return tx.Create(&relation).Error
		}
		return findErr
	})
}

// RemoveRelation 删除两个实体之间的永久关系。
func RemoveRelation(db *gorm.DB, sourceID, targetID uint, relationType string) error {
	result := db.Where("source_entity_id = ? AND target_entity_id = ? AND relation_type = ?",
		sourceID, targetID, relationType).Delete(&schema.ERModelRelation{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return utils.Errorf("relation not found to remove")
	}
	return nil
}
