package reactloops

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/self_reflection_template.tpl
var selfReflectionTemplate string

// buildReflectionPrompt 构建反思 prompt，使用模板和 nonce 保护
func (r *ReActLoop) buildReflectionPrompt(
	reflection *ActionReflection,
	nonce string,
	relevantMemories string,
	previousReflections string,
) (string, error) {
	// 构建 JSON Schema
	schema := buildReflectionSchema()

	// 准备模板数据
	data := map[string]interface{}{
		"Nonce":         nonce,
		"ActionType":    reflection.ActionType,
		"IterationNum":  reflection.IterationNum,
		"ExecutionTime": reflection.ExecutionTime.String(),
		"ResultStatus": func() string {
			if reflection.Success {
				return "✓ SUCCESS"
			}
			return "✗ FAILED"
		}(),
		"ErrorMessage": reflection.ErrorMessage,
		"Schema":       schema,
	}

	// 添加环境影响
	if reflection.EnvironmentalImpact != nil {
		data["EnvironmentalImpact"] = map[string]interface{}{
			"StateChanges":    strings.Join(reflection.EnvironmentalImpact.StateChanges, ", "),
			"SideEffects":     strings.Join(reflection.EnvironmentalImpact.SideEffects, ", "),
			"PositiveEffects": strings.Join(reflection.EnvironmentalImpact.PositiveEffects, ", "),
			"NegativeEffects": strings.Join(reflection.EnvironmentalImpact.NegativeEffects, ", "),
		}
	}

	// 添加相关记忆
	if relevantMemories != "" {
		data["RelevantMemories"] = relevantMemories
	}

	// 添加之前的反思
	if previousReflections != "" {
		data["PreviousReflections"] = previousReflections
	}

	// 添加 SPIN 检测相关的数据（如果满足条件）
	spinData := r.getSpinDetectionData()
	if spinData != nil {
		data["SpinDetection"] = spinData
	}

	// 使用模板渲染 prompt
	prompt, err := utils.RenderTemplate(selfReflectionTemplate, data)
	if err != nil {
		return "", utils.Wrap(err, "render self-reflection template failed")
	}

	return prompt, nil
}

// getSpinDetectionData 获取 SPIN 检测相关的数据
// 如果满足简单检测条件，返回 action 历史和 timeline 信息
func (r *ReActLoop) getSpinDetectionData() map[string]interface{} {
	// 首先检查是否满足简单检测条件
	if !r.IsInSameActionTypeSpin() {
		return nil
	}

	r.actionHistoryMutex.Lock()
	historyLen := len(r.actionHistory)
	if historyLen < r.sameActionTypeSpinThreshold {
		r.actionHistoryMutex.Unlock()
		return nil
	}

	// 获取最近的 action 记录用于分析
	recentActions := make([]*ActionRecord, r.sameActionTypeSpinThreshold)
	copy(recentActions, r.actionHistory[historyLen-r.sameActionTypeSpinThreshold:])
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
		return nil
	}

	// 获取 Timeline 信息
	timelineContent := r.getTimelineContentForSpinDetection()

	// 格式化 action 历史为字符串
	var actionsText strings.Builder
	for i, action := range recentActions {
		actionsText.WriteString(fmt.Sprintf("**第 %d 次执行**（迭代 %d）：\n", i+1, action.IterationIndex))
		actionsText.WriteString(fmt.Sprintf("- Action 类型: %s\n", action.ActionType))
		actionsText.WriteString(fmt.Sprintf("- Action 名称: %s\n", action.ActionName))
		actionsText.WriteString("- Action 参数:\n")
		paramsJSON, err := json.MarshalIndent(action.ActionParams, "  ", "  ")
		if err == nil {
			actionsText.WriteString("  ```json\n")
			actionsText.WriteString("  " + string(paramsJSON) + "\n")
			actionsText.WriteString("  ```\n")
		}
		actionsText.WriteString("\n")
	}

	return map[string]interface{}{
		"RecentActionsText": actionsText.String(),
		"TimelineContent":   timelineContent,
		"ActionType":        firstActionType,
		"ConsecutiveCount":  len(recentActions),
	}
}

// buildReflectionSchema 构建反思结果的 JSON Schema
func buildReflectionSchema() string {
	schema := aitool.NewObjectSchemaWithAction(
		aitool.WithStringParam(
			"@action",
			aitool.WithParam_Description("必须是 'self_reflection'"),
			aitool.WithParam_EnumString("self_reflection"),
			aitool.WithParam_Required(true),
		),
		// SPIN 检测相关字段（仅在提供了 SPIN 检测数据时使用）
		aitool.WithBoolParam(
			"is_spinning",
			aitool.WithParam_Description("是否发生了 SPIN 情况（可选）：仅在提供了 SPIN 检测数据时判断。SPIN 指 AI Agent 反复做出相同或相似的决策，没有推进任务"),
			aitool.WithParam_Required(false),
		),
		aitool.WithStringParam(
			"spin_reason",
			aitool.WithParam_Description("SPIN 原因（可选）：如果 is_spinning 为 true，说明发生 SPIN 的原因"),
			aitool.WithParam_Required(false),
		),
		aitool.WithStringArrayParam(
			"suggestions",
			aitool.WithParam_Description("建议（可选）：针对类似情况的改进建议，按需提供。如果检测到 SPIN，请将打破循环的建议整合到此字段中"),
			aitool.WithParam_Required(false),
		),
	)
	return schema
}

// getPreviousReflectionsContext 获取之前反思的上下文
func (r *ReActLoop) getPreviousReflectionsContext(nonce string) string {
	history := r.GetReflectionHistory()
	if len(history) == 0 {
		return ""
	}

	// 只取最近 3 次反思
	start := 0
	if len(history) > 3 {
		start = len(history) - 3
	}

	recentReflections := history[start:]

	var buf strings.Builder
	for _, reflection := range recentReflections {
		buf.WriteString(reflection.Dump(nonce))
		buf.WriteString("\n")
	}

	return buf.String()
}
