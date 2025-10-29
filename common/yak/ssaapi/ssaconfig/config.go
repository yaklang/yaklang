package ssaconfig

import (
	"context"
)

type Config struct {
	ctx            context.Context
	Mode           Mode
	BaseInfo       *BaseInfo
	CodeSource     *CodeSourceInfo
	SSACompile     *SSACompileConfig
	SyntaxFlow     *SyntaxFlowConfig
	SyntaxFlowScan *SyntaxFlowScanConfig
	SyntaxFlowRule *SyntaxFlowRuleConfig

	// 其他配置项可以在这里添加
	ExtraInfo map[string][]any `json:"-"` // 用于存储外部传入的其他信息
}

type Option func(*Config) error

type Mode int

const (
	ModeProjectBase           Mode = 1 << iota // 0 - 基础模式
	ModeSSACompile            Mode = 1 << iota // 1 - 编译模式
	ModeSyntaxFlowScanManager Mode = 1 << iota // 2 - 扫描管理器模式
	ModeSyntaxFlow            Mode = 1 << iota // 3 - SyntaxFlow模式
	ModeSyntaxFlowRule        Mode = 1 << iota // 4 - 规则模式
	ModeCodeSource            Mode = 1 << iota // 5 - 源码配置模式

	ModeSyntaxFlowScan Mode = ModeProjectBase | ModeSyntaxFlow | ModeSyntaxFlowRule | ModeSyntaxFlowScanManager
	// all
	ModeAll = ModeProjectBase | ModeSSACompile | ModeSyntaxFlow | ModeSyntaxFlowRule | ModeCodeSource | ModeSyntaxFlowScanManager
)

func New(mode Mode, opts ...Option) (*Config, error) {
	cfg := &Config{
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

func (c *Config) IsSyntaxFlowScanConfig() bool {
	return c.Mode == ModeSyntaxFlowScan
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

func WithContext(ctx context.Context) Option {
	return func(c *Config) error {
		c.ctx = ctx
		return nil
	}
}

func (c *Config) GetContext() context.Context {
	if c == nil {
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
	with := func(value TValue) Option {
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
	return with
}
