package ssa

import (
	"github.com/yaklang/yaklang/common/utils/diagnostics"
)

// TrackOption 可配置选项，用于 TrackBuildWithOptions
type TrackOption func(*trackBuildConfig)

type trackBuildConfig struct {
	kind     diagnostics.TrackKind
	level    diagnostics.Level
	doLog    bool
	depth    int  // LazyBuild 调用栈深度，-1 表示不记录
	useDepth bool // 为 true 时执行 PushBuildDepth/PopBuildDepth 并传入 depth
}

func defaultTrackBuildConfig() trackBuildConfig {
	return trackBuildConfig{
		kind:     TrackKindBuild,
		level:    diagnostics.LevelHigh,
		doLog:    true,
		depth:    -1,
		useDepth: false,
	}
}

// WithTrackKind 指定 TrackKind
func WithTrackKind(kind diagnostics.TrackKind) TrackOption {
	return func(c *trackBuildConfig) { c.kind = kind }
}

// WithTrackLevel 指定触发记录的 Level
func WithTrackLevel(level diagnostics.Level) TrackOption {
	return func(c *trackBuildConfig) { c.level = level }
}

// WithTrackLog 指定是否输出 LogLow
func WithTrackLog(doLog bool) TrackOption {
	return func(c *trackBuildConfig) { c.doLog = doLog }
}

// WithTrackDepthEnabled 启用 depth 时执行 Recorder.PushBuildDepth/PopBuildDepth，并将 depth 传入记录
func WithTrackDepthEnabled(enable bool) TrackOption {
	return func(c *trackBuildConfig) { c.useDepth = enable }
}

// TrackBuildWithOptions 可配置 option 的 Build 记录入口，供 LazyBuild 等需要定制 Kind/Level/Log/Depth 的场景使用
func TrackBuildWithOptions(rec *diagnostics.Recorder, name string, fn func() error, opts ...TrackOption) error {
	if rec == nil {
		rec = diagnostics.DefaultRecorder()
	}
	cfg := defaultTrackBuildConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.useDepth && rec != nil {
		depth := rec.PushBuildDepth()
		defer rec.PopBuildDepth()
		cfg.depth = depth
	}
	enabled := diagnostics.Enabled(cfg.level)
	_, err := rec.TrackWithParams(enabled, cfg.doLog, cfg.kind, name, cfg.depth, fn)
	return err
}

// TrackBuild SSA 快捷入口：固定 TrackKindBuild + LevelHigh，rec 为 nil 时使用 DefaultRecorder
func TrackBuild(rec *diagnostics.Recorder, name string, fn func() error) error {
	return TrackBuildWithOptions(rec, name, fn)
}
