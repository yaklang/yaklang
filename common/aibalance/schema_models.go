package aibalance

// schema_models.go 收纳「仅 aibalance 使用」的 GORM 模型定义。
//
// 这些类型原先定义在 common/schema/ai_infra.go，但它们只被 aibalance 包引用，
// 不属于整个 yaklang 通用的 profile/project schema。为了避免污染全局 schema、
// 也避免它们被默认写进每个 yak 用户的 profile 数据库，这里把它们搬回 aibalance
// 包内自治管理。表名通过 TableName() / GORM 默认命名保持不变，DB 完全兼容。
//
// 关键词: aibalance schema 归位, 独属 aibalance 模型, ai_infra 拆分, 不污染全局 schema

import (
	"time"

	"github.com/jinzhu/gorm"
)

type AiProvider struct {
	gorm.Model

	WrapperName         string `json:"wrapper_name" gorm:"index"`
	ModelName           string `json:"model_name" gorm:"index"`
	TypeName            string `json:"type_name" gorm:"index"`
	DomainOrURL         string `json:"domain_or_url" gorm:"index"`
	APIKey              string `json:"api_key" gorm:"index"`
	NoHTTPS             bool   `json:"no_https"`
	ProviderMode        string `json:"provider_mode" gorm:"default:'chat'"` // Provider 模式: "chat" 或 "embedding"
	OptionalAllowReason string `json:"optional_allow_reason" gorm:"default:''"`

	// ActiveCacheControl 决定是否在路由到该 provider 之前主动给最末 system 消息
	// 注入 cache_control:{"type":"ephemeral"} baseline 标记。
	// 关键词: aibalance Provider ActiveCacheControl Flag, 显式缓存通用化, dashscope/anthropic ephemeral 缓存
	//
	// 取值语义 (与 RewriteMessagesForProviderInstance 对齐):
	//   - true  -> 该 provider 视为 cache-control aware: 客户端自带 cc 时 pass-through,
	//              客户端无 cc 时给最末 system 注入 ephemeral cc (与 model 名无关)
	//   - false -> 老路径兜底: tongyi+dashscope 白名单走显式缓存注入, 其它 provider strip cc
	//
	// 默认 false 是为了完全保留现有 tongyi+白名单 provider 行为, 不写 DB 迁移脚本。
	ActiveCacheControl bool `json:"active_cache_control" gorm:"default:false"`

	// 可用性指标
	SuccessCount  int64 `json:"success_count"`  // 成功请求总数
	FailureCount  int64 `json:"failure_count"`  // 失败请求总数
	TotalRequests int64 `json:"total_requests"` // 总请求数

	// 最后一次请求信息
	LastRequestTime   time.Time `json:"last_request_time"`   // 最后一次请求时间
	LastRequestStatus bool      `json:"last_request_status"` // 最后一次请求状态 (true=成功, false=失败)
	LastLatency       int64     `json:"last_latency"`        // 最后一次请求延迟 (毫秒)

	// 健康状态
	IsHealthy             bool      `json:"is_healthy"`                                    // 提供者是否健康
	HealthCheckTime       time.Time `json:"health_check_time"`                             // 最后一次健康检查时间
	IsFirstCheckCompleted bool      `json:"is_first_check_completed" gorm:"default:false"` // 首次健康检查是否完成

	// 工具调用兼容能力 (capability matrix v1, 仅 tool_calls 维度)
	// 关键词: aibalance Tool Calls Capability Matrix, ToolCallsRound1Mode, ToolCallsRound2Mode
	//
	// Round1 = 客户端首次带 tools=[...] 的请求 (让模型决定要不要调工具)
	// Round2 = 客户端发完整 round-trip 消息 (含 assistant.tool_calls + role=tool 回灌)
	//
	// 取值语义:
	//   ""        : unknown, 未 probe (新 provider 默认), server 走 auto-fallback (透传 + 空回检测 + react 重试)
	//   "native"  : 上游原生支持 OpenAI tool_calls 协议, 直接透传
	//   "react"   : 上游不识别 tool_calls, aibalance 在请求侧降级 (round1 tools 注入 system prompt / round2 flatten)
	//               + 响应侧反解析 [tool_call name=...]args[/tool_call] 文本回 OpenAI tool_calls 结构
	//
	// 兼容策略:
	//   - 老 113 个 provider 默认值全空, 行为完全不变 (走 unknown auto-fallback)
	//   - 运维通过 Portal 按钮 / cmd `capability probe-tools` 手动触发 probe 后落 DB
	//   - 环境变量 AIBALANCE_FLATTEN_TOOLCALLS_ALL / AIBALANCE_FLATTEN_TOOLCALLS_FOR_MODELS 仍是最高优先级
	ToolCallsRound1Mode string    `json:"tool_calls_round1_mode" gorm:"default:''"`
	ToolCallsRound2Mode string    `json:"tool_calls_round2_mode" gorm:"default:''"`
	ToolCallsProbeAt    time.Time `json:"tool_calls_probe_at"`
	ToolCallsProbeError string    `json:"tool_calls_probe_error" gorm:"default:''"`
}

type AiApiKeys struct {
	gorm.Model
	APIKey        string    `json:"api_key" gorm:"index"`
	AllowedModels string    `json:"allowed_models"`
	InputBytes    int64     `json:"input_bytes"`                // 输入字节数统计
	OutputBytes   int64     `json:"output_bytes"`               // 输出字节数统计
	UsageCount    int64     `json:"usage_count"`                // 使用次数统计
	SuccessCount  int64     `json:"success_count"`              // 成功请求数
	FailureCount  int64     `json:"failure_count"`              // 失败请求数
	LastUsedTime  time.Time `json:"last_used_time"`             // 上次使用时间
	Active        bool      `json:"active" gorm:"default:true"` // API Key 激活状态

	// Web Search 使用统计
	WebSearchCount int64 `json:"web_search_count" gorm:"default:0"` // Web Search 使用次数

	// 流量限制相关字段
	TrafficLimit       int64 `json:"traffic_limit" gorm:"default:0"`            // 流量限额(字节)，0表示不限制
	TrafficUsed        int64 `json:"traffic_used" gorm:"default:0"`             // 已使用流量(经倍数计算后)
	TrafficLimitEnable bool  `json:"traffic_limit_enable" gorm:"default:false"` // 是否启用流量限制

	// Token 维度限额（与字节维度并行，按上游 SSE 末帧 usage 经多维倍率加权后累加）
	// 关键词: AiApiKeys TokenLimit, Token 计费, 与字节计费并行
	TokenLimit       int64 `json:"token_limit" gorm:"default:0"`            // Token 限额(raw token)，0 表示不限制
	TokenUsed        int64 `json:"token_used" gorm:"default:0"`             // 已使用 Token (经四维倍率加权后)
	TokenLimitEnable bool  `json:"token_limit_enable" gorm:"default:false"` // 是否启用 Token 限制

	// Creator tracking (for OPS user audit)
	CreatedByOpsID   uint   `json:"created_by_ops_id" gorm:"index"`   // Creator OpsUser.ID (0 means admin created)
	CreatedByOpsName string `json:"created_by_ops_name" gorm:"index"` // Creator username

	// 用户绑定与管理元信息字段（为 OAuth 等外部系统接入预留）
	// 关键词: AiApiKeys Username Remark MetaInfo, OAuth 绑定, 用户名可重复
	Username string `json:"username" gorm:"index"` // 绑定用户名（可重复，用于按用户聚合查询）
	Remark   string `json:"remark" gorm:"type:text"`     // 备注（自由文本）
	MetaInfo string `json:"metainfo" gorm:"type:text"`   // 绑定信息（JSON 文本，存储 OAuth 等外部系统的用户信息）
}

type LoginSession struct {
	gorm.Model

	SessionID string    `json:"session_id" gorm:"index"`
	ExpiresAt time.Time `json:"expires_at"`

	// User information for role-based access control
	UserID   uint   `json:"user_id" gorm:"index"`             // Associated user ID (0 for root admin)
	Username string `json:"username" gorm:"index"`            // Username for quick access
	UserRole string `json:"user_role" gorm:"default:'admin'"` // User role: admin/ops
}

// OpsUser represents an operations user
type OpsUser struct {
	gorm.Model

	Username     string `json:"username" gorm:"unique_index"`          // Username
	Password     string `json:"password"`                              // Password (bcrypt encrypted)
	OpsKey       string `json:"ops_key" gorm:"unique_index"`           // ops-{uuid} format key for API access
	Role         string `json:"role" gorm:"default:'ops'"`             // Role: admin/ops
	Active       bool   `json:"active" gorm:"default:true"`            // Whether the user is active
	DefaultLimit int64  `json:"default_limit" gorm:"default:52428800"` // Default traffic limit (50MB)
}

// OpsActionLog records operations user actions
type OpsActionLog struct {
	gorm.Model

	OperatorID   uint   `json:"operator_id" gorm:"index"`   // Operator user ID
	OperatorName string `json:"operator_name" gorm:"index"` // Operator username
	Action       string `json:"action" gorm:"index"`        // Action type: create_api_key, reset_ops_key, change_password
	TargetType   string `json:"target_type"`                // Target type: api_key, ops_user
	TargetID     string `json:"target_id"`                  // Target ID
	Detail       string `json:"detail" gorm:"type:text"`    // Action detail (JSON)
	IPAddress    string `json:"ip_address"`                 // Client IP address
}

// AiBalanceRateLimitConfig stores global rate-limit configuration for AIBalance (singleton row, ID=1)
type AiBalanceRateLimitConfig struct {
	gorm.Model

	DefaultRPM          int64  `json:"default_rpm" gorm:"default:600"`         // Global default RPM per API key, default 600
	FreeUserDelaySec    int64  `json:"free_user_delay_sec" gorm:"default:3"`   // Pre-call delay (seconds) for free users, when FreeUserDelayMaxSec<=0 acts as N (legacy N~2N random)
	ModelRPMOverrides   string `json:"model_rpm_overrides" gorm:"type:text"`   // JSON map: {"model-name": rpm_int, ...}
	ModelDelayOverrides string `json:"model_delay_overrides" gorm:"type:text"` // JSON map: legacy {"model-name": delay_sec_int} or new {"model-name": {"min": int, "max": int}}; only applies to free users

	// 免费用户日 Token 限额相关字段
	// 关键词: 免费用户 Token 日限额, 全局共享池, 模型级覆盖, 模型豁免
	FreeUserTokenLimitM         int64  `json:"free_user_token_limit_m" gorm:"default:1200"`      // 免费用户全局共享日 Token 限额，单位 M tokens，默认 1200M
	FreeUserTokenModelOverrides string `json:"free_user_token_model_overrides" gorm:"type:text"` // JSON map: {"model": {"limit_m": int, "exempt": bool}}; exempt=true 表示该 -free 模型不计费

	// 付费用户全局日 Token 总额度限额（与免费日限额并列的第二道硬门）
	// 关键词: PaidUserTokenLimitM, 付费用户全局日 Token 总额度, 第二道硬门, 429 余额不足
	//
	// 聚合所有付费 API Key 当天产生的加权计费 Token；超过此上限则所有付费请求返回 429。
	// 单位 M tokens（1 RMB = 10M 计费 Token）。0 = 不限制（仅靠单 key TokenLimit 控制）。
	// 与免费日限额一样在北京时间每日 06:00 清零。
	PaidUserTokenLimitM int64 `json:"paid_user_token_limit_m" gorm:"default:0"` // 付费用户全局共享日 Token 总额度，单位 M tokens，0=不限制

	// 免费延迟 N~M 区间随机改造
	// 关键词: FreeUserDelayMaxSec, N~M 随机延迟, 老 N~2N 兼容
	FreeUserDelayMaxSec int64 `json:"free_user_delay_max_sec" gorm:"default:0"` // 调用前延迟上限（秒）。0 时按老语义 N~2N

	// 输出 Token Per Second 限速
	// 关键词: FreeUserOutputTPS, ModelOutputTPSOverrides, token-per-second 节流
	FreeUserOutputTPS       int64  `json:"free_user_output_tps" gorm:"default:0"`       // 免费用户全局输出 TPS 上限，0 = 不限速
	ModelOutputTPSOverrides string `json:"model_output_tps_overrides" gorm:"type:text"` // JSON map: {"model-name": tps_int, ...}

	// 全局共享池软限额（仅影响免费模型走共享池的请求；模型独立桶不受影响）
	// 关键词: FreeUserTokenSoftLimitM, FreeUserSoftLimitTPS, 软限额 TPS 限速
	FreeUserTokenSoftLimitM int64 `json:"free_user_token_soft_limit_m" gorm:"default:0"` // 软限额阈值（M token），>0 时启用
	FreeUserSoftLimitTPS    int64 `json:"free_user_soft_limit_tps" gorm:"default:0"`     // 软限额触发后的输出 TPS，0 = 不限速

	// memfit-* 模型客户端版本控流配置
	// 关键词: memfit 版本控流, 客户端版本网关, MemfitVersionGate
	MemfitVersionGateEnabled  bool   `json:"memfit_version_gate_enabled" gorm:"default:false"` // 是否启用 memfit-* 客户端版本控流（默认关闭）
	MemfitVersionMinBuildTime string `json:"memfit_version_min_build_time" gorm:"type:text"`   // 允许的最早客户端 BuildTime（RFC3339 字符串，空表示不按时间过滤）

	// 自定义限流 / 错误返回文案配置
	// 关键词: Custom429 自定义限流文案, 429 notice, 按 limit_kind 覆盖
	//
	//   - Custom429Enabled       关闭时（默认）完全保持现有 429 文案，开启后才注入自定义内容
	//   - Custom429Notice        全局总文案，注入到所有限流 429 JSON body 的 notice 字段
	//   - Custom429KindOverrides JSON map：按 limit_kind 覆盖文案，键取值见 custom_429.go Custom429Kinds：
	//                            {"rpm":"...","token":"...","daily_token":"...","free_ip":"...","paid_daily_token":"...","memfit_version":"..."}
	//                            按 limit_kind 覆盖对应 message
	Custom429Enabled       bool   `json:"custom_429_enabled" gorm:"default:false"`
	Custom429Notice        string `json:"custom_429_notice" gorm:"type:text"`
	Custom429KindOverrides string `json:"custom_429_kind_overrides" gorm:"type:text"`

	// 模型用途降级规则（保护用量）
	// 关键词: ModelDowngradeRules, tier 降级, 轻量模型保护, memfit-standard-free 降级
	//
	// JSON 数组：[{"tier":"lightweight","from":"memfit-standard-free","to":"memfit-light-free"}]
	// server 在解析出 modelName 后，按请求头 X-Yak-AI-Model-Usage-Type 命中规则即改写 modelName。
	ModelDowngradeRules string `json:"model_downgrade_rules" gorm:"type:text"`

	// 单 IP 免费模型每日用量限额（保护公共免费接口公平，防单 IP 高频盗刷）
	// 关键词: FreeUserIPLimit, 单 IP 每日限额, 公共免费接口公平, 防盗刷
	//
	//   - FreeUserIPLimitEnable       是否启用单 IP 每日限额（默认开启）
	//   - FreeUserIPDailyRequestLimit 每个 IP 每日「计费免费模型」请求次数上限，0 = 不限
	//   - FreeUserIPDailyTokenLimitM  每个 IP 每日「计费免费模型」加权 Token 上限（M 单位），0 = 不限
	//
	// 仅统计计费免费模型（-free 后缀且未被 exempt 标记）；切日点与免费 Token 限额一致（北京时间每日 06:00）。
	FreeUserIPLimitEnable       bool  `json:"free_user_ip_limit_enable" gorm:"default:true"`
	FreeUserIPDailyRequestLimit int64 `json:"free_user_ip_daily_request_limit" gorm:"default:500"`
	FreeUserIPDailyTokenLimitM  int64 `json:"free_user_ip_daily_token_limit_m" gorm:"default:30"`

	// 一键限流 IP 的默认参数（管理员在面板「频率与速率」配置里可改）
	// 关键词: ThrottledIPDefaultRPM, ThrottledIPDefaultTPS, 一键限流默认值
	//
	// 一键限流某个 IP 时套用的默认 RPM（按 IP 维度的请求频率上限）与输出 TPS（流式 token/秒）。
	// 实际被限流的 IP 列表持久化在 AiBalanceThrottledIP 表，每条可单独覆盖这两个值。
	ThrottledIPDefaultRPM int64 `json:"throttled_ip_default_rpm" gorm:"default:3"`  // 一键限流默认 RPM（<=0 兜底 3）
	ThrottledIPDefaultTPS int64 `json:"throttled_ip_default_tps" gorm:"default:15"` // 一键限流默认输出 TPS（<=0 兜底 15）
}

func (a *AiBalanceRateLimitConfig) TableName() string {
	return "ai_balance_rate_limit_configs"
}

// AiBalanceClientVersionStat 记录 aibalance 客户端按 Yak 版本聚合的请求统计。
// 仅在 memfit-* 模型请求路径上写入；用于 portal 展示 Top N 客户端版本分布和最早出现时间。
// 关键词: AiBalanceClientVersionStat 客户端版本统计, version upsert, ai_balance_client_versions 表
type AiBalanceClientVersionStat struct {
	gorm.Model

	Version       string `json:"version" gorm:"uniqueIndex;not null"` // X-Yak-Version 值（dev/unknown/v1.2.3 等）
	BuildTime     string `json:"build_time"`                          // 最近一次上报的 X-Yak-Build-Time（原样保留）
	FirstSeenUnix int64  `json:"first_seen_unix"`                     // 首次见到该版本的 Unix 时间
	LastSeenUnix  int64  `json:"last_seen_unix" gorm:"index"`         // 最近一次见到该版本的 Unix 时间
	RequestCount  int64  `json:"request_count"`                       // 累计请求数
}

func (a *AiBalanceClientVersionStat) TableName() string {
	return "ai_balance_client_versions"
}

// WebSearchConfig stores global configuration for web search (singleton row, ID=1)
type WebSearchConfig struct {
	gorm.Model

	Proxy                  string `json:"proxy"`                      // Global proxy for all web search requests
	AllowFreeUserWebSearch bool   `json:"allow_free_user_web_search"` // Allow free users (Trace-ID only, no API key) to use web-search
	TotalWebSearchRequests int64  `json:"total_web_search_requests"`  // Persistent cumulative web-search request count (survives restarts)
}

func (w *WebSearchConfig) TableName() string {
	return "web_search_configs"
}

// WebSearchApiKey stores API keys for web search providers (Brave, Tavily, ChatGLM, Bocha, Unifuncs)
type WebSearchApiKey struct {
	gorm.Model

	SearcherType string `json:"searcher_type" gorm:"index"` // "brave", "tavily", "chatglm", "bocha" or "unifuncs"
	APIKey       string `json:"api_key"`
	BaseURL      string `json:"base_url"`                   // Optional custom base URL
	Proxy        string `json:"proxy"`                      // Optional proxy
	Active       bool   `json:"active" gorm:"default:true"` // Whether the key is active

	// Statistics
	SuccessCount        int64     `json:"success_count"`
	FailureCount        int64     `json:"failure_count"`
	ConsecutiveFailures int64     `json:"consecutive_failures"` // Reset to 0 on success, incremented on failure
	TotalRequests       int64     `json:"total_requests"`
	LastUsedTime        time.Time `json:"last_used_time"`
	LastLatency         int64     `json:"last_latency"` // Milliseconds
	IsHealthy           bool      `json:"is_healthy" gorm:"default:true"`
}

// AmapConfig stores global configuration for Amap API proxy (singleton row, ID=1)
type AmapConfig struct {
	gorm.Model

	AllowFreeUserAmap bool  `json:"allow_free_user_amap"` // Allow free users (TOTP only) to use amap proxy
	TotalAmapRequests int64 `json:"total_amap_requests"`  // Persistent cumulative amap request count (survives restarts)
}

func (a *AmapConfig) TableName() string {
	return "amap_configs"
}

// AmapApiKey stores API keys for Amap (Gaode Maps) API proxy
type AmapApiKey struct {
	gorm.Model

	APIKey string `json:"api_key"`
	Active bool   `json:"active" gorm:"default:true"` // Whether the key is active

	// Health check
	IsHealthy       bool      `json:"is_healthy" gorm:"default:true"`
	HealthCheckTime time.Time `json:"health_check_time"` // Last health check time
	LastCheckError  string    `json:"last_check_error"`  // Last check error message, empty means success

	// Statistics
	SuccessCount        int64     `json:"success_count"`
	FailureCount        int64     `json:"failure_count"`
	ConsecutiveFailures int64     `json:"consecutive_failures"` // Reset to 0 on success, incremented on failure
	TotalRequests       int64     `json:"total_requests"`
	LastUsedTime        time.Time `json:"last_used_time"`
	LastLatency         int64     `json:"last_latency"` // Milliseconds
}

func (a *AmapApiKey) TableName() string {
	return "amap_api_keys"
}

// AiProviderHealthRecord stores historical health check results for uptime tracking
type AiProviderHealthRecord struct {
	gorm.Model

	ProviderID   uint      `json:"provider_id" gorm:"index"`
	WrapperName  string    `json:"wrapper_name" gorm:"index"`
	IsHealthy    bool      `json:"is_healthy"`
	LatencyMs    int64     `json:"latency_ms"`
	CheckTime    time.Time `json:"check_time" gorm:"index"`
	ErrorMessage string    `json:"error_message" gorm:"type:text"`
}

func (a *AiProviderHealthRecord) TableName() string {
	return "ai_provider_health_records"
}

// AiDailyCacheStat 是 aibalance 每日「细粒度上游 token 用量与缓存命中」聚合行。
// 主键唯一约束 (date + wrapper_name + model_name + provider_type + provider_domain + api_key_hash)
// 由 RecordDailyCacheStats 用 UPSERT (gorm.Expr + ?) 累加。
// 180 天前的记录由 cleanup_scheduler 每天 0:01 删除，避免 SQLite 膨胀。
// 关键词: ai_daily_cache_stats, cached_tokens 持久化, aibalance 缓存命中比例
type AiDailyCacheStat struct {
	gorm.Model

	Date             string `json:"date" gorm:"size:10;unique_index:idx_cache_unique;index:idx_cache_date;not null"`
	WrapperName      string `json:"wrapper_name" gorm:"size:128;unique_index:idx_cache_unique;not null"`
	ModelName        string `json:"model_name" gorm:"size:128;unique_index:idx_cache_unique;not null"`
	ProviderTypeName string `json:"provider_type_name" gorm:"size:64;unique_index:idx_cache_unique;not null"`
	ProviderDomain   string `json:"provider_domain" gorm:"size:255;unique_index:idx_cache_unique;not null"`
	APIKeyHash       string `json:"api_key_hash" gorm:"size:32;unique_index:idx_cache_unique;not null"`
	APIKeyShrink     string `json:"api_key_shrink" gorm:"size:32;not null"`

	RequestCount     int64 `json:"request_count"`
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
	CachedTokens     int64 `json:"cached_tokens"`
}

func (a *AiDailyCacheStat) TableName() string {
	return "ai_daily_cache_stats"
}

// AiDailyUserSeen 是 aibalance 每日「客户端身份指纹去重」表。
// 一行 = 当天某 source_kind 下首次出现的 user_hash 指纹（INSERT IGNORE 重复不增行）。
// source_kind 取 "api_key" / "free_trace" / "free_ip" 三类。
// QueryDAU60Days 用 GROUP BY date,source_kind 统计 COUNT(DISTINCT user_hash)。
// 180 天前记录由 cleanup_scheduler 删除；同时 RecordDailyUserSeen 内置
// 1,000,000 行/天/source_kind 的硬上限做 DB 防爆。
// 关键词: ai_daily_user_seen, DAU 去重指纹, 防爆 cap
type AiDailyUserSeen struct {
	gorm.Model

	Date       string    `json:"date" gorm:"size:10;unique_index:idx_seen_unique;index:idx_seen_date;not null"`
	SourceKind string    `json:"source_kind" gorm:"size:16;unique_index:idx_seen_unique;not null"`
	UserHash   string    `json:"user_hash" gorm:"size:32;unique_index:idx_seen_unique;not null"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

func (a *AiDailyUserSeen) TableName() string {
	return "ai_daily_user_seen"
}

// AiDailySummary 是 aibalance 每日「极轻量聚合快照」表。
// 一天一行（Date 唯一），由内存 atomic 累加 + 后台 30s tick flush，
// 永久保留（每天 1 行，年增长 365 行，长期可忽略）。
// 关键词: ai_daily_summary, 每日聚合快照, 内存 atomic flush
type AiDailySummary struct {
	gorm.Model

	Date             string `json:"date" gorm:"size:10;unique_index;not null"`
	TotalRequests    int64  `json:"total_requests"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	CachedTokens     int64  `json:"cached_tokens"`
}

func (a *AiDailySummary) TableName() string {
	return "ai_daily_summary"
}

// FreeUserDailyTokenUsage 持久化免费用户「每日 Token 已用量」聚合行。
// 一行 = (date, model_name) 唯一；model_name == "" 表示「全局共享池」。
// 跨日由首次访问时 Date != today 触发归零（轻量、无定时任务）。
// 关键词: free_user_daily_token_usage, 免费 Token 限额, 全局/模型桶
type FreeUserDailyTokenUsage struct {
	gorm.Model

	Date       string `json:"date" gorm:"size:10;unique_index:idx_free_token;not null"`
	ModelName  string `json:"model_name" gorm:"size:128;unique_index:idx_free_token;not null"`
	TokensUsed int64  `json:"tokens_used"`
}

func (a *FreeUserDailyTokenUsage) TableName() string {
	return "free_user_daily_token_usage"
}

// FreeUserIPDailyUsage 持久化单个客户端 IP 的「每日免费模型用量」聚合行。
// 一行 = (date, ip) 唯一；聚合该 IP 当天所有「计费免费模型」的请求数与加权 Token。
// 用以限制单 IP 高频盗刷公共免费接口，保证免费额度对所有用户公平。
// 跨日由 date 维度天然拆分；旧日数据由 cleanup 任务每日清理（保留窗很短，仅够面板看今日），
// 避免按 IP 维度展开导致 DB 行数膨胀。
// 关键词: free_user_ip_daily_usage, 单 IP 每日免费用量, 防盗刷, 公平限额
type FreeUserIPDailyUsage struct {
	gorm.Model

	Date         string    `json:"date" gorm:"size:10;unique_index:idx_free_ip;index:idx_free_ip_date;not null"`
	IP           string    `json:"ip" gorm:"size:64;unique_index:idx_free_ip;index:idx_free_ip_ip;not null"`
	RequestCount int64     `json:"request_count"`
	TokensUsed   int64     `json:"tokens_used"`
	LastSeenAt   time.Time `json:"last_seen_at"`
}

func (a *FreeUserIPDailyUsage) TableName() string {
	return "free_user_ip_daily_usage"
}

// AiBalanceThrottledIP 持久化「被一键限流的客户端 IP」及其生效的 RPM / 输出 TPS 上限。
// 一行 = 一个被限流的 IP（IP 唯一）。与按日重置的免费 IP 用量表不同：限流是管理员
// 主动施加的持久动作，不随每日切日清空，需在面板手动解除（删除该行）。
// 命中后：按 IP 维度的 RPM 滑动窗口（独立于 apiKey|model 桶）+ 流式输出 TPS 限速同时生效。
// 关键词: ai_balance_throttled_ips, 一键限流 IP, per-IP RPM/TPS, 持久限流
type AiBalanceThrottledIP struct {
	gorm.Model

	IP     string `json:"ip" gorm:"size:64;unique_index;not null"` // 被限流的客户端 IP
	RPM    int64  `json:"rpm"`                                     // 该 IP 的请求频率上限（每分钟），<=0 表示不限 RPM
	TPS    int64  `json:"tps"`                                     // 该 IP 流式输出 TPS 上限（token/秒），<=0 表示不限 TPS
	Reason string `json:"reason" gorm:"type:text"`                 // 限流原因 / 备注（可选）
}

func (a *AiBalanceThrottledIP) TableName() string {
	return "ai_balance_throttled_ips"
}

// ==================== B 类表自动迁移 ====================
//
// AiProvider / AiApiKeys / LoginSession / OpsUser / OpsActionLog 原先登记在
// schema.ProfileTables 由全局自动迁移；搬回 aibalance 后改为在 aibalance 启动
// （LoadProvidersFromDatabase）时按需迁移，避免污染普通 yak 用户的 profile 库。
// 关键词: aibalance B 类表自治迁移, EnsureProviderTable, 不进 ProfileTables

// EnsureProviderTable ensures the AiProvider table exists.
func EnsureProviderTable() error {
	return GetDB().AutoMigrate(&AiProvider{}).Error
}

// EnsureApiKeysTable ensures the AiApiKeys table exists.
func EnsureApiKeysTable() error {
	return GetDB().AutoMigrate(&AiApiKeys{}).Error
}

// EnsureLoginSessionTable ensures the LoginSession table exists.
func EnsureLoginSessionTable() error {
	return GetDB().AutoMigrate(&LoginSession{}).Error
}

// EnsureOpsUserTable ensures the OpsUser table exists.
func EnsureOpsUserTable() error {
	return GetDB().AutoMigrate(&OpsUser{}).Error
}

// EnsureOpsActionLogTable ensures the OpsActionLog table exists.
func EnsureOpsActionLogTable() error {
	return GetDB().AutoMigrate(&OpsActionLog{}).Error
}
