package aiengine

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// AIEngineConfig 简化的 AI 引擎配置
// 提供更友好的 API 来配置 ReAct 实例
type AIEngineConfig struct {
	// 基础配置
	Context context.Context
	Timeout float64

	// AI 服务配置
	AIService string // AI 服务名称，如 "openai", "deepseek" 等

	// 执行配置
	MaxIteration int    // 最大迭代次数，默认 10
	SessionID    string // 会话 ID，用于持久化

	// 工具配置
	DisableToolUse        bool     // 禁用工具调用
	EnableAISearchTool    bool     // 启用 AI 搜索工具
	EnableForgeSearchTool bool     // 启用 Forge 搜索工具，默认启用
	IncludeToolNames      []string // 包含的工具名称
	ExcludeToolNames      []string // 排除的工具名称
	Keywords              []string // 关键词，用于工具搜索

	// 交互配置
	AllowUserInteract bool   // 允许用户交互
	ReviewPolicy      string // 审批策略: "yolo", "ai", "manual"
	UserInteractLimit int64  // 用户交互次数限制

	// 内容限制
	TimelineContentLimit int // Timeline 内容大小限制

	// 调试配置
	DebugMode bool // 调试模式

	// 事件处理回调
	OnEvent            func(*schema.AiOutputEvent)                                                     // 事件回调
	OnStream           func(react *aireact.ReAct, data []byte)                                         // 流式输出回调
	OnData             func(react *aireact.ReAct, data []byte)                                         // 数据回调
	OnFinished         func(react *aireact.ReAct, success bool, result map[string]any)                 // 完成回调
	OnInputRequiredRaw func(react *aireact.ReAct, event *schema.AiOutputEvent, question string) string // 需要用户输入回调
	OnInputRequired    func(react *aireact.ReAct, question string) string                              // 需要用户输入回调

	// 高级配置
	Focus    string // 焦点，用于聚焦某个任务，如 yaklang_code
	Workdir  string // 工作目录
	Language string // 响应语言偏好
}

// AIEngineConfigOption 配置选项函数
type AIEngineConfigOption func(*AIEngineConfig)

// NewAIEngineConfig 创建默认配置
func NewAIEngineConfig(options ...AIEngineConfigOption) *AIEngineConfig {
	config := &AIEngineConfig{
		Context:               context.Background(),
		MaxIteration:          10,
		SessionID:             "default",
		AllowUserInteract:     true,
		ReviewPolicy:          "yolo",
		EnableForgeSearchTool: true,
		OnEvent:               func(*schema.AiOutputEvent) {},
		OnStream:              func(*aireact.ReAct, []byte) {},
		OnData:                func(*aireact.ReAct, []byte) {},
		OnFinished:            func(*aireact.ReAct, bool, map[string]any) {},
		OnInputRequiredRaw:    func(*aireact.ReAct, *schema.AiOutputEvent, string) string { return "" },
		OnInputRequired:       func(*aireact.ReAct, string) string { return "" },
	}

	// 应用选项
	for _, opt := range options {
		opt(config)
	}

	if config.Timeout > 0 {
		config.Context, _ = context.WithTimeout(config.Context, utils.FloatSecondDuration(config.Timeout))
	}

	return config
}

// ========== 基础配置选项 ==========

// WithFocus 设置焦点
func WithFocus(focus string) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.Focus = focus
	}
}

func WithTimeout(timeout float64) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.Timeout = timeout
	}
}

// WithContext 设置上下文
func WithContext(ctx context.Context) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.Context = ctx
	}
}

// WithAIService 设置 AI 服务
func WithAIService(service string) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.AIService = service
	}
}

// WithMaxIteration 设置最大迭代次数
func WithMaxIteration(max int) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.MaxIteration = max
	}
}

// WithSessionID 设置会话 ID
func WithSessionID(sessionID string) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.SessionID = sessionID
	}
}

// WithWorkdir 设置工作目录
func WithWorkdir(workdir string) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.Workdir = workdir
	}
}

// WithLanguage 设置响应语言
func WithLanguage(lang string) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.Language = lang
	}
}

// ========== 工具配置选项 ==========

// WithDisableToolUse 禁用工具调用
func WithDisableToolUse(disable bool) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.DisableToolUse = disable
	}
}

// WithEnableAISearchTool 启用 AI 搜索工具
func WithEnableAISearchTool(enable bool) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.EnableAISearchTool = enable
	}
}

// WithEnableForgeSearchTool 启用 Forge 搜索工具
func WithEnableForgeSearchTool(enable bool) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.EnableForgeSearchTool = enable
	}
}

// WithIncludeToolNames 设置包含的工具名称
func WithIncludeToolNames(names ...string) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.IncludeToolNames = names
	}
}

// WithExcludeToolNames 设置排除的工具名称
func WithExcludeToolNames(names ...string) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.ExcludeToolNames = names
	}
}

// WithKeywords 设置关键词
func WithKeywords(keywords ...string) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.Keywords = keywords
	}
}

// ========== 交互配置选项 ==========

// WithAllowUserInteract 设置是否允许用户交互
func WithAllowUserInteract(allow bool) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.AllowUserInteract = allow
	}
}

// WithReviewPolicy 设置审批策略
// policy: "yolo" (自动通过), "ai" (AI 审批), "manual" (手动审批)
func WithReviewPolicy(policy string) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.ReviewPolicy = policy
	}
}

// WithUserInteractLimit 设置用户交互次数限制
func WithUserInteractLimit(limit int64) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.UserInteractLimit = limit
	}
}

// WithTimelineContentLimit 设置 Timeline 内容大小限制
func WithTimelineContentLimit(limit int) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.TimelineContentLimit = limit
	}
}

// ========== 调试配置选项 ==========

// WithDebugMode 设置调试模式
func WithDebugMode(debug bool) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.DebugMode = debug
	}
}

// ========== 事件处理回调选项 ==========

// WithOnEvent 设置事件回调
func WithOnEvent(callback func(*schema.AiOutputEvent)) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.OnEvent = callback
	}
}

// WithOnStream 设置流式输出回调
func WithOnStream(callback func(react *aireact.ReAct, data []byte)) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.OnStream = callback
	}
}

// WithOnData 设置数据回调
func WithOnData(callback func(react *aireact.ReAct, data []byte)) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.OnData = callback
	}
}

// WithOnFinished 设置完成回调
func WithOnFinished(callback func(react *aireact.ReAct, success bool, result map[string]any)) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.OnFinished = callback
	}
}

// WithOnInputRequiredRaw 设置需要用户输入回调
func WithOnInputRequiredRaw(callback func(react *aireact.ReAct, event *schema.AiOutputEvent, question string) string) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.OnInputRequiredRaw = callback
	}
}

// WithOnInputRequired 设置需要用户输入回调
func WithOnInputRequired(callback func(react *aireact.ReAct, question string) string) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.OnInputRequired = callback
	}
}

// ========== 快捷配置组合 ==========

// WithYOLOMode YOLO 模式：自动执行所有操作，无需用户确认
func WithYOLOMode() AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.ReviewPolicy = "yolo"
		c.AllowUserInteract = false
	}
}

// WithManualMode 手动模式：所有操作都需要用户确认
func WithManualMode() AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.ReviewPolicy = "manual"
		c.AllowUserInteract = true
	}
}

// WithAIReviewMode AI 审批模式：由 AI 决定是否需要用户确认
func WithAIReviewMode() AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.ReviewPolicy = "ai"
		c.AllowUserInteract = true
	}
}

// ConvertToYPBAIStartParams 将 AIEngineConfig 转换为 YPB 的 AIStartParams
// 用于与现有的 gRPC 接口兼容
func (c *AIEngineConfig) ConvertToYPBAIStartParams() *ypb.AIStartParams {
	return &ypb.AIStartParams{
		DisallowRequireForUserPrompt: !c.AllowUserInteract,
		ReviewPolicy:                 c.ReviewPolicy,
		ReActMaxIteration:            int64(c.MaxIteration),
		TimelineContentSizeLimit:     int64(c.TimelineContentLimit),
		UserInteractLimit:            c.UserInteractLimit,
		DisableToolUse:               c.DisableToolUse,
		EnableAISearchTool:           c.EnableAISearchTool,
		ExcludeToolNames:             c.ExcludeToolNames,
		IncludeSuggestedToolNames:    c.IncludeToolNames,
		IncludeSuggestedToolKeywords: c.Keywords,
		AIService:                    c.AIService,
		TimelineSessionID:            c.SessionID,
	}
}
