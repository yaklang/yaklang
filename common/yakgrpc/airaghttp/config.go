package airaghttp

import (
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"gopkg.in/yaml.v2"
)

// AIServiceConfig AI 服务配置
// 当三个字段全部为空时, 走全局分级 aiconfig (TieredAIConfig); 任意一个非空则切换为
// 单 callback 覆盖模式 (见 server.go buildAIEngineOptions).
type AIServiceConfig struct {
	Type   string `yaml:"type"`
	Model  string `yaml:"model"`
	APIKey string `yaml:"api_key"`
	Domain string `yaml:"domain"`
}

// RAGServerConfig RAG HTTP 服务的完整配置
// 可由 rag-server.yaml 加载, 也可由命令行参数覆盖.
type RAGServerConfig struct {
	// HTTP 监听配置
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	RoutePrefix string `yaml:"route_prefix"`

	// 认证: 空 = 不认证; 非空 = 要求 Authorization: Bearer <token>
	AuthToken string `yaml:"auth_token"`

	// 并发与超时控制
	Concurrent   int `yaml:"concurrent"`
	Timeout      int `yaml:"timeout"`
	MaxIteration int `yaml:"max_iteration"`

	// 回答语言偏好
	Language string `yaml:"language"`

	// 知识库来源
	// Collections 为空时使用 profile DB 内全部已存在集合.
	Collections []string `yaml:"collections"`
	// RagFiles 启动时导入的本地 .rag 文件.
	RagFiles []string `yaml:"rag_files"`

	// AI 服务配置
	AI AIServiceConfig `yaml:"ai"`

	// AITier 走全局分级 aiconfig 时的模型档位偏好.
	// "basic"/"lightweight"(默认) = 全程使用轻量模型, 更省 token;
	// "standard"/"intelligent" = 沿用分级默认(质量优先用高质模型).
	// 仅在未配置自定义 AI (UseCustomAIConfig=false) 时生效.
	AITier string `yaml:"ai_tier"`

	// ServeFrontend 是否在根路径 serve 内置只读搜索前端页面 (默认 true)
	ServeFrontend bool `yaml:"serve_frontend"`

	// Debug 调试模式 (打印 prompt/event)
	Debug bool `yaml:"debug"`
}

// NewDefaultConfig 返回默认配置
// 关键词: rag-server default config, port 9093
func NewDefaultConfig() *RAGServerConfig {
	return &RAGServerConfig{
		Host:         "0.0.0.0",
		Port:         9093,
		RoutePrefix:  "/api/rag-server",
		AuthToken:    "",
		Concurrent:   3,
		Timeout:      180,
		MaxIteration: 1,
		Language:     "zh",
		Collections:  nil,
		RagFiles:     nil,
		AI: AIServiceConfig{
			Type:   "aibalance",
			Model:  "",
			APIKey: "",
			Domain: "",
		},
		AITier:        "basic",
		ServeFrontend: true,
	}
}

// LoadConfigFromFile 从 yaml 文件加载配置 (缺省字段以默认值兜底)
// 关键词: rag-server.yaml load, yaml.Unmarshal, default fallback
func LoadConfigFromFile(path string) (*RAGServerConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, utils.Errorf("read config file failed: %v", err)
	}

	cfg := NewDefaultConfig()
	if err := yaml.Unmarshal(raw, cfg); err != nil {
		return nil, utils.Errorf("parse config yaml failed: %v", err)
	}
	cfg.fillDefaults()
	return cfg, nil
}

// fillDefaults 为关键字段补齐默认值, 避免 yaml 缺省导致非法监听等问题
func (c *RAGServerConfig) fillDefaults() {
	if c.Host == "" {
		c.Host = "0.0.0.0"
	}
	if c.Port <= 0 {
		c.Port = 9093
	}
	if c.RoutePrefix == "" {
		c.RoutePrefix = "/api/rag-server"
	}
	if c.Concurrent <= 0 {
		c.Concurrent = 3
	}
	if c.Timeout <= 0 {
		c.Timeout = 180
	}
	if c.MaxIteration <= 0 {
		c.MaxIteration = 1
	}
	if c.Language == "" {
		c.Language = "zh"
	}
	if c.AI.Type == "" {
		c.AI.Type = "aibalance"
	}
	if c.AITier == "" {
		c.AITier = "basic"
	}
}

// UseBasicTier 判断是否全程使用轻量(basic)模型档位
// 关键词: basic tier default, lightweight model, save token
func (c *RAGServerConfig) UseBasicTier() bool {
	switch strings.ToLower(strings.TrimSpace(c.AITier)) {
	case "standard", "intelligent", "quality", "high":
		return false
	default:
		// basic / lightweight / 空 = 默认走轻量档
		return true
	}
}

// UseCustomAIConfig 判断是否使用自定义单 callback AI 配置 (覆盖全局分级 aiconfig)
// 关键词: custom ai config detection, tiered vs single callback
func (c *RAGServerConfig) UseCustomAIConfig() bool {
	return c.AI.Model != "" || c.AI.APIKey != "" || c.AI.Domain != ""
}

// Option 配置覆盖项, 主要用于命令行参数覆盖 yaml 配置
type Option func(*RAGServerConfig)

func WithHost(host string) Option {
	return func(c *RAGServerConfig) {
		if host != "" {
			c.Host = host
		}
	}
}

func WithPort(port int) Option {
	return func(c *RAGServerConfig) {
		if port > 0 {
			c.Port = port
		}
	}
}

func WithRoutePrefix(prefix string) Option {
	return func(c *RAGServerConfig) {
		if prefix != "" {
			c.RoutePrefix = prefix
		}
	}
}

func WithAuthToken(token string) Option {
	return func(c *RAGServerConfig) {
		if token != "" {
			c.AuthToken = token
		}
	}
}

func WithConcurrent(n int) Option {
	return func(c *RAGServerConfig) {
		if n > 0 {
			c.Concurrent = n
		}
	}
}

func WithTimeout(sec int) Option {
	return func(c *RAGServerConfig) {
		if sec > 0 {
			c.Timeout = sec
		}
	}
}

func WithMaxIteration(n int) Option {
	return func(c *RAGServerConfig) {
		if n > 0 {
			c.MaxIteration = n
		}
	}
}

func WithLanguage(lang string) Option {
	return func(c *RAGServerConfig) {
		if lang != "" {
			c.Language = lang
		}
	}
}

func WithCollections(names ...string) Option {
	return func(c *RAGServerConfig) {
		if len(names) > 0 {
			c.Collections = names
		}
	}
}

func WithRagFiles(files ...string) Option {
	return func(c *RAGServerConfig) {
		if len(files) > 0 {
			c.RagFiles = files
		}
	}
}

func WithAIService(typ, model, apiKey, domain string) Option {
	return func(c *RAGServerConfig) {
		if typ != "" {
			c.AI.Type = typ
		}
		if model != "" {
			c.AI.Model = model
		}
		if apiKey != "" {
			c.AI.APIKey = apiKey
		}
		if domain != "" {
			c.AI.Domain = domain
		}
	}
}

func WithDebug(debug bool) Option {
	return func(c *RAGServerConfig) {
		c.Debug = debug
	}
}

func WithServeFrontend(enable bool) Option {
	return func(c *RAGServerConfig) {
		c.ServeFrontend = enable
	}
}

func WithAITier(tier string) Option {
	return func(c *RAGServerConfig) {
		if tier != "" {
			c.AITier = tier
		}
	}
}
