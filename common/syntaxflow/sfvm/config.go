package sfvm

import "github.com/yaklang/yaklang/common/utils/omap"

func NewConfig(opts ...Option) *Config {
	c := &Config{}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

type Config struct {
	debug              bool
	initialContextVars *omap.OrderedMap[string, ValueOperator]
	onResultCaptured   func(name string, results []ValueOperator, reason ...string) error
}

type Option func(*Config)

func WithInitialContextVars(o *omap.OrderedMap[string, ValueOperator]) Option {
	return func(config *Config) {
		config.initialContextVars = o
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

func WithResultCaptured(c func(name string, opts []ValueOperator, reason ...string) error) Option {
	return func(config *Config) {
		config.onResultCaptured = c
	}
}
