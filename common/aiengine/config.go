package aiengine

import (
	"context"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
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
	AIService                 string // AI 服务名称，如 "openai", "deepseek" 等
	AICallback                aicommon.AICallbackType
	QualityPriorityAICallback aicommon.AICallbackType
	SpeedPriorityAICallback   aicommon.AICallbackType

	// UserUsageCallback 是用户脚本通过 ai.usageCallback(...) 注册的 token usage 回调.
	// 由 WithAIConfig 从 aispec.AIConfigOption 列表中探测出来, 经 buildReActOptions
	// 透传到 aicommon.Config, 让 Tiered AI 路径在重新构造 chat opts 时再次注入,
	// 从而修复 React loop 内 chat 不触发用户 callback 的 bug.
	// 关键词: AIEngineConfig UsageCallback 透传
	UserUsageCallback func(*aispec.ChatUsage)

	// 执行配置
	MaxIteration int    // 最大迭代次数，默认 10
	SessionID    string // 会话 ID，用于持久化

	// 工具配置
	DisableToolUse        bool     // 禁用工具调用
	DisableAIForge        bool     // 禁用 Forge 调用
	DisableMCPServers     bool     // 禁用 MCPServers
	EnableAISearchTool    bool     // 启用 AI 搜索工具
	EnableForgeSearchTool bool     // 启用 Forge 搜索工具，默认启用
	IncludeToolNames      []string // 包含的工具名称
	ExcludeToolNames      []string // 排除的工具名称
	Keywords              []string // 关键词，用于工具搜索

	// ExtraMCPServers 会话级显式挂载的 MCP server（内存态，不读 profile DB、
	// 不进全局列表）。RestrictToSessionMCP 依赖此列表才能生效。
	ExtraMCPServers []*aicommon.ExtraMCPServer

	// RestrictToSessionMCP 为 true 时，工具集被钳制为仅会话注入的 MCP 工具，
	// 禁用内置工具/搜索/forge，避免 agent 误用本地 yak 工具（如 ssa-risk）。
	// 仅在配置了 ExtraMCPServers 时才有意义。
	RestrictToSessionMCP bool

	// 交互配置
	AllowUserInteract bool   // 允许用户交互
	ReviewPolicy      string // 审批策略: "yolo", "ai", "manual"
	UserInteractLimit int64  // 用户交互次数限制

	// 内容限制
	TimelineContentLimit int // Timeline 内容大小限制

	// 调试配置
	DebugMode bool // 调试模式

	// 事件处理回调
	OnEvent              func(react aicommon.AIEngineOperator, event *schema.AiOutputEvent)                                     // 事件回调
	OnStream             func(react aicommon.AIEngineOperator, event *schema.AiOutputEvent, NodeId string, data []byte)         // 流式输出回调
	OnStreamEnd          func(react aicommon.AIEngineOperator, event *schema.AiOutputEvent, NodeId string)                      // 流式输出结束
	OnStreamEndWithTotal func(react aicommon.AIEngineOperator, event *schema.AiOutputEvent, NodeId string, totalContent []byte) // 流式输出结束，带重组后的完整内容
	OnData               func(react aicommon.AIEngineOperator, event *schema.AiOutputEvent, NodeId string, data []byte)         // 数据回调
	OnFinished           func(react aicommon.AIEngineOperator)                                                                  // 完成回调, 不返回结果
	OnInputRequiredRaw   func(react aicommon.AIEngineOperator, event *schema.AiOutputEvent, question string) string             // 需要用户输入回调
	OnInputRequired      func(react aicommon.AIEngineOperator, question string) string                                          // 需要用户输入回调
	OnSessionID          func(sessionID string)                                                                                 // 会话 ID 就绪回调

	// 高级配置
	Focus    string // 焦点，用于聚焦某个任务，如 yaklang_code
	Workdir  string // 工作目录
	Language string // 响应语言偏好

	ExtOptions []aicommon.ConfigOption

	AttachedResources []*aicommon.AttachedResource

	ExtendedForgeFromZip []*ExtendedForgeZip
}

// AIEngineConfigOption 配置选项函数
type AIEngineConfigOption func(*AIEngineConfig)

// NewAIEngineConfig 创建默认配置
func NewAIEngineConfig(options ...AIEngineConfigOption) *AIEngineConfig {
	config := &AIEngineConfig{
		Context:               context.Background(),
		MaxIteration:          10,
		SessionID:             uuid.New().String(),
		AllowUserInteract:     true,
		ReviewPolicy:          "yolo",
		EnableForgeSearchTool: true,
		OnEvent:               func(aicommon.AIEngineOperator, *schema.AiOutputEvent) {},
		OnStream:              func(aicommon.AIEngineOperator, *schema.AiOutputEvent, string, []byte) {},
		OnStreamEnd:           func(aicommon.AIEngineOperator, *schema.AiOutputEvent, string) {},
		OnStreamEndWithTotal:  func(aicommon.AIEngineOperator, *schema.AiOutputEvent, string, []byte) {},
		OnData:                func(aicommon.AIEngineOperator, *schema.AiOutputEvent, string, []byte) {},
		OnFinished:            func(aicommon.AIEngineOperator) {},
		OnInputRequiredRaw:    func(aicommon.AIEngineOperator, *schema.AiOutputEvent, string) string { return "" },
		OnInputRequired:       func(aicommon.AIEngineOperator, string) string { return "" },
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

// notifySessionID 仅在引擎创建时通知一次 session，避免 SendMsg 内临时 config 重复生成 UUID 并二次通知前端。
func notifySessionID(config *AIEngineConfig) {
	if config == nil || config.OnSessionID == nil || config.SessionID == "" {
		return
	}
	config.OnSessionID(config.SessionID)
}

// ========== 基础配置选项 ==========

// WithFocus 设置焦点，用于让引擎聚焦某个任务（导出名为 aim.focus）
// 参数:
//   - focus: 聚焦标识，如 yaklang_code
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.focus("yaklang_code")
// println(opt)
// ```
func WithFocus(focus string) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.Focus = focus
	}
}

// WithTimeout 设置引擎运行的超时时间（导出名为 aim.timeout）
// 参数:
//   - timeout: 超时时间（秒）
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.timeout(60)
// println(opt)
// ```
func WithTimeout(timeout float64) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.Timeout = timeout
	}
}

// WithContext 设置上下文，用于控制引擎的取消（导出名为 aim.context）
// 参数:
//   - ctx: 上下文对象
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.context(context.Background())
// println(opt)
// ```
func WithContext(ctx context.Context) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.Context = ctx
	}
}

// WithAIService 设置使用的 AI 服务名称（导出名为 aim.aiService）
// 参数:
//   - service: AI 服务名，如 "openai"、"deepseek"
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.aiService("openai")
// println(opt)
// ```
func WithAIService(service string) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.AIService = service
	}
}

// WithMaxIteration 设置引擎最大迭代次数（导出名为 aim.maxIteration）
// 参数:
//   - max: 最大迭代次数（默认 10）
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.maxIteration(20)
// println(opt)
// ```
func WithMaxIteration(max int) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.MaxIteration = max
	}
}

// WithSessionID 设置会话 ID，用于持久化与续接（导出名为 aim.sessionID）
// 参数:
//   - sessionID: 会话 ID
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.sessionID("my-session")
// println(opt)
// ```
func WithSessionID(sessionID string) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.SessionID = sessionID
	}
}

// WithWorkdir 设置引擎工作目录（导出名为 aim.workdir）
// 参数:
//   - workdir: 工作目录路径
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.workdir("/tmp/work")
// println(opt)
// ```
func WithWorkdir(workdir string) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.Workdir = workdir
	}
}

// WithLanguage 设置引擎响应语言偏好（导出名为 aim.language）
// 参数:
//   - lang: 语言标识，如 "zh"、"en"
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.language("zh")
// println(opt)
// ```
func WithLanguage(lang string) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.Language = lang
	}
}

// ========== 工具配置选项 ==========

// WithDisableToolUse 禁用工具调用（导出名为 aim.disableToolUse）
// 参数:
//   - disable: 是否禁用工具调用
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.disableToolUse(true)
// println(opt)
// ```
func WithDisableToolUse(disable bool) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.DisableToolUse = disable
	}
}

// WithDisableAIForge 禁用 AI Forge 调用（导出名为 aim.disableAIForge）
// 参数:
//   - disable: 是否禁用 Forge
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.disableAIForge(true)
// println(opt)
// ```
func WithDisableAIForge(disable bool) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.DisableAIForge = disable
	}
}

// WithDisableMCPServers 禁用 MCP servers（导出名为 aim.disableMCPServers）
// 参数:
//   - disable: 是否禁用 MCP servers
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.disableMCPServers(true)
// println(opt)
// ```
func WithDisableMCPServers(disable bool) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.DisableMCPServers = disable
	}
}

// WithExtraMCPServers 注入会话级 MCP server（内存态，不读 profile DB）。
// 参数:
//   - servers: 一个或多个 MCP server 配置
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// // server 由 aicommon 构造（示意性示例）
// opt = aim.WithExtraMCPServers(server)
// println(opt)
// ```
func WithExtraMCPServers(servers ...*aicommon.ExtraMCPServer) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.ExtraMCPServers = append(c.ExtraMCPServers, servers...)
	}
}

// WithRestrictToSessionMCP 将工具集钳制为仅会话注入的 MCP 工具（禁用内置工具/搜索/forge）。
// 参数:
//   - restrict: 是否仅使用会话 MCP 工具
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.WithRestrictToSessionMCP(true)
// println(opt)
// ```
func WithRestrictToSessionMCP(restrict bool) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.RestrictToSessionMCP = restrict
	}
}

// WithEnableAISearchTool 启用 AI 搜索工具（导出名为 aim.enableAISearchTool）
// 参数:
//   - enable: 是否启用 AI 搜索工具
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.enableAISearchTool(true)
// println(opt)
// ```
func WithEnableAISearchTool(enable bool) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.EnableAISearchTool = enable
	}
}

// WithEnableForgeSearchTool 启用 Forge 搜索工具（导出名为 aim.enableForgeSearchTool）
// 参数:
//   - enable: 是否启用 Forge 搜索工具
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.enableForgeSearchTool(true)
// println(opt)
// ```
func WithEnableForgeSearchTool(enable bool) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.EnableForgeSearchTool = enable
	}
}

// WithIncludeToolNames 设置包含的工具名称白名单（导出名为 aim.includeToolNames）
// 参数:
//   - names: 一个或多个工具名
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.includeToolNames("ls", "cat")
// println(opt)
// ```
func WithIncludeToolNames(names ...string) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.IncludeToolNames = names
	}
}

// WithExcludeToolNames 设置排除的工具名称黑名单（导出名为 aim.excludeToolNames）
// 参数:
//   - names: 一个或多个工具名
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.excludeToolNames("rm", "exec")
// println(opt)
// ```
func WithExcludeToolNames(names ...string) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.ExcludeToolNames = names
	}
}

// WithKeywords 设置工具搜索关键词（导出名为 aim.keywords）
// 参数:
//   - keywords: 一个或多个关键词
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.keywords("scan", "vuln")
// println(opt)
// ```
func WithKeywords(keywords ...string) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.Keywords = keywords
	}
}

// ========== 交互配置选项 ==========

// WithAllowUserInteract 设置是否允许用户交互（导出名为 aim.allowUserInteract）
// 参数:
//   - allow: 是否允许用户交互
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.allowUserInteract(true)
// println(opt)
// ```
func WithAllowUserInteract(allow bool) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.AllowUserInteract = allow
	}
}

// WithReviewPolicy 设置审批策略（导出名为 aim.reviewPolicy）
// policy: "yolo" (自动通过), "ai" (AI 审批), "manual" (手动审批)
// 参数:
//   - policy: 审批策略
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.reviewPolicy("manual")
// println(opt)
// ```
func WithReviewPolicy(policy string) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.ReviewPolicy = policy
	}
}

// WithUserInteractLimit 设置用户交互次数上限（导出名为 aim.userInteractLimit）
// 参数:
//   - limit: 交互次数上限
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.userInteractLimit(3)
// println(opt)
// ```
func WithUserInteractLimit(limit int64) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.UserInteractLimit = limit
	}
}

// WithTimelineContentLimit 设置 Timeline 内容大小上限（导出名为 aim.timelineContentLimit）
// 参数:
//   - limit: 内容大小上限
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.timelineContentLimit(4096)
// println(opt)
// ```
func WithTimelineContentLimit(limit int) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.TimelineContentLimit = limit
	}
}

// ========== 调试配置选项 ==========

// WithDebugMode 设置调试模式（导出名为 aim.debugMode）
// 参数:
//   - debug: 是否启用调试模式
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.debugMode(true)
// println(opt)
// ```
func WithDebugMode(debug bool) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.DebugMode = debug
	}
}

// ========== 事件处理回调选项 ==========

// WithOnEvent 设置事件回调（导出名为 aim.onEvent）
// 参数:
//   - callback: 事件回调，参数为 (operator, event)
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.onEvent(func(op, e) { dump(e) })
// println(opt)
// ```
func WithOnEvent(callback func(aicommon.AIEngineOperator, *schema.AiOutputEvent)) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.OnEvent = callback
	}
}

// WithOnStream 设置流式输出回调（导出名为 aim.onStream）
// 参数:
//   - callback: 流式回调，参数为 (operator, event, nodeId, data)
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.onStream(func(op, e, nodeId, data) { print(string(data)) })
// println(opt)
// ```
func WithOnStream(callback func(react aicommon.AIEngineOperator, event *schema.AiOutputEvent, NodeId string, data []byte)) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.OnStream = callback
	}
}

// WithOnStreamEnd 设置流式输出结束回调（导出名为 aim.onStreamEnd）
// 参数:
//   - callback: 回调，参数为 (operator, event, nodeId)
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.onStreamEnd(func(op, e, nodeId) { println("stream end:", nodeId) })
// println(opt)
// ```
func WithOnStreamEnd(callback func(react aicommon.AIEngineOperator, event *schema.AiOutputEvent, NodeId string)) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.OnStreamEnd = callback
	}
}

// WithOnStreamContent 设置流式输出结束回调，并返回重组后的完整流内容（导出名为 aim.onStreamContent）
// 参数:
//   - callback: 回调，参数为 (operator, event, nodeId, totalContent)
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.onStreamContent(func(op, e, nodeId, total) { println(string(total)) })
// println(opt)
// ```
func WithOnStreamContent(callback func(react aicommon.AIEngineOperator, event *schema.AiOutputEvent, NodeId string, totalContent []byte)) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.OnStreamEndWithTotal = callback
	}
}

// WithOnData 设置数据回调（导出名为 aim.onData）
// 参数:
//   - callback: 回调，参数为 (operator, event, nodeId, data)
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.onData(func(op, e, nodeId, data) { dump(data) })
// println(opt)
// ```
func WithOnData(callback func(react aicommon.AIEngineOperator, event *schema.AiOutputEvent, NodeId string, data []byte)) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.OnData = callback
	}
}

// WithOnFinished 设置完成回调（导出名为 aim.onFinished）
// 参数:
//   - callback: 回调，参数为 operator
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.onFinished(func(op) { println("finished") })
// println(opt)
// ```
func WithOnFinished(callback func(react aicommon.AIEngineOperator)) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.OnFinished = callback
	}
}

// WithOnInputRequiredRaw 设置需要用户输入的回调（携带原始事件，导出名为 aim.onInputRequiredRaw）
// 参数:
//   - callback: 回调，参数为 (operator, event, question)，返回用户答复
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.onInputRequiredRaw(func(op, e, question) { return "yes" })
// println(opt)
// ```
func WithOnInputRequiredRaw(callback func(react aicommon.AIEngineOperator, event *schema.AiOutputEvent, question string) string) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.OnInputRequiredRaw = callback
	}
}

// WithOnInputRequired 设置需要用户输入的回调（导出名为 aim.onInputRequired）
// 参数:
//   - callback: 回调，参数为 (operator, question)，返回用户答复
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.onInputRequired(func(op, question) { return "yes" })
// println(opt)
// ```
func WithOnInputRequired(callback func(react aicommon.AIEngineOperator, question string) string) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.OnInputRequired = callback
	}
}

// WithOnSessionID 设置会话 ID 就绪回调（导出名为 aim.onSessionID）
// 参数:
//   - callback: 回调，参数为 sessionID
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.onSessionID(func(sessionID) { println(sessionID) })
// println(opt)
// ```
func WithOnSessionID(callback func(sessionID string)) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.OnSessionID = callback
	}
}

// ========== 快捷配置组合 ==========

// WithYOLOMode YOLO 模式：自动执行所有操作，无需用户确认（导出名为 aim.yoloMode）
// 参数:
//   - 无
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.yoloMode()
// println(opt)
// ```
func WithYOLOMode() AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.ReviewPolicy = "yolo"
		c.AllowUserInteract = false
	}
}

// WithManualMode 手动模式：所有操作都需要用户确认（导出名为 aim.manualMode）
// 参数:
//   - 无
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.manualMode()
// println(opt)
// ```
func WithManualMode() AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.ReviewPolicy = "manual"
		c.AllowUserInteract = true
	}
}

// WithAIReviewMode AI 审批模式：由 AI 决定是否需要用户确认（导出名为 aim.aiReviewMode）
// 参数:
//   - 无
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.aiReviewMode()
// println(opt)
// ```
func WithAIReviewMode() AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.ReviewPolicy = "ai"
		c.AllowUserInteract = true
	}
}

// WithAICallback 设置 AI 回调（导出名为 aim.aiCallback）
// 参数:
//   - callback: AI 回调函数
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// // callback 由 aicommon 提供（示意性示例）
// opt = aim.aiCallback(callback)
// println(opt)
// ```
func WithAICallback(callback aicommon.AICallbackType) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.AICallback = callback
	}
}

// WithQualityPriorityAICallback 设置质量优先的 AI 回调（导出名为 aim.qualityPriorityAICallback）
// 参数:
//   - callback: AI 回调函数
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.qualityPriorityAICallback(callback)
// println(opt)
// ```
func WithQualityPriorityAICallback(callback aicommon.AICallbackType) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.QualityPriorityAICallback = callback
	}
}

// WithSpeedPriorityAICallback 设置速度优先的 AI 回调（导出名为 aim.speedPriorityAICallback）
// 参数:
//   - callback: AI 回调函数
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.speedPriorityAICallback(callback)
// println(opt)
// ```
func WithSpeedPriorityAICallback(callback aicommon.AICallbackType) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.SpeedPriorityAICallback = callback
	}
}

// WithAttachedResource 为引擎附加自定义资源（导出名为 aim.attachedResource）
// 参数:
//   - typ: 资源类型
//   - key: 资源键
//   - value: 资源值
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.attachedResource("file", "path", "/tmp/a.txt")
// println(opt)
// ```
func WithAttachedResource(typ string, key string, value string) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.AttachedResources = append(c.AttachedResources, aicommon.NewAttachedResource(typ, key, value))
	}
}

// WithAttachedFilePath 为引擎附加一个文件路径资源（导出名为 aim.attachedFilePath）
// 参数:
//   - filePath: 文件路径
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.attachedFilePath("/tmp/report.txt")
// println(opt)
// ```
func WithAttachedFilePath(filePath string) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.AttachedResources = append(c.AttachedResources, aicommon.NewAttachedResource(aicommon.CONTEXT_PROVIDER_TYPE_FILE, aicommon.CONTEXT_PROVIDER_KEY_FILE_PATH, filePath))
	}
}

// WithAttachedFileContent 为引擎附加文件内容资源（导出名为 aim.attachedFileContent）
// 参数:
//   - content: 文件内容
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.attachedFileContent("hello world")
// println(opt)
// ```
func WithAttachedFileContent(content string) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.AttachedResources = append(c.AttachedResources, aicommon.NewAttachedResource(aicommon.CONTEXT_PROVIDER_TYPE_FILE, aicommon.CONTEXT_PROVIDER_KEY_FILE_CONTENT, content))
	}
}

// WithAttachedKnowledgeBase 为引擎附加一个知识库（导出名为 aim.attachedKnowledgeBase）
// 参数:
//   - knowledgeBaseName: 知识库名称
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.attachedKnowledgeBase("my-kb")
// println(opt)
// ```
func WithAttachedKnowledgeBase(knowledgeBaseName string) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.AttachedResources = append(c.AttachedResources, aicommon.NewAttachedResource(aicommon.CONTEXT_PROVIDER_TYPE_KNOWLEDGE_BASE, aicommon.CONTEXT_PROVIDER_KEY_NAME, knowledgeBaseName))
	}
}

// WithAttachedAITool 为引擎附加一个 AI 工具（导出名为 aim.attachedAITool）
// 参数:
//   - aitoolName: AI 工具名称
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.attachedAITool("ls")
// println(opt)
// ```
func WithAttachedAITool(aitoolName string) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.AttachedResources = append(c.AttachedResources, aicommon.NewAttachedResource(aicommon.CONTEXT_PROVIDER_TYPE_AITOOL, aicommon.CONTEXT_PROVIDER_KEY_NAME, aitoolName))
	}
}

// WithAttachedAIForge 为引擎附加一个 AI Forge（导出名为 aim.attachedAIForge）
// 参数:
//   - aiForgeName: AI Forge 名称
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.attachedAIForge("my-forge")
// println(opt)
// ```
func WithAttachedAIForge(aiForgeName string) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.AttachedResources = append(c.AttachedResources, aicommon.NewAttachedResource(aicommon.CONTEXT_PROVIDER_TYPE_AIFORGE, aicommon.CONTEXT_PROVIDER_KEY_NAME, aiForgeName))
	}
}

func loadAICallbackFromAIConfig(typeName string, opts ...aispec.AIConfigOption) aicommon.AICallbackType {
	chatter, err := ai.LoadChater(typeName, opts...)
	if err != nil {
		log.Errorf("load ai service failed: %v", err)
		return nil
	}
	return aicommon.AIChatToAICallbackType(chatter)
}

func applyUserUsageCallbackFromAIConfig(c *AIEngineConfig, opts ...aispec.AIConfigOption) {
	// 探测 opts 中的 UsageCallback, 透传给 React Config 让 Tiered AI 路径
	// (GetXxxAIModelCallback) 重新构造 chat opts 时能再次注入, 修复
	// ai.usageCallback(...) 在 React loop 内不触发的 bug.
	// 关键词: WithAIConfig UsageCallback 探测透传
	if probe := aispec.NewDefaultAIConfig(opts...); probe != nil && probe.UsageCallback != nil {
		c.UserUsageCallback = probe.UsageCallback
	}
}

// WithAIConfig 通过 AI 类型与 aispec 选项设置引擎使用的 AI（导出名为 aim.aiConfig）
// 参数:
//   - typeName: AI 类型名，如 "openai"
//   - opts: aispec AI 配置选项，如 ai.apiKey、ai.model
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.aiConfig("openai", ai.apiKey("sk-xxx"), ai.model("gpt-4"))
// println(opt)
// ```
func WithAIConfig(typeName string, opts ...aispec.AIConfigOption) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		if cb := loadAICallbackFromAIConfig(typeName, opts...); cb != nil {
			c.AICallback = cb
		}
		applyUserUsageCallbackFromAIConfig(c, opts...)
	}
}

// WithQualityPriorityAIConfig 设置质量优先档使用的 AI（导出名为 aim.qualityPriorityAIConfig）
// 参数:
//   - typeName: AI 类型名
//   - opts: aispec AI 配置选项
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.qualityPriorityAIConfig("openai", ai.apiKey("sk-xxx"))
// println(opt)
// ```
func WithQualityPriorityAIConfig(typeName string, opts ...aispec.AIConfigOption) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		if cb := loadAICallbackFromAIConfig(typeName, opts...); cb != nil {
			c.QualityPriorityAICallback = cb
		}
		applyUserUsageCallbackFromAIConfig(c, opts...)
	}
}

// WithSpeedPriorityAIConfig 设置速度优先档使用的 AI（导出名为 aim.speedPriorityAIConfig）
// 参数:
//   - typeName: AI 类型名
//   - opts: aispec AI 配置选项
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.speedPriorityAIConfig("openai", ai.apiKey("sk-xxx"))
// println(opt)
// ```
func WithSpeedPriorityAIConfig(typeName string, opts ...aispec.AIConfigOption) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		if cb := loadAICallbackFromAIConfig(typeName, opts...); cb != nil {
			c.SpeedPriorityAICallback = cb
		}
		applyUserUsageCallbackFromAIConfig(c, opts...)
	}
}

// WithExtOptions 设置扩展选项
func WithExtOptions(opts ...aicommon.ConfigOption) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.ExtOptions = opts
	}
}

// WithExtendedForgeFromZip 从 ZIP 文件加载扩展 Forge（导出名为 aim.extendedForgeFromZip）
// 参数:
//   - zipPath: ZIP 文件路径
//   - password: 可选的解压密码
//
// 返回值:
//   - 引擎配置可选项
//
// Example:
// ```
// opt = aim.extendedForgeFromZip("/tmp/forge.zip")
// println(opt)
// ```
func WithExtendedForgeFromZip(zipPath string, password ...string) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		passwd := ""
		if len(password) > 0 {
			passwd = password[0]
		}
		c.ExtendedForgeFromZip = append(c.ExtendedForgeFromZip, &ExtendedForgeZip{
			ZipPath:  zipPath,
			Password: passwd,
		})
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
