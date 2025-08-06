package localmodel

import (
	"time"
)

// Option 定义选项函数类型
type Option func(*ServiceConfig)

// WithHost 设置主机地址
func WithHost(host string) Option {
	return func(c *ServiceConfig) {
		if host != "" {
			c.Host = host
		}
	}
}

// WithPort 设置端口
func WithPort(port int32) Option {
	return func(c *ServiceConfig) {
		if port > 0 {
			c.Port = port
		}
	}
}

// WithEmbeddingModel 设置嵌入模型
func WithEmbeddingModel(model string) Option {
	return func(c *ServiceConfig) {
		if model != "" {
			c.Model = model
		}
	}
}

// WithModelPath 设置模型路径
func WithModelPath(path string) Option {
	return func(c *ServiceConfig) {
		if path != "" {
			c.ModelPath = path
		}
	}
}

// WithContextSize 设置上下文大小
func WithContextSize(size int) Option {
	return func(c *ServiceConfig) {
		if size > 0 {
			c.ContextSize = size
		}
	}
}

// WithContBatching 设置是否启用连续批处理
func WithContBatching(enabled bool) Option {
	return func(c *ServiceConfig) {
		c.ContBatching = enabled
	}
}

// WithBatchSize 设置批处理大小
func WithBatchSize(size int) Option {
	return func(c *ServiceConfig) {
		if size > 0 {
			c.BatchSize = size
		}
	}
}

// WithThreads 设置线程数
func WithThreads(threads int) Option {
	return func(c *ServiceConfig) {
		if threads > 0 {
			c.Threads = threads
		}
	}
}

// WithDetached 设置是否分离模式
func WithDetached(detached bool) Option {
	return func(c *ServiceConfig) {
		c.Detached = detached
	}
}

// WithDebug 设置调试模式
func WithDebug(debug bool) Option {
	return func(c *ServiceConfig) {
		c.Debug = debug
	}
}

// WithStartupTimeout 设置启动超时时间
func WithStartupTimeout(timeout time.Duration) Option {
	return func(c *ServiceConfig) {
		if timeout > 0 {
			c.StartupTimeout = timeout
		}
	}
}

// WithArgs 设置额外的命令行参数
func WithArgs(args ...string) Option {
	return func(c *ServiceConfig) {
		c.Args = append(c.Args, args...)
	}
}
