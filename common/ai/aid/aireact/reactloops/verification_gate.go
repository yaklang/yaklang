package reactloops

import (
	"context"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
)

const (
	verificationAutoTriggerMaxSnapshotAge = 30 * time.Second
	verificationAutoTriggerMinPromptDelta = 500
	verificationIterationTriggerInterval  = aicommon.DefaultPeriodicVerificationInterval
)

var verificationWatchdogIdleTimeout = 2 * time.Minute

type VerificationRuntimeSnapshot struct {
	GeneratedAt      time.Time `json:"generated_at"`
	IterationIndex   int       `json:"iteration_index"`
	LoopPromptTokens int       `json:"loop_prompt_tokens,omitempty"`
}

// RefreshVerificationRuntimeSnapshot rebuilds the runtime snapshot from the
// current loop state and replaces the previously stored snapshot pointer.
func (r *ReActLoop) RefreshVerificationRuntimeSnapshot() *VerificationRuntimeSnapshot {
	if r == nil {
		return nil
	}

	snapshot := r.buildVerificationRuntimeSnapshot(time.Now())
	r.actionHistoryMutex.Lock()
	defer r.actionHistoryMutex.Unlock()
	r.verificationRuntimeSnapshot = snapshot
	return r.verificationRuntimeSnapshot
}

// GetVerificationRuntimeSnapshot returns the currently stored snapshot pointer.
// Callers should treat the returned pointer as read-only.
func (r *ReActLoop) GetVerificationRuntimeSnapshot() *VerificationRuntimeSnapshot {
	if r == nil {
		return nil
	}

	r.actionHistoryMutex.Lock()
	defer r.actionHistoryMutex.Unlock()
	return r.verificationRuntimeSnapshot
}

func (r *ReActLoop) buildVerificationRuntimeSnapshot(generatedAt time.Time) *VerificationRuntimeSnapshot {
	if r == nil {
		return nil
	}

	snapshot := &VerificationRuntimeSnapshot{
		GeneratedAt:      generatedAt,
		LoopPromptTokens: r.getVerificationRuntimePromptTokens(),
	}
	r.actionHistoryMutex.Lock()
	snapshot.IterationIndex = r.currentIterationIndex
	r.actionHistoryMutex.Unlock()
	return snapshot
}

func (r *ReActLoop) setVerificationRuntimeSnapshot(snapshot *VerificationRuntimeSnapshot) {
	if r == nil {
		return
	}
	r.actionHistoryMutex.Lock()
	defer r.actionHistoryMutex.Unlock()
	r.verificationRuntimeSnapshot = snapshot
}

func (r *ReActLoop) getVerificationRuntimePromptTokens() int {
	if r == nil {
		return 0
	}
	if observation := r.GetLastPromptObservation(); observation != nil {
		return observation.PromptTokens
	}
	if status := r.GetLastPromptObservationStatus(); status != nil {
		return status.PromptTokens
	}
	return 0
}

// ShouldTriggerPeriodicCheckpointOnIteration reports whether periodic
// checkpoints such as perception/verification should run on this iteration.
func (r *ReActLoop) ShouldTriggerPeriodicCheckpointOnIteration(iterationIndex int) bool {
	if r == nil {
		return false
	}
	interval := r.periodicVerificationInterval
	if interval <= 0 {
		return true
	}
	if iterationIndex > 0 && iterationIndex%interval == 0 {
		return true
	}
	return r.maxIterations > 0 && iterationIndex > 0 && iterationIndex == r.maxIterations
}

// pushDeliveryFileToTimeline records a verification-confirmed output file as
// an Open Timeline entry. Only the file path + lightweight metadata
// (size / mime / mtime) is written; the file body is NEVER read or sampled.
//
// Rationale: previously, every confirmed delivery file was wired into
// ContextProviderManager via RegisterTracedContent + OutputFileContextProvider,
// which re-injected the full file body (capped at 40KB) into Pure Dynamic /
// AutoContext on EVERY subsequent prompt build. That flooded the dynamic
// segment with stale file contents and bloated tokens regardless of whether
// the AI actually needed them.
//
// New design: the only fact we want the model to remember is "such-and-such
// file was delivered at iteration N"; the body, if needed, can be re-read on
// demand via existing file-read / view-window actions. By going through
// Timeline.PushText, the entry naturally rides the frozen / open / batch
// compress lifecycle and is forgotten organically as the conversation moves on.
//
// 关键词: pushDeliveryFileToTimeline, [DELIVERY FILE] 极简元数据,
//
//	Open Timeline 自然淘汰, Pure Dynamic 反污染, AutoContext 反污染,
//	不读取文件正文, 不采样字节
// timelineProvider is the duck-typed interface pushDeliveryFileToTimeline
// uses to obtain the timeline instance. *aicommon.Config already satisfies
// it via its GetTimeline() method; tests can satisfy it with a mock that
// returns a real *aicommon.Timeline. This keeps AICallerConfigIf untouched
// while still letting the helper be exercised from the reactloops test pkg.
//
// 关键词: timelineProvider 鸭子类型, GetTimeline, 测试 mock 友好
type timelineProvider interface {
	GetTimeline() *aicommon.Timeline
}

func pushDeliveryFileToTimeline(cfg aicommon.AICallerConfigIf, filePath string) {
	if cfg == nil || filePath == "" {
		return
	}
	provider, ok := cfg.(timelineProvider)
	if !ok || provider == nil {
		log.Warnf("delivery file %s: config does not expose Timeline; skip", filePath)
		return
	}
	timeline := provider.GetTimeline()
	if timeline == nil {
		log.Warnf("delivery file %s: Timeline instance unavailable; skip", filePath)
		return
	}

	sizeStr := "unknown"
	mtimeStr := "unknown"
	if fi, err := os.Stat(filePath); err == nil {
		sizeStr = formatDeliveryFileSize(fi.Size())
		mtimeStr = fi.ModTime().UTC().Format(time.RFC3339)
	} else {
		log.Warnf("delivery file %s: stat failed (%v); recording path-only entry", filePath, err)
	}

	mimeStr := mime.TypeByExtension(filepath.Ext(filePath))
	if mimeStr == "" {
		mimeStr = "unknown"
	}

	text := fmt.Sprintf(
		"[DELIVERY FILE] path=%s\nsize=%s mime=%s\nmtime=%s",
		filePath, sizeStr, mimeStr, mtimeStr,
	)
	timeline.PushText(cfg.AcquireId(), text)
}

// formatDeliveryFileSize is a local copy of the size formatter used by the
// Workspace artifacts listing, kept here to avoid pulling aicommon-internal
// helpers across packages.
func formatDeliveryFileSize(size int64) string {
	switch {
	case size >= 1024*1024:
		return fmt.Sprintf("%.1fMB", float64(size)/(1024*1024))
	case size >= 1024:
		return fmt.Sprintf("%.1fKB", float64(size)/1024)
	default:
		return fmt.Sprintf("%dB", size)
	}
}

// ApplyVerificationResult stores verification side effects in the loop state.
func (r *ReActLoop) ApplyVerificationResult(result *aicommon.VerifySatisfactionResult) {
	if r == nil || result == nil {
		return
	}

	cfg := r.GetConfig()
	if cfg != nil && len(result.OutputFiles) > 0 {
		// 交付文件不再走 ContextProviderManager / Pure Dynamic; 改为 push 到
		// Open Timeline (仅文件名 + 元数据, 不读文件正文). EmitPinFilename
		// 仍保留, 走前端文件 pin 通道, 与 prompt 上下文无关.
		// 关键词: 交付文件 timeline 化, AutoContext 反污染, EmitPinFilename 保留
		for _, filePath := range result.OutputFiles {
			pushDeliveryFileToTimeline(cfg, filePath)
			if emitter := cfg.GetEmitter(); emitter != nil {
				emitter.EmitPinFilename(filePath)
			}
		}
	}

	r.PushSatisfactionRecordWithCompletedTaskIndex(
		result.Satisfied,
		result.Reasoning,
		result.CompletedTaskIndex,
		result.NextMovements,
		result.Evidence,
		result.OutputFiles,
		result.EvidenceOps,
	)
	if cfg != nil && len(result.EvidenceOps) > 0 {
		cfg.ApplySessionEvidenceOps(result.EvidenceOps)
	}
	r.MaybeTriggerPerceptionAfterVerification()
}

// VerifyUserSatisfactionNow forces a verification pass immediately, bypassing
// periodic checkpoint throttling. This is used by explicit AI-triggered
// verification actions.
//
// 并发模型: 通过 verificationInFlight (atomic.Bool, CAS) 保证同一时间只有一
// 个 verification AI 调用在飞行中. verificationMutex 不再覆盖 AI 调用本身
// (那会让 watchdog 也被卡死), 仅用于保护 snapshot 与 suppression depth 等
// 内部状态读写. 关键词: VerifyUserSatisfactionNow watchdog 解锁,
// verificationInFlight CAS, verificationMutex 缩窄作用域
func (r *ReActLoop) VerifyUserSatisfactionNow(
	ctx context.Context,
	originalQuery string,
	isToolCall bool,
	payload string,
) (*aicommon.VerifySatisfactionResult, error) {
	if r == nil || r.invoker == nil {
		return nil, nil
	}
	r.touchVerificationWatchdog()
	if !r.verificationInFlight.CompareAndSwap(false, true) {
		// 已有一次 verification 在跑, 不重入. 这条路径会被 explicit
		// AI-triggered 触发 (例如 verify action), 直接返回 nil/nil 让上层
		// 视为"本次跳过", 不影响主循环.
		if invoker := r.GetInvoker(); invoker != nil {
			invoker.AddToTimeline("[VERIFICATION_REENTRY_SKIP]", "VerifyUserSatisfactionNow skipped: another verification still in flight")
		}
		return nil, nil
	}
	defer r.verificationInFlight.Store(false)
	result, err := r.invoker.VerifyUserSatisfaction(ctx, originalQuery, isToolCall, payload)
	if err != nil {
		return nil, err
	}
	r.setVerificationRuntimeSnapshot(r.buildVerificationRuntimeSnapshot(time.Now()))
	r.ApplyVerificationResult(result)
	return result, nil
}

// MaybeVerifyUserSatisfaction gates generic automatic verification to avoid
// running it after every tool call. In addition to the shared periodic
// checkpoint rule, it only re-runs auto-verification when the last accepted
// verification baseline is absent, stale, or the loop prompt has changed
// materially.
// beginVerificationWatchdogToolSuppression marks the start of a synchronous blocking
// tool invocation on the ReAct thread. While the depth is >0, the verification watchdog
// timer must not fire or be rescheduled via touchVerificationWatchdog.
func (r *ReActLoop) beginVerificationWatchdogToolSuppression() {
	if r == nil || r.verificationMutex == nil {
		return
	}
	r.verificationMutex.Lock()
	defer r.verificationMutex.Unlock()
	r.verificationWatchdogToolSuppressionDepth++
	if r.verificationWatchdogToolSuppressionDepth == 1 && r.verificationWatchdogTimer != nil {
		r.verificationWatchdogTimer.Stop()
		r.verificationWatchdogTimer = nil
	}
}

// endVerificationWatchdogToolSuppression pairs with beginVerificationWatchdogToolSuppression.
// When the outermost tool call finishes, the watchdog timer is restarted from idle timeout.
func (r *ReActLoop) endVerificationWatchdogToolSuppression() {
	if r == nil || r.verificationMutex == nil {
		return
	}
	task := r.GetCurrentTask()
	r.verificationMutex.Lock()
	defer r.verificationMutex.Unlock()
	if r.verificationWatchdogToolSuppressionDepth > 0 {
		r.verificationWatchdogToolSuppressionDepth--
	}
	if r.verificationWatchdogToolSuppressionDepth > 0 {
		return
	}
	if task == nil {
		return
	}
	if r.verificationWatchdogTimer != nil {
		r.verificationWatchdogTimer.Stop()
		r.verificationWatchdogTimer = nil
	}
	r.verificationWatchdogTimer = time.AfterFunc(verificationWatchdogIdleTimeout, func() {
		r.triggerVerificationWatchdog(task)
	})
}

// MaybeVerifyUserSatisfaction gates generic automatic verification. 并发模型:
//   - verificationMutex 仍保留, 但作用域缩窄到 snapshot/throttle 双检, 不再
//     覆盖 AI 调用本身. 既有测试中对 verificationMutex 字段存在的依赖不受
//     影响.
//   - verificationInFlight (atomic.Bool, CAS) 是 AI 调用本身的并发屏障, 让
//     watchdog 在 verification 跑飞时能立刻感知而不阻塞.
//
// 关键词: MaybeVerifyUserSatisfaction watchdog 解锁, verificationInFlight CAS,
//
//	verificationMutex 缩窄作用域
func (r *ReActLoop) MaybeVerifyUserSatisfaction(
	ctx context.Context,
	originalQuery string,
	isToolCall bool,
	payload string,
) (*aicommon.VerifySatisfactionResult, bool, error) {
	if r == nil || r.invoker == nil {
		return nil, false, nil
	}
	r.touchVerificationWatchdog()
	currentSnapshot := r.buildVerificationRuntimeSnapshot(time.Now())
	if !r.shouldTriggerAutomaticVerification(currentSnapshot) {
		return nil, false, nil
	}
	if r.verificationMutex != nil {
		r.verificationMutex.Lock()
		currentSnapshot = r.buildVerificationRuntimeSnapshot(time.Now())
		shouldRun := r.shouldTriggerAutomaticVerification(currentSnapshot)
		r.verificationMutex.Unlock()
		if !shouldRun {
			return nil, false, nil
		}
	} else {
		currentSnapshot = r.buildVerificationRuntimeSnapshot(time.Now())
		if !r.shouldTriggerAutomaticVerification(currentSnapshot) {
			return nil, false, nil
		}
	}
	if !r.verificationInFlight.CompareAndSwap(false, true) {
		// 上一次 verification 还没回来, 本轮自动 verification 让位. 不算
		// 一次有效 verification (returned bool = false), 也不写 timeline,
		// 否则同一个卡死会被反复广播. 关键词: 自动 verification 让位
		return nil, false, nil
	}
	defer r.verificationInFlight.Store(false)
	result, err := r.invoker.VerifyUserSatisfaction(ctx, originalQuery, isToolCall, payload)
	if err != nil {
		return nil, true, err
	}
	r.setVerificationRuntimeSnapshot(currentSnapshot)
	r.ApplyVerificationResult(result)
	return result, true, nil
}

func (r *ReActLoop) startVerificationWatchdog(task aicommon.AIStatefulTask) {
	if r == nil || task == nil || r.verificationMutex == nil {
		return
	}
	r.verificationMutex.Lock()
	defer r.verificationMutex.Unlock()
	if r.verificationWatchdogToolSuppressionDepth > 0 {
		return
	}
	if r.verificationWatchdogTimer != nil {
		r.verificationWatchdogTimer.Stop()
	}
	r.verificationWatchdogTimer = time.AfterFunc(verificationWatchdogIdleTimeout, func() {
		r.triggerVerificationWatchdog(task)
	})
}

func (r *ReActLoop) touchVerificationWatchdog() {
	if r == nil {
		return
	}
	task := r.GetCurrentTask()
	if task == nil {
		return
	}
	if r.verificationMutex != nil {
		r.verificationMutex.Lock()
		suppressed := r.verificationWatchdogToolSuppressionDepth > 0
		r.verificationMutex.Unlock()
		if suppressed {
			return
		}
	}
	r.startVerificationWatchdog(task)
}

func (r *ReActLoop) stopVerificationWatchdogForTask(task aicommon.AIStatefulTask) {
	if r == nil || task == nil || r.verificationMutex == nil {
		return
	}
	r.verificationMutex.Lock()
	defer r.verificationMutex.Unlock()
	if r.verificationWatchdogTimer != nil {
		r.verificationWatchdogTimer.Stop()
		r.verificationWatchdogTimer = nil
	}
	r.verificationWatchdogToolSuppressionDepth = 0
}

func (r *ReActLoop) triggerVerificationWatchdog(task aicommon.AIStatefulTask) {
	if r == nil || task == nil || task.IsFinished() {
		return
	}
	if r.verificationMutex != nil {
		r.verificationMutex.Lock()
		suppressed := r.verificationWatchdogToolSuppressionDepth > 0
		r.verificationMutex.Unlock()
		if suppressed {
			return
		}
	}
	select {
	case <-task.GetContext().Done():
		return
	default:
	}
	if r.GetInvoker() == nil {
		return
	}
	// 关键: 若 verification 此刻仍在飞行 (可能因 AI 流卡死), watchdog
	// 不再去抢同一把锁 — 直接写一条 [ASYNC_VERIFICATION_WATCHDOG_BUSY]
	// timeline 痕迹后退出, 等下一个 idle 周期再重试. 这条路径是修复"AI
	// 流假活 + watchdog 跟着一起阻塞"问题的核心. 关键词:
	// triggerVerificationWatchdog 解锁, [ASYNC_VERIFICATION_WATCHDOG_BUSY]
	if r.verificationInFlight.Load() {
		r.GetInvoker().AddToTimeline("[ASYNC_VERIFICATION_WATCHDOG_BUSY]",
			"previous verification still in flight, watchdog skipped this round; will retry on next idle window")
		return
	}
	payload := r.buildVerificationWatchdogPayload(task)
	r.GetInvoker().AddToTimeline("[ASYNC_VERIFICATION_WATCHDOG]", payload)
	result, err := r.VerifyUserSatisfactionNow(task.GetContext(), task.GetUserInput(), false, payload)
	if err != nil {
		r.GetInvoker().AddToTimeline("verification_watchdog_error", err.Error())
		return
	}
	if result == nil {
		return
	}
	if result.Satisfied {
		task.Finish(nil)
		r.stopVerificationWatchdogForTask(task)
		return
	}
	r.GetInvoker().AddToTimeline("verification_watchdog_unsatisfied", result.Reasoning)
}

func (r *ReActLoop) buildVerificationWatchdogPayload(task aicommon.AIStatefulTask) string {
	if r == nil {
		return "Verification watchdog triggered because MaybeVerifyUserSatisfaction has been idle for too long."
	}
	payload := "Verification watchdog triggered because MaybeVerifyUserSatisfaction has been idle for too long."
	payload += fmt.Sprintf("\nCurrent iteration: %d.", r.GetCurrentIterationIndex())
	if last := r.GetLastAction(); last != nil {
		payload += "\nLast action: " + last.ActionType + "."
	}
	if task != nil {
		payload += "\nPlease verify whether the current work already satisfies the user goal."
	}
	return payload
}

func (r *ReActLoop) shouldTriggerAutomaticVerification(current *VerificationRuntimeSnapshot) bool {
	if r == nil || current == nil {
		return false
	}
	if r.maxIterations > 0 && current.IterationIndex == r.maxIterations {
		return true
	}
	previous := r.GetVerificationRuntimeSnapshot()
	if previous == nil {
		return r.ShouldTriggerPeriodicCheckpointOnIteration(current.IterationIndex)
	}
	if current.GeneratedAt.Sub(previous.GeneratedAt) >= verificationAutoTriggerMaxSnapshotAge {
		return true
	}
	if current.IterationIndex-previous.IterationIndex >= r.getVerificationIterationTriggerInterval() {
		return true
	}
	return verificationPromptTokenDelta(previous, current) >= verificationAutoTriggerMinPromptDelta
}

func verificationPromptTokenDelta(previous *VerificationRuntimeSnapshot, current *VerificationRuntimeSnapshot) int {
	if previous == nil || current == nil {
		return 0
	}
	delta := current.LoopPromptTokens - previous.LoopPromptTokens
	if delta < 0 {
		return -delta
	}
	return delta
}

func (r *ReActLoop) getVerificationIterationTriggerInterval() int {
	if r == nil {
		return verificationIterationTriggerInterval
	}
	return r.periodicVerificationInterval
}
