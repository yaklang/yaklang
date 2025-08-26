package entitybase

import (
	"errors"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type EntityBase struct {
	db        *gorm.DB
	baseInfo  *schema.EntityBaseInfo
	ragSystem *rag.RAGSystem
}

func (eb *EntityBase) GetInfo() (*schema.EntityBaseInfo, error) {
	if eb.baseInfo == nil {
		return nil, utils.Errorf("entity base info is nil")
	}
	return eb.baseInfo, nil
}

func (eb *EntityBase) AppendEntityAttr(entityId uint, attrs []*schema.ERModelAttribute) error {
	if len(attrs) == 0 {
		return nil
	}

	return utils.GormTransaction(eb.db, func(tx *gorm.DB) error {
		attributes, err := yakit.QueryAttributes(tx, &yakit.AttributeFilter{
			EntityID:     []uint{entityId},
			EntityBaseID: []uint{eb.baseInfo.ID},
		})
		if err != nil {
			return err
		}

		for _, attr := range attrs {
			found := false
			for _, attribute := range attributes {
				if attribute.AttributeName == attr.AttributeName && attribute.AttributeValue == attr.AttributeValue {
					found = true
				}
			}
			if !found {
				attr.EntityID = entityId
				attr.EntityBaseID = eb.baseInfo.ID
				err := yakit.CreateAttribute(tx, attr)
				if err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (eb *EntityBase) SearchEntity(entity *schema.ERModelEntity) ([]*schema.ERModelEntity, error) {
	// name and type query
	entities, err := yakit.QueryEntities(eb.db, &yakit.EntityFilter{
		Name: []string{entity.EntityName},
		Type: []string{entity.EntityType},
	})
	if err != nil {
		return nil, err
	}

	if len(entities) > 0 {
		return entities, nil
	}

	// unique attribute query
	var attributeIndexedId []uint
	for _, attribute := range entity.Attributes {
		if attribute.UniqueIdentifier {
			id, ok := yakit.UniqueAttributesIndexEntity(eb.db, eb.baseInfo.ID, attribute.AttributeName, attribute.AttributeValue)
			if ok {
				attributeIndexedId = append(attributeIndexedId, id)
			}
		}
	}

	return yakit.QueryEntities(eb.db, &yakit.EntityFilter{
		ID: attributeIndexedId,
	})
}

func (eb *EntityBase) VectorSearchEntity(entity *schema.ERModelEntity) ([]*schema.ERModelEntity, error) {
	// todo vector search
	return nil, utils.Errorf("not implemented")
}

func (eb *EntityBase) CreateEntity(entity *schema.ERModelEntity) error {
	return yakit.CreateEntityAndAttr(eb.db, entity)
}

func (eb *EntityBase) GetRAGSystem() *rag.RAGSystem {
	return eb.ragSystem
}

func CreateEntityBase(db *gorm.DB, name, description string, opts ...any) (*EntityBase, error) {
	entityBaseInfo := schema.EntityBaseInfo{
		EntityBaseName: name,
		Description:    description,
	}
	return &EntityBase{
		db:       db,
		baseInfo: &entityBaseInfo,
	}, nil
}

// NewEntityBase 创建新的实体库实例（先获取，获取不到则创建）
func NewEntityBase(db *gorm.DB, name, description string, opts ...any) (*EntityBase, error) {
	var entityBaseInfo schema.EntityBaseInfo
	err := db.Model(&schema.EntityBaseInfo{}).Where("entity_base_name = ?", name).First(&entityBaseInfo).Error

	var needCreateInfo bool
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			needCreateInfo = true
		} else {
			return nil, utils.Errorf("查询实体库信息失败: %v", err)
		}
	}

	collectionExists := rag.CollectionIsExists(db, name)

	if needCreateInfo && !collectionExists {
		err = utils.GormTransaction(db, func(tx *gorm.DB) error {
			entityBaseInfo = schema.EntityBaseInfo{
				EntityBaseName: name,
				Description:    description,
			}
			return yakit.CreateEntityBase(tx, &entityBaseInfo)
		})
		if err != nil {
			return nil, utils.Errorf("创建实体库信息失败: %v", err)
		}

		ragSystem, err := rag.CreateCollection(db, name, description, opts...)
		if err != nil {
			_ = utils.GormTransaction(db, func(tx *gorm.DB) error {
				return yakit.DeleteEntityBase(tx, int64(entityBaseInfo.ID))
			})
			return nil, utils.Errorf("创建RAG集合失败: %v", err)
		}

		return &EntityBase{
			db:        db,
			baseInfo:  &entityBaseInfo,
			ragSystem: ragSystem,
		}, nil
	}

	// 如果实体库信息存在但 RAG Collection 不存在，创建 RAG Collection
	if !needCreateInfo && !collectionExists {
		ragSystem, err := rag.CreateCollection(db, name, entityBaseInfo.Description, opts...)
		if err != nil {
			return nil, utils.Errorf("创建RAG集合失败: %v", err)
		}

		return &EntityBase{
			db:        db,
			baseInfo:  &entityBaseInfo,
			ragSystem: ragSystem,
		}, nil
	}

	// 如果实体库信息不存在但 RAG Collection 存在，创建实体库信息
	if needCreateInfo && collectionExists {
		err = utils.GormTransaction(db, func(tx *gorm.DB) error {
			entityBaseInfo = schema.EntityBaseInfo{
				EntityBaseName: name,
				Description:    description,
			}
			return yakit.CreateEntityBase(tx, &entityBaseInfo)
		})
		if err != nil {
			return nil, utils.Errorf("创建实体库信息失败: %v", err)
		}
	}

	// 如果都存在，直接加载
	ragSystem, err := rag.LoadCollection(db, name)
	if err != nil {
		return nil, utils.Errorf("加载RAG集合失败: %v", err)
	}

	return &EntityBase{
		db:        db,
		baseInfo:  &entityBaseInfo,
		ragSystem: ragSystem,
	}, nil
}
