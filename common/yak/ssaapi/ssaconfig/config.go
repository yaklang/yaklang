package ssaconfig

import (
	"context"
	"encoding/json"
	"errors"
)

type Config struct {
	ctx            context.Context       `json:"-"`
	Mode           Mode                  `json:"Mode,omitempty"` // 配置模式，可以从 JSON 指定
	BaseInfo       *BaseInfo             `json:"BaseInfo,omitempty"`
	CodeSource     *CodeSourceInfo       `json:"CodeSource,omitempty"`
	SSACompile     *SSACompileConfig     `json:"SSACompile,omitempty"`
	SyntaxFlow     *SyntaxFlowConfig     `json:"SyntaxFlow,omitempty"`
	SyntaxFlowScan *SyntaxFlowScanConfig `json:"SyntaxFlowScan,omitempty"`
	SyntaxFlowRule *SyntaxFlowRuleConfig `json:"SyntaxFlowRule,omitempty"`
	Output         *OutputConfig         `json:"Output,omitempty"`

	// 其他配置项可以在这里添加
	ExtraInfo map[string][]any `json:"-"` // 用于存储外部传入的其他信息
}

type Option func(*Config) error

type Mode int

const (
	ModeProjectBase           Mode = 1 << iota // - 基础模式
	modeSSACompile            Mode = 1 << iota // - 编译模式
	ModeSyntaxFlowScanManager Mode = 1 << iota // - 扫描管理器模式
	ModeSyntaxFlow            Mode = 1 << iota // - SyntaxFlow模式
	ModeSyntaxFlowRule        Mode = 1 << iota // - 规则模式
	ModeCodeSource            Mode = 1 << iota // - 源码配置模式
	ModeOutput                Mode = 1 << iota // - CLI输出配置模式

	ModeSSACompile = ModeProjectBase | modeSSACompile | ModeCodeSource

	ModeSyntaxFlowScan Mode = ModeProjectBase | ModeSyntaxFlow | ModeSyntaxFlowRule | ModeSyntaxFlowScanManager
	ModeProjectCompile      = ModeProjectBase | ModeCodeSource | modeSSACompile
	// all
	ModeAll = ModeProjectBase | modeSSACompile | ModeSyntaxFlow | ModeSyntaxFlowRule | ModeCodeSource | ModeSyntaxFlowScanManager | ModeOutput
)

func New(mode Mode, opts ...Option) (*Config, error) {
	cfg := &Config{
		ctx:       context.Background(),
		ExtraInfo: map[string][]any{},
	}
	cfg.Mode = mode
	// New intentionally does not eagerly initialize nested config structs.
	// With... option functions should check c.Mode and create defaults when needed.
	for _, opt := range opts {
		if err := opt(cfg); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

func NewSyntaxFlowScanConfig(opts ...Option) (*Config, error) {
	return New(ModeSyntaxFlowScan, opts...)
}

// NewCLIScanConfig 创建CLI扫描配置，包含编译、扫描、输出等完整功能
func NewCLIScanConfig(opts ...Option) (*Config, error) {
	return New(ModeAll, opts...)
}

func (c *Config) IsSyntaxFlowScanConfig() bool {
	return c.Mode == ModeSyntaxFlowScan
}

func (c *Config) ToJSONRaw() ([]byte, error) {
	if c == nil {
		return nil, nil
	}
	return json.Marshal(c)
}

func (c *Config) ToJSONString() (string, error) {
	if c == nil {
		return "", nil
	}
	data, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (c *Config) Update(options ...Option) error {
	if c == nil {
		return nil
	}
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(c); err != nil {
			return err
		}
	}
	return nil
}

// default factory functions - used by With... option helpers to create nested configs
func defaultCodeSourceConfig() *CodeSourceInfo {
	return &CodeSourceInfo{}
}

func defaultSSACompileConfig() *SSACompileConfig {
	return &SSACompileConfig{
		StrictMode:    false,
		PeepholeSize:  0,
		ExcludeFiles:  []string{},
		ReCompile:     false,
		MemoryCompile: false,
		Concurrency:   1,
	}
}

func defaultSyntaxFlowConfig() *SyntaxFlowConfig {
	return &SyntaxFlowConfig{
		Memory:         false,
		ResultSaveKind: SFResultSaveNone,
	}
}

func defaultSyntaxFlowScanConfig() *SyntaxFlowScanConfig {
	return &SyntaxFlowScanConfig{
		IgnoreLanguage: false,
		Concurrency:    5,
	}
}

func defaultBaseInfo() *BaseInfo {
	return &BaseInfo{}
}

func defaultSyntaxFlowRuleConfig() *SyntaxFlowRuleConfig {
	return &SyntaxFlowRuleConfig{}
}

func defaultOutputConfig() *OutputConfig {
	return &OutputConfig{
		OutputFormat: "sarif", // 默认输出格式
	}
}

// --- Context Get/Set 方法 ---

func WithContext(ctx context.Context) Option {
	return func(c *Config) error {
		c.ctx = ctx
		return nil
	}
}

func (c *Config) GetContext() context.Context {
	if c == nil || c.ctx == nil {
		return context.Background()
	}
	return c.ctx
}

func (c *Config) IsContextCancel() bool {
	if c == nil || c.ctx == nil {
		return false
	}
	select {
	case <-c.ctx.Done():
		return true
	default:
		return false
	}
}

// ## ------------------- extra option helper ------------------- ##

// --- ExtraInfo 扩展信息 Get/Set 方法 ---

func (c *Config) GetExtraInfo(key string) ([]any, bool) {
	if c == nil || c.ExtraInfo == nil {
		return nil, false
	}
	val, ok := c.ExtraInfo[key]
	return val, ok
}

func (c *Config) SetExtraInfo(key string, value any) {
	if c == nil {
		return
	}
	if c.ExtraInfo == nil {
		c.ExtraInfo = map[string][]any{}
	}
	c.ExtraInfo[key] = append(c.ExtraInfo[key], value)
}

type ExtraOption[C any] struct {
	fn    func(C, any)
	value any
}

func ApplyExtraOptions[C any](config C, c *Config) {
	for name, option := range c.ExtraInfo {
		_ = name
		for _, option := range option {
			if extraOpt, ok := option.(ExtraOption[C]); ok {
				extraOpt.fn(config, extraOpt.value)
			}
		}
	}
}

// type WithFunction[T any] func(T) Option

func SetOption[TValue, TCache any](
	name string,
	fn func(TCache, TValue),
) func(TValue) Option {
	return func(value TValue) Option {
		return func(c *Config) error {
			c.SetExtraInfo(name, ExtraOption[TCache]{
				fn: func(u TCache, a any) {
					if v, ok := a.(TValue); ok {
						fn(u, v)
					}
				},
				value: value,
			})
			return nil
		}
	}
}

// ## -------------------- json

func (c *Config) JSON() string {
	jsonBytes, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	return string(jsonBytes)
}

func LoadConfigFromJSON(data []byte) (*Config, error) {
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	if c.Mode == 0 {
		return nil, errors.New("mode not specified in config json file")
	}
	return &c, nil
}

func WithConfigJson(jsonStr string) Option {
	return func(c *Config) error {
		var temp Config
		if err := json.Unmarshal([]byte(jsonStr), &temp); err != nil {
			return err
		}
		*c = temp
		return nil
	}
}

func WithJsonRawConfig(raw []byte) Option {
	return func(c *Config) error {
		if raw == nil {
			return nil
		}
		err := json.Unmarshal(raw, &c)
		if err != nil {
			return err
		}
		return nil
	}
}
