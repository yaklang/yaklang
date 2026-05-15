package aibalance

import (
	"math/rand"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

const (
	defaultRPMValue     = 600
	rpmWindowDuration   = 60 * time.Second
	cleanupInterval     = 2 * time.Minute
	staleEntryThreshold = 5 * time.Minute

	// 限流触发后内部"排队"驻留区间。
	// 每次触发限流就把当前计数 +1，并在 [min, max] 内随机选一个过期时间，
	// 到点自动 -1。语义上对齐上游客户端命中 429 后的 sleep 退避时长，
	// 比 60s 固定窗口更能贴合"当前真实排队压力"。
	// 关键词: rate limit queue residence backoff sleep
	defaultRejectResidenceMin = 3 * time.Second
	defaultRejectResidenceMax = 6 * time.Second
)

// keyRPMState tracks per-API-key sliding window request timestamps.
type keyRPMState struct {
	mu       sync.Mutex
	requests []time.Time
}

// trimExpired removes timestamps older than the RPM window.
func (s *keyRPMState) trimExpired(now time.Time) {
	cutoff := now.Add(-rpmWindowDuration)
	i := 0
	for i < len(s.requests) && s.requests[i].Before(cutoff) {
		i++
	}
	if i > 0 {
		s.requests = s.requests[i:]
	}
}

// ChatRateLimiter implements per-API-key RPM rate limiting for chat completions,
// with optional per-model RPM overrides, per-model delay overrides for free
// users, and a transient "queue length" counter for rate-limit rejections.
//
// 排队计数策略：
//   - 每次 RPM 拒绝调用 recordRejection，会把"过期时间"压入 rejectExpiries。
//   - 过期时间 = now + random[rejectResidenceMin, rejectResidenceMax]。
//   - GetQueueCount 在返回前会丢掉所有过期时间 <= now 的条目，
//     等价于"3~6 秒后该条目自动 -1"。
//
// 这样使排队数语义贴近上游客户端命中 429 后的 sleep 退避时长，
// 而不是 60s 固定窗口下的累计拒绝数。
// 关键词: chat rate limiter queue residence, 排队驻留, 退避
type ChatRateLimiter struct {
	states     sync.Map // map[apiKey]*keyRPMState
	rejectMu   sync.Mutex
	// rejectExpiries 存放每次拒绝对应的过期时间戳（绝对时间）。
	// 注意：由于每次插入的过期时间是 now + 随机驻留，因此切片整体并不严格有序，
	// 修剪逻辑必须使用全量扫描的 in-place 过滤，而不能假设单调递增。
	rejectExpiries     []time.Time
	rejectResidenceMin time.Duration
	rejectResidenceMax time.Duration
	defaultRPM         atomic.Int64
	modelRPM           sync.Map // map[modelName]int64 RPM override
	modelDelay         sync.Map // map[modelName]int64 free-user pre-call delay override (seconds)
	stopCh             chan struct{}
	stopOnce           sync.Once
	startOnce          sync.Once
}

// NewChatRateLimiter creates a new chat rate limiter. The background cleanup
// goroutine is started lazily on first CheckRateLimit, so an unused limiter
// (e.g. NewServerConfig() in tests that never send chat requests) does not
// contribute a leaked goroutine to TestGoroutineTracing's baseline.
// 关键词: NewChatRateLimiter lazy cleanup, goroutine baseline 净化
func NewChatRateLimiter() *ChatRateLimiter {
	rl := &ChatRateLimiter{
		stopCh:             make(chan struct{}),
		rejectResidenceMin: defaultRejectResidenceMin,
		rejectResidenceMax: defaultRejectResidenceMax,
	}
	rl.defaultRPM.Store(defaultRPMValue)
	return rl
}

// ensureCleanupStarted lazily starts the background cleanupLoop on first use.
// Idempotent across concurrent callers (sync.Once). If Stop() was already
// called, cleanupLoop will start and exit immediately on the first ticker
// select, which is harmless.
// 关键词: ChatRateLimiter lazy 启动 cleanupLoop, startOnce 幂等
func (rl *ChatRateLimiter) ensureCleanupStarted() {
	rl.startOnce.Do(func() {
		go rl.cleanupLoop()
	})
}

// SetRejectResidence 调整限流拒绝条目在排队计数中的驻留区间。
// 主要用于：
//   - 运维侧根据上游客户端实际退避策略动态调整。
//   - 测试中注入更短的驻留区间以加速验证。
//
// 入参语义：
//   - min < 0 视为 0，max < min 时统一抬升为 min（防止上下限交叉）。
//   - 同步加 rejectMu，避免与 recordRejection 并发读写撕裂。
//
// 关键词: SetRejectResidence rate limiter queue 驻留区间
func (rl *ChatRateLimiter) SetRejectResidence(minDur, maxDur time.Duration) {
	if minDur < 0 {
		minDur = 0
	}
	if maxDur < minDur {
		maxDur = minDur
	}
	rl.rejectMu.Lock()
	defer rl.rejectMu.Unlock()
	rl.rejectResidenceMin = minDur
	rl.rejectResidenceMax = maxDur
}

// SetDefaultRPM updates the global default RPM limit.
func (rl *ChatRateLimiter) SetDefaultRPM(rpm int64) {
	if rpm <= 0 {
		rpm = defaultRPMValue
	}
	rl.defaultRPM.Store(rpm)
}

// SetModelRPM sets an RPM override for a specific model.
func (rl *ChatRateLimiter) SetModelRPM(model string, rpm int64) {
	if rpm <= 0 {
		rl.modelRPM.Delete(model)
		return
	}
	rl.modelRPM.Store(model, rpm)
}

// ClearModelRPM removes all per-model RPM overrides.
func (rl *ChatRateLimiter) ClearModelRPM() {
	rl.modelRPM.Range(func(key, _ any) bool {
		rl.modelRPM.Delete(key)
		return true
	})
}

// SetModelDelay sets a free-user pre-call delay override (in seconds) for a
// specific model. Passing a negative value removes the override so the
// model falls back to the global free-user delay. Passing 0 stores an
// explicit "no delay" override that wins over the global default.
func (rl *ChatRateLimiter) SetModelDelay(model string, delaySec int64) {
	if delaySec < 0 {
		rl.modelDelay.Delete(model)
		return
	}
	rl.modelDelay.Store(model, delaySec)
}

// ClearModelDelay removes all per-model delay overrides.
func (rl *ChatRateLimiter) ClearModelDelay() {
	rl.modelDelay.Range(func(key, _ any) bool {
		rl.modelDelay.Delete(key)
		return true
	})
}

// GetEffectiveDelay returns the pre-call delay (in seconds) for a free-user
// request to modelName, falling back to the provided global default if no
// per-model override is configured. A configured override of 0 is also
// honored as "no delay" because SetModelDelay deletes zero entries.
func (rl *ChatRateLimiter) GetEffectiveDelay(modelName string, fallbackSec int64) int64 {
	if v, ok := rl.modelDelay.Load(modelName); ok {
		if delay, ok2 := v.(int64); ok2 {
			return delay
		}
	}
	return fallbackSec
}

// trimRejectExpiredLocked drops rejection entries whose deadline is on or
// before now. Caller must hold rl.rejectMu.
//
// 因为每条记录的过期时间是 now + 随机驻留，切片整体不是严格有序的，
// 这里走全量 in-place 过滤而不是前缀截断。条目数量受瞬时拒绝速率约束，
// 实际生产环境下规模极小（最多几十到几百），O(n) 完全可以接受。
// 关键词: trim reject expiries, 排队驻留过滤
func (rl *ChatRateLimiter) trimRejectExpiredLocked(now time.Time) {
	if len(rl.rejectExpiries) == 0 {
		return
	}
	kept := rl.rejectExpiries[:0]
	for _, deadline := range rl.rejectExpiries {
		if deadline.After(now) {
			kept = append(kept, deadline)
		}
	}
	rl.rejectExpiries = kept
}

// recordRejection records a rate-limit denial at now and returns how many
// rejections are still "in the queue" (i.e. whose residence deadline has
// not yet expired), including the one just recorded.
//
// 过期时间计算：deadline = now + random[rejectResidenceMin, rejectResidenceMax]。
// 如果 min == max，则退化为固定驻留。
// 关键词: recordRejection 排队 驻留 随机
func (rl *ChatRateLimiter) recordRejection(now time.Time) int64 {
	rl.rejectMu.Lock()
	defer rl.rejectMu.Unlock()
	rl.trimRejectExpiredLocked(now)

	residence := rl.rejectResidenceMin
	if rl.rejectResidenceMax > rl.rejectResidenceMin {
		span := int64(rl.rejectResidenceMax - rl.rejectResidenceMin)
		// rand.Int63n(span+1) yields a value in [0, span], 上下界都可达。
		residence = rl.rejectResidenceMin + time.Duration(rand.Int63n(span+1))
	}
	rl.rejectExpiries = append(rl.rejectExpiries, now.Add(residence))
	return int64(len(rl.rejectExpiries))
}

// GetQueueCount returns the current number of rate-limit rejections that are
// still inside their residence window. Each rejection contributes for a
// random duration in [rejectResidenceMin, rejectResidenceMax] (default 3-6s)
// after which it is auto-decremented. This is intentionally NOT a 60s
// sliding-window counter; it matches the wall-clock backoff that callers
// actually sleep before retrying.
//
// 关键词: GetQueueCount 排队数 限流驻留
func (rl *ChatRateLimiter) GetQueueCount() int64 {
	rl.rejectMu.Lock()
	defer rl.rejectMu.Unlock()
	now := time.Now()
	rl.trimRejectExpiredLocked(now)
	return int64(len(rl.rejectExpiries))
}

// getEffectiveRPM returns the RPM limit for a given model,
// falling back to the global default.
func (rl *ChatRateLimiter) getEffectiveRPM(modelName string) int64 {
	if v, ok := rl.modelRPM.Load(modelName); ok {
		return v.(int64)
	}
	return rl.defaultRPM.Load()
}

// CheckRateLimit checks whether a request from apiKey for modelName is allowed.
// Returns (allowed, currentQueueLength).
// If allowed, the request is automatically recorded in the sliding window.
// The rate-limit bucket is keyed by (apiKey, modelName) so that per-model RPM
// overrides are enforced independently instead of sharing a single bucket.
func (rl *ChatRateLimiter) CheckRateLimit(apiKey string, modelName string) (bool, int64) {
	rl.ensureCleanupStarted()
	now := time.Now()
	rpm := rl.getEffectiveRPM(modelName)

	bucketKey := apiKey + "|" + modelName
	newState := &keyRPMState{
		requests: []time.Time{now},
	}
	val, loaded := rl.states.LoadOrStore(bucketKey, newState)
	if !loaded {
		return true, rl.GetQueueCount()
	}

	state := val.(*keyRPMState)
	state.mu.Lock()
	defer state.mu.Unlock()

	state.trimExpired(now)

	if int64(len(state.requests)) >= rpm {
		qLen := rl.recordRejection(now)
		return false, qLen
	}

	state.requests = append(state.requests, now)
	return true, rl.GetQueueCount()
}

// cleanupLoop periodically removes stale API key entries.
func (rl *ChatRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now()
			rl.rejectMu.Lock()
			rl.trimRejectExpiredLocked(now)
			rl.rejectMu.Unlock()

			cutoff := now.Add(-staleEntryThreshold)
			count := 0
			rl.states.Range(func(key, value any) bool {
				state := value.(*keyRPMState)
				state.mu.Lock()
				latest := time.Time{}
				if len(state.requests) > 0 {
					latest = state.requests[len(state.requests)-1]
				}
				state.mu.Unlock()
				if latest.Before(cutoff) {
					rl.states.Delete(key)
					count++
				}
				return true
			})
			if count > 0 {
				log.Infof("chat rate limiter: cleaned up %d stale api-key entries", count)
			}
		case <-rl.stopCh:
			return
		}
	}
}

// ModelRPMStat describes the aggregated recent request count for a single
// model across all API keys inside the current sliding window
// (see rpmWindowDuration, currently 60 seconds).
type ModelRPMStat struct {
	Model        string `json:"model"`
	RPM          int64  `json:"rpm"`
	EffectiveRPM int64  `json:"effective_rpm"`
}

// GetModelRPMStats aggregates recent traffic across all API-key buckets
// and returns per-model counters for models whose total request count in
// the sliding window is >= minRPM. Result is sorted by RPM descending.
//
// Notes:
//   - Internal state keys have the form "<apiKey>|<modelName>"; we use the
//     last '|' as the separator so that API keys containing '|' (unlikely
//     but possible) do not break aggregation.
//   - Expired timestamps are trimmed while iterating so stats reflect the
//     same 60s window used by CheckRateLimit.
func (rl *ChatRateLimiter) GetModelRPMStats(minRPM int64) []ModelRPMStat {
	if minRPM < 0 {
		minRPM = 0
	}
	now := time.Now()
	perModel := make(map[string]int64)

	rl.states.Range(func(k, v any) bool {
		key, ok := k.(string)
		if !ok {
			return true
		}
		sepIdx := strings.LastIndex(key, "|")
		if sepIdx < 0 || sepIdx == len(key)-1 {
			return true
		}
		model := key[sepIdx+1:]
		state, ok := v.(*keyRPMState)
		if !ok || state == nil {
			return true
		}
		state.mu.Lock()
		state.trimExpired(now)
		count := int64(len(state.requests))
		state.mu.Unlock()
		if count <= 0 {
			return true
		}
		perModel[model] += count
		return true
	})

	result := make([]ModelRPMStat, 0, len(perModel))
	for model, count := range perModel {
		if count < minRPM {
			continue
		}
		result = append(result, ModelRPMStat{
			Model:        model,
			RPM:          count,
			EffectiveRPM: rl.getEffectiveRPM(model),
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].RPM != result[j].RPM {
			return result[i].RPM > result[j].RPM
		}
		return result[i].Model < result[j].Model
	})
	return result
}

// Stop stops the background cleanup goroutine.
func (rl *ChatRateLimiter) Stop() {
	rl.stopOnce.Do(func() {
		close(rl.stopCh)
	})
}
