package core

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// recordingProber 记录调用并按用户名/密码返回结果。
type recordingProber struct {
	mu       sync.Mutex
	calls    []Credential
	inflight int32
	maxSeen  int32
	gate     chan struct{} // 非nil时：每次探测阻塞直到收到信号
	result   func(cred Credential) Result
}

func (p *recordingProber) Probe(ctx context.Context, target Target, cred Credential, opts Options) Result {
	cur := atomic.AddInt32(&p.inflight, 1)
	for {
		max := atomic.LoadInt32(&p.maxSeen)
		if cur <= max || atomic.CompareAndSwapInt32(&p.maxSeen, max, cur) {
			break
		}
	}
	defer atomic.AddInt32(&p.inflight, -1)
	p.mu.Lock()
	p.calls = append(p.calls, cred)
	p.mu.Unlock()

	if p.gate != nil {
		select {
		case <-p.gate:
		case <-ctx.Done():
			return Result{Outcome: OutcomeCancelled, Err: ErrCancelled}
		}
	}
	if p.result != nil {
		return p.result(cred)
	}
	return Result{Outcome: OutcomeAuthFailed}
}

func newTestScheduler(prober Prober, sink func(Result)) *Scheduler {
	return &Scheduler{
		Prober:            prober,
		Protocol:          "test",
		GlobalConcurrency: 4,
		TargetConcurrency: 2,
		QueueSize:         8,
		Sink:              sink,
	}
}

func TestSchedulerFullCartesian(t *testing.T) {
	prober := &recordingProber{}
	var results []Result
	var mu sync.Mutex
	s := newTestScheduler(prober, func(r Result) {
		mu.Lock()
		defer mu.Unlock()
		results = append(results, r)
	})
	targets := []string{"a:1", "b:2"}
	users := []string{"u1", "u2"}
	passes := []string{"p1", "p2", "p3"}

	stats, err := s.Run(context.Background(), NewCartesianSource(targets, users, passes))
	if err != nil {
		t.Fatal(err)
	}
	if int(stats.Executed) != len(targets)*len(users)*len(passes) {
		t.Fatalf("expected %d executed, got %d", len(targets)*len(users)*len(passes), stats.Executed)
	}
	if len(results) != len(targets)*len(users)*len(passes) {
		t.Fatalf("expected %d results, got %d", len(targets)*len(users)*len(passes), len(results))
	}
}

func TestSchedulerOrderingMatchesMixer(t *testing.T) {
	// 与旧 mixer(target, pass, user) 一致：user 内层、pass 中层、target 外层
	src := NewCartesianSource([]string{"t1", "t2"}, []string{"u1", "u2"}, []string{"p1", "p2"})
	var got []string
	for {
		c, ok := src.Next(context.Background())
		if !ok {
			break
		}
		got = append(got, fmt.Sprintf("%s/%s/%s", c.Target, c.Password, c.Username))
	}
	want := []string{
		"t1/p1/u1", "t1/p1/u2",
		"t1/p2/u1", "t1/p2/u2",
		"t2/p1/u1", "t2/p1/u2",
		"t2/p2/u1", "t2/p2/u2",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d combos, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("combo %d: got %s want %s", i, got[i], want[i])
		}
	}
}

func TestSchedulerOkToStop(t *testing.T) {
	prober := &recordingProber{result: func(cred Credential) Result {
		if cred.Password == "hit" {
			return Result{Outcome: OutcomeAuthSuccess}
		}
		return Result{Outcome: OutcomeAuthFailed}
	}}
	s := newTestScheduler(prober, func(Result) {})
	s.OkToStop = true

	passes := make([]string, 0, 100)
	for i := 0; i < 100; i++ {
		passes = append(passes, fmt.Sprintf("p%d", i))
	}
	passes[7] = "hit"
	stats, err := s.Run(context.Background(), NewCartesianSource([]string{"t:1"}, []string{"u"}, passes))
	if err != nil {
		t.Fatal(err)
	}
	// 命中后目标短路：执行次数应远小于完整 100 次（受并发窗口影响有少量超出）。
	if stats.Executed > 20 {
		t.Fatalf("ok-to-stop not honored: executed %d", stats.Executed)
	}
	// 注：skip 计数依赖派发时序（高负载下命中前可能已全部派发），
	// 行为正确性由 executed 上限断言保证。
}

func TestSchedulerFinalOutcomeStopsTarget(t *testing.T) {
	var calls int32
	prober := &recordingProber{result: func(cred Credential) Result {
		atomic.AddInt32(&calls, 1)
		return Result{Outcome: OutcomeTargetUnavailable}
	}}
	s := newTestScheduler(prober, func(Result) {})
	s.TargetConcurrency = 1
	stats, err := s.Run(context.Background(), NewCartesianSource([]string{"t:1"}, []string{"u1", "u2"}, []string{"p1", "p2", "p3"}))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Executed != 1 {
		t.Fatalf("target should stop after first final outcome, executed=%d", stats.Executed)
	}
}

func TestSchedulerLockoutBudget(t *testing.T) {
	prober := &recordingProber{result: func(cred Credential) Result {
		return Result{Outcome: OutcomeAccountLocked}
	}}
	s := newTestScheduler(prober, func(Result) {})
	s.LockoutBudget = 2
	stats, err := s.Run(context.Background(), NewCartesianSource([]string{"t:1"}, []string{"u1"}, []string{"p1", "p2", "p3", "p4", "p5"}))
	if err != nil {
		t.Fatal(err)
	}
	// 并发窗口内最多多探测 TargetConcurrency-1 次（有界竞态），
	// 锁定预算在下一个调度窗口必然生效。
	if stats.Executed > int64(1+s.TargetConcurrency) {
		t.Fatalf("lockout budget not enforced, executed=%d", stats.Executed)
	}
	if stats.Lockouts != stats.Executed {
		t.Fatalf("lockout stats mismatch")
	}
}

func TestSchedulerRetryAfterBackoff(t *testing.T) {
	prober := &recordingProber{result: func(cred Credential) Result {
		return Result{Outcome: OutcomeRateLimited, RetryAfter: 50 * time.Millisecond}
	}}
	s := newTestScheduler(prober, func(Result) {})
	s.TargetConcurrency = 1
	start := time.Now()
	stats, err := s.Run(context.Background(), NewCartesianSource([]string{"t:1"}, []string{"u"}, []string{"p1", "p2", "p3"}))
	if err != nil {
		t.Fatal(err)
	}
	// 3 次尝试 × 50ms 退避至少 100ms（第 3 次尝试前的两次退避）。
	if time.Since(start) < 90*time.Millisecond {
		t.Fatalf("retry-after backoff too fast: executed=%d elapsed<90ms", stats.Executed)
	}
}

func TestSchedulerOnlyNeedPasswordDynamic(t *testing.T) {
	var once sync.Once
	prober := &recordingProber{result: func(cred Credential) Result {
		var res Result
		res.Outcome = OutcomeAuthFailed
		once.Do(func() { res.OnlyNeedPassword = true })
		return res
	}}
	s := newTestScheduler(prober, func(Result) {})
	stats, err := s.Run(context.Background(), NewCartesianSource(
		[]string{"t:1"}, []string{"u1", "u2", "u3"}, []string{"p1", "p2", "p3"}))
	if err != nil {
		t.Fatal(err)
	}
	// 动态开启后，每个密码只试一次：至多 u 数量（首次窗口）+ 剩余密码数。
	if stats.Executed > 3+3 {
		t.Fatalf("only-need-password dedup not applied: executed=%d", stats.Executed)
	}
}

func TestSchedulerUserEliminated(t *testing.T) {
	prober := &recordingProber{result: func(cred Credential) Result {
		if cred.Username == "bad" {
			return Result{Outcome: OutcomeAuthFailed, UserEliminated: true}
		}
		return Result{Outcome: OutcomeAuthFailed}
	}}
	s := newTestScheduler(prober, func(Result) {})
	s.TargetConcurrency = 1
	stats, err := s.Run(context.Background(), NewCartesianSource(
		[]string{"t:1"}, []string{"bad", "good"}, []string{"p1", "p2"}))
	if err != nil {
		t.Fatal(err)
	}
	// bad 用户第一次尝试后即被消除：bad 只执行 1 次。
	prober.mu.Lock()
	defer prober.mu.Unlock()
	bad := 0
	for _, c := range prober.calls {
		if c.Username == "bad" {
			bad++
		}
	}
	if bad != 1 {
		t.Fatalf("eliminated user probed %d times, want 1", bad)
	}
	if stats.Skipped == 0 {
		t.Fatal("expected skips")
	}
}

func TestSchedulerCancelStopsInflight(t *testing.T) {
	gate := make(chan struct{})
	prober := &recordingProber{gate: gate, result: func(Credential) Result {
		return Result{Outcome: OutcomeAuthFailed}
	}}
	s := newTestScheduler(prober, func(Result) {})
	s.GlobalConcurrency = 2
	s.TargetConcurrency = 2

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_, _ = s.Run(ctx, NewCartesianSource([]string{"t:1"}, []string{"u"}, []string{"p1", "p2", "p3", "p4"}))
		close(done)
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()
	// 取消后探针的 ctx 应立即生效（gate 分支返回 Cancelled），Run 应在期限内退出。
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
	close(gate)
}

func TestSchedulerRespectsGlobalConcurrency(t *testing.T) {
	prober := &recordingProber{}
	s := newTestScheduler(prober, func(Result) {})
	s.GlobalConcurrency = 3
	s.TargetConcurrency = 3
	stats, err := s.Run(context.Background(), NewCartesianSource(
		[]string{"t:1", "t:2"}, []string{"u1"}, []string{"p1", "p2", "p3", "p4", "p5", "p6"}))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Executed == 0 {
		t.Fatal("no executions")
	}
	if prober.maxSeen > 3 {
		t.Fatalf("global concurrency exceeded: max inflight %d > 3", prober.maxSeen)
	}
}

func TestSchedulerRespectsTargetConcurrency(t *testing.T) {
	prober := &recordingProber{}
	s := newTestScheduler(prober, func(Result) {})
	s.GlobalConcurrency = 10
	s.TargetConcurrency = 1
	stats, err := s.Run(context.Background(), NewCartesianSource(
		[]string{"t:1"}, []string{"u1"}, []string{"p1", "p2", "p3", "p4", "p5", "p6"}))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Executed == 0 {
		t.Fatal("no executions")
	}
	// 单目标：任意时刻 inflight 不应超过 1。
	if prober.maxSeen > 1 {
		t.Fatalf("target concurrency exceeded: max inflight %d > 1", prober.maxSeen)
	}
}

func TestSchedulerRateLimit(t *testing.T) {
	prober := &recordingProber{}
	s := newTestScheduler(prober, func(Result) {})
	s.GlobalConcurrency = 8
	s.TargetConcurrency = 8
	s.MaxPerSecond = 100 // 10ms 间隔
	start := time.Now()
	stats, err := s.Run(context.Background(), NewCartesianSource(
		[]string{"t:1"}, []string{"u"}, []string{"p1", "p2", "p3", "p4", "p5"}))
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 40*time.Millisecond {
		t.Fatalf("rate limit not effective: 5 attempts in %v", time.Since(start))
	}
	if stats.Executed != 5 {
		t.Fatalf("executed=%d want 5", stats.Executed)
	}
}

func TestSchedulerPreCheck(t *testing.T) {
	prober := &recordingProber{}
	s := newTestScheduler(prober, func(Result) {})
	s.PreCheck = func(target string) bool { return strings.HasPrefix(target, "good") }
	stats, err := s.Run(context.Background(), NewCartesianSource(
		[]string{"bad:1", "good:2"}, []string{"u"}, []string{"p"}))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Executed != 1 {
		t.Fatalf("pre-check not enforced, executed=%d", stats.Executed)
	}
}

func TestSchedulerPanicRecovery(t *testing.T) {
	sentinel := "PANIC-SECRET-PASSWORD"
	var got []Result
	var mu sync.Mutex
	prober := ProberFunc(func(ctx context.Context, target Target, cred Credential, opts Options) Result {
		panic(fmt.Sprintf("boom with password %s", sentinel))
	})
	s := newTestScheduler(prober, func(r Result) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, r)
	})
	stats, err := s.Run(context.Background(), NewCartesianSource([]string{"t:1"}, []string{"u"}, []string{"p"}))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Executed != 1 {
		t.Fatalf("executed=%d", stats.Executed)
	}
	r := got[0]
	if r.Outcome != OutcomeUnknown || r.Err != ErrPanic {
		t.Fatalf("expected unknown/panic result, got %+v", r)
	}
	if strings.Contains(r.String(), sentinel) {
		t.Fatalf("panic detail leaked into result: %s", r.String())
	}
}

func TestSchedulerNoGoroutineLeak(t *testing.T) {
	before := runtime.NumGoroutine()
	for i := 0; i < 5; i++ {
		prober := &recordingProber{}
		s := newTestScheduler(prober, func(Result) {})
		if _, err := s.Run(context.Background(), NewCartesianSource(
			[]string{"t:1", "t:2"}, []string{"u1", "u2"}, []string{"p1", "p2"})); err != nil {
			t.Fatal(err)
		}
	}
	// 允许少量运行时自身波动。
	if n := runtime.NumGoroutine(); n > before+10 {
		t.Fatalf("goroutine leak: before=%d after=%d", before, n)
	}
}

func TestCredentialRedaction(t *testing.T) {
	c := Credential{Username: "admin", Password: "super-secret"}
	s := c.String()
	if strings.Contains(s, "super-secret") {
		t.Fatalf("credential string leaks password: %s", s)
	}
	if !strings.Contains(s, "admin") {
		t.Fatalf("username should be visible: %s", s)
	}
	id := c.ID()
	if strings.Contains(id, "super-secret") {
		t.Fatal("credential ID leaks password")
	}
	empty := Credential{Username: "u", Password: ""}
	if !strings.Contains(empty.String(), "<empty>") {
		t.Fatalf("empty password marker missing: %s", empty.String())
	}
}

func TestResultRedaction(t *testing.T) {
	cred := Credential{Username: "admin", Password: "leak-me"}
	r := Result{
		Outcome:            OutcomeAuthFailed,
		Protocol:           "test",
		TargetID:           "1.2.3.4:22",
		CredID:             cred.ID(),
		Transport:          TransportTLS,
		RawCredentialIndex: 42,
		ErrDetail:          "auth rejected",
	}
	s := r.String()
	if strings.Contains(s, "leak-me") {
		t.Fatalf("result leaks password: %s", s)
	}
}
