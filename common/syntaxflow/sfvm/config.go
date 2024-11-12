package sfvm

import (
	"context"

	"github.com/yaklang/yaklang/common/utils/omap"
)

func NewConfig(opts ...Option) *Config {
	c := &Config{
		ctx: context.Background(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

type ResultCapturedCallback func(name string, results ValueOperator) error
type processCallback func(float64, string)

type Config struct {
	debug                     bool
	StrictMatch               bool
	FailFast                  bool
	initialContextVars        *omap.OrderedMap[string, ValueOperator]
	onResultCapturedCallbacks []ResultCapturedCallback
	ctx                       context.Context
	processCallback           processCallback

	taskID string
}

func (c *Config) GetContext() context.Context {
	return c.ctx
}

type Option func(*Config)

func WithInitialContextVars(o *omap.OrderedMap[string, ValueOperator]) Option {
	return func(config *Config) {
		config.initialContextVars = o
	}
}

func WithProcessCallback(p processCallback) Option {
	return func(config *Config) {
		config.processCallback = p
	}
}

// WithExecTaskID set taskID for exec this result will be save with this taskID
func WithExecTaskID(taskID string) Option {
	return func(config *Config) {
		config.taskID = taskID
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
