package aireact

import (
	"context"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// RegisterBuiltinMiniAITasks 注册内置的 mini AI handler。
// 在 NewReAct 时调用。
func RegisterBuiltinMiniAITasks(registry *MiniAITaskRegistry) {
	registry.Register(aicommon.CallerLabelMiniPromptOptimize, handlePromptOptimize)
	registry.Register(aicommon.CallerLabelMiniTimelineSummary, handleTimelineSummary)
	registry.Register(aicommon.CallerLabelMiniTodoDraft, handleTodoDraft)
}

// builtinMiniTaskDescriptions 提供内置 mini AI task 的中文描述，供前端展示。
var builtinMiniTaskDescriptions = map[string]string{
	"prompt_optimize":  "提示词优化：让 AI 优化用户提示词，使其更清晰、更结构化",
	"timeline_summary": "时间线总结：快速总结当前 Agent 的执行时间线",
	"todo_draft":       "待办草拟：综合 timeline 将用户一句话转为符合三要素规范的 TODO 草稿",
}

// handlePromptOptimize 提示词优化 handler。
//
// 客户端发送:
//
//	AIInputEvent{
//	  IsSyncMessage: true,
//	  SyncType:      "ai_mini_task",
//	  SyncJsonInput: {"task_name":"prompt_optimize","prompt":"用户原始提示词"},
//	}
//
// handler 通过 Config 调度 Speed 辅助任务，单模型模式下使用 LiteCall，
// LiteForge 自动注入 recent timeline 让优化更上下文感知。
func handlePromptOptimize(ctx context.Context, taskCtx *MiniAITaskContext, params map[string]any) (result any, err error) {
	originalPrompt := getStringParam(params, "prompt")
	if originalPrompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}

	if taskCtx == nil || taskCtx.Config == nil {
		return nil, fmt.Errorf("mini AI task config is not available")
	}
	taskCtx.Config.ScheduleAuxiliaryTask(ctx, aicommon.CallerLabelMiniPromptOptimize,
		func() string {
			return fmt.Sprintf(`你是一个提示词优化专家。请优化以下用户提示词，使其更清晰、更结构化、更适合 AI agent 执行。

原始提示词:
%s

要求:
1. 补充必要的上下文和执行步骤
2. 明确期望的输出格式
3. 保持简洁，不要过度膨胀
4. 直接输出优化后的提示词`, originalPrompt)
		},
		func(action *aicommon.Action) {
			result = map[string]any{
				"original":         originalPrompt,
				"optimized_prompt": action.GetString("optimized_prompt"),
				"reason":           action.GetString("reason"),
			}
		},
		aicommon.WithAuxiliaryOnError(func(cause error) { err = fmt.Errorf("invoke liteforge failed: %w", cause) }),
		aicommon.WithAuxiliaryOutputs(
			aitool.WithStringParam("optimized_prompt",
				aitool.WithParam_Description("优化后的提示词"),
				aitool.WithParam_Required(true),
			),
			aitool.WithStringParam("reason",
				aitool.WithParam_Description("优化理由简述"),
			),
		),
	)
	if result == nil && err == nil {
		err = fmt.Errorf("prompt optimization returned no result")
	}
	return result, err
}

// handleTimelineSummary Timeline 快速总结 handler。
//
// 客户端发送:
//
//	AIInputEvent{
//	  IsSyncMessage: true,
//	  SyncType:      "ai_mini_task",
//	  SyncJsonInput: {"task_name":"timeline_summary"},
//	}
//
// handler 不手动 dump timeline，而是依赖 LiteForge 自动注入 recent timeline
// (走 DumpRecentForPrompt，已有 token 预算和去噪)，prompt 只需给出总结指令。
func handleTimelineSummary(ctx context.Context, taskCtx *MiniAITaskContext, params map[string]any) (result any, err error) {
	if taskCtx == nil || taskCtx.Timeline == nil {
		return nil, fmt.Errorf("timeline is not available")
	}
	if taskCtx.Config == nil {
		return nil, fmt.Errorf("mini AI task config is not available")
	}
	taskCtx.Config.ScheduleAuxiliaryTask(ctx, aicommon.CallerLabelMiniTimelineSummary,
		func() string {
			return `请对当前 AI Agent 的执行时间线进行简洁总结，提炼关键操作和结果。

要求:
1. 用 2-3 句话概括整体进展
2. 列出关键操作要点
3. 如果有未完成的任务，简要提及`
		},
		func(action *aicommon.Action) {
			result = map[string]any{
				"summary":     action.GetString("summary"),
				"key_points":  action.GetStringSlice("key_points"),
				"entry_count": taskCtx.Timeline.GetIdToTimelineItem().Len(),
			}
		},
		aicommon.WithAuxiliaryOnError(func(cause error) { err = fmt.Errorf("invoke liteforge failed: %w", cause) }),
		aicommon.WithAuxiliaryOutputs(
			aitool.WithStringParam("summary",
				aitool.WithParam_Description("时间线总结"),
				aitool.WithParam_Required(true),
			),
			aitool.WithStringArrayParam("key_points",
				aitool.WithParam_Description("关键操作要点列表"),
			),
		),
	)
	if result == nil && err == nil {
		err = fmt.Errorf("timeline summary returned no result")
	}
	return result, err
}

// handleTodoDraft 待办草拟 handler。
//
// 客户端发送:
//
//	AIInputEvent{
//	  IsSyncMessage: true,
//	  SyncType:      "ai_mini_task",
//	  SyncJsonInput: {"task_name":"todo_draft","user_input":"用户的一句话"},
//	}
//
// handler 综合用户输入 + LiteForge 自动注入的 recent timeline，
// 让 speed 优先模型生成一个符合「待办条目文本规范 · 三要素」的 TODO 草稿。
// 本 handler 不修改系统 TODO 状态，只返回草稿供前端展示；
// 用户确认后由前端通过其他机制（todo_delta / user_intervention）完成实际写入。
func handleTodoDraft(ctx context.Context, taskCtx *MiniAITaskContext, params map[string]any) (result any, err error) {
	userInput := getStringParam(params, "user_input")
	if userInput == "" {
		return nil, fmt.Errorf("user_input is required")
	}

	if taskCtx == nil || taskCtx.Config == nil {
		return nil, fmt.Errorf("mini AI task config is not available")
	}
	taskCtx.Config.ScheduleAuxiliaryTask(ctx, aicommon.CallerLabelMiniTodoDraft,
		func() string {
			return fmt.Sprintf(`你是一个待办条目草拟助手。请根据用户的一句话输入，结合当前 Agent 的执行时间线，生成一个清晰、明确的待办条目草稿。

用户输入:
%s

待办条目必须包含三要素:
1. **具体目标**: 写明操作对象与用户验收要求，按具体验收对象命名，禁止打包多个独立目标为一条
2. **来源**: 标注来自用户要求还是具体 Observation / 工具调用
3. **验收方法**: 写明什么产物、对照或观察足以支持关闭，什么结果仍需继续

注意:
- 如果当前 timeline 中已有相关进展，在来源和验收方法中体现已有上下文
- 条目文本要简洁但信息完整，一段话覆盖三要素
- 不要生成多个条目，只生成一条最贴合用户输入的待办`, userInput)
		},
		func(action *aicommon.Action) {
			result = map[string]any{
				"todo_text":           action.GetString("todo_text"),
				"target":              action.GetString("target"),
				"source":              action.GetString("source"),
				"acceptance_criteria": action.GetString("acceptance_criteria"),
			}
		},
		aicommon.WithAuxiliaryOnError(func(cause error) { err = fmt.Errorf("invoke liteforge failed: %w", cause) }),
		aicommon.WithAuxiliaryOutputs(
			aitool.WithStringParam("todo_text",
				aitool.WithParam_Description("待办条目文本，需覆盖具体目标、来源、验收方法三要素"),
				aitool.WithParam_Required(true),
			),
			aitool.WithStringParam("target",
				aitool.WithParam_Description("具体目标简述"),
				aitool.WithParam_Required(true),
			),
			aitool.WithStringParam("source",
				aitool.WithParam_Description("来源简述：用户要求或具体 Observation"),
				aitool.WithParam_Required(true),
			),
			aitool.WithStringParam("acceptance_criteria",
				aitool.WithParam_Description("验收方法简述：什么产物或观察足以支持关闭"),
				aitool.WithParam_Required(true),
			),
		),
	)
	if result == nil && err == nil {
		err = fmt.Errorf("todo draft returned no result")
	}
	return result, err
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
