package reactloops

import (
	_ "embed"
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

	// 使用模板渲染 prompt
	prompt, err := utils.RenderTemplate(selfReflectionTemplate, data)
	if err != nil {
		return "", utils.Wrap(err, "render self-reflection template failed")
	}

	return prompt, nil
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
		aitool.WithStringArrayParam(
			"learning_insights",
			aitool.WithParam_Description("关键学习点（可选）：从本次执行中学到的重要经验，简洁描述即可"),
			aitool.WithParam_Required(false),
		),
		aitool.WithStringArrayParam(
			"future_suggestions",
			aitool.WithParam_Description("未来建议（可选）：针对类似情况的改进建议，按需提供"),
			aitool.WithParam_Required(false),
		),
		aitool.WithStringParam(
			"impact_assessment",
			aitool.WithParam_Description("影响评估（可选）：简要说明本次操作的影响，如无特殊影响可省略"),
			aitool.WithParam_Required(false),
		),
		aitool.WithStringParam(
			"effectiveness_rating",
			aitool.WithParam_Description("效果评级（可选）：评估操作效果"),
			aitool.WithParam_EnumString("highly_effective", "effective", "moderately_effective", "ineffective", "counterproductive"),
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
