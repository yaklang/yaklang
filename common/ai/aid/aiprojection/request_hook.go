// Package aiprojection parses prompts and projects them into provider-visible
// messages before ChatBase sends a request. Its pre-send hook also records cache
// observations and can replace the outgoing message list.
package aiprojection

import (
	"strconv"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils"
)

// gCache 是 aiprojection 全局唯一的缓存观察表
// 关键词: aicache, gCache
var gCache = newGlobalCache(defaultMaxRequests)

// gPrinter 是 aiprojection 全局唯一的节流打印器
// 关键词: aicache, gPrinter
var gPrinter = newThrottlePrinter(minPrintInterval)

// Register the combined projection and observation hook once per process.
func init() {
	aispec.RegisterChatBaseHijackHook(ProjectAndObserve)
}

// ProjectAndObserve is the ChatBase pre-send hook. It parses the outer prompt
// once, records cache statistics, then projects the parsed structure into messages.
// ChatBase copies hijacked messages to RawMessages before serializing either
// chat-completions or responses requests. Debug dump I/O runs asynchronously.
func ProjectAndObserve(model, msg string) *aispec.ChatBaseHijackResult {
	if msg == "" {
		return nil
	}
	parsed := Parse(msg)
	split := parsed.CacheSplit()
	rep := gCache.Record(split, model)
	// 传 gCache 让 advice 多输出 reusable_aitag_in_dynamic 跨 turn 诊断;
	// gCache 自身已经更新了 dynamicSubtagSightings。
	// 关键词: Observe advice 跨 turn 状态, AITag 漂移诊断
	rep.Advices = buildAdvicesWithCache(rep, split, gCache)
	gPrinter.Trigger(rep)
	utils.Debug(func() {
		// dumpDebug 是文件 I/O，放后台 goroutine 调度；不阻塞同步 hook。
		go dumpDebug(rep, split, gCache)
	})

	// 把本次观测的 SeqId 作为关联 ID 透传给 ChatBase, ChatBase 会把它复制到
	// SSE 末帧 ChatUsage.MirrorCorrelationID 上, 让上层 (例如 cachebench)
	// 用稳定 ID 把 dump 文件 (000XXX.txt 名为 SeqId) 与 token usage 精确 join,
	// 避免之前按数组下标对齐时因 stream-finished 漏 callback 累计错位的归因 bug.
	// 关键词: ProjectAndObserve CorrelationID, dump usage 精确 join
	projectedPrompt, actionTools := projectActionSchemaTags(msg)
	projectionSections := parsed.Sections()
	if len(actionTools) > 0 {
		projectionSections = Parse(projectedPrompt).Sections()
	}
	projection := Project(ProjectionInput{Sections: projectionSections, ActionTools: actionTools})
	result := &aispec.ChatBaseHijackResult{}
	if projection.Metadata.CacheProjected || len(actionTools) > 0 {
		result.IsHijacked = true
		result.Messages = projection.Messages
		result.Tools = projection.Tools
	}
	if rep != nil && rep.SeqId > 0 {
		result.CorrelationID = strconv.FormatInt(rep.SeqId, 10)
	}
	return result
}

// ResetForTest 仅测试使用: 重置全局状态
// 关键词: aicache, ResetForTest
func ResetForTest() {
	gCache = newGlobalCache(defaultMaxRequests)
	gPrinter = newThrottlePrinter(minPrintInterval)
}

// SetMinCachableUserSegmentBytesForTest 仅测试使用: 临时覆盖 P2.1 短 prompt
// 阈值合并阈值 (minCachableUserSegmentBytes), 返回 restore 函数恢复原值.
//
// 跨包测试需要走 4/3 段 happy path 但 fixture 内容很短 (低于默认 1024 byte)
// 时使用; 同包测试可用 cache_projection_test.go 的 disableProjectionThresholdMerge.
//
// 用法:
//
//	restore := aiprojection.SetMinCachableUserSegmentBytesForTest(0)
//	defer restore()
//
// 关键词: aicache, P2.1, 阈值合并, 测试 helper, 跨包覆盖
func SetMinCachableUserSegmentBytesForTest(threshold int) (restore func()) {
	saved := minCachableUserSegmentBytes
	minCachableUserSegmentBytes = threshold
	return func() {
		minCachableUserSegmentBytes = saved
	}
}
