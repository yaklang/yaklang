package asyncdb

import (
	"context"
	"sync"
	"time"
)

type config struct {
	lock sync.RWMutex
	// buffer
	bufferSize int

	name string

	// save
	enableSave  bool
	fetchSize   int
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

func WithFetchSize(size int) Option {
	return func(c *config) {
		c.fetchSize = size
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
		c.saveSize = max(defaultBatchSize, size)
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

const defaultBufferSize = 1000

func NewConfig(opts ...Option) *config {
	cfg := &config{
		bufferSize:  defaultBufferSize, // Default buffer size
		ctx:         context.Background(),
		fetchSize:   defaultBatchSize,
		saveSize:    defaultBatchSize,
		saveTimeout: 500 * time.Millisecond, // 0.5s
	}
	for _, opt := range opts {
		opt(cfg)
	}
	cfg.bufferSize = (max(cfg.fetchSize, cfg.saveSize)) * 4
	return cfg
}
