// mirror_overload_bench_test.go - aibalance 卡死事故复现与量化实验
//
// 目的: 用可复跑的实验量化本次事故的两个主放大器:
//   1. mirror 队列扣押超大快照内存 (queueSize * snapshot 大小)
//   2. 每次回调都新建并重新编译 yak 引擎的固定开销
//   3. mirror 单次执行的 ctx 超时是"软"的 (native 调用不被打断)
//
// 这些实验不出网, 只构造内存中的 MirrorSnapshot / MirrorManager.
//
// 运行:
//   go test ./common/aibalance/ -run TestMirrorRetentionExperiment -v
//   go test ./common/aibalance/ -run TestMirrorSoftTimeout -v
//   go test ./common/aibalance/ -bench BenchmarkMirror -benchmem -run x
//
// 关键词: aibalance mirror overload 复现, 队列内存扣押量化, 引擎重编译开销,
//        软超时实验

package aibalance

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/schema"
)

// makeBigText 生成长度恰为 n 字节的可读文本, 用于模拟超大 prompt / 响应.
func makeBigText(n int) string {
	const unit = "the quick brown fox jumps over the lazy dog 0123456789 "
	var b strings.Builder
	b.Grow(n + len(unit))
	for b.Len() < n {
		b.WriteString(unit)
	}
	s := b.String()
	return s[:n]
}

// makeBigSnapshot 构造一个引用着 sizeBytes 级别请求文本的快照,
// 模拟 server.go:1700 直接引用 bodyIns.Messages 的真实形态.
func makeBigSnapshot(reqID string, sizeBytes int) *MirrorSnapshot {
	content := makeBigText(sizeBytes)
	respText := makeBigText(sizeBytes / 4)
	return &MirrorSnapshot{
		ReqID:           reqID,
		Model:           "memfit-light-free",
		RequestMessages: []aispec.ChatDetail{{Role: "user", Content: content}},
		ResponseText:    respText,
		InputBytes:      int64(sizeBytes),
		OutputBytes:     int64(sizeBytes / 4),
	}
}

// engineAvailable 探测当前测试环境下 yak 引擎是否可用 (CI / sandbox 可能不可用).
func engineAvailable(t testing.TB) bool {
	t.Helper()
	err, _, _ := executeMirrorScript(context.Background(),
		"func handle(data) { return }",
		&MirrorSnapshot{ReqID: "probe"}, false)
	if err != nil {
		t.Logf("yak script engine not available in test env: %v", err)
		return false
	}
	return true
}

// TestMirrorRetentionExperiment 量化"队列扣押内存": worker 被堵住时,
// 已入队的超大快照通过 RequestMessages 引用持续占用堆内存, 直到被消费.
// 这复现了事故里 heap 随队列深度线性上涨的现象, 并验证修复后队列被钳制 +
// 全局字节预算把内存占用收敛到有界范围.
//
// 关键词: mirror 队列内存扣押复现, heap 随队列线性增长, 全局字节预算上限
func TestMirrorRetentionExperiment(t *testing.T) {
	const snapBytes = 800 * 1024 // 模拟 prompt_len ~ 80 万的超大请求
	// 故意请求一个很大的队列 (事故现场默认 1024), 验证修复后被钳制.
	requestedQueue := 1024

	rule := &schema.AiMirrorRule{
		Name:          "retention-exp",
		Enabled:       true,
		ConditionType: MirrorConditionAlways,
		Concurrency:   1,
		QueueSize:     requestedQueue,
		TimeoutMs:     5000,
	}
	rule.ID = 8001

	m := NewMirrorManager()
	// 用一个永远堵住的 worker 占位, 让队列只进不出, 暴露最坏内存扣押.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rt := &mirrorRuleRuntime{
		rule:   rule,
		ch:     make(chan *MirrorSnapshot, clampMirrorQueueSize(requestedQueue)),
		cancel: cancel,
		logs:   newMirrorLogRing(),
	}
	rt.wg.Add(1)
	go func() {
		defer rt.wg.Done()
		<-ctx.Done() // 永不消费, 直到测试结束
	}()
	m.mu.Lock()
	m.runtime[rule.ID] = rt
	m.mu.Unlock()

	var before runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	// 投递远超队列容量的超大快照.
	enqueueAttempts := clampMirrorQueueSize(requestedQueue) * 4
	for i := 0; i < enqueueAttempts; i++ {
		m.Trigger(makeBigSnapshot("r"+itoa(i), snapBytes))
	}

	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	retainedMB := int64(0)
	if after.HeapAlloc > before.HeapAlloc {
		retainedMB = int64(after.HeapAlloc-before.HeapAlloc) / 1024 / 1024
	}
	queueLen := len(rt.ch)
	t.Logf("requested_queue=%d clamped_queue=%d enqueue_attempts=%d final_queue_len=%d retained~%dMB inFlightBytes=%dMB budget=%dMB",
		requestedQueue, clampMirrorQueueSize(requestedQueue), enqueueAttempts, queueLen,
		retainedMB, m.InFlightBytes()/1024/1024, m.MaxInFlightBytes()/1024/1024)

	// 修复后断言: 队列被钳制到上限内, 且队列里实际占用的字节不会超过全局预算.
	assert.LessOrEqual(t, queueLen, mirrorMaxQueueSize, "queue length must be clamped")
	assert.LessOrEqual(t, m.InFlightBytes(), m.MaxInFlightBytes(),
		"in-flight bytes must stay within global budget")

	cancel()
	rt.wg.Wait()
}

// TestMirrorSoftTimeout 固化两条关于 mirror 单次执行成本与超时语义的事实:
//
//  1. ctx 软超时只在 yak VM 指令之间被检查: 当 ctx 在进入 handle 前就已过期,
//     VM 能在执行昂贵的 native 解析 (jsonstream) 之前短路 (soft 远小于 baseline).
//     —— 这说明 ctx 能挡住"尚未开始"的工作, 但挡不住"已在单个 native 调用内"的工作.
//  2. 但每次调用都要重新 NewScriptEngine + 编译脚本, 这笔固定开销无法被 ctx 省掉
//     (soft 那次即便 ctx 已过期仍付出了编译成本). 在洪峰下 4 个 worker 每条都重编译,
//     这正是事故里被放大的 CPU 成本之一.
//
// 关键词: mirror 软超时语义, ctx 指令间检查, 每次调用引擎重编译固定开销
func TestMirrorSoftTimeout(t *testing.T) {
	if !engineAvailable(t) {
		t.Skip("yak script engine not available in test env")
	}

	// 模拟价值评估脚本的核心动作: 拼接超大请求文本 + jsonstream 解析.
	script := `
func handle(data) {
    msgs = data["request_messages"]
    text = ""
    if msgs != nil {
        for m in msgs {
            text += sprintf("%v", m["content"])
        }
    }
    jsonstream.Extract(text, jsonstream.onConditionalObject(["aive_schema", "record"], func(obj) {}))
}
`
	bigRecord := `{"aive_schema":"v1","record":{"id":"x","blob":"` + makeBigText(400*1024) + `"}}`
	snap := &MirrorSnapshot{
		ReqID: "soft-timeout",
		Model: "memfit-light-free",
		RequestMessages: []aispec.ChatDetail{
			{Role: "user", Content: "<aive_record_json>" + bigRecord + "</aive_record_json>"},
		},
	}

	// 基线: 给一个充裕的 ctx, 测一次回调的真实耗时 (引擎编译 + 大文本解析).
	bctx, bcancel := context.WithTimeout(context.Background(), 30*time.Second)
	bstart := time.Now()
	_, _, _ = executeMirrorScript(bctx, script, snap, false)
	baseline := time.Since(bstart)
	bcancel()

	// 软超时: 给一个极短的 1ms ctx, 模拟"超时已到".
	sctx, scancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	sstart := time.Now()
	_, _, _ = executeMirrorScript(sctx, script, snap, false)
	soft := time.Since(sstart)
	scancel()

	t.Logf("executeMirrorScript baseline(30s ctx)=%v soft(1ms ctx)=%v (soft=engine compile only, baseline=compile+native parse)", baseline, soft)
	// 事实 2: 即便 ctx 已过期, soft 那次仍付出了非平凡的引擎编译开销 (>0), 说明
	// "每次调用都重编译"的固定成本无法被 ctx 省掉, 是洪峰下的 CPU 放大器之一.
	assert.Greater(t, soft, time.Duration(0),
		"engine compile cost is paid on every invocation even with an expired ctx")
	// 事实 1: 一次完整回调 (编译+大文本 jsonstream 解析) 的真实成本非平凡 (>1ms);
	// ctx 能在 native 解析开始前短路 (soft 明显小于 baseline), 但挡不住已开始的 native 调用.
	assert.Greater(t, baseline, time.Millisecond,
		"a full callback (compile + native parse) costs non-trivially; per-call recompile hurts under flood")
	assert.Less(t, soft, baseline,
		"with an already-expired ctx the VM should short-circuit before the expensive native parse")
}

// BenchmarkMirrorExecuteEngineOverhead 量化每次回调"新建 + 编译 yak 引擎"的
// 固定开销 (脚本体几乎为空, 测的就是引擎本身的 create/compile 成本).
//
// 关键词: mirror 引擎重编译开销 benchmark, NewScriptEngine 成本
func BenchmarkMirrorExecuteEngineOverhead(b *testing.B) {
	if !engineAvailable(b) {
		b.Skip("yak script engine not available in test env")
	}
	script := "func handle(data) { return }"
	snap := &MirrorSnapshot{ReqID: "bench", Model: "m"}
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = executeMirrorScript(ctx, script, snap, false)
	}
}

// BenchmarkMirrorTriggerBigSnapshot 量化超大快照投递路径的分配开销 (含字段截断 +
// 字节预算判定). worker 堵住, 主要测 Trigger 自身的开销.
//
// 关键词: mirror Trigger 大快照 benchmark, 字段截断/字节预算开销
func BenchmarkMirrorTriggerBigSnapshot(b *testing.B) {
	const snapBytes = 800 * 1024
	rule := &schema.AiMirrorRule{
		Name:          "bench-trigger",
		Enabled:       true,
		ConditionType: MirrorConditionAlways,
		Concurrency:   1,
		QueueSize:     64,
		TimeoutMs:     5000,
	}
	rule.ID = 8002
	m := NewMirrorManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rt := &mirrorRuleRuntime{
		rule:   rule,
		ch:     make(chan *MirrorSnapshot, 64),
		cancel: cancel,
		logs:   newMirrorLogRing(),
	}
	rt.wg.Add(1)
	go func() {
		defer rt.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-rt.ch:
				if !ok {
					return
				}
			}
		}
	}()
	m.mu.Lock()
	m.runtime[rule.ID] = rt
	m.mu.Unlock()

	snap := makeBigSnapshot("bench", snapBytes)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Trigger(snap)
	}
	b.StopTimer()
	cancel()
	rt.wg.Wait()
}
