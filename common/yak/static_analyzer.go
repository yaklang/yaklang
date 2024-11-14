package yak

import (
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/result"
)

type StaticAnalyzeConfig struct {
	kind       static_analyzer.StaticAnalyzeKind
	pluginType string
}

func NewStaticAnalyzeConfig() *StaticAnalyzeConfig {
	return &StaticAnalyzeConfig{
		kind:       static_analyzer.Analyze,
		pluginType: "yak",
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

func StaticAnalyze(code string, opts ...StaticAnalyzeOption) []*result.StaticAnalyzeResult {
	config := NewStaticAnalyzeConfig()
	for _, opt := range opts {
		opt(config)
	}
	return static_analyzer.StaticAnalyze(code, config.pluginType, config.kind)
}
