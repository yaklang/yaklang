package yak

import (
	"context"

	"github.com/yaklang/yaklang/common/yak/static_analyzer"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/result"
)

type StaticAnalyzeConfig struct {
	kind       static_analyzer.StaticAnalyzeKind
	pluginType string
	ctx        context.Context
}

func NewStaticAnalyzeConfig() *StaticAnalyzeConfig {
	return &StaticAnalyzeConfig{
		kind:       static_analyzer.Analyze,
		pluginType: "yak",
		ctx:        context.Background(),
	}
}

type StaticAnalyzeOption func(*StaticAnalyzeConfig)

func WithStaticAnalyzeKindScore() StaticAnalyzeOption {
	return func(c *StaticAnalyzeConfig) {
		c.kind = static_analyzer.Score
	}
}

func WithStaticAnalyzeKindAnalyze() StaticAnalyzeOption {
	return func(c *StaticAnalyzeConfig) {
		c.kind = static_analyzer.Analyze
	}
}

func WithStaticAnalyzePluginType(typ string) StaticAnalyzeOption {
	return func(c *StaticAnalyzeConfig) {
		c.pluginType = typ
	}
}

func WithStaticAnalyzeContext(ctx context.Context) StaticAnalyzeOption {
	return func(c *StaticAnalyzeConfig) {
		c.ctx = ctx
	}
}

func StaticAnalyze(code string, opts ...StaticAnalyzeOption) []*result.StaticAnalyzeResult {
	config := NewStaticAnalyzeConfig()
	for _, opt := range opts {
		opt(config)
	}
	return static_analyzer.StaticAnalyzeWithContext(config.ctx, code, config.pluginType, config.kind)
}
