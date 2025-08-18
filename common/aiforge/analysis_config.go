package aiforge

import (
	"context"
	"github.com/yaklang/yaklang/common/aireducer"
	"github.com/yaklang/yaklang/common/chunkmaker"
	"github.com/yaklang/yaklang/common/log"
)

type AnalysisConfig struct {
	Ctx               context.Context
	ExtraPrompt       string
	AnalyzeLog        func(format string, args ...interface{})
	AnalyzeStatusCard func(id string, data interface{}, tags ...string)

	AnalyzeStreamChunkCallback func(chunk chunkmaker.Chunk)
	chunkOption                []chunkmaker.Option
	fallbackOptions            []any
}

func NewAnalysisConfig(opts ...any) *AnalysisConfig {
	cfg := &AnalysisConfig{
		ExtraPrompt: "",
		AnalyzeLog: func(format string, args ...interface{}) {
			log.Infof(format, args...)
		},
		AnalyzeStatusCard: func(id string, data interface{}, tags ...string) {
			log.Infof("Status card [%s]: %v tag: %v", id, data, tags)
		},
		fallbackOptions: []any{},
		Ctx:             context.Background(),
	}

	for _, opt := range opts {
		if optFunc, ok := opt.(AnalysisOption); ok {
			optFunc(cfg)
		} else {
			if chunkOpt, ok := opt.(*chunkmaker.Option); ok {
				cfg.chunkOption = append(cfg.chunkOption, *chunkOpt)
			}
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

func WithAnalyzeLog(handler func(format string, args ...interface{})) AnalysisOption {
	return func(config *AnalysisConfig) {
		config.AnalyzeLog = func(format string, args ...interface{}) {
			log.Infof(format, args...)
			handler(format, args...)
		}
	}
}

func WithAnalyzeContext(ctx context.Context) AnalysisOption {
	return func(config *AnalysisConfig) {
		config.Ctx = ctx
	}
}

func WithAnalyzeStatusCard(handler func(id string, data interface{}, tags ...string)) AnalysisOption {
	return func(config *AnalysisConfig) {
		config.AnalyzeStatusCard = func(id string, data interface{}, tags ...string) {
			log.Infof("Status card [%s]: %v tag: %v", id, data, tags)
			handler(id, data, tags...)
		}
	}
}

func WithAnalyzeStreamChunkCallback(handler func(chunk chunkmaker.Chunk)) AnalysisOption {
	return func(config *AnalysisConfig) {
		config.AnalyzeStreamChunkCallback = handler
	}
}
