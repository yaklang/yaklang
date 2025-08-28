package chunkmaker

import (
	"context"
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

type ChunkMaker interface {
	Close() error
	OutputChannel() <-chan Chunk
}

type Config struct {
	ctx                 context.Context
	cancel              context.CancelFunc
	chunkSize           int64
	enableTimeTrigger   bool
	timeTriggerInterval time.Duration

	// separator for chunk data
	separator string
}

type Option func(c *Config)

func WithTimeTrigger(interval time.Duration) Option {
	return func(c *Config) {
		if interval <= 0 {
			return
		}
		c.enableTimeTrigger = true
		c.timeTriggerInterval = interval
	}
}

func WithSeparatorTrigger(separator string) Option {
	return func(c *Config) {
		c.separator = separator
	}
}

func WithTimeTriggerSeconds(interval float64) Option {
	return func(c *Config) {
		c.enableTimeTrigger = true
		c.timeTriggerInterval = utils.FloatSecondDuration(interval)
	}
}

func WithCtx(ctx context.Context) Option {
	return func(c *Config) {
		c.ctx = ctx
	}
}

func NewConfig(opts ...Option) *Config {
	c := &Config{
		chunkSize: 1024 * 4,
	}

	for _, opt := range opts {
		opt(c)
	}

	if c.ctx == nil {
		c.ctx, c.cancel = context.WithCancel(context.Background())
	}

	if c.cancel == nil && c.ctx != nil {
		// wrapper with cancel
		c.ctx, c.cancel = context.WithCancel(c.ctx)
	}

	return c
}

func WithChunkSize(size int64) Option {
	return func(c *Config) {
		c.chunkSize = size
	}
}
