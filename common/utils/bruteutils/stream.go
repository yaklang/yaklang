package bruteutils

import (
	"context"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/brute/core"
	"github.com/yaklang/yaklang/common/log"
)

// StreamBruteContext 以流式有界调度执行一次爆破任务。
//
// 与旧实现（先物化全部 目标×用户×密码 组合，内存 O(T×U×P)）不同，
// 本实现基于 brute/core.Scheduler：组合惰性生成、经容量受限的有界队列分发，
// 内存复杂度 O(队列容量 + Worker 数 + 活跃目标状态)，并提供全局限速、
// 单目标并发、账户锁定预算与 Retry-After 退避。
// 对外 API 与行为语义（OkToStop / FinishingThreshold / UserEliminated /
// OnlyNeedPassword / delayer / beforeBruteCallback）保持兼容。
func (b *BruteUtil) StreamBruteContext(
	ctx context.Context, typeStr string, target, users, pass []string,
	resultCallback BruteItemResultCallback,
) error {
	if len(target) == 0 || len(users) == 0 || len(pass) == 0 {
		return nil
	}
	if b.callback == nil {
		return errNilCallback
	}
	b.SetResultCallback(resultCallback)
	log.Infof("brute task with target[%v] user[%v] password[%v]", len(target), len(users), len(pass))

	// 兼容旧 delayer 语义（每次尝试后随机等待 [min,max] 秒）：
	// 限速间隔取 max（最保守），抖动取 max-min。
	maxPerSecond := 0.0
	jitter := time.Duration(0)
	if b.delayer != nil {
		minDelay, maxDelay := b.delayer.GetDelayRange()
		if maxDelay > 0 {
			maxPerSecond = 1.0 / float64(maxDelay)
		}
		if d := maxDelay - minDelay; d > 0 {
			jitter = d
		}
	}

	handler := b.callback
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// 凭证交接：探测前存入（有界，≤在途数），sink 消费后删除。
	// 旧 API 的 BruteItemResult 需要明文凭证展示命中结果；
	// 其字符串表示已脱敏，core.Result 也不含明文。
	var credHandoff sync.Map // RawCredentialIndex → core.Credential

	sink := func(res core.Result) {
		legacy := &BruteItemResult{
			Type:             typeStr,
			Ok:               res.Outcome == core.OutcomeAuthSuccess,
			Finished:         res.Outcome.IsFinalForTarget(),
			UserEliminated:   res.UserEliminated,
			OnlyNeedPassword: res.OnlyNeedPassword,
			Target:           res.TargetID,
		}
		if cred, ok := credHandoff.LoadAndDelete(res.RawCredentialIndex); ok {
			c := cred.(core.Credential)
			legacy.Username = c.Username
			legacy.Password = c.Password
		}
		legacy.ExtraInfo = res.Extra
		if b.resultCallback != nil {
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Errorf("resultCallback panic: %v", r)
					}
				}()
				b.resultCallback(legacy)
			}()
		}
	}

	scheduler := &core.Scheduler{
		Prober: core.ProberFunc(func(pctx context.Context, ptarget core.Target, cred core.Credential, opts core.Options) core.Result {
			credHandoff.Store(cred.Index, cred)
			item := &BruteItem{
				Type:     typeStr,
				Target:   ptarget.Raw,
				Username: cred.Username,
				Password: cred.Password,
				Context:  pctx,
			}
			return coreResultFromLegacy(pctx, item, handler(item))
		}),
		Protocol: typeStr,
		// 兼容映射：旧 targetsSwg = 同时处理的目标数（目标内按 TargetTaskConcurrent 并发）。
		GlobalConcurrency:  b.targetsConcurrent,
		TargetConcurrency:  b.TargetTaskConcurrent,
		MaxPerSecond:       maxPerSecond,
		Jitter:             jitter,
		QueueSize:          2048,
		OkToStop:           b.OkToStop,
		FinishingThreshold: b.FinishingThreshold,
		OnlyNeedPassword:   b.OnlyNeedPassword,
		PreCheck:           b.beforeBruteCallback,
		Sink:               sink,
	}

	// 审计日志：只记录任务规模、协议与目标数量，不含凭证。
	log.Infof("brute stream started: protocol=%s targets=%d users=%d passwords=%d",
		typeStr, len(target), len(users), len(pass))

	stats, err := scheduler.Run(ctx, core.NewCartesianSource(target, users, pass))
	if err != nil && ctx.Err() != nil {
		log.Infof("brute stream cancelled: protocol=%s executed=%d skipped=%d", typeStr, stats.Executed, stats.Skipped)
		return nil
	}
	log.Infof("brute stream finished: protocol=%s executed=%d skipped=%d lockouts=%d rateLimited=%d",
		typeStr, stats.Executed, stats.Skipped, stats.Lockouts, stats.RateLimits)
	return nil
}

var errNilCallback = &streamError{"brute callback is not set"}

type streamError struct{ msg string }

func (e *streamError) Error() string { return e.msg }

// coreResultFromLegacy 把旧 BruteItemResult 适配为结构化核心结果。
func coreResultFromLegacy(ctx context.Context, item *BruteItem, legacy *BruteItemResult) core.Result {
	if legacy == nil {
		// 处理器无返回：按认证失败处理（与旧行为一致）。
		legacy = item.Result()
	}
	outcome := core.OutcomeAuthFailed
	switch {
	case legacy.Ok:
		outcome = core.OutcomeAuthSuccess
	case legacy.Finished:
		// 旧 Finished 语义：目标级终止（网络错误/协议不符等）。
		outcome = core.OutcomeTargetUnavailable
	}
	if ctx.Err() != nil {
		outcome = core.OutcomeCancelled
	}
	return core.Result{
		Outcome:  outcome,
		Protocol: item.Type,
		TargetID: item.Target,
		CredID:   core.Credential{Username: legacy.Username, Password: legacy.Password}.ID(),
		Extra:    legacy.ExtraInfo,
		Err:      core.ErrAuthRejected,
	}
}
