package reactloops

import (
	"fmt"
	"strings"
	"time"
)

// ActionReflection 存储单次行动的反思结果
type ActionReflection struct {
	// 基本信息
	ActionType    string                 `json:"action_type"`
	ActionParams  map[string]interface{} `json:"action_params"`
	ExecutionTime time.Duration          `json:"execution_time"`
	IterationNum  int                    `json:"iteration_num"`
	Success       bool                   `json:"success"`
	ErrorMessage  string                 `json:"error_message,omitempty"`

	// 环境影响
	EnvironmentalImpact *EnvironmentalImpact `json:"environmental_impact,omitempty"`

	// 学习内容
	LearningInsights    []string  `json:"learning_insights,omitempty"`
	FutureSuggestions   []string  `json:"future_suggestions,omitempty"`
	ImpactAssessment    string    `json:"impact_assessment,omitempty"`
	EffectivenessRating string    `json:"effectiveness_rating,omitempty"`
	ReflectionLevel     string    `json:"reflection_level"`
	ReflectionTimestamp time.Time `json:"reflection_timestamp"`
}

// Dump 生成适合放入 Prompt 的格式化字符串（使用 nonce 保护）
func (r *ActionReflection) Dump(nonce string) string {
	var buf strings.Builder

	buf.WriteString(fmt.Sprintf("<|REFLECTION_HISTORY_%s|>\n", nonce))
	buf.WriteString(fmt.Sprintf("## Action: %s (Iteration %d)\n\n", r.ActionType, r.IterationNum))
	buf.WriteString(fmt.Sprintf("**Level**: %s | **Status**: %s | **Time**: %v\n",
		r.ReflectionLevel, func() string {
			if r.Success {
				return "✓ SUCCESS"
			}
			return "✗ FAILED"
		}(), r.ExecutionTime))

	if r.ErrorMessage != "" {
		buf.WriteString(fmt.Sprintf("**Error**: %s\n", r.ErrorMessage))
	}
	buf.WriteString("\n")

	if len(r.LearningInsights) > 0 {
		buf.WriteString("### Key Learnings\n")
		for i, insight := range r.LearningInsights {
			buf.WriteString(fmt.Sprintf("%d. %s\n", i+1, insight))
		}
		buf.WriteString("\n")
	}

	if len(r.FutureSuggestions) > 0 {
		buf.WriteString("### Recommendations\n")
		for i, suggestion := range r.FutureSuggestions {
			buf.WriteString(fmt.Sprintf("%d. %s\n", i+1, suggestion))
		}
		buf.WriteString("\n")
	}

	if r.ImpactAssessment != "" {
		buf.WriteString(fmt.Sprintf("### Impact\n%s\n\n", r.ImpactAssessment))
	}

	if r.EffectivenessRating != "" {
		buf.WriteString(fmt.Sprintf("**Effectiveness**: %s\n", r.EffectivenessRating))
	}

	buf.WriteString(fmt.Sprintf("<|REFLECTION_HISTORY_END_%s|>\n", nonce))
	return buf.String()
}

// ToMemoryContent 转换为适合保存到 aimem 的内容格式
func (r *ActionReflection) ToMemoryContent() string {
	var buf strings.Builder

	buf.WriteString(fmt.Sprintf("[CRITICAL REFLECTION] Action '%s' Execution Analysis\n\n",
		r.ActionType))

	// 强语气：明确说明执行结果
	if r.Success {
		buf.WriteString(fmt.Sprintf("✓ EXECUTION SUCCESSFUL in %v at iteration %d\n",
			r.ExecutionTime, r.IterationNum))
	} else {
		buf.WriteString(fmt.Sprintf("✗ EXECUTION FAILED after %v at iteration %d\n",
			r.ExecutionTime, r.IterationNum))
		if r.ErrorMessage != "" {
			buf.WriteString(fmt.Sprintf("FAILURE CAUSE: %s\n", r.ErrorMessage))
		}
	}
	buf.WriteString("\n")

	// 强语气：关键学习点
	if len(r.LearningInsights) > 0 {
		buf.WriteString("CRITICAL LEARNINGS - MUST REMEMBER:\n")
		for i, insight := range r.LearningInsights {
			buf.WriteString(fmt.Sprintf("%d. %s\n", i+1, insight))
		}
		buf.WriteString("\n")
	}

	// 强语气：未来指令
	if len(r.FutureSuggestions) > 0 {
		buf.WriteString("MANDATORY RECOMMENDATIONS FOR FUTURE ACTIONS:\n")
		for i, suggestion := range r.FutureSuggestions {
			buf.WriteString(fmt.Sprintf("%d. %s\n", i+1, suggestion))
		}
		buf.WriteString("\n")
	}

	// 影响评估
	if r.ImpactAssessment != "" {
		buf.WriteString(fmt.Sprintf("IMPACT ANALYSIS:\n%s\n\n", r.ImpactAssessment))
	}

	// 效果评级
	if r.EffectivenessRating != "" {
		buf.WriteString(fmt.Sprintf("EFFECTIVENESS RATING: %s\n\n",
			strings.ToUpper(strings.ReplaceAll(r.EffectivenessRating, "_", " "))))
	}

	// 环境影响（强语气）
	if r.EnvironmentalImpact != nil {
		if len(r.EnvironmentalImpact.PositiveEffects) > 0 {
			buf.WriteString("POSITIVE IMPACTS ACHIEVED:\n")
			for _, effect := range r.EnvironmentalImpact.PositiveEffects {
				buf.WriteString(fmt.Sprintf("✓ %s\n", effect))
			}
			buf.WriteString("\n")
		}
		if len(r.EnvironmentalImpact.NegativeEffects) > 0 {
			buf.WriteString("NEGATIVE IMPACTS TO AVOID:\n")
			for _, effect := range r.EnvironmentalImpact.NegativeEffects {
				buf.WriteString(fmt.Sprintf("✗ %s\n", effect))
			}
			buf.WriteString("\n")
		}
	}

	return strings.TrimSpace(buf.String())
}

// EnvironmentalImpact 环境影响分析
type EnvironmentalImpact struct {
	StateChanges    []string               `json:"state_changes"`
	ResourceUsage   map[string]interface{} `json:"resource_usage"`
	SideEffects     []string               `json:"side_effects"`
	PositiveEffects []string               `json:"positive_effects"`
	NegativeEffects []string               `json:"negative_effects"`
}
