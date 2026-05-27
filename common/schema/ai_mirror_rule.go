package schema

import (
	"time"

	"github.com/jinzhu/gorm"
)

// AiMirrorRule represents a mirror rule used by aibalance to forward a snapshot
// of every completed chat request to a user-defined yak callback script.
//
// 关键词: AiMirrorRule, aibalance mirror, callback yak script, traffic mirror
//
// 触发条件 (ConditionType):
//   - "action_eq":           解析响应主输出 (不含 reason) 中 yaklang JSON 协议
//                            字段 @action, 等于 ActionName 时触发
//   - "any_toolcall":        响应中累积过任意 OpenAI 原生 tool_calls 时触发
//   - "action_call_tool_eq": @action ∈ {call-tool, directly_call_tool, require_tool}
//                            且 payload 中 tool 字段等于 ToolName 时触发
//   - "always":              永真, 每次成功请求都触发
//
// 计数器 (TotalTriggered / TotalSuccess / TotalFailed / TotalDropped) 由后台 worker
// 通过原子操作累加; portal 读侧直接 SELECT 字段值即可, 不强依赖立即一致.
type AiMirrorRule struct {
	gorm.Model

	Name          string `json:"name" gorm:"index;not null"`
	Enabled       bool   `json:"enabled" gorm:"default:true"`
	ConditionType string `json:"condition_type" gorm:"size:64;not null"` // action_eq / any_toolcall / action_call_tool_eq / always

	// ActionName 仅在 ConditionType=action_eq / action_call_tool_eq 时有意义.
	ActionName string `json:"action_name" gorm:"size:128"`

	// ToolName 仅在 ConditionType=action_call_tool_eq 时有意义.
	ToolName string `json:"tool_name" gorm:"size:128"`

	// CallbackScript 是一段 yak 脚本, 必须定义 handle(data) 函数.
	CallbackScript string `json:"callback_script" gorm:"type:text"`

	// Concurrency 是 worker pool 的 goroutine 数 (同时运行的回调上限).
	Concurrency int `json:"concurrency" gorm:"default:4"`

	// QueueSize 是等待队列长度 (buffered channel 容量); 满后投递失败计为 dropped.
	QueueSize int `json:"queue_size" gorm:"default:1024"`

	// TimeoutMs 单次脚本执行超时, 超时强制 cancel; 0 / 负数 = 不超时 (不推荐).
	TimeoutMs int64 `json:"timeout_ms" gorm:"default:30000"`

	// 累计指标 (原子操作)
	TotalTriggered  int64     `json:"total_triggered"`
	TotalSuccess    int64     `json:"total_success"`
	TotalFailed     int64     `json:"total_failed"`
	TotalDropped    int64     `json:"total_dropped"`
	LastTriggeredAt time.Time `json:"last_triggered_at"`
}

// TableName 显式声明表名, 与现有 aibalance schema 风格 (snake_case + 复数) 一致.
func (a *AiMirrorRule) TableName() string {
	return "ai_mirror_rules"
}

func init() {
	RegisterDatabaseSchema(KEY_SCHEMA_PROFILE_DATABASE, &AiMirrorRule{})
}
