package reactloops

import (
	"context"
	"fmt"
	"io"
	"strings"
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

// shouldTriggerReflection 决定是否需要触发反思以及反思级别
func (r *ReActLoop) shouldTriggerReflection(
	action *LoopAction,
	operator *LoopActionHandlerOperator,
	iterationCount int,
) ReflectionLevel {
	// 如果未启用自我反思，直接返回 None
	if !r.enableSelfReflection {
		return ReflectionLevel_None
	}

	// 优先使用 action 通过 operator 设置的反思级别
	operatorLevel := operator.GetReflectionLevel()
	if operatorLevel != ReflectionLevel_None {
		log.Infof("use action-defined reflection level: %s", operatorLevel.String())
		return operatorLevel
	}

	// 检查是否执行失败
	isTerminated, err := operator.IsTerminated()
	if isTerminated && err != nil {
		// 失败场景：触发关键反思
		log.Infof("action[%s] failed, trigger critical reflection", action.ActionType)
		return ReflectionLevel_Critical
	}

	// 内置简单动作：最小反思
	if action.ActionType == "directly_answer" || action.ActionType == "finish" {
		return ReflectionLevel_Minimal
	}

	// 高迭代次数：采用间隔反思策略减少噪声
	if iterationCount > 5 {
		// 每 5 轮进行一次标准反思
		if (iterationCount-6)%5 == 0 {
			log.Infof("periodic reflection at iteration[%d] (interval: 5)", iterationCount)
			return ReflectionLevel_Standard
		}
		// 非反思轮次：仅最小反思（不调用 AI，减少噪声）
		log.Debugf("skip AI reflection at iteration[%d], use minimal level", iterationCount)
		return ReflectionLevel_Minimal
	}

	// 默认情况：最小反思
	return ReflectionLevel_Minimal
}

// executeReflection 执行自我反思
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

	// 第一步：根据反思级别搜索相关记忆
	relevantMemories := r.searchRelevantMemories(reflection, reflectionLevel)
	previousReflections := r.getPreviousReflectionsContext(nonce)

	// 第二步：构建反思 prompt（使用模板和 nonce 保护）
	prompt, err := r.buildReflectionPrompt(reflection, nonce, relevantMemories, previousReflections)
	if err != nil {
		log.Errorf("failed to build reflection prompt: %v", err)
		return
	}

	log.Infof("reflection prompt built with nonce[%s], memory_size[%d], prev_reflections[%d]",
		nonce, len(relevantMemories), strings.Count(previousReflections, "##"))

	// 第三步：使用 CallAITransaction 进行稳定的 AI 调用
	err = aicommon.CallAITransaction(
		config,
		prompt,
		config.CallAI,
		func(rsp *aicommon.AIResponse) error {
			// 获取流式输出
			stream := rsp.GetOutputStreamReader(
				"self-reflection",
				true,
				emitter,
			)

			// 构建 action 提取选项，让 action 自动处理字段流式输出
			actionOptions := []aicommon.ActionMakerOption{
				aicommon.WithActionNonce(nonce),
				aicommon.WithActionAlias("self_reflection"),
			}

			// 注册字段流式处理器 - action 会自动拆解这些字段
			if !utils.IsNil(task) {
				actionOptions = append(actionOptions, aicommon.WithActionFieldStreamHandler(
					[]string{"learning_insights", "future_suggestions", "impact_assessment", "effectiveness_rating"},
					func(key string, reader io.Reader) {
						// 流式输出到前端
						nodeId := "self-reflection-" + key
						emitter.EmitStreamEvent(
							nodeId,
							time.Now(),
							reader,
							task.GetIndex(),
							func() {
								log.Debugf("self-reflection field[%s] stream finished", key)
							},
						)
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

			log.Infof("AI reflection parsed: insights[%d], suggestions[%d], assessment[%v]",
				len(reflection.LearningInsights), len(reflection.FutureSuggestions),
				reflection.ImpactAssessment != "")

			return nil
		},
	)

	if err != nil {
		log.Warnf("failed to perform AI reflection transaction: %v", err)
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

	// action 自动解析了 learning_insights 数组 (直接作为字符串数组)
	reflection.LearningInsights = params.GetStringSlice("learning_insights")

	// action 自动解析了 future_suggestions 数组 (直接作为字符串数组)
	reflection.FutureSuggestions = params.GetStringSlice("future_suggestions")

	// action 自动解析了字符串字段
	reflection.ImpactAssessment = params.GetString("impact_assessment")
	reflection.EffectivenessRating = params.GetString("effectiveness_rating")

	log.Infof("filled reflection from action: insights[%d], suggestions[%d]",
		len(reflection.LearningInsights), len(reflection.FutureSuggestions))
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
