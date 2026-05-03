// Package aicache 在 aispec.ChatBase 入口做镜像观测，
// 把每次 prompt 按 PROMPT_SECTION 外层标签切片，统计前缀缓存命中率，
// 节流打印诊断行；DEBUG 模式下把每次 prompt 完整落盘，
// 便于事后分析"哪儿污染了缓存"。
//
// 关键词: aicache, 镜像观测, 缓存命中率, prompt 落盘
package aicache

import (
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils"
)

// gCache 是 aicache 全局唯一的缓存表
// 关键词: aicache, gCache
var gCache = newGlobalCache(defaultMaxRequests)

// gPrinter 是 aicache 全局唯一的节流打印器
// 关键词: aicache, gPrinter
var gPrinter = newThrottlePrinter(minPrintInterval)

// init 在 aicache 包加载时把 Observe 注册到 aispec 的 ChatBase 镜像 hook
// 关键词: aicache, init, RegisterChatBaseMirrorObserver
func init() {
	aispec.RegisterChatBaseMirrorObserver(Observe)
}

// Observe 是 aicache 的核心入口，对每次 ChatBase 调用做镜像观测
// 不返回错误、不阻塞主流程；observer 自身的所有错误都内部消化
//
// 处理流程：
//  1. Split: 按 PROMPT_SECTION 外层标签切片
//  2. Record: 全局缓存表登记 + LCP 前缀命中率计算
//  3. Advice: 生成测算建议
//  4. Trigger: 节流打印
//  5. Dump: DEBUG 模式下落盘完整 prompt
//
// 关键词: aicache, Observe, 镜像入口
func Observe(model, msg string) {
	if msg == "" {
		return
	}
	split := Split(msg)
	rep := gCache.Record(split, model)
	rep.Advices = buildAdvices(rep, split)
	gPrinter.Trigger(rep)
	utils.Debug(func() {
		dumpDebug(rep, split, gCache)
	})
}

// ResetForTest 仅供测试使用：重置全局状态
// 关键词: aicache, ResetForTest
func ResetForTest() {
	gCache = newGlobalCache(defaultMaxRequests)
	gPrinter = newThrottlePrinter(minPrintInterval)
}
