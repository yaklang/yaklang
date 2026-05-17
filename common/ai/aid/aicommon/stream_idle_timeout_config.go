package aicommon

import (
	"time"

	"github.com/yaklang/yaklang/common/log"
)

const (
	// ConfigKeyEnableAIStreamIdleTimeout toggles the StreamIdleTimeoutReader
	// wrap around post-action synchronous AI calls (verification /
	// Critical-level reflection). Default is true; the operator can flip it
	// off via SetConfig at runtime to restore the pre-fix behavior in case a
	// regression is suspected.
	//
	// 关键词: EnableAIStreamIdleTimeout, feature flag, 流空闲超时灰度开关
	ConfigKeyEnableAIStreamIdleTimeout = "EnableAIStreamIdleTimeout"

	// ConfigKeyAIStreamTTFBTimeoutSeconds overrides the time-to-first-byte
	// threshold (in seconds) used when wrapping AI response streams. 0 (or
	// missing) means "use default".
	//
	// 关键词: AIStreamTTFBTimeoutSeconds, 首字节超时
	ConfigKeyAIStreamTTFBTimeoutSeconds = "AIStreamTTFBTimeoutSeconds"

	// ConfigKeyAIStreamIdleTimeoutSeconds overrides the inter-byte idle
	// threshold (in seconds) used when wrapping AI response streams. 0 (or
	// missing) means "use default".
	//
	// 关键词: AIStreamIdleTimeoutSeconds, 字节间空闲超时
	ConfigKeyAIStreamIdleTimeoutSeconds = "AIStreamIdleTimeoutSeconds"

	// DefaultAIStreamTTFBTimeout is the default time-to-first-byte threshold
	// applied to post-action AI calls.
	//
	// 45s is intentionally generous: most providers respond within a few
	// seconds, but cold-start / model-load scenarios can exceed 30s. Anything
	// beyond 45s with zero bytes is almost certainly a stuck connection.
	DefaultAIStreamTTFBTimeout = 45 * time.Second

	// DefaultAIStreamIdleTimeout is the default inter-byte idle threshold
	// applied to post-action AI calls.
	//
	// 60s is intentionally generous to accommodate long reasoning pauses or
	// tool-side delays during streaming. Anything beyond 60s without a single
	// new byte indicates a "live-lock" stream worth aborting.
	DefaultAIStreamIdleTimeout = 60 * time.Second
)

// ResolveAIStreamIdleThresholds returns the effective (ttfb, idle) thresholds
// for wrapping post-action AI streams. When the feature flag is off both
// return values are 0 — the wrapper still tracks timing stats but never
// aborts, which matches the P0 "observe only" mode.
//
// 关键词: ResolveAIStreamIdleThresholds, feature flag, 阈值解析
func ResolveAIStreamIdleThresholds(cfg KeyValueConfigIf) (ttfb, idle time.Duration) {
	if cfg == nil {
		return DefaultAIStreamTTFBTimeout, DefaultAIStreamIdleTimeout
	}
	if !cfg.GetConfigBool(ConfigKeyEnableAIStreamIdleTimeout, true) {
		return 0, 0
	}
	ttfb = DefaultAIStreamTTFBTimeout
	idle = DefaultAIStreamIdleTimeout
	if v := cfg.GetConfigInt(ConfigKeyAIStreamTTFBTimeoutSeconds); v > 0 {
		ttfb = time.Duration(v) * time.Second
	}
	if v := cfg.GetConfigInt(ConfigKeyAIStreamIdleTimeoutSeconds); v > 0 {
		idle = time.Duration(v) * time.Second
	}
	return ttfb, idle
}

// LogStreamTimingSnapshot writes a single structured log line summarizing a
// stream timing snapshot. tag identifies the call site (e.g. "VERIFY_AI_TIMING"
// or "REFLECTION_AI_TIMING") so downstream log analysis can attribute timings
// to the correct AI call.
//
// 关键词: LogStreamTimingSnapshot, 结构化计时日志, P0 埋点
func LogStreamTimingSnapshot(tag string, snap StreamTimingSnapshot) {
	log.Infof(
		"[%s] started_at=%s ttfb=%v duration=%v bytes=%d timed_out=%v",
		tag,
		snap.StartedAt.Format(time.RFC3339Nano),
		snap.TTFB,
		snap.Duration,
		snap.BytesRead,
		snap.TimedOut,
	)
}
