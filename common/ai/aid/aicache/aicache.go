// Package aicache 在 aispec.ChatBase 入口做镜像观测 + 可选 messages 改写。
//
// Observe 是合并后的入口：
//  1. 缓存分析（Split → Record → buildAdvices → 节流打印 → DEBUG 落盘）保持
//     与原 mirror 行为一致
//  2. 在结尾尝试 hijackHighStatic：如果 prompt 包含 high-static 段，则把它
//     拆出来包成 <|AI_CACHE_SYSTEM_high-static|>...<|AI_CACHE_SYSTEM_END_high-static|>
//     作为 role:system 单独消息。
//
//     从 §7.7 起进一步把剩余 user 段按 timeline 的 Frozen/Open 边界拆成
//     两条 user 消息（高命中前缀 user1 + 易变 user2）；不可拆分时退化到
//     原 2 段 [system, user] 形态。返回的 ChatBaseMirrorResult{IsHijacked:true}
//     由 ChatBase 灌入 ctx.RawMessages 走现有 RawMessages 透传通道。
//
//     §7.7.7 职责重排：3 段路径下 hijacker 自己给 system + user1 主动打
//     ephemeral cache_control（包成 []*aispec.ChatContent 形态），实现
//     "system 短前缀 + system+user1 长前缀" 的双 cc 命中（E14 实测 70%）。
//     下游 aibalance.RewriteMessagesForExplicitCache 检测到客户端自带 cc 后
//     完全 pass-through，避免双注入风险。2 段退化路径仍由 hijacker 输出
//     string content，让 aibalance 走 baseline 单 cc 兜底。
//
// 关键词: aicache, Observe, mirror, hijack 合一, role:system 注入,
//        3 段拆分, frozen/open 边界, §7.7, §7.7.7 hijacker 自管双 cc
package aicache

import (
	"strconv"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils"
)

// gCache 是 aicache 全局唯一的缓存表
// 关键词: aicache, gCache
var gCache = newGlobalCache(defaultMaxRequests)

// gPrinter 是 aicache 全局唯一的节流打印器
// 关键词: aicache, gPrinter
var gPrinter = newThrottlePrinter(minPrintInterval)

// init 在 aicache 包加载时把 Observe 注册到 aispec 的 mirror observer 链
// 关键词: aicache, init, RegisterChatBaseMirrorObserver
func init() {
	aispec.RegisterChatBaseMirrorObserver(Observe)
}

// Observe 是 aicache 合并后的核心入口：
//  1. 同步完成缓存分析（Split / Record / Advice / Trigger）
//  2. dumpDebug 文件 I/O 用独立 goroutine 调度，避免阻塞 mirror 同步分发
//  3. 在结尾返回 hijack 决策：若 prompt 中有 high-static 段，则返回
//     IsHijacked=true 让 ChatBase 把切出来的 messages 走 RawMessages 透传
//
// observer 自身的所有错误均内部消化，不向上抛。
//
// 关键词: aicache, Observe, mirror 入口, hijack 决策
func Observe(model, msg string) *aispec.ChatBaseMirrorResult {
	if msg == "" {
		return nil
	}
	split := Split(msg)
	rep := gCache.Record(split, model)
	rep.Advices = buildAdvices(rep, split)
	gPrinter.Trigger(rep)
	utils.Debug(func() {
		// dumpDebug 是文件 I/O，放后台 goroutine 调度；不阻塞 mirror 同步分发
		// 关键词: aicache, dumpDebug 异步, mirror 同步分发
		go dumpDebug(rep, split, gCache)
	})

	// 把本次观测的 SeqId 作为关联 ID 透传给 ChatBase, ChatBase 会把它复制到
	// SSE 末帧 ChatUsage.MirrorCorrelationID 上, 让上层 (例如 cachebench)
	// 用稳定 ID 把 dump 文件 (000XXX.txt 名为 SeqId) 与 token usage 精确 join,
	// 避免之前按数组下标对齐时因 stream-finished 漏 callback 累计错位的归因 bug.
	// 关键词: aicache Observe MirrorCorrelationID, dump usage 精确 join
	result := hijackHighStatic(msg)
	if result == nil {
		result = &aispec.ChatBaseMirrorResult{}
	}
	if rep != nil && rep.SeqId > 0 {
		result.MirrorCorrelationID = strconv.FormatInt(rep.SeqId, 10)
	}
	return result
}

// ResetForTest 仅供测试使用：重置全局状态
// 关键词: aicache, ResetForTest
func ResetForTest() {
	gCache = newGlobalCache(defaultMaxRequests)
	gPrinter = newThrottlePrinter(minPrintInterval)
}
