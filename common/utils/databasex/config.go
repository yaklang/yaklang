package databasex

import (
	"context"
	"time"
)

type config struct {
	// buffer
	bufferSize int

	// save
	saveSize    int
	saveTimeout time.Duration

	// context
	ctx context.Context
}

type Option func(*config)

func WithBufferSize(size int) Option {
	return func(c *config) {
		c.bufferSize = size
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
