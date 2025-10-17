package aireducer

import (
	"context"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/chunkmaker"
	"github.com/yaklang/yaklang/common/utils"
)

/*
Mermaid diagram for Reducer:

graph TD
  A[用户输入/触发事件] --> B[Reducer 执行器]
  C[初始状态/上次状态] --> B

  B --> D{执行处理逻辑}
  D --> E[生成执行结果]

  E --> F{结果分发}
  F --> G[用户输出部分]
  F --> H[状态更新部分]

  G --> I[返回给用户]
  H --> J[更新系统状态]
  J --> K[状态持久化存储]
  K --> C

  subgraph "Reducer 核心"
      B
      D
      E
  end

  subgraph "状态管理"
      C
      H
      J
      K
  end

  subgraph "用户交互"
      A
      G
      I
  end

  style B fill:#e1f5fe
  style F fill:#f3e5f5
  style K fill:#e8f5e8
*/

type ReducerCallbackType func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error

type Config struct {
	ctx    context.Context
	cancel context.CancelFunc

	// save status in timeline and memory
	Memory *aid.PromptContextProvider

	// time trigger mean chunk trigger interval, if set to 0, it will not trigger by time.
	TimeTriggerInterval time.Duration
	ChunkSize           int64
	SeparatorTrigger    string

	// lines trigger mean chunk trigger by line numbers, if set to 0, it will not trigger by lines.
	LineTrigger int

	// EnableLineNumber adds line numbers to each line in the chunk content
	EnableLineNumber bool

	// Reducer Worker Callback
	callback       ReducerCallbackType
	finishCallback func(config *Config, memory *aid.PromptContextProvider) error
}

type Option func(*Config)

func WithTimeTriggerInterval(interval time.Duration) Option {
	return func(c *Config) {
		c.TimeTriggerInterval = interval
	}
}

func WithTimeTriggerIntervalSeconds(seconds float64) Option {
	return func(c *Config) {
		c.TimeTriggerInterval = time.Duration(seconds) * time.Second
	}
}

func WithContext(ctx context.Context) Option {
	return func(c *Config) {
		c.ctx = ctx
	}
}

func WithReducerCallback(callback ReducerCallbackType) Option {
	return func(c *Config) {
		c.callback = callback
	}
}

// aireducer.reducerCallback is called when a new chunk is ready to be processed.
//
// Example:
// ```
//
//	aireducer.NewReducerFromFile("example.txt", aireducer.reducerCallback((config, memory, chunk) => {
//			// handle chunk
//	}))
//
// ```
func WithSimpleCallback(callback func(chunk chunkmaker.Chunk)) Option {
	return func(c *Config) {
		c.callback = func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) (ret error) {
			defer func() {
				if err := recover(); err != nil {
					ret = utils.Error(err)
				}
			}()
			callback(chunk)
			return
		}
	}
}

func WithFinishCallback(callback func(config *Config, memory *aid.PromptContextProvider) error) Option {
	return func(c *Config) {
		c.finishCallback = callback
	}
}

func WithMemory(memory *aid.PromptContextProvider) Option {
	return func(c *Config) {
		c.Memory = memory
	}
}

func WithChunkSize(size int64) Option {
	return func(c *Config) {
		c.ChunkSize = size
	}
}

func WithSeparatorTrigger(separator string) Option {
	return func(c *Config) {
		c.SeparatorTrigger = separator
	}
}

// WithLines sets the line trigger for chunking. When set to a positive value,
// chunks will be created every N lines. If the N lines content is smaller than
// ChunkSize, it will be kept intact. If larger than ChunkSize, it will be split
// according to ChunkSize (ChunkSize is a hard limit).
//
// Example:
// ```
//
//	aireducer.NewReducerFromFile("file.txt", aireducer.WithLines(10), aireducer.WithChunkSize(1024))
//	// This will create chunks every 10 lines, but if 10 lines exceed 1024 bytes,
//	// they will be split at 1024 byte boundaries.
//
// ```
func WithLines(lines int) Option {
	return func(c *Config) {
		c.LineTrigger = lines
	}
}

// WithEnableLineNumber enables line number prefixing for chunk content.
// When enabled, each line in the chunk will be prefixed with line numbers.
// This option works with all chunking modes and respects ChunkSize limits.
func WithEnableLineNumber(enable bool) Option {
	return func(c *Config) {
		c.EnableLineNumber = enable
	}
}

func NewConfig(opts ...Option) *Config {
	c := &Config{
		Memory:              aid.GetDefaultMemory(),
		TimeTriggerInterval: 0,
	}
	for _, opt := range opts {
		opt(c)
	}

	if c.ChunkSize <= 0 {
		c.ChunkSize = 1024
	}

	if c.ctx == nil {
		c.ctx, c.cancel = context.WithCancel(context.Background())
	} else {
		var cancel context.CancelFunc
		c.ctx, cancel = context.WithCancel(c.ctx)
		c.cancel = cancel
	}
	return c
}

func (c *Config) ChunkMakerOption() []chunkmaker.Option {
	return []chunkmaker.Option{
		chunkmaker.WithTimeTrigger(c.TimeTriggerInterval),
		chunkmaker.WithChunkSize(c.ChunkSize),
		chunkmaker.WithSeparatorTrigger(c.SeparatorTrigger),
		chunkmaker.WithCtx(c.ctx),
	}
}
