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
	// verificationAutoTriggerMaxSnapshotAge 控制"距上次 verification 多久后强制再跑"的时间门.
	// 调整自 120s -> 180s, 依据用户实测反馈 + docs/16-verification-frequency-experiment.md:
	// 原 120s 阈值在长任务节奏下仍偶发"时间门刚好触发"导致额外开销.
	// 新阈值 180s 与 iter 门 (6) + 冷静期 (3) 配合, 形成更克制的时间兜底:
	// 仅在 3 分钟无 verify 时才走时间门, 平时主要由 iter 门 / token 门掌控节奏.
	// 关键词: verificationAutoTriggerMaxSnapshotAge 180s, 时间门进一步放宽,
	//        iter+冷静期主导节流
	verificationAutoTriggerMaxSnapshotAge = 180 * time.Second

	// verificationAutoTriggerMinPromptDelta 控制软 token 门 (加速器门) 的触发阈值.
	// 保持 1500, 但语义变化: 仅当 iter 差 >= verificationTokenGateMinIterCooldown
	// 时这个门才生效. 数据爆炸阶段单 iter token 突增也不再立刻打断 iter 节拍,
	// 必须等冷静期过完软门才能 fire, 避免 token 门反复打断 iter 基础节拍.
	// 关键词: verificationAutoTriggerMinPromptDelta 1500 软门, 受冷静期约束,
	//        token 门反复打断 iter 节拍修复
	verificationAutoTriggerMinPromptDelta = 1500

	// verificationIterationTriggerInterval 控制 iter 门 (每 N 轮强制 verify 兜底).
	// 引用 aicommon.DefaultPeriodicVerificationInterval (已调整为 6), 作为
	// 时间/token 门都不触发时的基础节拍兜底.
	// 关键词: verificationIterationTriggerInterval iter 门基础节拍, 跟随 6
	verificationIterationTriggerInterval = aicommon.DefaultPeriodicVerificationInterval

	// verificationTokenGateMinIterCooldown 控制软 token 门的"冷静期":
	// 当上次 verification 与当前 iter 差 < cooldown 时, 即使软 token 门触发
	// 也不允许 fire (除非达到硬 token 门). 这是为了修复 "数据爆炸阶段
	// token 门反复打断 iter 基础节拍" 的尖峰问题. iter 门 / 时间门 / 末轮
	// 兜底 / 硬 token 门不受冷静期影响.
	// 取值 3: 与 iter 门 6 形成 "1:2" 节拍 (走完基础节拍一半后才允许加速器),
	// 既保留对数据爆炸的响应性又避免每 iter 都触发.
	// 关键词: verificationTokenGateMinIterCooldown 3, token 门冷静期,
	//        iter 节拍优先, 数据爆炸场景修复
	verificationTokenGateMinIterCooldown = 3

	// verificationAutoTriggerHardPromptDelta 是 token 门的"硬阈值"上界:
	// 当单次 prompt token 增量 >= 此值时, 视为单次超大数据爆炸 (例如
	// 一次性抓回 5000+ token 的 HTML / 大文件), 此时即使在冷静期内也
	// 立即 fire, 避免错过关键 verify 时机. 取值 5000 (~ 软门 1500 的 3.3 倍),
	// 既能覆盖单 iter 大爆炸又不会被中等 token 增量误触.
	// 关键词: verificationAutoTriggerHardPromptDelta 5000, 硬门兜底,
	//        单次爆炸豁免冷静期
	verificationAutoTriggerHardPromptDelta = 5000

	// verificationFirstFireIterationThreshold 控制 baseline 未建立时的"首次提前触发"门:
	// 当 previous == nil (从未 verify 过) 且 current iter >= 此阈值时立即 fire,
	// 让 AI 在初期就能拿到一次反馈建立 baseline, 而非等到 iter 门 (6) 才触发.
	// 取值 3: iter=1 通常只是 perception 起步, iter=2 是首次工具反馈, iter=3
	// 已积累 2-3 次工具调用结果, 足以建立有意义的 baseline; 同时与冷静期 3
	// 数值对齐, 减少认知负担.
	// 关键词: verificationFirstFireIterationThreshold 3, 首次提前触发,
	//        baseline 早期建立, AI 早期校准
	verificationFirstFireIterationThreshold = 3
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
// 清零语义 (与 VerifyUserSatisfactionNow 显式路径对齐):
//   - 触发 fire 前的 currentSnapshot 仅用于"门判断", 不再作为新基线落盘
//   - fire 完成后, 用 fire 结束时刻的实时 snapshot 替换 prev (时间 / iter /
//     token 三维度统一清零), 让多门交叉触发后下一轮 verification 节奏
//     稳定公平, 不会被 AI 调用耗时白送给时间门
//
// 关键词: MaybeVerifyUserSatisfaction watchdog 解锁, verificationInFlight CAS,
//
//	verificationMutex 缩窄作用域, fire 完成后清零基线统一,
//	时间门 iter 门冷静期同步清零
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
	// fire 完成后, 用 fire 结束时刻的实时 snapshot 替换 prev, 而不是 fire
	// 开始前计算的 currentSnapshot. 这样 prev.GeneratedAt / prev.IterationIndex
	// / prev.LoopPromptTokens 三个维度都以 fire 完成时刻为新基线, 让时间门
	// (180s) / iter 门 (6) / 冷静期 (3 iter) 下次判断都从"上次 verify 真正
	// 结束"那一刻起算, 而不是被 AI 调用耗时 (常 5-30s) 白送给时间门.
	//
	// 修复前问题: 自动路径用 fire 开始前的 currentSnapshot, 显式路径
	// (VerifyUserSatisfactionNow) 用 fire 结束后的 buildVerificationRuntimeSnapshot
	// (time.Now()), 两条路径"清零"语义不一致. 在多门交叉触发场景下, 自动
	// 路径会让 prev.GeneratedAt 比真实 fire 完成时间早 AI 调用耗时那么多,
	// 进而让下一轮时间门 (180s) 比期望提前到位, 各门接力交叉触发, verify
	// 频率不公平地被推高.
	//
	// 修复后: 两条路径统一以"fire 结束时刻"为新基线, 任意一个门触发后,
	// 时间/iter/token 三个维度都被同步、及时地清零, 多门交叉触发后下一
	// 轮 verification 节奏稳定可预期.
	//
	// 注意: 主循环 fire 期间是同步阻塞的, currentIterationIndex 与
	// LoopPromptTokens 不会变化, 所以这两个字段的值与 currentSnapshot
	// 一致; 唯一变化的是 GeneratedAt (从 fire 开始时间 -> fire 完成时间).
	// 关键词: setVerificationRuntimeSnapshot fire 完成后基线, 交叉触发节流公平,
	//        自动路径与显式路径一致, 时间门 iter 门冷静期统一清零,
	//        AI 调用耗时不再白送时间门, 多门交叉触发节奏修复
	r.setVerificationRuntimeSnapshot(r.buildVerificationRuntimeSnapshot(time.Now()))
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

// shouldTriggerAutomaticVerification 决定本轮是否需要发起一次自动 verification.
// 5 个门按角色分层 (与原 OR 关系不同, 新版有优先级 + 冷静期):
//
//	1) 末轮兜底: iter == maxIterations 必触发, 保证最终一定有一次 verify.
//	2) 首次提前门: baseline 未建立 (previous == nil) 且当前 iter
//	   >= verificationFirstFireIterationThreshold 时立即 fire, 让 AI
//	   能尽早拿到一次反馈建立 baseline, 避免错误方向跑满 iter 门才被纠正.
//	3) 基础节拍 (时间门): now - prevGeneratedAt >= verificationAutoTriggerMaxSnapshotAge,
//	   保证长时间无 verify 不会被遗忘.
//	4) 基础节拍 (iter 门): iter 差 >= periodicVerificationInterval, 默认 6 轮一兜底.
//	5) 硬兜底 (硬 token 门): 单次 token 增量 >= verificationAutoTriggerHardPromptDelta
//	   立即 fire, 不被冷静期压制, 用于单次超大爆炸场景.
//	6) 加速器 (软 token 门): 仅当 iter 差 >= verificationTokenGateMinIterCooldown 时,
//	   token 差 >= verificationAutoTriggerMinPromptDelta 才允许 fire. 冷静期内即使
//	   软门触发也不 fire, 避免数据爆炸阶段反复打断 iter 基础节拍.
//
// 关键词: shouldTriggerAutomaticVerification 节流分层, 首次提前门,
//
//	token 门冷静期, 硬门豁免, iter 基础节拍优先
func (r *ReActLoop) shouldTriggerAutomaticVerification(current *VerificationRuntimeSnapshot) bool {
	if r == nil || current == nil {
		return false
	}
	// 末轮兜底
	if r.maxIterations > 0 && current.IterationIndex == r.maxIterations {
		return true
	}
	previous := r.GetVerificationRuntimeSnapshot()
	if previous == nil {
		// 当 periodicVerificationInterval <= 0 时表示 "禁用节流, 每次 iter
		// 都 fire" 的测试/调试模式 (语义与 ShouldTriggerPeriodicCheckpointOnIteration
		// 保持一致), 直接 fire 兼容旧行为, 不走首次提前门阈值.
		// 关键词: periodicVerificationInterval 0 退化为每次 fire, 测试兼容
		if r.periodicVerificationInterval <= 0 {
			return true
		}
		// 首次提前门: baseline 未建立时, iter >= 阈值 (3) 即 fire,
		// 让 AI 早期校准方向. 不再等到 iter 门 (6) 才首次 verify.
		return current.IterationIndex >= verificationFirstFireIterationThreshold
	}
	// 基础节拍: 时间门 (180s)
	if current.GeneratedAt.Sub(previous.GeneratedAt) >= verificationAutoTriggerMaxSnapshotAge {
		return true
	}
	// 基础节拍: iter 门 (6)
	iterDelta := current.IterationIndex - previous.IterationIndex
	if iterDelta >= r.getVerificationIterationTriggerInterval() {
		return true
	}
	// 硬兜底: 单次超大爆炸豁免冷静期 (>= 5000 tokens)
	tokenDelta := verificationPromptTokenDelta(previous, current)
	if tokenDelta >= verificationAutoTriggerHardPromptDelta {
		return true
	}
	// 加速器: 冷静期 (< 3 iter) 内抑制软 token 门,
	// 修复数据爆炸阶段每个工具调用都 verify 的尖峰问题.
	if iterDelta < verificationTokenGateMinIterCooldown {
		return false
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

func (r *ReActLoop) getVerificationIterationTriggerInterval() int {
	if r == nil {
		return verificationIterationTriggerInterval
	}
	return r.periodicVerificationInterval
}
