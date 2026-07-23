package reactloops

import (
	"context"
	"fmt"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

const (
	// verificationAutoTriggerMaxSnapshotAge 是时间门阈值: 距上次 verification
	// 完成超过该时长后, 由看门狗强制再跑一次. 时间门由 watchdog 承担, 起算
	// 点是上次 verification 真正完成的时刻 (snapshot.GeneratedAt), 不论该次
	// verification 是自动 token 门、save_evidence 显式动作还是看门狗自身触发的.
	verificationAutoTriggerMaxSnapshotAge = 1800 * time.Second

	// verificationAutoTriggerMinPromptDelta 控制软 token 门 (加速器门) 的触发阈值.
	verificationAutoTriggerMinPromptDelta = 10 * 1024

	// verificationAutoTriggerHardPromptDelta 是 token 门的"硬阈值"上界,
	// 设为软门的 2 倍, 单次超大 token 增量时立即 fire.
	verificationAutoTriggerHardPromptDelta = 2 * verificationAutoTriggerMinPromptDelta
)

// verificationWatchdogMinInterval 是看门狗 timer 的最小兜底间隔, 防止在
// 剩余时间 ≈ 0 (刚 verify 完不久) 时 timer 被立即触发而狂调 verification.
var verificationWatchdogMinInterval = 2 * time.Minute

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

// ApplyVerificationResult stores verification side effects in the loop state.
func (r *ReActLoop) ApplyVerificationResult(result *aicommon.VerifySatisfactionResult) {
	if r == nil || result == nil {
		return
	}

	cfg := r.GetConfig()

	r.PushSatisfactionRecordWithCompletedTaskIndex(
		result.Satisfied,
		result.Reasoning,
		result.CompletedTaskIndex,
		result.NextMovements,
		result.Evidence,
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
func (r *ReActLoop) BeginVerificationWatchdogToolSuppression() {
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
func (r *ReActLoop) EndVerificationWatchdogToolSuppression() {
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
	r.rescheduleVerificationWatchdog(task)
}

// MaybeVerifyUserSatisfaction gates generic automatic verification.
// 自动门只负责 token 维度 (硬门 + 软门) 与末轮兜底; 时间门由看门狗
// (startVerificationWatchdog / rescheduleVerificationWatchdog) 基于
// snapshot.GeneratedAt 承担.
//
// 并发模型:
//   - verificationMutex 仍保留, 但作用域缩窄到 snapshot/throttle 双检, 不再
//     覆盖 AI 调用本身. 既有测试中对 verificationMutex 字段存在的依赖不受
//     影响.
//   - verificationInFlight (atomic.Bool, CAS) 是 AI 调用本身的并发屏障, 让
//     watchdog 在 verification 跑飞时能立刻感知而不阻塞.
//
// 清零语义 (与 VerifyUserSatisfactionNow 显式路径对齐):
//   - 触发 fire 前的 currentSnapshot 仅用于"门判断", 不作为新基线落盘
//   - fire 完成后, 用 fire 结束时刻的实时 snapshot 替换 prev, 让下次
//     时间门 (看门狗) / token 门都从"上次 verify 真正完成"那一刻起算
//
// 关键词: MaybeVerifyUserSatisfaction 自动 token 门, 时间门归 watchdog,
//
//	verificationInFlight CAS, fire 完成后清零基线
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
	// fire 完成后, 用 fire 结束时刻的实时 snapshot 替换 prev. 这样
	// prev.GeneratedAt / prev.IterationIndex / prev.LoopPromptTokens
	// 都以 fire 完成时刻为新基线, 让下次时间门 (看门狗, 基于
	// verificationAutoTriggerMaxSnapshotAge) / token 门都从"上次 verify
	// 真正完成"那一刻起算, 不被 AI 调用耗时 (常 5-30s) 白送给时间门.
	// 主循环 fire 期间同步阻塞, currentIterationIndex 与 LoopPromptTokens
	// 不变, 唯一变化的是 GeneratedAt.
	// 关键词: setVerificationRuntimeSnapshot fire 完成后基线,
	//        时间门归 watchdog, token 门归自动门, 清零统一
	r.setVerificationRuntimeSnapshot(r.buildVerificationRuntimeSnapshot(time.Now()))
	r.ApplyVerificationResult(result)
	return result, true, nil
}

// nextVerificationWatchdogDelay 计算看门狗 timer 下次触发的等待时长.
// 时间门由看门狗承担: 距上次 verification 完成时刻 (snapshot.GeneratedAt)
// 已经过的时间从 verificationAutoTriggerMaxSnapshotAge 中扣除, 剩余时间
// 即为下次应触发的延迟. 不论上次 verification 是自动 token 门、save_evidence
// 显式动作还是看门狗自身触发的, 都以 snapshot.GeneratedAt 为唯一起算点.
// 剩余时间过小 (刚 verify 完) 时用 verificationWatchdogMinInterval 兜底,
// 防止 timer 立即触发而狂调 verification.
func (r *ReActLoop) nextVerificationWatchdogDelay() time.Duration {
	previous := r.GetVerificationRuntimeSnapshot()
	if previous == nil || previous.GeneratedAt.IsZero() {
		// baseline 未建立, 用最小兜底间隔尽快触发首次 verification.
		return verificationWatchdogMinInterval
	}
	elapsed := time.Since(previous.GeneratedAt)
	remaining := verificationAutoTriggerMaxSnapshotAge - elapsed
	if remaining < verificationWatchdogMinInterval {
		return verificationWatchdogMinInterval
	}
	return remaining
}

// rescheduleVerificationWatchdog 重置看门狗 timer 为基于上次 verification
// 完成时刻计算的剩余等待时间. 调用方须已持有 verificationMutex.
func (r *ReActLoop) rescheduleVerificationWatchdog(task aicommon.AIStatefulTask) {
	if r == nil || task == nil || r.verificationMutex == nil {
		return
	}
	if r.verificationWatchdogTimer != nil {
		r.verificationWatchdogTimer.Stop()
		r.verificationWatchdogTimer = nil
	}
	delay := r.nextVerificationWatchdogDelay()
	r.verificationWatchdogTimer = time.AfterFunc(delay, func() {
		r.triggerVerificationWatchdog(task)
	})
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
	r.rescheduleVerificationWatchdog(task)
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
	// 子 Agent 进度旁路: 如果有活跃子 Agent 在运行 (dispatch_sub_react_agents
	// 阻塞了主循环), 跳过自动验证 — 此时主循环在等子 Agent, 不应误判任务已完成.
	// 关键词: verification watchdog sub-agent 旁路, dispatch 等待不误触发
	if registry := r.GetSubAgentProgressRegistry(); registry != nil && registry.IsAnyActive() {
		r.GetInvoker().AddToTimeline("[VERIFICATION_WATCHDOG_SUB_AGENT_ACTIVE]",
			"sub-agent(s) still active, verification watchdog skipped to avoid premature task finish")
		return
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
	// watchdog 不再替 AI 收口 (退出职责已完全迁移到 AI 主动 finish action +
	// maxIter 软中断兜底). verification 现在是纯观测/建议角色: 无论观测结果
	// 如何, 都只写一条 timeline nudge, 推动 AI 自己决定是否调 finish.
	// 关键词: watchdog nudge, 不再自动 task.Finish, 退出只走 finished,
	// verification 纯观测角色
	if result.Satisfied {
		r.GetInvoker().AddToTimeline("[VERIFICATION_WATCHDOG_SUGGEST_FINISH]",
			"verification observed that the current task goal appears achieved (reasoning: "+
				result.Reasoning+"). If you confirm there is no remaining work, call the `finish` action to terminate the ReAct loop; otherwise keep pushing execution forward.")
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

// shouldTriggerAutomaticVerification 决定本轮是否需要发起一次自动 verification.
// 自动门只负责 token 维度 (硬门 + 软门) 与末轮兜底; 时间门已迁移到看门狗
// (startVerificationWatchdog / rescheduleVerificationWatchdog), 由 watchdog
// 基于 snapshot.GeneratedAt 计算距上次 verification 完成的剩余时间来承担.
//
//  1. 末轮兜底: iter == maxIterations 必触发, 保证最终一定有一次 verify.
//  2. 硬 token 门: token delta >= verificationAutoTriggerHardPromptDelta 立即 fire.
//  3. 软 token 门: token delta >= verificationAutoTriggerMinPromptDelta 作为加速器.
//
// 关键词: shouldTriggerAutomaticVerification 仅 token 门, 时间门归 watchdog,
//
//	末轮兜底保留
func (r *ReActLoop) shouldTriggerAutomaticVerification(current *VerificationRuntimeSnapshot) bool {
	if r == nil || current == nil {
		return false
	}
	// 末轮兜底
	if r.maxIterations > 0 && current.IterationIndex == r.maxIterations {
		return true
	}
	// 当 periodicVerificationInterval <= 0 表示 "禁用节流, 每次 iter 都 fire"
	// 的测试/调试模式, 直接 fire.
	if r.periodicVerificationInterval <= 0 {
		return true
	}
	previous := r.GetVerificationRuntimeSnapshot()
	if previous == nil {
		// baseline 未建立, 等 token 门自然触发; 时间门由 watchdog 负责.
		return false
	}
	// token 门 (硬门 + 软门)
	tokenDelta := verificationPromptTokenDelta(previous, current)
	if tokenDelta >= verificationAutoTriggerHardPromptDelta {
		return true
	}
	return tokenDelta >= verificationAutoTriggerMinPromptDelta
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
