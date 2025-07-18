package databasex

import (
	"context"
	"time"
)

type config struct {
	// buffer
	bufferSize int

	name string

	// save
	enableSave  bool
	saveSize    int
	saveTimeout time.Duration

	// context
	ctx context.Context
}

type Option func(*config)

func WithName(name string) Option {
	return func(c *config) {
		c.name = name
	}
}

func WithBufferSize(size int) Option {
	return func(c *config) {
		c.bufferSize = size
	}
}

func WithEnableSave(enables ...bool) Option {
	return func(c *config) {
		if len(enables) > 0 {
			c.enableSave = enables[0]
		} else {
			c.enableSave = true // default to true if not specified
		}
	}
}

func WithSaveSize(size int) Option {
	return func(c *config) {
		c.saveSize = size
	}
}

func WithSaveTimeout(timeout time.Duration) Option {
	return func(c *config) {
		c.saveTimeout = timeout
	}
}

func WithContext(ctx context.Context) Option {
	return func(c *config) {
		c.ctx = ctx
	}
}

func NewConfig(opts ...Option) *config {
	cfg := &config{
		bufferSize: 100, // Default buffer size
		ctx:        context.Background(),

		saveSize:    100,
		saveTimeout: 500 * time.Millisecond, // 0.5s
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}
