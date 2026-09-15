package aireact

import (
	"context"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// RegisterBuiltinMiniAITasks 注册内置的 mini AI handler。
// 在 NewReAct 时调用。
func RegisterBuiltinMiniAITasks(registry *MiniAITaskRegistry) {
	registry.Register("prompt_optimize", handlePromptOptimize)
	registry.Register("timeline_summary", handleTimelineSummary)
}

// builtinMiniTaskDescriptions 提供内置 mini AI task 的中文描述，供前端展示。
var builtinMiniTaskDescriptions = map[string]string{
	"prompt_optimize":   "提示词优化：让 AI 优化用户提示词，使其更清晰、更结构化",
	"timeline_summary":  "时间线总结：快速总结当前 Agent 的执行时间线",
}

// handlePromptOptimize 提示词优化 handler。
//
// 客户端发送:
//   AIInputEvent{
//     IsSyncMessage: true,
//     SyncType:      "ai_mini_task",
//     SyncJsonInput: {"task_name":"prompt_optimize","prompt":"用户原始提示词"},
//   }
//
// handler 内部调用 InvokeSpeedPriorityLiteForge (speed 优先模型)，
// LiteForge 自动注入 recent timeline 让优化更上下文感知。
func handlePromptOptimize(ctx context.Context, taskCtx *MiniAITaskContext, params map[string]any) (any, error) {
	originalPrompt := getStringParam(params, "prompt")
	if originalPrompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}

	prompt := fmt.Sprintf(`你是一个提示词优化专家。请优化以下用户提示词，使其更清晰、更结构化、更适合 AI agent 执行。

原始提示词:
%s

要求:
1. 补充必要的上下文和执行步骤
2. 明确期望的输出格式
3. 保持简洁，不要过度膨胀
4. 直接输出优化后的提示词`, originalPrompt)

	action, err := taskCtx.ReAct.InvokeSpeedPriorityLiteForge(
		ctx,
		"prompt_optimize",
		prompt,
		[]aitool.ToolOption{
			aitool.WithStringParam("optimized_prompt",
				aitool.WithParam_Description("优化后的提示词"),
				aitool.WithParam_Required(true),
			),
			aitool.WithStringParam("reason",
				aitool.WithParam_Description("优化理由简述"),
			),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("invoke liteforge failed: %w", err)
	}

	return map[string]any{
		"original":         originalPrompt,
		"optimized_prompt": action.GetString("optimized_prompt"),
		"reason":           action.GetString("reason"),
	}, nil
}

// handleTimelineSummary Timeline 快速总结 handler。
//
// 客户端发送:
//   AIInputEvent{
//     IsSyncMessage: true,
//     SyncType:      "ai_mini_task",
//     SyncJsonInput: {"task_name":"timeline_summary"},
//   }
//
// handler 不手动 dump timeline，而是依赖 LiteForge 自动注入 recent timeline
// (走 DumpRecentForPrompt，已有 token 预算和去噪)，prompt 只需给出总结指令。
func handleTimelineSummary(ctx context.Context, taskCtx *MiniAITaskContext, params map[string]any) (any, error) {
	if taskCtx.Timeline == nil {
		return nil, fmt.Errorf("timeline is not available")
	}

	prompt := `请对当前 AI Agent 的执行时间线进行简洁总结，提炼关键操作和结果。

要求:
1. 用 2-3 句话概括整体进展
2. 列出关键操作要点
3. 如果有未完成的任务，简要提及`

	action, err := taskCtx.ReAct.InvokeSpeedPriorityLiteForge(
		ctx,
		"timeline_summary",
		prompt,
		[]aitool.ToolOption{
			aitool.WithStringParam("summary",
				aitool.WithParam_Description("时间线总结"),
				aitool.WithParam_Required(true),
			),
			aitool.WithStringArrayParam("key_points",
				aitool.WithParam_Description("关键操作要点列表"),
			),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("invoke liteforge failed: %w", err)
	}

	return map[string]any{
		"summary":     action.GetString("summary"),
		"key_points":  action.GetStringSlice("key_points"),
		"entry_count": taskCtx.Timeline.GetIdToTimelineItem().Len(),
	}, nil
}

// getStringParam 从 params map 中安全提取 string 值。
func getStringParam(params map[string]any, key string) string {
	if params == nil {
		return ""
	}
	if v, ok := params[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
		// JSON 数字也转成 string
		return fmt.Sprint(v)
	}
	return ""
}
