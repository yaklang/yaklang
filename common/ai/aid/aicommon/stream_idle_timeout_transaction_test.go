package aicommon_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	mockcfg "github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/log"
)

// hangingExpReader 永远不写也不结束 — 复现 "AI provider 建联后不下发字节".
type hangingExpReader struct {
	block chan struct{}
}

func newHangingExpReader() *hangingExpReader { return &hangingExpReader{block: make(chan struct{})} }
func (h *hangingExpReader) Read(p []byte) (int, error) {
	<-h.block
	return 0, io.EOF
}
func (h *hangingExpReader) Close() error {
	select {
	case <-h.block:
	default:
		close(h.block)
	}
	return nil
}

// TestExperiment_StreamIdleTimeoutBreaksAITransactionHang 实验复现:
//
// 假设: 当 CallAI 返回的 AIResponse 的输出流是一个 "活但不下发字节" 的流时,
// postHandler 内部消费该流会永久阻塞, 整个 ReAct 主循环也跟着卡死.
//
// 验证手段: 在 postHandler 里用 aicommon.NewStreamIdleTimeoutReader 套一层
// idle-timeout 包装, ttfb=200ms, 直接 io.ReadAll 后断言:
//   - 不再永久阻塞: io.ReadAll 在 ~ttfb 时间内返回 ErrStreamIdleTimeout
//   - retry 用尽后 CallAITransaction 返回非 nil 错误, 不会无限期阻塞
//   - 整个实验 wall-clock 远低于 "假设卡死" 的语义阈值
//
// 这条测试是 P0 "复现实验" 的最小可执行形式: 它把"流空闲超时"作为治理
// 假设直接证伪 (没有包装时阻塞 / 有包装时秒级返回).
//
// 关键词: 流空闲超时复现实验, ErrStreamIdleTimeout, CallAITransaction
func TestExperiment_StreamIdleTimeoutBreaksAITransactionHang(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := mockcfg.NewMockedAIConfig(ctx)
	cfg.SetConfig("AiTransactionAutoRetry", 2)

	var callAICount atomic.Int64
	hanger := newHangingExpReader()
	defer hanger.Close()

	callAi := func(req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		callAICount.Add(1)
		rsp := cfg.NewAIResponse()
		// 直接走 EmitOutputStreamWithoutConsumption + Close, 跳过外层
		// TeeAIResponse 包装 — 我们要观察的就是 raw stream 假活.
		rsp.EmitOutputStreamWithoutConsumption(hanger)
		rsp.Close()
		return rsp, nil
	}

	var lastTimedOut atomic.Bool

	postHandler := func(rsp *aicommon.AIResponse) error {
		raw := rsp.GetOutputStreamReader("stall-exp", true, cfg.GetEmitter())

		idleReader := aicommon.NewStreamIdleTimeoutReader(raw, 200*time.Millisecond, 0)
		defer func() {
			snap := idleReader.Snapshot()
			aicommon.LogStreamTimingSnapshot("STALL_EXP_TIMING", snap)
			if snap.TimedOut {
				lastTimedOut.Store(true)
			}
			_ = idleReader.Close()
		}()

		_, err := io.ReadAll(idleReader)
		return err
	}

	start := time.Now()
	transErr := aicommon.CallAITransaction(cfg, "stall-exp-prompt", callAi, postHandler)
	elapsed := time.Since(start)

	// 上限给得保守: 2 次 retry, 每次 200ms ttfb, 加上 transaction 内部固定的
	// 100ms 间隔, 极端情况下应该在 2s 内全部完成. 5s 是充足的兜底.
	if elapsed > 5*time.Second {
		t.Fatalf("CallAITransaction did not unblock in time, elapsed=%v", elapsed)
	}
	if transErr == nil {
		t.Fatalf("expected non-nil transaction error after all retries exhausted")
	}
	if !strings.Contains(transErr.Error(), "stream idle timeout") {
		t.Fatalf("expected error to mention stream idle timeout, got: %v", transErr)
	}
	if !lastTimedOut.Load() {
		t.Fatalf("expected last attempt's idle reader to report TimedOut=true")
	}
	if callAICount.Load() < int64(2) {
		t.Fatalf("expected CallAITransaction to retry at least once, attempts=%d", callAICount.Load())
	}
}

// TestExperiment_BaselineHangsWithoutWrapper 实验对照组: 在不套
// StreamIdleTimeoutReader 的情况下, 同一个 hanging 流会让 io.ReadAll
// 永久阻塞 — 我们这里用一个超短 deadline ctx 来证明这一点 (deadline 到达
// 前 io.ReadAll 不会返回任何东西).
//
// 注: 该测试故意"卡住", 但通过外层 deadline 兜底; 如果未来 io 标准库改变
// 使裸 Reader 也能 fail-fast, 这条断言会失效, 提醒我们 review 修复
// 必要性.
//
// 关键词: 复现实验 baseline, 裸 Reader 永久阻塞
func TestExperiment_BaselineHangsWithoutWrapper(t *testing.T) {
	hanger := newHangingExpReader()
	defer hanger.Close()

	resultCh := make(chan error, 1)
	go func() {
		_, err := io.ReadAll(hanger)
		resultCh <- err
	}()

	select {
	case err := <-resultCh:
		t.Fatalf("baseline reader unexpectedly returned, err=%v", err)
	case <-time.After(300 * time.Millisecond):
		// 符合预期: 裸 Reader 不会 fail-fast.
		log.Infof("baseline hang confirmed: io.ReadAll did not return within 300ms")
	}
	hanger.Close()
	select {
	case <-resultCh:
	case <-time.After(time.Second):
		t.Fatalf("baseline goroutine did not unwind after Close")
	}
}

// TestExperiment_StreamIdleTimeoutDisabledFallsBackToBaseline 验证: 当 feature
// flag 关闭 (即 thresholds 都为 0) 时, StreamIdleTimeoutReader 不再 fail-fast,
// 而是退化为透传 + 计时观测; 此时同样的 hanging 流就会阻塞 — 这是 P0 "纯观测
// 模式"语义的回归保证.
//
// 关键词: feature flag 关闭灰度回滚, 纯观测模式不 fail-fast
func TestExperiment_StreamIdleTimeoutDisabledFallsBackToBaseline(t *testing.T) {
	hanger := newHangingExpReader()
	defer hanger.Close()

	idleReader := aicommon.NewStreamIdleTimeoutReader(hanger, 0, 0)
	defer idleReader.Close()

	resultCh := make(chan error, 1)
	go func() {
		_, err := io.ReadAll(idleReader)
		resultCh <- err
	}()

	select {
	case err := <-resultCh:
		if errors.Is(err, aicommon.ErrStreamIdleTimeout) {
			t.Fatalf("disabled mode must not fail-fast with idle timeout, got %v", err)
		}
	case <-time.After(300 * time.Millisecond):
		// 符合预期: 没有 timeout 阈值时, reader 沿用底层 Reader 的阻塞语义.
	}
	hanger.Close()
	select {
	case <-resultCh:
	case <-time.After(time.Second):
		t.Fatalf("goroutine did not unwind after Close")
	}
}
