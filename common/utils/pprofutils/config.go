package pprofutils

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type Config struct {
	cpuProfileFile       string
	memProfileFile       string
	ctx                  context.Context
	onCPUProfileFinished func(string, error)
	onCPUProfileStarted  func(string)
	onMemProfileStarted  func(string)
	onMemProfileFinished func(string, error)
}

func NewConfig() *Config {
	return &Config{
		cpuProfileFile: "",
	}
}

type Option func(*Config)

func WithCPUProfileFile(file string) Option {
	return func(c *Config) {
		c.cpuProfileFile = file
	}
}

func WithMemProfileFile(file string) Option {
	return func(c *Config) {
		c.memProfileFile = file
	}
}

func WithContext(ctx context.Context) Option {
	return func(c *Config) {
		c.ctx = ctx
	}
}

func WithTimeout(i float64) Option {
	return func(c *Config) {
		if i <= 0 {
			i = 15
		}
		c.ctx = utils.TimeoutContextSeconds(i)
	}
}

func WithFinished(h func(string)) Option {
	return func(config *Config) {
		config.onMemProfileFinished = func(s string, err error) {
			if err != nil {
				log.Errorf("memory profile finished: %s, error: %v", s, err)
			}
			if s != "" {
				h(s)
			}
		}
		config.onCPUProfileFinished = func(s string, err error) {
			if err != nil {
				log.Errorf("cpu profile finished: %s, error: %v", s, err)
			}
			if s != "" {
				h(s)
			}
		}
	}
}

func WithOnCPUProfileFinished(fn func(string, error)) Option {
	return func(c *Config) {
		c.onCPUProfileFinished = fn
	}
}

func WithOnCPUProfileStarted(fn func(string)) Option {
	return func(c *Config) {
		c.onCPUProfileStarted = fn
	}
}

func WithOnMemProfileStarted(fn func(string)) Option {
	return func(c *Config) {
		c.onMemProfileStarted = fn
	}
}

func WithOnMemProfileFinished(fn func(string, error)) Option {
	return func(c *Config) {
		c.onMemProfileFinished = fn
	}
}
