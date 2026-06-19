package aiforge

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/rag/entityrepos"
	"github.com/yaklang/yaklang/common/aireducer"
	"github.com/yaklang/yaklang/common/chunkmaker"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type AnalysisConfig struct {
	Ctx                   context.Context
	ExtraPrompt           string
	AnalyzeLog            func(format string, args ...interface{})
	AnalyzeStatusCard     func(id string, data interface{}, tags ...string)
	AnalyzeConcurrency    int
	AllowMultiHopAIRefine bool
	// VisionAICallback, when non-nil, is passed to temporary LiteForge execution as the
	// preferred AI callback (after other options) so multimodal / vision routing uses the caller's model stack.
	VisionAICallback aicommon.AICallbackType

	chunkOption     []chunkmaker.Option
	fallbackOptions []any
}

func NewAnalysisConfig(opts ...any) *AnalysisConfig {
	throttle := utils.NewThrottle(3)
	cfg := &AnalysisConfig{
		ExtraPrompt:        "",
		AnalyzeConcurrency: 20,
		AnalyzeLog: func(format string, args ...interface{}) {
			log.Infof(format, args...)
		},
		AnalyzeStatusCard: func(id string, data interface{}, tags ...string) {
			throttle(func() {
				log.Infof("Status card [%s]: %v tag: %v", id, data, tags)
			})
		},
		fallbackOptions: []any{},
		Ctx:             context.Background(),
	}

	for _, opt := range opts {
		if optFunc, ok := opt.(AnalysisOption); ok {
			optFunc(cfg)
		} else {
			if chunkOpt, ok := opt.(chunkmaker.Option); ok {
				cfg.chunkOption = append(cfg.chunkOption, chunkOpt)
			}
			cfg.fallbackOptions = append(cfg.fallbackOptions, opt)
		}
	}
	return cfg
}

func (a *AnalysisConfig) ReducerOptions() []aireducer.Option {
	var options []aireducer.Option
	for _, opt := range a.fallbackOptions {
		if optFunc, ok := opt.(aireducer.Option); ok {
			options = append(options, optFunc)
		}
	}
	return options
}

func (a *AnalysisConfig) KHopOption() []entityrepos.KHopQueryOption {
	var options []entityrepos.KHopQueryOption
	for _, opt := range a.fallbackOptions {
		if optFunc, ok := opt.(entityrepos.KHopQueryOption); ok {
			options = append(options, optFunc)
		}
	}
	return options
}

func (a *AnalysisConfig) ForgeExecOption(schema string) []any {
	options := append([]any(nil), a.fallbackOptions...)
	options = append(options, WithOutputJSONSchema(schema))
	options = append(options, LiteForgeExecWithContext(a.Ctx))
	if a.VisionAICallback != nil {
		// Use WithFastAICallback so LiteForge's CallSpeedPriorityAI / CallAI paths both hit the
		// same callback without re-wrapping it as intelligent/lightweight tiers (WithAICallback does that).
		options = append(options, aicommon.WithFastAICallback(a.VisionAICallback))
	}
	return options
}

type AnalysisOption func(config *AnalysisConfig)

// WithExtraPrompt 为图片/上下文分析追加额外提示词（导出名为 liteforge.imageExtraPrompt）
// 参数:
//   - prompt: 额外提示词
//
// 返回值:
//   - 分析可选项
//
// Example:
// ```
// opt = liteforge.imageExtraPrompt("focus on the error message in the screenshot")
// println(opt)
// ```
func WithExtraPrompt(prompt string) AnalysisOption {
	return func(config *AnalysisConfig) {
		config.ExtraPrompt = prompt
	}
}

// WithAnalyzeLog 设置分析过程的日志回调（导出名为 liteforge.analyzeLog）
// 参数:
//   - handler: 日志回调函数，参数为格式化字符串与参数
//
// 返回值:
//   - 分析可选项
//
// Example:
// ```
// opt = liteforge.analyzeLog(func(format, args...) { println(sprintf(format, args...)) })
// println(opt)
// ```
func WithAnalyzeLog(handler func(format string, args ...interface{})) AnalysisOption {
	return func(config *AnalysisConfig) {
		config.AnalyzeLog = func(format string, args ...interface{}) {
			log.Infof(format, args...)
			handler(format, args...)
		}
	}
}

func WithAllowMultiHopAIRefine(allow ...bool) AnalysisOption {
	return func(config *AnalysisConfig) {
		if len(allow) == 0 {
			allow = []bool{true}
		}
		config.AllowMultiHopAIRefine = allow[0]
	}
}

// WithAnalyzeContext 设置分析使用的上下文，用于控制取消（导出名为 liteforge.analyzeCtx）
// 参数:
//   - ctx: 上下文对象
//
// 返回值:
//   - 分析可选项
//
// Example:
// ```
// opt = liteforge.analyzeCtx(context.Background())
// println(opt)
// ```
func WithAnalyzeContext(ctx context.Context) AnalysisOption {
	return func(config *AnalysisConfig) {
		config.Ctx = ctx
	}
}

// WithAnalyzeStatusCard 设置分析过程的状态卡片回调（导出名为 liteforge.analyzeStatusCard）
// 参数:
//   - handler: 状态卡片回调，参数为 (id, data, tags...)
//
// 返回值:
//   - 分析可选项
//
// Example:
// ```
// opt = liteforge.analyzeStatusCard(func(id, data, tags...) { println(id, data) })
// println(opt)
// ```
func WithAnalyzeStatusCard(handler func(id string, data interface{}, tags ...string)) AnalysisOption {
	return func(config *AnalysisConfig) {
		config.AnalyzeStatusCard = func(id string, data interface{}, tags ...string) {
			if handler == nil {
				log.Infof("Status card [%s]: %v tag: %v", id, data, tags)
				return
			}
			handler(id, data, tags...)
		}
	}
}

func WithAnalyzeConcurrency(concurrency int) AnalysisOption {
	return func(config *AnalysisConfig) {
		config.AnalyzeConcurrency = concurrency
	}
}

// WithVisionAICallback sets the AI callback used for LiteForge when analyzing images.
// When set, it is applied after other forge exec options so it takes precedence over
// defaults or other ConfigOption callbacks bundled in AnalyzeImage* opts.
// Typical value is aicommon.MustGetVisionAIModelCallback() (TierVision); if unset,
// AnalyzeImage defaults to that callback.
func WithVisionAICallback(cb aicommon.AICallbackType) AnalysisOption {
	return func(config *AnalysisConfig) {
		config.VisionAICallback = cb
	}
}
