package aireducer

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/chunkmaker"
	"time"
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

type ReducerCallbackType func(config *Config, memory *aid.Memory, chunk chunkmaker.Chunk) error

type Config struct {
	ctx    context.Context
	cancel context.CancelFunc

	// save status in timeline and memory
	Memory *aid.Memory

	// time trigger mean chunk trigger interval, if set to 0, it will not trigger by time.
	TimeTriggerInterval time.Duration
	ChunkSize           int64

	// Reducer Worker Callback
	callback ReducerCallbackType
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
