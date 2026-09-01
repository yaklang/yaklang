package core

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

// Scheduler 是流式有界爆破调度器。
//
// 与旧 StreamBruteContext（先物化全部 T×U×P 组合）不同：
// 组合通过 CombinationSource 惰性产出，经容量受限的有界队列分发给
// 全局并发受限的 worker；内存复杂度为
//
//	O(队列容量 + Worker 数 + 活跃目标状态)
//
// 每个目标维护一个小状态机（已消除用户名、已用密码、锁定计数、退避期限），
// 全部状态由 worker 内联更新，不为目标创建任何长期 goroutine。
type Scheduler struct {
	// Prober 必填。
	Prober Prober
	// Protocol 结果里回填的协议名。
	Protocol string

	GlobalConcurrency  int           // 全局并发上限，默认 200
	TargetConcurrency  int           // 单目标(服务)并发上限，默认 1
	MaxPerSecond       float64       // 全局每秒尝试上限，0 为不限
	Jitter             time.Duration // 每次尝试的随机抖动上限
	QueueSize          int           // 有界队列容量，默认 1024
	LockoutBudget      int           // 单目标 AccountLocked 预算，默认 3，<=0 为 3
	MaxRetryAfter      time.Duration // 单次退避上限，默认 30s
	OkToStop           bool          // 命中成功后终止该目标
	FinishingThreshold int           // 终止性结果达到该数后停止该目标
	OnlyNeedPassword   bool          // 只按密码去重（也可由结果动态开启）
	DefaultTimeout     time.Duration // 单次探测超时，默认 10s
	TLSPolicy          TLSPolicy
	// PreCheck 可选：目标开始前的预检（指纹/合理性检查），返回 false 跳过该目标。
	PreCheck func(target string) bool
	// Sink 必填：结果回调。
	Sink func(Result)
}

// Stats 是调度过程的聚合统计。
type Stats struct {
	Generated  int64 // 已从源取出的组合数
	Executed   int64 // 实际发起探测的次数
	Skipped    int64 // 因目标/用户名/密码短路而跳过的组合数
	Elapsed    time.Duration
	ByOutcome  map[Outcome]int64
	Lockouts   int64 // AccountLocked 结果总数
	RateLimits int64 // RateLimited 结果总数
	Targets    int64 // 活跃目标数
}

// targetState 是单目标小状态机。
type targetState struct {
	id     string
	gate   *chanGate
	ctx    context.Context
	cancel context.CancelFunc
	// checked 原子标记 PreCheck 已执行（无锁 fast path）
	checked atomic.Bool

	mu               sync.Mutex
	eliminatedUsers  map[string]struct{}
	usedPasswords    map[string]struct{}
	onlyNeedPassword bool
	finishedCount    int
	lockoutCount     int
	backoffUntil     time.Time
	stopped          bool
}

func (t *targetState) shouldSkip(user, password string) (skip bool, markPassword bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopped {
		return true, false
	}
	if _, ok := t.eliminatedUsers[user]; ok {
		return true, false
	}
	if t.onlyNeedPassword {
		if _, ok := t.usedPasswords[password]; ok {
			return true, false
		}
		return false, true
	}
	return false, false
}

func (t *targetState) markPasswordUsed(password string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.usedPasswords == nil {
		t.usedPasswords = make(map[string]struct{})
	}
	t.usedPasswords[password] = struct{}{}
}

// Run 消费组合源直到耗尽、取消或全部目标短路。
// Run 返回后所有 worker 与连接均已退出（探针保证取消后按 deadline 关闭连接）。
func (s *Scheduler) Run(ctx context.Context, source CombinationSource) (*Stats, error) {
	if s.Prober == nil {
		return nil, ErrNoProber
	}
	if s.Sink == nil {
		return nil, ErrNoSink
	}
	if s.Protocol == "" {
		s.Protocol = "unknown"
	}
	if s.GlobalConcurrency <= 0 {
		s.GlobalConcurrency = 200
	}
	if s.TargetConcurrency <= 0 {
		s.TargetConcurrency = 1
	}
	if s.QueueSize <= 0 {
		s.QueueSize = 1024
	}
	if s.LockoutBudget <= 0 {
		s.LockoutBudget = 3
	}
	if s.MaxRetryAfter <= 0 {
		s.MaxRetryAfter = 30 * time.Second
	}
	if s.DefaultTimeout <= 0 {
		s.DefaultTimeout = DefaultTimeout
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	stats := &Stats{ByOutcome: make(map[Outcome]int64)}
	globalGate := newChanGate(s.GlobalConcurrency)
	limiter := newRateLimiter(s.MaxPerSecond, s.Jitter)

	var (
		targetsMu sync.Mutex
		targets   = make(map[string]*targetState)
		active    int64 // 未停止的目标数
		wg        sync.WaitGroup
		statsMu   sync.Mutex
		genDone   atomic.Bool // 生成器已耗尽（不会再有新目标）
	)

	getTarget := func(id string) *targetState {
		targetsMu.Lock()
		defer targetsMu.Unlock()
		if t, ok := targets[id]; ok {
			return t
		}
		tctx, tcancel := context.WithCancel(runCtx)
		t := &targetState{
			id:     id,
			gate:   newChanGate(s.TargetConcurrency),
			ctx:    tctx,
			cancel: tcancel,
		}
		targets[id] = t
		active++
		return t
	}

	stopTarget := func(t *targetState, reason string) {
		t.mu.Lock()
		if t.stopped {
			t.mu.Unlock()
			return
		}
		t.stopped = true
		t.mu.Unlock()
		t.cancel()
		targetsMu.Lock()
		active--
		targetsMu.Unlock()
		log.Infof("brute target[%s] protocol[%s] stopped: %s", t.id, s.Protocol, reason)
	}

	// allStopped 判定"全部已知目标已停止且不会再有新目标"。
	// 生成器是否已耗尽（queue 已关闭）必须参与判断：
	// 否则首个目标 OkToStop 时，后续目标的状态机尚未创建（组合还在队列），
	// len(targets)==1 && active==0 会误判为全部完成并取消整个运行
	//（实测：多目标 + OkToStop + 限速下第二个目标被整体饿死）。
	allStopped := func() bool {
		targetsMu.Lock()
		defer targetsMu.Unlock()
		return genDone.Load() && len(targets) > 0 && active == 0
	}

	queue := make(chan Combination, s.QueueSize)

	// 生成器：把惰性源灌入有界队列（唯一的生产者 goroutine）。
	genErr := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(queue)
		defer genDone.Store(true)
		for {
			comb, ok := source.Next(runCtx)
			if !ok {
				return
			}
			statsMu.Lock()
			stats.Generated++
			statsMu.Unlock()
			select {
			case queue <- comb:
			case <-runCtx.Done():
				genErr <- runCtx.Err()
				return
			}
		}
	}()

	start := time.Now()
	defer func() {
		statsMu.Lock()
		stats.Elapsed = time.Since(start)
		stats.Targets = int64(len(targets))
		statsMu.Unlock()
	}()

	emit := func(res Result) {
		statsMu.Lock()
		stats.Executed++
		stats.ByOutcome[res.Outcome]++
		if res.Outcome == OutcomeAccountLocked {
			stats.Lockouts++
		}
		if res.Outcome == OutcomeRateLimited {
			stats.RateLimits++
		}
		statsMu.Unlock()
		s.safeSink(&res)
	}

	for dispatching := true; dispatching; {
		var comb Combination
		var ok bool
		select {
		case comb, ok = <-queue:
			if !ok {
				dispatching = false
				continue
			}
		case <-runCtx.Done():
			dispatching = false
			continue
		}

		if err := runCtx.Err(); err != nil {
			dispatching = false
			continue
		}

		t := getTarget(comb.Target)

		skip, markPassword := t.shouldSkip(comb.Username, comb.Password)
		if skip {
			statsMu.Lock()
			stats.Skipped++
			statsMu.Unlock()
			if allStopped() {
				cancel()
				dispatching = false
			}
			continue
		}
		if markPassword {
			t.markPasswordUsed(comb.Password)
		}

		// 目标预检（每个目标一次，失败则短路该目标）。
		if s.PreCheck != nil && !t.checked.Load() && t.checked.CompareAndSwap(false, true) {
			if !s.PreCheck(comb.Target) {
				stopTarget(t, "pre-check failed")
				continue
			}
		}

		// 先全局、后目标的信号量顺序获取；取消时立即放弃。
		if err := globalGate.acquire(runCtx); err != nil {
			dispatching = false
			continue
		}
		if err := t.gate.acquire(runCtx); err != nil {
			globalGate.release()
			// 目标 ctx 取消有两种可能：runCtx 取消（退出）或目标被短路（跳过后续）。
			if runCtx.Err() != nil {
				dispatching = false
			}
			continue
		}

		wg.Add(1)
		go func(t *targetState, comb Combination) {
			defer wg.Done()
			defer globalGate.release()
			defer t.gate.release()

			// 全局限速 + 抖动；目标停止则不再发起。
			if err := limiter.Take(t.ctx); err != nil {
				return
			}

			// 目标级退避（RateLimited/Retry-After），在 worker 内等待，无额外 goroutine。
			for {
				t.mu.Lock()
				until := t.backoffUntil
				t.mu.Unlock()
				if until.IsZero() || !time.Now().Before(until) {
					break
				}
				if err := sleepCtx(t.ctx, time.Until(until)); err != nil {
					return
				}
			}

			if err := t.ctx.Err(); err != nil {
				return
			}
			t.mu.Lock()
			eliminated := false
			if _, ok := t.eliminatedUsers[comb.Username]; ok {
				eliminated = true
			}
			stopped := t.stopped
			t.mu.Unlock()
			if eliminated || stopped {
				statsMu.Lock()
				stats.Skipped++
				statsMu.Unlock()
				return
			}

			probeStart := time.Now()
			res := s.probeWithRecover(t.ctx, comb)
			res.Elapsed = time.Since(probeStart)
			res.Protocol = s.Protocol
			res.TargetID = comb.Target
			res.CredID = Credential{Username: comb.Username, Password: comb.Password}.ID()
			res.RawCredentialIndex = comb.Index

			emit(res)

			// ---- 目标状态机转移 ----
			t.mu.Lock()
			if res.UserEliminated {
				if t.eliminatedUsers == nil {
					t.eliminatedUsers = make(map[string]struct{})
				}
				t.eliminatedUsers[comb.Username] = struct{}{}
			}
			if res.OnlyNeedPassword {
				t.onlyNeedPassword = true
				if t.usedPasswords == nil {
					t.usedPasswords = make(map[string]struct{})
				}
				t.usedPasswords[comb.Password] = struct{}{}
			}
			if res.Outcome == OutcomeAccountLocked {
				t.lockoutCount++
			}
			if res.RetryAfter > 0 {
				backoff := res.RetryAfter
				if backoff > s.MaxRetryAfter {
					backoff = s.MaxRetryAfter
				}
				if next := time.Now().Add(backoff); next.After(t.backoffUntil) {
					t.backoffUntil = next
				}
			}
			if res.Outcome.IsFinalForTarget() {
				t.finishedCount++
			}
			lockoutExceeded := t.lockoutCount >= s.LockoutBudget
			thresholdExceeded := s.FinishingThreshold > 0 && t.finishedCount >= s.FinishingThreshold
			okFound := res.Outcome == OutcomeAuthSuccess && s.OkToStop
			t.mu.Unlock()

			switch {
			case okFound:
				stopTarget(t, "credentials found (ok-to-stop)")
			case lockoutExceeded:
				stopTarget(t, "account lockout budget exhausted")
			case thresholdExceeded:
				stopTarget(t, "finishing threshold reached")
			case res.Outcome.IsFinalForTarget() && s.FinishingThreshold <= 0:
				// 无阈值配置时，终止性结果直接短路目标（与旧 Finished 行为一致）。
				stopTarget(t, "final outcome: "+res.Outcome.String())
			}

			if allStopped() {
				cancel()
			}
		}(t, comb)
	}

	// 先等待全部在途 worker 退出（worker 依赖 t.ctx 完成探测），
	// 再取消剩余目标 ctx 释放资源；否则最后派发的任务会在探测前被取消。
	wg.Wait()
	cancel()
	statsMu.Lock()
	defer statsMu.Unlock()

	select {
	case err := <-genErr:
		if err != nil && ctx.Err() == nil {
			return stats, nil // 生成器因调度取消而退出不算错误
		}
	default:
	}
	if ctx.Err() != nil {
		return stats, ctx.Err()
	}
	return stats, nil
}

// probeWithRecover 执行探针并兜底 panic：panic 栈绝不进入结果，
// 只记录结构化类别与固定描述。
func (s *Scheduler) probeWithRecover(ctx context.Context, comb Combination) (res Result) {
	defer func() {
		if r := recover(); r != nil {
			res = Result{
				Outcome:        OutcomeUnknown,
				Transport:      TransportUnknown,
				Err:            ErrPanic,
				ErrDetail:      "probe panicked; details suppressed (see logs)",
				UserEliminated: false,
			}
			log.Errorf("brute probe panic (protocol=%s target=%s cred#%d): %s", s.Protocol, comb.Target, comb.Index,
				RedactText(fmt.Sprintf("%v", r), comb.Password))
		}
	}()

	target, err := ParseTarget(comb.Target)
	if err != nil {
		return Result{
			Outcome:   OutcomeProtocolMismatch,
			Err:       ErrProtocolParse,
			ErrDetail: "invalid target: " + redactSecret(err.Error()),
		}
	}

	res = s.Prober.Probe(ctx, target, Credential{
		Username: comb.Username,
		Password: comb.Password,
		Index:    comb.Index,
	}, Options{
		Timeout:   s.DefaultTimeout,
		TLSPolicy: s.TLSPolicy,
	})
	// 兜底脱敏：即使探针犯错也不让明文密码外泄。
	res.ErrDetail = redactSecret(res.ErrDetail)
	return res
}

func (s *Scheduler) safeSink(res *Result) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("brute result sink panic (protocol=%s): %v\nstack:\n%v", s.Protocol, r, debug.Stack())
		}
	}()
	s.Sink(*res)
}

// 哨兵错误。
type sentinelError string

func (e sentinelError) Error() string { return string(e) }

const (
	ErrNoProber = sentinelError("brute scheduler: prober is required")
	ErrNoSink   = sentinelError("brute scheduler: sink is required")
)
