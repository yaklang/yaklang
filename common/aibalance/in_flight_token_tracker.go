package aibalance

import (
	"sync"
)

// 关键词: InFlightTokenTracker, 在途流预扣 token 计数器, 过冲防御
//
// 设计动机：
//   CheckFreeUserDailyTokenLimit 只看 DB 里已结算过的 tokens_used，
//   它无法防止"100 个请求几乎同时通过 check 然后并发跑 stream"导致的过冲
//   （每个都看到 used < limit → 全部放行 → 都跑完后 used 远超 limit）。
//
//   解决方案：在进程内维护一个"在途请求预估 token 总量"的计数器，按
//   daily token check 同样的"桶 (global / per-model)"分组。daily check
//   在比较 limit 时把 in-flight 估算加上去：
//     effective_used = bucket_db_used + in_flight_estimate
//   100 并发同时来时，前几个被放行的请求立刻把 in_flight 推高，
//   后续请求看到 effective_used >= limit 就被 429 拒绝，过冲被卡住。
//
//   服务重启 in-flight 归零（这是 OK 的：重启意味着所有 in-flight 流都
//   被切断了，DB bucket 仍然准确）。

// InFlightTokenTracker 用 sync.Map 风格的逐 key 互斥保存"每桶当前在途请求
// 预扣 token 总量"。所有方法都是 goroutine-safe。
//
// bucketKey 语义与 daily token 的桶选择对齐：
//   - ""        → 全局共享池 (freeUserGlobalBucketModel)
//   - modelName → 该模型独立桶 (FreeUserTokenModelOverride.LimitM > 0)
//   - exempt 模型不参与 in-flight 追踪 (永远放行 → 不需要预扣)
//
// 关键词: InFlightTokenTracker bucketKey 对齐 daily check
type InFlightTokenTracker struct {
	mu        sync.RWMutex
	perBucket map[string]int64
}

// NewInFlightTokenTracker 构造一个空 tracker。
// 关键词: NewInFlightTokenTracker
func NewInFlightTokenTracker() *InFlightTokenTracker {
	return &InFlightTokenTracker{
		perBucket: make(map[string]int64),
	}
}

// Add 给指定桶累加 in-flight 预扣。delta <= 0 时是 no-op。
// 关键词: InFlightTokenTracker.Add 预扣累加
func (t *InFlightTokenTracker) Add(bucketKey string, delta int64) {
	if t == nil || delta <= 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.perBucket[bucketKey] += delta
}

// Remove 释放 in-flight 预扣。delta <= 0 时是 no-op；总量减到 0 后会 clamp 到 0
// 避免下游算术错误（罕见场景下 Add/Remove 不平衡也不会出现负数）。
// 关键词: InFlightTokenTracker.Remove 释放 clamp-to-zero
func (t *InFlightTokenTracker) Remove(bucketKey string, delta int64) {
	if t == nil || delta <= 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	cur := t.perBucket[bucketKey]
	cur -= delta
	if cur < 0 {
		cur = 0
	}
	if cur == 0 {
		// 桶清零时直接删 key，避免 map 长期堆积空 entry
		delete(t.perBucket, bucketKey)
		return
	}
	t.perBucket[bucketKey] = cur
}

// Get 返回指定桶当前在途预扣总量。桶不存在返回 0。
// 关键词: InFlightTokenTracker.Get
func (t *InFlightTokenTracker) Get(bucketKey string) int64 {
	if t == nil {
		return 0
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.perBucket[bucketKey]
}

// Snapshot 返回 (bucketKey -> in-flight) 的拷贝，便于 portal 监控展示。
// 关键词: InFlightTokenTracker.Snapshot portal 展示
func (t *InFlightTokenTracker) Snapshot() map[string]int64 {
	if t == nil {
		return map[string]int64{}
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make(map[string]int64, len(t.perBucket))
	for k, v := range t.perBucket {
		out[k] = v
	}
	return out
}
