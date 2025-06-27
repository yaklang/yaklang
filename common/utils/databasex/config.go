package databasex

import (
	"context"
	"sync"
	"time"
)

type config struct {
	waitGroup   *sync.WaitGroup
	bufferSize  int
	saveSize    int
	saveTimeout time.Duration
	ctx         context.Context
}

type Option func(*config)

func WithWaitGroup(wg *sync.WaitGroup) Option {
	return func(c *config) {
		c.waitGroup = wg
	}
}

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
		waitGroup:  &sync.WaitGroup{},

		saveSize:    1000,
		saveTimeout: 500 * time.Millisecond, // 0.5s
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}
