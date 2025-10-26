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
			aitool.WithParam_Description("Action type identifier, must be 'self_reflection'"),
			aitool.WithParam_EnumString("self_reflection"),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringArrayParam(
			"learning_insights",
			aitool.WithParam_Description("Key learning insights from this action execution. Each insight should be a concise, actionable observation about what worked well or what could be improved."),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringArrayParam(
			"future_suggestions",
			aitool.WithParam_Description("Concrete suggestions for handling similar situations in the future. Focus on actionable recommendations."),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam(
			"impact_assessment",
			aitool.WithParam_Description("Overall assessment of the action's impact on the system and task progress. Explain whether the impact was positive, negative, or neutral, and why."),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam(
			"effectiveness_rating",
			aitool.WithParam_Description("Rate the action's effectiveness on a scale"),
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
