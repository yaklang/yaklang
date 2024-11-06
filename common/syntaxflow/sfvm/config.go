package sfvm

import (
	"context"

	"github.com/yaklang/yaklang/common/utils/omap"
)

func NewConfig(opts ...Option) *Config {
	c := &Config{}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

type ResultCapturedCallback func(name string, results ValueOperator) error

type Config struct {
	debug                     bool
	StrictMatch               bool
	FailFast                  bool
	initialContextVars        *omap.OrderedMap[string, ValueOperator]
	onResultCapturedCallbacks []ResultCapturedCallback
	ctx                       context.Context
}

type Option func(*Config)

func WithInitialContextVars(o *omap.OrderedMap[string, ValueOperator]) Option {
	return func(config *Config) {
		config.initialContextVars = o
	}
}

func WithFailFast(b ...bool) Option {
	return func(config *Config) {
		if len(b) <= 0 {
			config.FailFast = true
			return
		}
		config.FailFast = b[0]
	}
}

func WithContext(ctx context.Context) Option {
	return func(config *Config) {
		config.ctx = ctx
	}
}

func WithEnableDebug(b ...bool) Option {
	return func(config *Config) {
		if len(b) <= 0 {
			config.debug = true
			return
		}
		config.debug = b[0]
	}
}

func WithStrictMatch(b bool) Option {
	return func(config *Config) {
		config.StrictMatch = b
	}
}

func WithResultCaptured(c ResultCapturedCallback) Option {
	return func(config *Config) {
		config.onResultCapturedCallbacks = append(config.onResultCapturedCallbacks, c)
	}
}
