package reactloops

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// SpinDetectionResult 表示 SPIN 检测的结果
type SpinDetectionResult struct {
	IsSpinning       bool     `json:"is_spinning"`
	Reason           string   `json:"reason"`
	Suggestions      []string `json:"suggestions"`
	NextActions      []string `json:"next_actions"`
	ActionType       string   `json:"action_type"`
	ConsecutiveCount int      `json:"consecutive_count"`
}

// IsInSpin 综合判断是否发生了 SPIN 情况
// 首先检查简单检测（IsInSameActionTypeSpin），如果触发则进一步使用 AI 检测
func (r *ReActLoop) IsInSpin() (bool, *SpinDetectionResult) {
	// 首先进行简单的 Action 类型检测
	if r.IsInSameActionTypeSpin() {
		// 如果简单检测触发，进行 AI 深度检测
		result, err := r.IsInSameLogicSpinWithAI()
		if err != nil {
			log.Warnf("AI spin detection failed: %v, fallback to simple detection", err)
			// AI 检测失败时，返回简单检测结果
			history := r.GetLastNAction(r.sameActionTypeSpinThreshold)
			if len(history) >= r.sameActionTypeSpinThreshold {
				lastAction := history[len(history)-1]
				return true, &SpinDetectionResult{
					IsSpinning:       true,
					Reason:           fmt.Sprintf("连续 %d 次执行相同的 Action 类型: %s", len(history), lastAction.ActionType),
					Suggestions:      []string{"尝试使用不同的 Action 类型", "检查任务目标是否明确", "考虑是否需要用户澄清"},
					NextActions:      []string{"尝试不同的策略", "重新评估任务目标"},
					ActionType:       lastAction.ActionType,
					ConsecutiveCount: len(history),
				}
			}
		}
		if result != nil && result.IsSpinning {
			return true, result
		}
		// 如果 AI 检测认为不是 SPIN，但简单检测触发了，仍然返回简单检测结果
		history := r.GetLastNAction(r.sameActionTypeSpinThreshold)
		if len(history) >= r.sameActionTypeSpinThreshold {
			lastAction := history[len(history)-1]
			return true, &SpinDetectionResult{
				IsSpinning:       true,
				Reason:           fmt.Sprintf("连续 %d 次执行相同的 Action 类型: %s", len(history), lastAction.ActionType),
				Suggestions:      []string{"尝试使用不同的 Action 类型", "检查任务目标是否明确"},
				ActionType:       lastAction.ActionType,
				ConsecutiveCount: len(history),
			}
		}
	}
	return false, nil
}

// IsInSameActionTypeSpin 检测是否连续 N 次执行了相同的 Action 类型
// 这是一个低成本的检测方法，不需要 AI 参与
func (r *ReActLoop) IsInSameActionTypeSpin() bool {
	r.actionHistoryMutex.Lock()
	defer r.actionHistoryMutex.Unlock()

	threshold := r.sameActionTypeSpinThreshold
	if threshold <= 0 {
		threshold = 3 // 默认阈值
	}

	historyLen := len(r.actionHistory)
	if historyLen < threshold {
		return false
	}

	// 检查最近 threshold 次是否都是相同的 Action 类型
	lastActionType := r.actionHistory[historyLen-1].ActionType
	for i := historyLen - threshold; i < historyLen; i++ {
		if r.actionHistory[i].ActionType != lastActionType {
			return false
		}
	}

	log.Infof("detected same action type spin: %d consecutive actions of type %s", threshold, lastActionType)
	return true
}

// IsInSameLogicSpinWithAI 使用 AI 进行深度 SPIN 检测
// 分析 Action 参数、Timeline 和上下文，判断是否发生了逻辑层面的 SPIN
func (r *ReActLoop) IsInSameLogicSpinWithAI() (*SpinDetectionResult, error) {
	r.actionHistoryMutex.Lock()
	historyLen := len(r.actionHistory)
	if historyLen < r.sameLogicSpinThreshold {
		r.actionHistoryMutex.Unlock()
		return &SpinDetectionResult{IsSpinning: false}, nil
	}

	// 获取最近的 action 记录用于分析
	recentActions := make([]*ActionRecord, r.sameLogicSpinThreshold)
	copy(recentActions, r.actionHistory[historyLen-r.sameLogicSpinThreshold:])
	r.actionHistoryMutex.Unlock()

	// 检查是否都是相同的 Action 类型
	firstActionType := recentActions[0].ActionType
	allSameType := true
	for _, action := range recentActions {
		if action.ActionType != firstActionType {
			allSameType = false
			break
		}
	}

	if !allSameType {
		// 如果不是相同类型，不进行 AI 检测
		return &SpinDetectionResult{IsSpinning: false}, nil
	}

	// 获取 Timeline 信息
	timelineContent := r.getTimelineContentForSpinDetection()

	// 构建 AI 检测的 prompt
	prompt := r.buildSpinDetectionPrompt(recentActions, timelineContent)

	// 调用 InvokeLiteForge 进行 AI 检测
	ctx := r.GetConfig().GetContext()
	if r.GetCurrentTask() != nil {
		ctx = r.GetCurrentTask().GetContext()
	}

	// 定义输出 schema
	outputSchema := []aitool.ToolOption{
		aitool.WithBoolParam("is_spinning",
			aitool.WithParam_Description("是否发生了 SPIN 情况（AI Agent 反复做出相同或相似的决策，没有推进任务）"),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam("reason",
			aitool.WithParam_Description("如果发生了 SPIN，说明发生 SPIN 的原因"),
			aitool.WithParam_Required(false),
		),
		aitool.WithStringArrayParam("suggestions",
			aitool.WithParam_Description("如果发生了 SPIN，提供足够多的建议或下一步操作建议，帮助 AI 继续执行任务"),
			aitool.WithParam_Required(false),
		),
		aitool.WithStringArrayParam("next_actions",
			aitool.WithParam_Description("如果发生了 SPIN，提供具体的下一步操作建议（Action 类型或策略）"),
			aitool.WithParam_Required(false),
		),
	}

	action, err := r.invoker.InvokeLiteForge(ctx, "spin_detection", prompt, outputSchema)
	if err != nil {
		return nil, utils.Wrap(err, "invoke liteforge for spin detection failed")
	}

	if utils.IsNil(action) {
		return nil, utils.Error("spin detection action is nil")
	}

	// 解析结果
	result := &SpinDetectionResult{
		ActionType:       firstActionType,
		ConsecutiveCount: len(recentActions),
	}

	params := action.GetParams()
	result.IsSpinning = params.GetBool("is_spinning")
	if result.IsSpinning {
		result.Reason = params.GetString("reason")
		result.Suggestions = params.GetStringSlice("suggestions")
		result.NextActions = params.GetStringSlice("next_actions")
	}

	return result, nil
}

// getTimelineContentForSpinDetection 获取用于 SPIN 检测的 Timeline 内容
func (r *ReActLoop) getTimelineContentForSpinDetection() string {
	config := r.GetConfig()
	if config == nil {
		return ""
	}

	// 尝试通过类型断言获取 Timeline
	// Config 结构体实现了 GetTimeline 方法，但接口中没有定义
	// 这里我们尝试类型断言，如果失败则返回空字符串
	var timeline *aicommon.Timeline
	if cfg, ok := config.(interface{ GetTimeline() *aicommon.Timeline }); ok {
		timeline = cfg.GetTimeline()
	} else {
		return ""
	}

	if timeline == nil {
		return ""
	}

	// 获取最近的 Timeline 条目（限制数量以避免过长）
	// 获取最近 20 条 Timeline 条目用于分析
	outputs := timeline.ToTimelineItemOutputLastN(20)
	if len(outputs) == 0 {
		return ""
	}

	var content strings.Builder
	content.WriteString("最近的 Timeline 条目：\n\n")
	for i, output := range outputs {
		content.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, output.Type, output.Content))
		if i >= 19 { // 限制最多 20 条
			break
		}
	}

	return content.String()
}

// buildSpinDetectionPrompt 构建 SPIN 检测的 prompt
func (r *ReActLoop) buildSpinDetectionPrompt(actions []*ActionRecord, timelineContent string) string {
	var prompt strings.Builder

	prompt.WriteString("你是一个 AI Agent 行为分析专家。请分析以下 Action 执行历史，判断是否发生了 SPIN 情况。\n\n")
	prompt.WriteString("SPIN 的定义：AI Agent 反复做出相同或相似的决策，没有让任务得到推进。\n\n")

	prompt.WriteString("## Action 执行历史\n\n")
	for i, action := range actions {
		prompt.WriteString(fmt.Sprintf("### 第 %d 次执行 (迭代 %d)\n", i+1, action.IterationIndex))
		prompt.WriteString(fmt.Sprintf("- Action 类型: %s\n", action.ActionType))
		prompt.WriteString(fmt.Sprintf("- Action 名称: %s\n", action.ActionName))
		prompt.WriteString("- Action 参数:\n")
		paramsJSON, err := json.MarshalIndent(action.ActionParams, "  ", "  ")
		if err == nil {
			prompt.WriteString("  ```json\n")
			prompt.WriteString("  " + string(paramsJSON) + "\n")
			prompt.WriteString("  ```\n")
		}
		prompt.WriteString("\n")
	}

	if timelineContent != "" {
		prompt.WriteString("## Timeline 上下文\n\n")
		prompt.WriteString(timelineContent)
		prompt.WriteString("\n\n")
	}

	prompt.WriteString("## 分析要求\n\n")
	prompt.WriteString("1. 判断这些 Action 是否在重复执行相同的逻辑，没有推进任务\n")
	prompt.WriteString("2. 如果发生了 SPIN，请详细说明原因\n")
	prompt.WriteString("3. 如果发生了 SPIN，请提供足够多的建议和下一步操作，帮助 AI Agent 打破循环\n")
	prompt.WriteString("4. 建议应该具体、可操作，包括可能的 Action 类型或策略\n\n")

	prompt.WriteString("请以 JSON 格式返回分析结果。")

	return prompt.String()
}
