package aireducer

import (
	"context"
	"time"

	"github.com/yaklang/yaklang/common/chunkmaker"
	"github.com/yaklang/yaklang/common/utils"
)

// ReducerCallbackType keeps memory as a compatibility placeholder, always nil.
type ReducerCallbackType func(config *Config, memory any, chunk chunkmaker.Chunk) error

// Config controls chunking and callbacks. Processing state belongs to the caller.
type Config struct {
	ctx    context.Context
	cancel context.CancelFunc

	// time trigger mean chunk trigger interval, if set to 0, it will not trigger by time.
	TimeTriggerInterval time.Duration
	ChunkSize           int64
	SeparatorTrigger    string

	// SeparatorAsBoundary switches the separator from a "trigger every
	// occurrence" to a "preferred cut boundary within ChunkSize window".
	// Useful when packing many pre-structured blocks into one AI call.
	SeparatorAsBoundary bool

	// lines trigger mean chunk trigger by line numbers, if set to 0, it will not trigger by lines.
	LineTrigger int

	// EnableLineNumber adds line numbers to each line in the chunk content
	EnableLineNumber bool

	// Reducer Worker Callback
	callback       ReducerCallbackType
	finishCallback func(config *Config, memory any) error
}

type Option func(*Config)

// WithTimeTriggerInterval 设置基于时间的 chunk 触发间隔（导出名为 aireducer.timeTriggerInterval）
// 参数:
//   - interval: 触发间隔（time.Duration），为 0 时不按时间触发
//
// 返回值:
//   - 切分可选项
//
// Example:
// ```
// opt = aireducer.timeTriggerInterval(time.Second)
// println(opt)
// ```
func WithTimeTriggerInterval(interval time.Duration) Option {
	return func(c *Config) {
		c.TimeTriggerInterval = interval
	}
}

// WithTimeTriggerIntervalSeconds 以秒为单位设置基于时间的 chunk 触发间隔（导出名为 aireducer.timeTriggerIntervalSeconds）
// 参数:
//   - seconds: 触发间隔（秒）
//
// 返回值:
//   - 切分可选项
//
// Example:
// ```
// opt = aireducer.timeTriggerIntervalSeconds(1.5)
// println(opt)
// ```
func WithTimeTriggerIntervalSeconds(seconds float64) Option {
	return func(c *Config) {
		c.TimeTriggerInterval = time.Duration(seconds) * time.Second
	}
}

// WithContext 设置 reducer 运行的上下文，用于控制取消（导出名为 aireducer.context）
// 参数:
//   - ctx: 上下文对象
//
// 返回值:
//   - 切分可选项
//
// Example:
// ```
// opt = aireducer.context(context.Background())
// println(opt)
// ```
func WithContext(ctx context.Context) Option {
	return func(c *Config) {
		c.ctx = ctx
	}
}

// WithReducerCallback 设置 chunk 处理回调（导出名为 aireducer.callback / aireducer.reducerCallback）
// 参数:
//   - callback: 回调函数，参数为 (config, memory, chunk)，memory 始终为 nil，累计状态由调用方保存
//
// 返回值:
//   - 切分可选项
//
// Example:
// ```
// opt = aireducer.callback(func(config, memory, chunk) { println(string(chunk.Data())) })
// println(opt)
// ```
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
		c.callback = func(config *Config, _ any, chunk chunkmaker.Chunk) (ret error) {
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

// WithFinishCallback keeps memory as a compatibility placeholder, always nil.
func WithFinishCallback(callback func(config *Config, memory any) error) Option {
	return func(c *Config) {
		c.finishCallback = callback
	}
}

// WithMemory preserves aireducer.memory calls without retaining the supplied value.
// Deprecated: this compatibility option has no effect.
func WithMemory(_ any) Option {
	return func(*Config) {}
}

// WithChunkSize 设置每个 chunk 的最大字节数（导出名为 aireducer.chunkSize）
// 参数:
//   - size: chunk 最大字节数（默认 1024）
//
// 返回值:
//   - 切分可选项
//
// Example:
// ```
// count = 0
// aireducer.String("aaaaabbbbbccccc", func(chunk) { count++ }, aireducer.chunkSize(5))~
// println(count)   // OUT: 3
// ```
func WithChunkSize(size int64) Option {
	return func(c *Config) {
		c.ChunkSize = size
	}
}

// WithSeparatorTrigger 设置切分分隔符，遇到分隔符即触发一个 chunk（导出名为 aireducer.separator）
// 参数:
//   - separator: 分隔符字符串
//
// 返回值:
//   - 切分可选项
//
// Example:
// ```
// count = 0
// aireducer.String("a\nb\nc\n", func(chunk) { count++ }, aireducer.separator("\n"))~
// println(count)   // OUT: 3
// ```
func WithSeparatorTrigger(separator string) Option {
	return func(c *Config) {
		c.SeparatorTrigger = separator
	}
}

// WithSeparatorAsBoundary switches the separator semantics from "trigger every
// occurrence" (default) to "use the separator as a preferred cut boundary".
// When true, the reducer fills up to ChunkSize and, inside each chunk, cuts at
// the LAST separator occurrence in the window so that pre-structured blocks
// stay intact. Combine with WithSeparatorTrigger(sep) + WithChunkSize(n).
//
// 参数:
//   - asBoundary: 是否将分隔符作为切分边界而非每次触发
//
// 返回值:
//   - 切分可选项
//
// Example:
// ```
// opt = aireducer.separatorAsBoundary(true)
// println(opt)
// ```
func WithSeparatorAsBoundary(asBoundary bool) Option {
	return func(c *Config) {
		c.SeparatorAsBoundary = asBoundary
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
//
// 参数:
//   - lines: 每隔多少行触发一个 chunk，正数生效
//
// 返回值:
//   - 切分可选项
func WithLines(lines int) Option {
	return func(c *Config) {
		c.LineTrigger = lines
	}
}

// WithEnableLineNumber enables line number prefixing for chunk content.
// When enabled, each line in the chunk will be prefixed with line numbers.
// This option works with all chunking modes and respects ChunkSize limits.
//
// 参数:
//   - enable: 是否为每行内容添加行号前缀
//
// 返回值:
//   - 切分可选项
//
// Example:
// ```
// opt = aireducer.lineNumber(true)
// println(opt)
// ```
func WithEnableLineNumber(enable bool) Option {
	return func(c *Config) {
		c.EnableLineNumber = enable
	}
}

func NewConfig(opts ...Option) *Config {
	c := &Config{
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
		chunkmaker.WithSeparatorAsBoundary(c.SeparatorAsBoundary),
		chunkmaker.WithCtx(c.ctx),
	}
}
