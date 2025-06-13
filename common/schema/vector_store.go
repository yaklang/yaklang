package schema

import (
	"database/sql/driver"
	"encoding/json"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
)

// 为了在数据库中存储浮点数数组，我们需要创建一个自定义类型
type FloatArray []float64

// Value 实现 driver.Valuer 接口，用于将 []float64 转换为数据库可以存储的值
func (f FloatArray) Value() (driver.Value, error) {
	if f == nil {
		return nil, nil
	}
	bytes, err := json.Marshal(f)
	return string(bytes), err
}

// Scan 实现 sql.Scanner 接口，用于将数据库存储的值转换回 []float64
func (f *FloatArray) Scan(value interface{}) error {
	if value == nil {
		*f = nil
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return utils.Errorf("不支持的类型: %T", value)
	}
	return json.Unmarshal(bytes, f)
}

// MetadataMap 用于存储文档元数据
type MetadataMap map[string]interface{}

// Value 实现 driver.Valuer 接口
func (m MetadataMap) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	bytes, err := json.Marshal(m)
	return string(bytes), err
}

// Scan 实现 sql.Scanner 接口
func (m *MetadataMap) Scan(value interface{}) error {
	if value == nil {
		*m = nil
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return utils.Errorf("不支持的类型: %T", value)
	}
	return json.Unmarshal(bytes, m)
}

// VectorStoreCollection 表示向量存储中的集合
type VectorStoreCollection struct {
	gorm.Model

	// 集合名称
	Name string `gorm:"unique_index;index:idx_name" json:"name"`

	// 集合描述
	Description string `json:"description"`

	// 模型名称
	ModelName string `json:"model_name"`

	// 存储该集合中的向量维度
	Dimension int `json:"dimension"`
}

// VectorStoreDocument 表示向量存储中的文档
type VectorStoreDocument struct {
	gorm.Model

	// 文档唯一标识符
	DocumentID string `gorm:"unique_index" json:"document_id"`

	// 所属集合的ID
	CollectionID uint `json:"collection_id" gorm:"index"`

	// 文档元数据，以JSON格式存储
	Metadata MetadataMap `gorm:"type:text" json:"metadata"`

	// 文档的嵌入向量，以JSON格式存储
	Embedding FloatArray `gorm:"type:text" json:"embedding"`
}

func init() {
	// 注册到数据库模式中
	RegisterDatabaseSchema(KEY_SCHEMA_PROFILE_DATABASE, &VectorStoreCollection{}, &VectorStoreDocument{})
}
