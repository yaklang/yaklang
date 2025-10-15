package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
)

// AIMemoryCollection 存储AI记忆的HNSW索引信息
type AIMemoryCollection struct {
	gorm.Model

	// 会话ID，每个会话有一个独立的HNSW索引
	SessionID string `json:"session_id" gorm:"unique_index;not null"`

	// HNSW Graph 的二进制序列化数据
	GraphBinary []byte `json:"graph_binary" gorm:"type:blob"`

	// HNSW 参数配置
	M           int     `json:"m" gorm:"default:16"`             // 最大邻居数
	Ml          float64 `json:"ml" gorm:"default:0.25"`          // 层生成因子
	EfSearch    int     `json:"ef_search" gorm:"default:20"`     // 搜索时的候选节点数
	EfConstruct int     `json:"ef_construct" gorm:"default:200"` // 构建时的候选节点数

	// 向量维度（固定为7维 - C.O.R.E. P.A.C.T.）
	Dimension int `json:"dimension" gorm:"default:7"`
}

func (a *AIMemoryCollection) TableName() string {
	return "ai_memory_collections"
}

func (a *AIMemoryCollection) BeforeSave() error {
	if a.SessionID == "" {
		return utils.Errorf("session_id must be set")
	}
	if a.Dimension == 0 {
		a.Dimension = 7 // 默认7维
	}
	return nil
}
