// Package aistats 实现"用户命中统计"采集 (User AI Stats).
//
// 它把 Agent 运行过程中的工具命中 / SKILL 命中 / action 执行 / AI 调用 token 用量
// 异步 (非阻塞有界队列 + 单 worker) 落进 profile DB 的两张表:
//
//   - AIStatsEntityHit: per-entity (skill|tool) 命中计数, 供意图识别层做 Top-N 反馈排序;
//   - AIUserDailyStats: per-user / per-day 汇总 (actions / ai_calls / tool_calls_total /
//     skills_loaded / tokens_input / tokens_output), 供趋势查询.
//
// 硬约束 (与 aive 同构):
//   - 非阻塞: 仅向有界 channel 非阻塞投递, 队列满直接丢弃 + 日志, 绝不阻塞主循环.
//   - 不崩溃: worker / 落库 / 全程 recover.
//   - DB nil-safe: 每次落库前探针 consts.GetGormProfileDatabase(), DB 不可用时静默丢弃;
//     绝不在 init() 里调用 GetGormProfileDatabase() (会触发全局 DB 懒加载副作用).
//   - 不本地存储的额外约束不适用: 本模块的职责就是本地落库 (双表).
//
// 关键词: aistats, UserAIStats, 命中统计, per-entity hit, daily stats, 非阻塞有界队列
package aistats

import (
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// resolveUserKey 从 Config 解析 user 标识. 优先 PersistentSessionId, 回退 "default".
func resolveUserKey(cfg *aicommon.Config) string {
	if cfg == nil {
		return DefaultUserKey
	}
	if cfg.PersistentSessionId != "" {
		return cfg.PersistentSessionId
	}
	return DefaultUserKey
}

// today 返回当前本地日期 YYYY-MM-DD.
func today() string {
	return time.Now().Format("2006-01-02")
}
