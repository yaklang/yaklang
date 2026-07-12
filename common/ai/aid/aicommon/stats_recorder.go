package aicommon

import (
	"sync"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
)

// stats_recorder.go 是"用户命中统计"采集的瘦注册缝 (零额外重依赖).
//
// 设计与 value_feedback.go 同构: aicommon 只放接口 + 注册函数 + nil-safe 的 Submit* 入口;
// 真正的落库实现 (双表: per-entity 命中计数 + per-user/day 汇总) 放在 aistats 包,
// 由 aistats 在 init() 里调用 RegisterStatsRecorder 注册进来. reactloops / loopinfra /
// aid 等底层只调用 SubmitToolHit / SubmitSkillHit / SubmitAction / SubmitAICall, 不直接
// 依赖 aistats, 从而避免 import 环.
//
// 硬约束 (与 aive 一致):
//   - 非阻塞: 实现必须自带有界队列, 绝不阻塞调用方.
//   - 不崩溃: Submit* 全程 recover, 绝不 panic 到主流程.
//   - nil-safe: 未注册或 cfg 缺失时 no-op.
//
// 命中来源 (source) 枚举见下方常量, 决定细分计数列.
//
// 关键词: StatsRecorder, RegisterStatsRecorder, SubmitToolHit, SubmitSkillHit,
//
//	SubmitAction, SubmitAICall, 命中统计注册缝, UserAIStats

// 命中来源枚举, 决定 AIStatsEntityHit 的细分计数列.
const (
	StatsSourceToolDirect         = "direct"         // tool: 直接调用 (directly_call_tool 等)
	StatsSourceToolRequested      = "requested"      // tool: 申请路径 (require_tool / task_call_tool)
	StatsSourceSkillUserForce     = "user_force"     // skill: 用户强制加载 (load_skill sync 事件 / EnabledCapabilities)
	StatsSourceSkillAILoad        = "ai_load"        // skill: AI 自主 loading_skills action
	StatsSourceSkillIntentCatalog = "intent_catalog" // skill: 意图识别入选 catalog top-10
)

// StatsRecorder 由 aistats 实现并注册. 实现必须是非阻塞的 (内部有界队列),
// 绝不能阻塞或 panic 到调用方.
type StatsRecorder interface {
	// RecordToolHit 记录一次工具命中. source 取 StatsSourceTool* 常量.
	RecordToolHit(cfg *Config, toolName, source string)
	// RecordSkillHit 记录一次 SKILL 命中. source 取 StatsSourceSkill* 常量.
	RecordSkillHit(cfg *Config, skillName, source string)
	// RecordAction 记录一次 loop action 执行.
	RecordAction(cfg *Config, actionType string)
	// RecordAICall 记录一次 AI (模型) 调用及其 token 用量.
	RecordAICall(cfg *Config, model string, usage *aispec.ChatUsage)
}

var (
	statsRecorder   StatsRecorder
	statsRecorderMu sync.RWMutex
)

// RegisterStatsRecorder 由 aistats 在 init() 中调用注册统计实现.
// 默认开启: 注册后即生效.
func RegisterStatsRecorder(recorder StatsRecorder) {
	statsRecorderMu.Lock()
	defer statsRecorderMu.Unlock()
	statsRecorder = recorder
}

func getStatsRecorder() StatsRecorder {
	statsRecorderMu.RLock()
	defer statsRecorderMu.RUnlock()
	return statsRecorder
}

// SubmitToolHit 记录一次工具命中. 未注册或 cfg 缺失时 no-op; 全程 recover.
func SubmitToolHit(cfg *Config, toolName, source string) {
	rec := getStatsRecorder()
	if rec == nil || cfg == nil || toolName == "" {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Warnf("SubmitToolHit recovered panic: %v", r)
		}
	}()
	rec.RecordToolHit(cfg, toolName, source)
}

// SubmitSkillHit 记录一次 SKILL 命中. 未注册或 cfg 缺失时 no-op; 全程 recover.
func SubmitSkillHit(cfg *Config, skillName, source string) {
	rec := getStatsRecorder()
	if rec == nil || cfg == nil || skillName == "" {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Warnf("SubmitSkillHit recovered panic: %v", r)
		}
	}()
	rec.RecordSkillHit(cfg, skillName, source)
}

// SubmitAction 记录一次 loop action 执行. 未注册或 cfg 缺失时 no-op; 全程 recover.
func SubmitAction(cfg *Config, actionType string) {
	rec := getStatsRecorder()
	if rec == nil || cfg == nil || actionType == "" {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Warnf("SubmitAction recovered panic: %v", r)
		}
	}()
	rec.RecordAction(cfg, actionType)
}

// SubmitAICall 记录一次 AI (模型) 调用. 未注册或 cfg 缺失时 no-op; 全程 recover.
func SubmitAICall(cfg *Config, model string, usage *aispec.ChatUsage) {
	rec := getStatsRecorder()
	if rec == nil || cfg == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Warnf("SubmitAICall recovered panic: %v", r)
		}
	}()
	rec.RecordAICall(cfg, model, usage)
}
