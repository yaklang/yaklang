package schema

import (
	"time"

	"github.com/yaklang/gorm"
)

// AIStatsEntityHit 是 per-entity 的命中计数表 (用户级 profile DB).
//
// 用途: SKILL / 工具的命中反馈. 命中来源 (source) 细分:
//   - tool:  direct (直接调用) / requested (申请)
//   - skill: user_force (用户强制加载) / ai_load (AI 自主加载) / intent_catalog (意图识别入选目录)
//
// 这张表是「重要反馈点」的数据落地, 供意图识别层做 Top-N 命中数排序,
// 让高频使用的 SKILL / 工具优先进入 prompt 目录.
//
// 关键词: AIStatsEntityHit, 命中计数, 反馈排序, per-entity stats
type AIStatsEntityHit struct {
	gorm.Model

	// EntityType 取值 "skill" | "tool".
	EntityType string `json:"entity_type" gorm:"unique_index:idx_ai_stats_entity_hit"`

	// EntityName 是技能名 / 工具名.
	EntityName string `json:"entity_name" gorm:"unique_index:idx_ai_stats_entity_hit"`

	// HitCount 是该实体的累计命中次数 (所有来源之和).
	HitCount int `json:"hit_count" gorm:"default:0;index"`

	// DirectCount 细分: 直接调用 (tool) 或用户强制加载 (skill) 的次数.
	DirectCount int `json:"direct_count" gorm:"default:0"`

	// RequestedCount 细分: 申请路径 (tool) 的命中次数. skill 为 0.
	RequestedCount int `json:"requested_count" gorm:"default:0"`

	// AutoLoadedCount 细分: AI 意图驱动加载 (skill) 或 catalog 入选 (skill) 的次数.
	// tool 一般为 0.
	AutoLoadedCount int `json:"auto_loaded_count" gorm:"default:0"`

	// LastHitAt 是最近一次命中时间, 用于反馈排序的同分时间序与衰减计算.
	LastHitAt time.Time `json:"last_hit_at"`
}

func (a *AIStatsEntityHit) TableName() string {
	return "ai_stats_entity_hits"
}

// AIStatsEntityType 枚举值, 用于 AIStatsEntityHit.EntityType.
const (
	AIStatsEntityTypeSkill = "skill"
	AIStatsEntityTypeTool  = "tool"
)

// AIUserDailyStats 是 per-user / per-day 的汇总统计表 (用户级 profile DB).
//
// 用途: 回答「每天执行了多少 Action / 多少次 AI 调用 / 多少次工具调用 /
// 加载了多少 SKILL / 消耗多少 token」这类趋势问题. 一行 = 一个用户的一天.
//
// 关键词: AIUserDailyStats, 每日汇总, 用户特征, UserAIStats
type AIUserDailyStats struct {
	gorm.Model

	// UserKey 是用户标识, 优先取 PersistentSessionId, 回退 "default".
	UserKey string `json:"user_key" gorm:"unique_index:idx_ai_user_daily_stats"`

	// Day 是日期 (YYYY-MM-DD, 本地时区).
	Day string `json:"day" gorm:"unique_index:idx_ai_user_daily_stats"`

	// Actions 是当天执行的 loop action 总数.
	Actions int `json:"actions" gorm:"default:0"`

	// AICalls 是当天发起的 AI (模型) 调用次数.
	AICalls int `json:"ai_calls" gorm:"default:0"`

	// ToolCallsTotal 是当天工具调用总次数 (成功 + 失败).
	ToolCallsTotal int `json:"tool_calls_total" gorm:"default:0"`

	// SkillsLoaded 是当天加载 (任意来源) 的 SKILL 累计次数.
	SkillsLoaded int `json:"skills_loaded" gorm:"default:0"`

	// TokensInput 是当天输入 token 累计 (来自 ChatUsage.PromptTokens).
	TokensInput int64 `json:"tokens_input" gorm:"default:0"`

	// TokensOutput 是当天输出 token 累计 (来自 ChatUsage.CompletionTokens).
	TokensOutput int64 `json:"tokens_output" gorm:"default:0"`
}

func (a *AIUserDailyStats) TableName() string {
	return "ai_user_daily_stats"
}
