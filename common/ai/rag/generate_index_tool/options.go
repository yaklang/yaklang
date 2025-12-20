package generate_index_tool

import (
	"os"
)

// OptionFunc 选项函数类型
type OptionFunc func(*IndexOptions)

// WithCacheDir 设置缓存目录
func WithCacheDir(dir string) OptionFunc {
	return func(opts *IndexOptions) {
		opts.CacheDir = dir
	}
}

// WithForceBypassCache 强制绕过缓存
func WithForceBypassCache(bypass bool) OptionFunc {
	return func(opts *IndexOptions) {
		opts.ForceBypassCache = bypass
	}
}

// WithIncludeMetadata 设置是否包含元数据
func WithIncludeMetadata(include bool) OptionFunc {
	return func(opts *IndexOptions) {
		opts.IncludeMetadata = include
	}
}

// WithBatchSize 设置批处理大小
func WithBatchSize(size int) OptionFunc {
	return func(opts *IndexOptions) {
		if size > 0 {
			opts.BatchSize = size
		}
	}
}

// WithConcurrentWorkers 设置并发工作协程数
func WithConcurrentWorkers(workers int) OptionFunc {
	return func(opts *IndexOptions) {
		if workers > 0 {
			opts.ConcurrentWorkers = workers
		}
	}
}

// WithProgressCallback 设置进度回调
func WithProgressCallback(callback ProgressCallback) OptionFunc {
	return func(opts *IndexOptions) {
		opts.ProgressCallback = callback
	}
}

// WithContentProcessor 设置内容处理器
func WithContentProcessor(processor ContentProcessor) OptionFunc {
	return func(opts *IndexOptions) {
		opts.ContentProcessor = processor
	}
}

// WithCacheManager 设置缓存管理器
func WithCacheManager(manager CacheManager) OptionFunc {
	return func(opts *IndexOptions) {
		opts.CacheManager = manager
	}
}

// WithDefaultAIProcessor 使用默认AI处理器（不需要外部依赖注入）
func WithDefaultAIProcessor(customPrompt ...string) OptionFunc {
	return func(opts *IndexOptions) {
		opts.ContentProcessor = NewDefaultAIContentProcessor(customPrompt...)
	}
}

// WithTempCacheDir 使用临时目录作为缓存目录
func WithTempCacheDir() OptionFunc {
	return func(opts *IndexOptions) {
		opts.CacheDir = os.TempDir()
	}
}

// ApplyOptions 应用选项函数列表
func ApplyOptions(base *IndexOptions, optFuncs ...OptionFunc) *IndexOptions {
	if base == nil {
		base = DefaultIndexOptions()
	}

	for _, optFunc := range optFuncs {
		optFunc(base)
	}

	return base
}
