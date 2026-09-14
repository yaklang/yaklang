package core

import (
	"context"
	"testing"
	"time"
)

// 复现：两个目标、OkToStop、限速 0.2/s + jitter —— 第二目标必须被执行
func TestSchedulerTwoTargetsOkToStopWithRate(t *testing.T) {
	var calls []string
	var mu = make(chan struct{}, 1)
	mu <- struct{}{}
	record := func(s string) {
		<-mu
		calls = append(calls, s)
		mu <- struct{}{}
	}

	s := &Scheduler{
		Prober: ProberFunc(func(ctx context.Context, target Target, cred Credential, opts Options) Result {
			record(target.Host)
			return Result{Outcome: OutcomeAuthSuccess}
		}),
		Protocol:          "repro",
		GlobalConcurrency: 50,
		TargetConcurrency: 1,
		MaxPerSecond:      0.2,
		Jitter:            4 * time.Second,
		OkToStop:          true,
		Sink:              func(Result) {},
	}
	start := time.Now()
	stats, err := s.Run(context.Background(), NewCartesianSource(
		[]string{"t1.example:1", "t2.example:2"}, []string{"u"}, []string{"p"}))
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if len(calls) != 2 {
		t.Fatalf("两个目标都应被探测，实际 %d 次: %v (elapsed=%v executed=%d skipped=%d)",
			len(calls), calls, elapsed, stats.Executed, stats.Skipped)
	}
	t.Logf("耗时=%v executed=%d", elapsed, stats.Executed)
}

// TestSchedulerOkToStopDoesNotStarveLaterTargets 防回归：
// 首个目标 OkToStop 命中时，尚未创建状态机的后续目标（组合仍在队列）
// 不得被误判"全部完成"而整体取消（曾导致多目标+限速下后续目标饿死）。
func TestSchedulerOkToStopDoesNotStarveLaterTargets(t *testing.T) {
	probed := make(chan string, 8)
	s := &Scheduler{
		Prober: ProberFunc(func(ctx context.Context, target Target, cred Credential, opts Options) Result {
			probed <- target.Host
			return Result{Outcome: OutcomeAuthSuccess}
		}),
		Protocol:          "regress",
		GlobalConcurrency: 50,
		TargetConcurrency: 1,
		MaxPerSecond:      0.2, // 5s/令牌：制造首个目标先完成、后续组合仍在队列的窗口
		Jitter:            3 * time.Second,
		OkToStop:          true,
		Sink:              func(Result) {},
	}
	stats, err := s.Run(context.Background(), NewCartesianSource(
		[]string{"first.example:1", "second.example:2"}, []string{"u"}, []string{"p"}))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Executed != 2 {
		t.Fatalf("both targets must execute, executed=%d", stats.Executed)
	}
}
