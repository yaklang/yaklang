package reactloops

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/self_reflection_template.tpl
var selfReflectionTemplate string

// buildReflectionPrompt 构建反思 prompt.
//
// 设计要点:
//   - 反思 prompt 的"前缀"(high-static + frozen-block + timeline-open) 直接
//     复用主循环最近一次 prompt 的对应段, 用同一套 boundary 标签 (AI_CACHE_SYSTEM /
//     AI_CACHE_FROZEN / PROMPT_SECTION) 套回, 字节对齐, 享受 LLM provider 侧
//     的 prefix cache 命中, 显著降低反思 AI 调用的 token 成本与首字延迟.
//   - 反思特有内容 (action 详情 + SPIN 检测信息 + 输出 schema) 单独走 dynamic
//     段, 用反思 goroutine 自己的 nonce 包裹, 不影响 prefix cache.
//   - 去掉了历史的 RelevantMemories / PreviousReflections / EnvironmentalImpact
//     大段, 减少 cache 杀手 — SPIN 决策的核心依据是 timeline-open + 最近 action
//     历史, 这两者已经在 prefix + dynamic 段里覆盖.
//
// 关键词: buildReflectionPrompt, prefix cache 复用, dynamic 段隔离,
//
//	反思 prompt 精简
func (r *ReActLoop) buildReflectionPrompt(
	reflection *ActionReflection,
	nonce string,
) (string, error) {
	schema := buildReflectionSchema()

	resultStatus := "SUCCESS"
	if !reflection.Success {
		resultStatus = "FAILED"
	}

	data := map[string]interface{}{
		"Nonce":         nonce,
		"ActionType":    reflection.ActionType,
		"ToolName":      reflection.ToolName,
		"IterationNum":  reflection.IterationNum,
		"ExecutionTime": reflection.ExecutionTime.String(),
		"ResultStatus":  resultStatus,
		"ErrorMessage":  reflection.ErrorMessage,
		"Schema":        schema,
	}

	if spinData := r.getSpinDetectionData(); spinData != nil {
		data["SpinDetection"] = spinData
	}

	dynamic, err := utils.RenderTemplate(selfReflectionTemplate, data)
	if err != nil {
		return "", utils.Wrap(err, "render self-reflection template failed")
	}

	// 复用主循环最近一次 prompt 的 high-static / frozen-block / timeline-open
	// 三段, 与主循环保持字节对齐. semi-dynamic 段反思场景不需要, 留空让
	// BuildTaggedPromptSections 自然过滤.
	highStatic, frozenBlock, timelineOpen := r.getCachedPrefixBodies()
	prompt := aicommon.BuildTaggedPromptSections(
		highStatic,
		frozenBlock,
		"", // semi-dynamic-1 (反思不需要 SkillsContext / RecentToolsCache)
		"", // semi-dynamic-2 (反思不需要主循环的 TaskInstruction / Schema)
		timelineOpen,
		dynamic,
		nonce,
	)
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

	// 检查是否都是相同的 ActionType + ToolName(与 IsInSameActionTypeSpin 双维度对齐)
	// 关键词: 反思 prompt 双维度过滤
	firstActionType := recentActions[0].ActionType
	firstToolName := recentActions[0].ToolName
	allSameType := true
	for _, action := range recentActions {
		if action.ActionType != firstActionType || action.ToolName != firstToolName {
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

	r.spinCounterMu.Lock()
	currentSpinWarning := r.consecutiveSpinWarnings
	r.spinCounterMu.Unlock()
	escalationLevel := currentSpinWarning + 1 // next level after this detection
	if escalationLevel < 1 {
		escalationLevel = 1
	}

	return map[string]interface{}{
		"RecentActionsText": actionsText.String(),
		"TimelineContent":   timelineContent,
		"ActionType":        firstActionType,
		"ToolName":          firstToolName,
		"ConsecutiveCount":  len(recentActions),
		"SpinWarningCount":  currentSpinWarning,
		"EscalationLevel":   escalationLevel,
	}
}

// buildReflectionSchema 构建反思结果的 JSON Schema
func buildReflectionSchema() string {
	suggestionsDesc := "建议（可选）：针对类似情况的改进建议，按需提供。如果检测到 SPIN，请将打破循环的建议整合到此字段中。"

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
			aitool.WithParam_Description(suggestionsDesc),
			aitool.WithParam_Required(false),
		),
		aitool.WithBoolParam(
			"is_task_progressing",
			aitool.WithParam_Description("任务是否正常推进中（可选）：即使使用了相同类型的 action，如果参数不同、目标不同、有实质进展，则为 true。返回 true 时 SPIN 计数将被清零。"),
			aitool.WithParam_Required(false),
		),
	)
	return schema
}

