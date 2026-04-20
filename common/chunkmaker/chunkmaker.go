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

	// separatorAsBoundary changes the separator semantics:
	//   - false (default): the separator is a *trigger*; every occurrence flushes
	//     a chunk (up to chunkSize). Good for structured logs / small records.
	//   - true: the separator is a *boundary hint*; the chunk maker fills up to
	//     chunkSize and, inside each chunk, prefers to cut at the LAST separator
	//     occurrence so that logical blocks stay intact. Good for packing large
	//     pre-structured payloads (e.g. "--- candidate ---" blocks) before an
	//     AI call where per-chunk bytes should approach chunkSize.
	separatorAsBoundary bool
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

// WithSeparatorAsBoundary switches the separator semantics from "trigger every
// occurrence" (default) to "use the separator as a preferred split boundary".
// When enabled, the chunk maker fills up to chunkSize and, inside each chunk,
// cuts at the LAST separator occurrence in the window so that logical blocks
// are not sliced in the middle. If no separator is found, the chunk is cut at
// chunkSize. Used together with WithSeparatorTrigger(sep) + WithChunkSize(n).
func WithSeparatorAsBoundary(asBoundary bool) Option {
	return func(c *Config) {
		c.separatorAsBoundary = asBoundary
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
