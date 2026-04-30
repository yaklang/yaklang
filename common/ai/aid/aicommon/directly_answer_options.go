package aicommon

// DirectlyAnswerOption configures DirectlyAnswer behavior.
// 由 aicommon 提供以便 reactloops 子包跨包使用，避免对 aireact 包形成循环依赖。
type DirectlyAnswerOption func(*DirectlyAnswerConfig)

// DirectlyAnswerConfig 是 DirectlyAnswer 调用的可配置参数。
type DirectlyAnswerConfig struct {
	ReferenceMaterial       string
	ReferenceMaterialIdx    int
	SkipEmitResultAfterDone bool // 调用方将自行 emit 最终结果时设置为 true
}

// WithDirectlyAnswerReferenceMaterial sets reference material to emit with the stream output.
// 报告/原始素材会以参考资料形式挂在 AI 答复事件上，并写入 workdir。
func WithDirectlyAnswerReferenceMaterial(material string, idx int) DirectlyAnswerOption {
	return func(c *DirectlyAnswerConfig) {
		c.ReferenceMaterial = material
		c.ReferenceMaterialIdx = idx
	}
}

// WithDirectlyAnswerSkipEmitResult skips emitting result after stream is done.
// 调用方需要自己负责 EmitResultAfterStream 时使用。
func WithDirectlyAnswerSkipEmitResult() DirectlyAnswerOption {
	return func(c *DirectlyAnswerConfig) {
		c.SkipEmitResultAfterDone = true
	}
}

// ApplyDirectlyAnswerOptions 从 opts ...any 中筛出 DirectlyAnswerOption 并应用到一个新的配置实例。
// 兼容历史调用方传入任意类型的 opts 的情况。
func ApplyDirectlyAnswerOptions(opts []any) *DirectlyAnswerConfig {
	cfg := &DirectlyAnswerConfig{}
	for _, opt := range opts {
		if fn, ok := opt.(DirectlyAnswerOption); ok && fn != nil {
			fn(cfg)
		}
	}
	return cfg
}
