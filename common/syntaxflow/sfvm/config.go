package sfvm

import (
	"context"
	"sync"

	"github.com/yaklang/yaklang/common/utils/omap"
)

func NewConfig(opts ...Option) *Config {
	c := &Config{
		ctx:      context.Background(),
		FailFast: true,
		Mutex:    sync.Mutex{},
	}
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
	processCallback           func(idx int, msg string)
	Mutex                     sync.Mutex
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

func WithProcessCallback(p func(int, string)) Option {
	return func(config *Config) {
		config.processCallback = p
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
		if ctx != nil {
			config.ctx = ctx
		}
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

func WithStrictMatch(b ...bool) Option {
	return func(config *Config) {
		if len(b) > 0 {
			config.StrictMatch = b[0]
		} else {
			config.StrictMatch = true
		}
	}
}

func WithResultCaptured(c ResultCapturedCallback) Option {
	return func(config *Config) {
		config.onResultCapturedCallbacks = append(config.onResultCapturedCallbacks, c)
	}
}

func WithConfig(other *Config) Option {
	return func(self *Config) {
		self.StrictMatch = other.StrictMatch
		self.FailFast = other.FailFast
		self.initialContextVars = other.initialContextVars
		self.onResultCapturedCallbacks = other.onResultCapturedCallbacks
		self.ctx = other.ctx
		self.processCallback = other.processCallback
	}
}

func (c *Config) Copy() *Config {
	return &Config{
		debug:                     c.debug,
		StrictMatch:               c.StrictMatch,
		FailFast:                  c.FailFast,
		initialContextVars:        c.initialContextVars,
		onResultCapturedCallbacks: c.onResultCapturedCallbacks,
		ctx:                       c.ctx,
		processCallback:           c.processCallback,
	}
}
