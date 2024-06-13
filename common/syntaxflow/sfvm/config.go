package sfvm

import "github.com/yaklang/yaklang/common/utils/omap"

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
	initialContextVars        *omap.OrderedMap[string, ValueOperator]
	onResultCapturedCallbacks []ResultCapturedCallback
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
