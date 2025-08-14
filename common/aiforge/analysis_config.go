package aiforge

import (
	"context"
	"github.com/yaklang/yaklang/common/aireducer"
	"github.com/yaklang/yaklang/common/log"
)

type AnalysisConfig struct {
	Ctx             context.Context
	ExtraPrompt     string
	AnalyzeLog      func(format string, args ...interface{})
	fallbackOptions []any
}

func NewAnalysisConfig(opts ...any) *AnalysisConfig {
	cfg := &AnalysisConfig{
		ExtraPrompt: "",
		AnalyzeLog: func(format string, args ...interface{}) {
			log.Infof(format, args...)
		},
		fallbackOptions: []any{},
		Ctx:             context.Background(),
	}

	for _, opt := range opts {
		if optFunc, ok := opt.(AnalysisOption); ok {
			optFunc(cfg)
		} else {
			cfg.fallbackOptions = append(cfg.fallbackOptions, opt)
		}
	}
	return cfg
}

func (a *AnalysisConfig) ReducerOptions() []aireducer.Option {
	var options []aireducer.Option
	for _, opt := range a.fallbackOptions {
		if optFunc, ok := opt.(aireducer.Option); ok {
			options = append(options, optFunc)
		}
	}
	return options
}

type AnalysisOption func(config *AnalysisConfig)

func WithExtraPrompt(prompt string) AnalysisOption {
	return func(config *AnalysisConfig) {
		config.ExtraPrompt = prompt
	}
}

func WithAnalysisLog(handler func(format string, args ...interface{})) AnalysisOption {
	return func(config *AnalysisConfig) {
		config.AnalyzeLog = handler
	}
}

func WithAnalyzeContext(ctx context.Context) AnalysisOption {
	return func(config *AnalysisConfig) {
		config.Ctx = ctx
	}
}
