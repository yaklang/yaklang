package dbcache

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
	enableSave      bool
	fetchSize       int
	saveSize        int
	saveTimeout     time.Duration
	saveParallelism int
	persistLimit    int
	// saveByteBudget flushes the current batch once another item would exceed
	// it. saveByteSize is func(T) int stored as any because config is not generic.
	saveByteBudget int
	saveByteSize   any

	// context
	ctx context.Context

	skipEviction any
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

// WithInitialBufferSize controls the initial asynchronous queue capacity,
// independently of database batch sizes. The queue still grows as needed.
func WithInitialBufferSize(size int) Option {
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
		c.saveSize = max(defaultBatchSize, size)
	}
}

func WithSaveTimeout(timeout time.Duration) Option {
	return func(c *config) {
		c.saveTimeout = timeout
	}
}

func WithSaveParallelism(parallelism int) Option {
	return func(c *config) {
		if parallelism <= 0 {
			parallelism = 1
		}
		c.saveParallelism = parallelism
	}
}

// WithSaveByteBudget flushes a non-empty batch before appending an item
// that would make the estimated payload larger than budget. A single item
// larger than budget is still saved alone. budget <= 0 disables the check.
func WithSaveByteBudget[T any](budget int, sizeOf func(T) int) Option {
	return func(c *config) {
		if budget < 0 {
			budget = 0
		}
		c.saveByteBudget = budget
		c.saveByteSize = sizeOf
	}
}

func WithPersistLimit(limit int) Option {
	return func(c *config) {
		if limit < 0 {
			limit = 0
		}
		c.persistLimit = limit
	}
}

func WithContext(ctx context.Context) Option {
	return func(c *config) {
		c.ctx = ctx
	}
}

func WithSkipEviction[T any](skip func(T) bool) Option {
	return func(c *config) {
		c.skipEviction = skip
	}
}

func NewConfig(opts ...Option) *config {
	cfg := &config{
		ctx:             context.Background(),
		fetchSize:       defaultBatchSize,
		saveSize:        defaultBatchSize,
		saveTimeout:     500 * time.Millisecond, // 0.5s
		saveParallelism: 1,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	if cfg.bufferSize <= 0 {
		cfg.bufferSize = (max(cfg.fetchSize, cfg.saveSize)) * 4
	}
	return cfg
}
