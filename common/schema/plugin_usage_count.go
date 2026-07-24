package schema

import "github.com/yaklang/gorm"

// PluginUsageCount 记录插件的全局使用次数，注册在 profile 库（与 yak_scripts 同库），
// 供 QueryYakScript 按使用次数排序（单库子查询，避免跨库 JOIN）。
// 与 project 库的 ExecHistory 分工：ExecHistory 存按项目隔离的完整历史（恢复现场用），
// PluginUsageCount 只存全局计数（排序/排行用），写入时由 SavePluginExecutionHistory 双写。
type PluginUsageCount struct {
	gorm.Model

	PluginId   int64  `json:"plugin_id" gorm:"unique_index"`
	PluginName string `json:"plugin_name" gorm:"index"`
	PluginUUID string `json:"plugin_uuid"`
	PluginType string `json:"plugin_type" gorm:"index"`
	HeadImg    string `json:"head_img"`
	Count      int64  `json:"count"`
	LastUsedAt int64  `json:"last_used_at" gorm:"index"`
}