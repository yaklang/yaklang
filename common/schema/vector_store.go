package schema

import (
	"database/sql/driver"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	META_Doc_Index  = "meta_document_index"
	META_Doc_Name   = "meta_document_name"
	META_Base_Index = "meta_base_index"
)

// FloatArray 用于在数据库中存储浮点数数组的自定义类型
// 实现了 driver.Valuer 和 sql.Scanner 接口，支持数据库存储和读取
type FloatArray []float32

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

type RAGDocumentType string

const (
	RAGDocumentType_Entity       RAGDocumentType = "entity"
	RAGDocumentType_Relationship RAGDocumentType = "relationship"
	RAGDocumentType_Knowledge    RAGDocumentType = "knowledge"
	RAGDocumentType_KHop         RAGDocumentType = "khop"
	RAGDocumentType_Unclassified RAGDocumentType = ""
)

// MetadataMap 用于存储文档元数据的自定义类型
// 实现了 driver.Valuer 和 sql.Scanner 接口，支持在数据库中存储 map 类型数据
type MetadataMap map[string]interface{}

func (m MetadataMap) GetDocIndex() (string, bool) {
	index, ok := m[META_Doc_Index]
	return utils.InterfaceToString(index), ok
}

func (m MetadataMap) GetBaseIndex() (string, bool) {
	index, ok := m[META_Base_Index]
	return utils.InterfaceToString(index), ok
}

// Value 实现 driver.Valuer 接口，用于将 map 转换为数据库可以存储的值
func (m MetadataMap) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	bytes, err := json.Marshal(m)
	return string(bytes), err
}

// Scan 实现 sql.Scanner 接口，用于将数据库存储的值转换回 map
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

// GroupInfos 用于存储 HNSW 图结构中节点连接信息数组的自定义类型
// 实现了 driver.Valuer 和 sql.Scanner 接口，支持在数据库中存储复杂结构数组
type GroupInfos []GroupInfo

// Value 实现 driver.Valuer 接口，用于将 GroupInfos 转换为数据库可以存储的值
func (g GroupInfos) Value() (driver.Value, error) {
	if g == nil {
		return nil, nil
	}
	bytes, err := json.Marshal(g)
	return string(bytes), err
}

// Scan 实现 sql.Scanner 接口，用于将数据库存储的值转换回 GroupInfos
func (g *GroupInfos) Scan(value interface{}) error {
	if value == nil {
		*g = nil
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

	// 处理空字符串或空字节数组的情况
	if len(bytes) == 0 {
		*g = GroupInfos{}
		return nil
	}

	return json.Unmarshal(bytes, g)
}

// GroupInfo 用于存储 HNSW 图结构中节点连接信息的自定义类型
// 实现了 driver.Valuer 和 sql.Scanner 接口，支持在数据库中存储复杂结构
type GroupInfo struct {
	LayerLevel int
	Key        string
	Neighbors  []string
}

// Value 实现 driver.Valuer 接口，用于将 GroupInfo 转换为数据库可以存储的值
func (g GroupInfo) Value() (driver.Value, error) {
	bytes, err := json.Marshal(g)
	return string(bytes), err
}

// Scan 实现 sql.Scanner 接口，用于将数据库存储的值转换回 GroupInfo
func (g *GroupInfo) Scan(value interface{}) error {
	if value == nil {
		*g = GroupInfo{}
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
	return json.Unmarshal(bytes, g)
}

// VectorStoreCollection 表示向量存储中的集合
// 用于管理一组具有相同向量维度和配置的文档
type VectorStoreCollection struct {
	gorm.Model

	// 集合名称，在系统中唯一
	Name string `gorm:"unique_index;" json:"name"`

	// 集合描述信息
	Description string `gorm:"type:text" json:"description"`

	// 使用的嵌入模型名称
	ModelName string `gorm:"index" json:"model_name"`

	// 向量维度，所有文档的嵌入向量必须具有相同的维度
	Dimension int `gorm:"not null" json:"dimension"`

	// HNSW 算法参数配置
	M                int     `gorm:"default:16" json:"m"`                        // 最大邻居数，影响图的连接密度
	Ml               float64 `gorm:"default:0.25" json:"ml"`                     // 层生成因子，控制层级分布
	EfSearch         int     `gorm:"default:20" json:"ef_search"`                // 搜索时的候选节点数
	EfConstruct      int     `gorm:"default:200" json:"ef_construct"`            // 构建时的候选节点数
	DistanceFuncType string  `gorm:"default:'cosine'" json:"distance_func_type"` // 距离函数类型（cosine、euclidean等）

	// HNSW 图连接信息，存储为 JSON 格式
	GroupInfos GroupInfos `gorm:"type:text" json:"group_infos"`
}

func (v *VectorStoreCollection) TableName() string {
	return "rag_vector_collection_test"
}

// VectorStoreDocument 表示向量存储中的文档
// 包含文档的嵌入向量、元数据和 HNSW 图相关信息
type VectorStoreDocument struct {
	gorm.Model

	// entity / relationship / knowledge
	DocumentType    RAGDocumentType
	EntityID        string `gorm:"index"`
	RelatedEntities string // text split by ","

	// 文档唯一标识符，在整个系统中唯一
	DocumentID string `gorm:"uniqueIndex:idx_document_id_collection_id;not null" json:"document_id"`

	// 所属集合的ID，建立外键关系
	CollectionID uint `gorm:"uniqueIndex:idx_document_id_collection_id;not null" json:"collection_id"`

	// 文档元数据，以 JSON 格式存储，包含原始文本、来源等信息
	Metadata MetadataMap `gorm:"type:text" json:"metadata"`

	// 文档的嵌入向量，以 JSON 格式存储
	Embedding FloatArray `gorm:"type:text;not null" json:"embedding"`

	// 文档的原始文本
	Content string `gorm:"type:text" json:"content"`

	// HNSW 算法中节点存在的最高层级
	MaxLayer int `gorm:"default:0" json:"max_layer"`

	RuntimeID string
}

func (v *VectorStoreDocument) TableName() string {
	return "rag_vector_document_test"
}

// StringArray 用于存储字符串数组的自定义类型
// 实现了 driver.Valuer 和 sql.Scanner 接口，支持数据库存储和读取
type StringArray []string

// Value 实现 driver.Valuer 接口，用于将 []string 转换为数据库可以存储的值
func (s StringArray) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	// 使用逗号分隔的字符串格式存储
	return strings.Join(s, ","), nil
}

// Scan 实现 sql.Scanner 接口，用于将数据库存储的值转换回 []string
func (s *StringArray) Scan(value interface{}) error {
	if value == nil {
		*s = nil
		return nil
	}
	var str string
	switch v := value.(type) {
	case []byte:
		str = string(v)
	case string:
		str = v
	default:
		return utils.Errorf("不支持的类型: %T", value)
	}

	// 处理空字符串的情况
	if str == "" {
		*s = StringArray{}
		return nil
	}

	// 使用逗号分隔字符串并去除空白
	parts := utils.StringSplitAndStrip(str, ",")
	*s = StringArray(parts)
	return nil
}

type KnowledgeBaseInfo struct {
	gorm.Model

	// 知识库名称(唯一)
	KnowledgeBaseName string `gorm:"unique_index;not null" json:"knowledge_base_name"`

	// 知识库描述
	KnowledgeBaseDescription string `gorm:"type:text" json:"knowledge_base_description"`

	// 知识库类型
	KnowledgeBaseType string `gorm:"index;not null" json:"knowledge_base_type"`
}

func (v *KnowledgeBaseInfo) TableName() string {
	return "rag_knowledge_base_test"
}

// KnowledgeBase 表示知识库条目
// 用于存储各种标准、指南等知识库信息
type KnowledgeBaseEntry struct {
	gorm.Model

	// 知识库名称
	KnowledgeBaseID int64 `gorm:"not null" json:"knowledge_base_id"`

	RelatedEntityUUIDS string // split by ","

	// 知识标题(和知识库名称应该是联合唯一索引)
	KnowledgeTitle string `gorm:"not null" json:"knowledge_title"`

	// 知识类型（如：CoreConcept、Standard、Guideline等）
	KnowledgeType string `gorm:"index;not null" json:"knowledge_type"`

	// 重要性评分（1-10）
	ImportanceScore int `gorm:"index" json:"importance_score"`

	// 关键词列表，用于快速搜索和分类
	Keywords StringArray `gorm:"type:text" json:"keywords"`

	// 知识详细信息，包含具体内容描述
	KnowledgeDetails string `gorm:"type:text" json:"knowledge_details"`

	// 知识摘要，简要概述
	Summary string `gorm:"type:text" json:"summary"`

	// 来源页码或章节编号
	SourcePage int `gorm:"index" json:"source_page"`

	// 潜在问题列表，这些问题可能与该知识条目相关
	PotentialQuestions StringArray `gorm:"type:text" json:"potential_questions"`

	// 潜在问题向量，用于快速搜索潜在问题
	PotentialQuestionsVector FloatArray `gorm:"type:text" json:"potential_questions_vector"`

	// 唯一标识符，用于在向量索引中唯一标识该知识条目
	HiddenIndex string `gorm:"unique_index"`
}

func (e *KnowledgeBaseEntry) TableName() string {
	return "rag_knowledge_entry_test"
}

func (e *KnowledgeBaseEntry) BeforeSave() error {
	if e.HiddenIndex == "" {
		e.HiddenIndex = uuid.NewString()
	}
	return nil
}

func init() {
	// 注册数据库表结构到系统中
	RegisterDatabaseSchema(KEY_SCHEMA_PROFILE_DATABASE, &VectorStoreCollection{}, &VectorStoreDocument{}, &KnowledgeBaseInfo{}, &KnowledgeBaseEntry{})
}
