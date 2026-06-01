package reactloops

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// ReflectionLevel 定义反思的深度级别
type ReflectionLevel int

const (
	// ReflectionLevel_None 不进行反思
	ReflectionLevel_None ReflectionLevel = iota
	// ReflectionLevel_Minimal 最小反思：仅记录执行结果
	ReflectionLevel_Minimal
	// ReflectionLevel_Standard 标准反思：评估基本影响
	ReflectionLevel_Standard
	// ReflectionLevel_Deep 深度反思：详细分析环境变化和影响
	ReflectionLevel_Deep
	// ReflectionLevel_Critical 关键反思：失败场景的深度根因分析
	ReflectionLevel_Critical
)

// String returns the string representation of ReflectionLevel
func (r ReflectionLevel) String() string {
	switch r {
	case ReflectionLevel_None:
		return "none"
	case ReflectionLevel_Minimal:
		return "minimal"
	case ReflectionLevel_Standard:
		return "standard"
	case ReflectionLevel_Deep:
		return "deep"
	case ReflectionLevel_Critical:
		return "critical"
	default:
		return "unknown"
	}
}

// shouldTriggerReflection 决定是否需要触发反思以及反思级别.
//
// 重构后语义(降噪 + 收紧):
//   - 未启用自我反思 -> None
//   - operator 显式设置了 ReflectionLevel -> 按 operator 透传(action 自定义优先级)
//   - 执行失败(isTerminated && err != nil) -> Critical(同步,失败必须立即归因)
//   - 其它情形: 只有 iteration > 5 且 SPIN 双维度命中时才返回 Standard;
//     否则一律 None — 不再返回 Minimal, 也不再为 directly_answer/finish
//     额外注入反思. 这样未触发 SPIN 的常规执行流不会产生任何反思事件 /
//     timeline 噪声, 也不会调用 AI.
//
// 关键词: shouldTriggerReflection 重构, 只在 SPIN 触发反思, 去 Minimal 噪声
func (r *ReActLoop) shouldTriggerReflection(
	action *LoopAction,
	operator *LoopActionHandlerOperator,
	iterationCount int,
) ReflectionLevel {
	if !r.enableSelfReflection {
		return ReflectionLevel_None
	}

	// action 通过 operator 显式声明的反思级别优先(用于自定义 action 的关键反思需求)
	operatorLevel := operator.GetReflectionLevel()
	if operatorLevel != ReflectionLevel_None {
		log.Infof("use action-defined reflection level: %s", operatorLevel.String())
		return operatorLevel
	}

	// 执行失败 -> 触发关键反思(同步, 用于失败归因, 必须在下一轮 prompt 前到位)
	isTerminated, err := operator.IsTerminated()
	if isTerminated && err != nil {
		log.Infof("action[%s] failed, trigger critical reflection", action.ActionType)
		return ReflectionLevel_Critical
	}

	// 其它情形: 只在 iteration > 5 且检测到 SPIN 时反思. SPIN 阈值已经提到 8
	// (双维度判定), 实际触发非常稀疏 — 这正是我们想要的 "不干扰常规执行流"
	if iterationCount > 5 && r.IsInSameActionTypeSpin() {
		log.Infof("SPIN detected at iteration[%d], trigger standard reflection (async)", iterationCount)
		return ReflectionLevel_Standard
	}

	return ReflectionLevel_None
}

// MaybeExecuteReflection 是 exec.go 主循环的统一反思入口.
//
// 调度策略:
//   - Critical 级别(失败归因) -> 同步阻塞执行, 因为下一轮 prompt 必须包含
//     失败上下文才能让 AI 做合理决策.
//   - 其它级别(主要是 SPIN 触发的 Standard) -> fire-and-forget 异步执行,
//     主循环不等待结果, 完成后通过 invoker.AddToTimeline 让下一轮 prompt
//     自然吸收 SPIN warning. 这样反思 AI 调用不会卡住主循环执行流, 用户也
//     不会觉得"卡了".
//
// 关键词: MaybeExecuteReflection, 反思异步化, fire-and-forget,
//
//	Critical 同步 / Standard 异步, SPIN 不干扰执行
func (r *ReActLoop) MaybeExecuteReflection(
	action *LoopAction,
	actionParams *aicommon.Action,
	operator *LoopActionHandlerOperator,
	reflectionLevel ReflectionLevel,
	iterationCount int,
	executionDuration time.Duration,
) {
	if reflectionLevel == ReflectionLevel_None {
		return
	}

	// 失败归因: 必须同步, 让下一轮 prompt 立即看到失败反思
	if reflectionLevel == ReflectionLevel_Critical {
		r.executeReflection(action, actionParams, operator, reflectionLevel, iterationCount, executionDuration)
		return
	}

	// 其它级别(SPIN/Standard): 异步执行, 主循环立即返回
	r.reflectionInflight.Add(1)
	go func() {
		defer r.reflectionInflight.Done()
		defer func() {
			if rec := recover(); rec != nil {
				log.Errorf("async reflection panic recovered for action[%s]: %v",
					action.ActionType, rec)
			}
		}()
		r.executeReflection(action, actionParams, operator, reflectionLevel, iterationCount, executionDuration)
	}()
}

// executeReflection 执行一次反思流程(同步). 由 MaybeExecuteReflection 决定
// 在主线程同步调用还是 goroutine 异步调用.
//
// 注: 调用方持有的 operator / actionParams 在 action handler 执行后即不再
// 被主循环写入, 异步路径里安全使用. 关键词: 反思执行流, 同步实现, 不可单独
// 在主循环里用作 fire-and-forget — 走 MaybeExecuteReflection 入口.
func (r *ReActLoop) executeReflection(
	action *LoopAction,
	actionParams *aicommon.Action,
	operator *LoopActionHandlerOperator,
	reflectionLevel ReflectionLevel,
	iterationCount int,
	executionDuration time.Duration,
) {
	if reflectionLevel == ReflectionLevel_None {
		return
	}

	ctx := r.GetConfig().GetContext()
	if !utils.IsNil(operator.GetTask()) {
		ctx = operator.GetTask().GetContext()
	}

	// 检查上下文是否已取消
	select {
	case <-ctx.Done():
		log.Warn("context canceled, skip reflection execution")
		return
	default:
	}

	log.Infof("start to execute reflection for action[%s] with level[%s]", action.ActionType, reflectionLevel.String())

	// 收集反思数据
	reflection := r.collectReflectionData(
		action,
		actionParams,
		operator,
		reflectionLevel,
		iterationCount,
		executionDuration,
	)

	// 根据反思级别决定是否需要 AI 分析
	// SPIN 检测已整合到 AI 反思中，不再单独执行
	if reflectionLevel >= ReflectionLevel_Standard {
		r.performAIReflection(ctx, reflection, reflectionLevel)
	}

	// 核心：将反思添加到 Timeline（使用强语气）
	// Timeline 的 diff 会自动触发记忆系统生成记忆
	r.addReflectionToTimeline(reflection)

	// 缓存反思结果供 prompt 使用
	r.cacheReflection(reflection)

	log.Infof("reflection execution completed for action[%s] at level[%s]",
		action.ActionType, reflectionLevel.String())
}

// collectReflectionData 收集反思所需的基本数据
func (r *ReActLoop) collectReflectionData(
	action *LoopAction,
	actionParams *aicommon.Action,
	operator *LoopActionHandlerOperator,
	reflectionLevel ReflectionLevel,
	iterationCount int,
	executionDuration time.Duration,
) *ActionReflection {
	isTerminated, err := operator.IsTerminated()
	success := !(isTerminated && err != nil)

	// 合并基本参数和自定义反思数据
	allParams := make(map[string]interface{})
	for k, v := range actionParams.GetParams() {
		allParams[k] = v
	}
	customData := operator.GetReflectionData()
	for k, v := range customData {
		allParams[k] = v
	}

	reflection := &ActionReflection{
		ActionType:          action.ActionType,
		ActionParams:        allParams,
		ToolName:            extractToolNameFromAction(actionParams),
		ExecutionTime:       executionDuration,
		IterationNum:        iterationCount,
		Success:             success,
		ReflectionLevel:     reflectionLevel.String(),
		ReflectionTimestamp: time.Now(),
	}

	if err != nil {
		reflection.ErrorMessage = err.Error()
	}

	// 对于标准及以上级别，收集环境影响数据
	if reflectionLevel >= ReflectionLevel_Standard {
		reflection.EnvironmentalImpact = r.analyzeEnvironmentalImpact(action, operator, success)
	}

	return reflection
}

// analyzeEnvironmentalImpact 分析环境影响
func (r *ReActLoop) analyzeEnvironmentalImpact(
	action *LoopAction,
	operator *LoopActionHandlerOperator,
	success bool,
) *EnvironmentalImpact {
	impact := &EnvironmentalImpact{
		StateChanges:    []string{},
		ResourceUsage:   make(map[string]interface{}),
		SideEffects:     []string{},
		PositiveEffects: []string{},
		NegativeEffects: []string{},
	}

	// 记录状态变化
	if operator.IsContinued() {
		impact.StateChanges = append(impact.StateChanges, "action_continued")
	}
	isTerminated, _ := operator.IsTerminated()
	if isTerminated {
		impact.StateChanges = append(impact.StateChanges, "loop_terminated")
	}

	// 记录正面/负面影响
	if success {
		impact.PositiveEffects = append(impact.PositiveEffects, fmt.Sprintf("action[%s] executed successfully", action.ActionType))
	} else {
		impact.NegativeEffects = append(impact.NegativeEffects, fmt.Sprintf("action[%s] execution failed", action.ActionType))
	}

	// 检查是否有反馈信息
	feedback := operator.GetFeedback()
	if feedback != nil && feedback.Len() > 0 {
		impact.SideEffects = append(impact.SideEffects, fmt.Sprintf("feedback_generated: %d bytes", feedback.Len()))
	}

	return impact
}

// performAIReflection 使用 AI 进行深度反思分析，集成记忆系统
func (r *ReActLoop) performAIReflection(ctx context.Context, reflection *ActionReflection, reflectionLevel ReflectionLevel) {
	log.Infof("start AI reflection for action[%s] at level[%s]", reflection.ActionType, reflectionLevel.String())

	emitter := r.GetEmitter()
	config := r.GetConfig()
	task := r.GetCurrentTask()
	nonce := utils.RandStringBytes(4)

	// 反思 prompt 构建: 复用主循环 prefix cache (high-static / frozen-block /
	// timeline-open), dynamic 段只放反思特有内容 (action 详情 + SPIN + schema).
	// 去掉 RelevantMemories / PreviousReflections — 它们是 cache 杀手, 且 SPIN
	// 决策的真正依据已经在 timeline-open + action 历史里覆盖.
	// 关键词: 反思 prompt 复用 prefix cache, 去内存噪声
	prompt, err := r.buildReflectionPrompt(reflection, nonce)
	if err != nil {
		log.Errorf("failed to build reflection prompt: %v", err)
		return
	}

	log.Infof("reflection prompt built with nonce[%s], prompt_bytes[%d]",
		nonce, len(prompt))

	// 第三步：使用 CallAITransaction 进行稳定的 AI 调用
	// 同步反思 AI 调用 post-action 卡死兜底: 给反思流套一层 idle-timeout
	// reader, 与 verification 共享同一 feature flag / 默认阈值. 关闭时退化
	// 为纯计时观测 (P0 埋点), 始终输出 [REFLECTION_AI_TIMING] 结构化日志.
	// 关键词: performAIReflection 流空闲超时, P0 埋点, P1 兜底
	reflectionTTFB, reflectionIdle := aicommon.ResolveAIStreamIdleThresholds(config)
	var reflectionTimedOut atomic.Bool

	err = aicommon.CallAITransaction(
		config,
		prompt,
		config.CallSpeedPriorityAI,
		func(rsp *aicommon.AIResponse) error {
			reflectionTimedOut.Store(false)
			boundEmitter := rsp.BindEmitter(emitter)
			rawStream := rsp.GetOutputStreamReader(
				"self-reflection",
				true,
				emitter,
			)
			idleReader := aicommon.NewStreamIdleTimeoutReader(rawStream, reflectionTTFB, reflectionIdle)
			defer func() {
				snap := idleReader.Snapshot()
				aicommon.LogStreamTimingSnapshot("REFLECTION_AI_TIMING", snap)
				if snap.TimedOut {
					reflectionTimedOut.Store(true)
				}
				_ = idleReader.Close()
			}()
			stream := io.Reader(idleReader)

			// 构建 action 提取选项，让 action 自动处理字段流式输出
			actionOptions := []aicommon.ActionMakerOption{
				aicommon.WithActionNonce(nonce),
				aicommon.WithActionAlias("self_reflection"),
			}

			// 注册字段流式处理器 - action 会自动拆解这些字段.
			// 节点 id 改为独立的 "self-reflection-suggestions" (而非沿用主循环
			// 的 "thought"), 让前端可以单独识别/折叠/i18n 自我反思内容, 也
			// 不会污染主循环思考流的 UI 节点.
			// 前缀文案统一为中文 "自我反思:", 与 i18n nodeId 文案保持一致.
			// 关键词: 自我反思流节点 id, self-reflection-suggestions, 中文前缀
			if !utils.IsNil(task) {
				actionOptions = append(actionOptions, aicommon.WithActionFieldStreamHandler(
					[]string{"suggestions"},
					func(key string, reader io.Reader) {

						r.loadingStatus("自我反思中 / Self-Reflection...")
						raw, err := io.ReadAll(reader)
						if err != nil {
							log.Warnf("failed to read suggestions stream: %v", err)
							return
						}
						var sgs = make([]string, 0)
						json.Unmarshal(raw, &sgs)
						for _, i := range sgs {
							pr, pw := utils.NewPipe()
							boundEmitter.EmitDefaultStreamEvent(
								"self-reflection-suggestions",
								pr,
								task.GetId(),
							)
							pw.WriteString("- 自我反思: " + i + "\n")
							pw.Close()
						}
					},
				))
			}

			// 从流中提取结构化的反思结果 - action 会自动解析 JSON 字段
			action, actionErr := aicommon.ExtractActionFromStream(
				ctx,
				stream,
				"self_reflection",
				actionOptions...,
			)

			if actionErr != nil {
				log.Warnf("failed to extract reflection action: %v", actionErr)
				return actionErr
			}

			if utils.IsNil(action) {
				return utils.Error("reflection action is nil")
			}

			// 等待流和解析完成
			action.WaitParse(ctx)
			action.WaitStream(ctx)

			// action 会自动将字段 set 到 params 中，直接读取即可
			r.fillReflectionFromAction(action, reflection)

			log.Infof("AI reflection parsed: suggestions[%d]",
				len(reflection.Suggestions))

			return nil
		},
		aicommon.WithAIRequest_CallerLabel("reflection"),
	)

	if err != nil {
		log.Warnf("failed to perform AI reflection transaction: %v", err)
		// P1 兜底: AI 反思流卡死且重试用光时, 仅写一条 [REFLECTION_TIMEOUT]
		// timeline 痕迹, 不阻塞主循环. 反思本身是 fire-and-forget, 失败
		// 不应升级为致命错误. 关键词: performAIReflection 流空闲降级,
		// [REFLECTION_TIMEOUT]
		if reflectionTimedOut.Load() {
			r.GetInvoker().AddToTimeline("[REFLECTION_TIMEOUT]", fmt.Sprintf(
				"AI reflection for action[%s] hit stream idle timeout (ttfb=%v idle=%v); skipped, loop continues without spin warning escalation",
				reflection.ActionType, reflectionTTFB, reflectionIdle,
			))
		}
		return
	}

	log.Infof("AI reflection completed successfully for action[%s]", reflection.ActionType)
}

// fillReflectionFromAction 从 action 中填充反思结果
// action 会自动解析 JSON 字段，我们直接读取即可
func (r *ReActLoop) fillReflectionFromAction(action *aicommon.Action, reflection *ActionReflection) {
	if utils.IsNil(action) {
		log.Warn("action is nil, cannot fill reflection")
		return
	}

	params := action.GetParams()

	// action 自动解析了 suggestions 数组 (直接作为字符串数组)
	reflection.Suggestions = params.GetStringSlice("suggestions")

	// 处理任务推进判断：如果 AI 明确认为任务正常推进，优先清零 spin 计数
	reflection.IsTaskProgressing = params.GetBool("is_task_progressing")
	if reflection.IsTaskProgressing {
		r.ResetSpinWarning()
		log.Infof("task is progressing normally (different params/targets), spin warning counter reset")
	}

	// 处理 SPIN 检测结果（整合到自我反思中）
	// 若 is_task_progressing 已为 true，则忽略 is_spinning（任务有进展优先）
	reflection.IsSpinning = params.GetBool("is_spinning")
	if reflection.IsSpinning && !reflection.IsTaskProgressing {
		reflection.SpinReason = params.GetString("spin_reason")

		r.addSpinWarningToTimeline(reflection)
		r.IncrementSpinWarning()
	} else if !reflection.IsTaskProgressing {
		r.ResetSpinWarning()
	}

	log.Infof("filled reflection from action: suggestions[%d], spinning[%v], task_progressing[%v]",
		len(reflection.Suggestions), reflection.IsSpinning, reflection.IsTaskProgressing)
}

// addSpinWarningToTimeline 将 SPIN 警告添加到 Timeline
// 根据 consecutiveSpinWarnings 计数逐级引入结构化方法论施加认知压力
func (r *ReActLoop) addSpinWarningToTimeline(reflection *ActionReflection) {
	if !reflection.IsSpinning {
		return
	}

	spinCount := r.consecutiveSpinWarnings + 1 // +1 because IncrementSpinWarning is called after this
	log.Warnf("SPIN detected (consecutive #%d): %s", spinCount, reflection.SpinReason)

	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("[SPIN DETECTED #%d] AI Agent is stuck in a loop\n\n", spinCount))
	msg.WriteString(fmt.Sprintf("**Action type**: %s\n", reflection.ActionType))
	msg.WriteString(fmt.Sprintf("**Reason**: %s\n\n", reflection.SpinReason))

	if len(reflection.Suggestions) > 0 {
		msg.WriteString("**Suggestions**:\n")
		for i, suggestion := range reflection.Suggestions {
			msg.WriteString(fmt.Sprintf("%d. %s\n", i+1, suggestion))
		}
		msg.WriteString("\n")
	}

	// Escalating methodological pressure based on consecutive spin count
	switch {
	case spinCount >= 3:
		msg.WriteString(buildSWOTFramework(reflection.ActionType))
	case spinCount >= 2:
		msg.WriteString(buildSMARTFramework(reflection.ActionType))
	default:
		msg.WriteString(buildFiveWhysFramework(reflection.ActionType))
	}

	if r.maxConsecutiveSpinWarnings > 0 {
		remaining := r.maxConsecutiveSpinWarnings - spinCount
		if remaining <= 1 {
			msg.WriteString(fmt.Sprintf(
				"\n---\n**FINAL WARNING**: This loop will be FORCE-TERMINATED after %d more spin(s). "+
					"You MUST select a DIFFERENT action type NOW.\n", remaining))
		}
	}

	invoker := r.GetInvoker()
	if invoker != nil {
		invoker.AddToTimeline("logic_spin_warning", msg.String())
		log.Infof("SPIN warning #%d added to timeline with escalation framework", spinCount)
	}
}

// buildFiveWhysFramework returns a "5 Whys" root-cause analysis prompt
// Used at spin escalation level 1 to force basic causal reasoning.
func buildFiveWhysFramework(actionType string) string {
	return fmt.Sprintf(`**[MANDATORY] Root-Cause Analysis — 5 Whys**

Before choosing your next action, answer each question in order:
1. **Why** am I repeating '%s'? → identify the immediate trigger
2. **Why** didn't the previous execution advance the task? → identify what's missing
3. **Why** haven't I tried a different action type? → identify the constraint
4. **Why** does this constraint exist? → identify the real blocker
5. **What is the minimum viable DIFFERENT action** that bypasses this blocker?

You MUST pick an action that is NOT '%s' in the next iteration.
`, actionType, actionType)
}

// buildSMARTFramework returns a S.M.A.R.T goal-setting prompt
// Used at spin escalation level 2 to force concrete next-step planning.
func buildSMARTFramework(actionType string) string {
	return fmt.Sprintf(`**[MANDATORY] S.M.A.R.T Next-Step Planning**

You have been spinning on '%s' for 2 consecutive reflections. Define your next action using S.M.A.R.T:

- **S**pecific: What EXACT action (different from '%s') will you take? Name the action type and its parameters.
- **M**easurable: What concrete output proves it succeeded? (e.g. "file content retrieved", "HTTP response received")
- **A**chievable: Is this action available in your tool set? If not, pick one that IS.
- **R**elevant: How does this action DIRECTLY advance the original task goal?
- **T**ime-bound: This must complete in a SINGLE iteration — no multi-step plans.

CONSTRAINT: Action type '%s' is STRONGLY DISCOURAGED. Justify if you must use it again.
`, actionType, actionType, actionType)
}

// buildSWOTFramework returns a SWOT analysis prompt
// Used at spin escalation level 3 (final pressure before force-exit).
func buildSWOTFramework(actionType string) string {
	return fmt.Sprintf(`**[CRITICAL — FINAL ESCALATION] SWOT Analysis of Current Approach**

You have been spinning on '%s' for 3+ consecutive reflections. Perform a SWOT analysis NOW:

**Strengths** — What information/context do you ALREADY possess that doesn't require re-collection?
**Weaknesses** — What specific gap in your reasoning causes the repeat of '%s'? (Be brutally honest)
**Opportunities** — List ALL alternative action types you have NOT tried. Pick one.
**Threats** — If you repeat '%s' one more time, the loop will be force-terminated with UNSUCCESSFUL status.

HARD CONSTRAINT: Action type '%s' is now BANNED.
  → If you select '%s' again, the system will terminate this loop.
  → You MUST select a fundamentally different action type.
  → If no other action applies, use 'finish' or 'directly_answer' to exit gracefully.
`, actionType, actionType, actionType, actionType, actionType)
}

// GetReflectionHistory 获取历史反思记录
func (r *ReActLoop) GetReflectionHistory() []*ActionReflection {
	historyRaw := r.GetVariable("self_reflections")
	if utils.IsNil(historyRaw) {
		return []*ActionReflection{}
	}

	if history, ok := historyRaw.([]*ActionReflection); ok {
		return history
	}

	return []*ActionReflection{}
}
