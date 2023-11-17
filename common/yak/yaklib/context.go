package yaklib

import (
	"context"
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

// Seconds 返回一个超时时间为 d 秒的 Context 接口（即上下文接口）
// 它实际是 context.WithTimeoutSeconds 的别名
// Example:
// ```
// ctx = context.Seconds(10)
// ```
func _seconds(d float64) context.Context {
	return utils.TimeoutContextSeconds(d)
}

// WithTimeoutSeconds 返回超时时间为 d 秒的 Context 接口（即上下文接口）
// Example:
// ```
// ctx = context.WithTimeoutSeconds(10)
// ```
func _withTimeoutSeconds(d float64) context.Context {
	return utils.TimeoutContextSeconds(d)
}

// New 返回空的 Context 接口（即上下文接口）
// 它实际是 context.Background 的别名
// Example:
// ```
// ctx = context.New()
// ```
func _newContext() context.Context {
	return context.Background()
}

// Background 返回空的 Context 接口（即上下文接口）
// Example:
// ```
// ctx = context.Background()
// ```
func _background() context.Context {
	return context.Background()
}

// WithCancel 返回继承自 parent 的 Context 接口（即上下文接口）和取消函数
// 当调用返回的取消函数或者 parent 的取消函数时，整个上下文会被取消
// Example:
// ```
// ctx, cancel = context.WithCancel(context.Background())
// defer cancel()
// ```
func _withCancel(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithCancel(parent)
}

// WithTimeout 返回继承自 parent 的 Context 接口（即上下文接口）和取消函数
// 当调用返回的取消函数或者超时，整个上下文会被取消
// Example:
// ```
// dur, err = time.ParseDuration("10s")
// ctx, cancel := context.WithTimeout(context.Background(), dur)
// defer cancel()
// ```
func _withTimeout(parent context.Context, d float64) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, utils.FloatSecondDuration(d))
}

// WithDeadline 返回继承自 parent 的 Context 接口（即上下文接口）和取消函数
// 当调用返回的取消函数或者超出指定时间，整个上下文会被取消
// Example:
// ```
// dur, err = time.ParseDuration("10s")
// after = time.Now().Add(dur)
// ctx, cancel := context.WithDeadline(context.Background(), after)
// defer cancel()
// ```
func _withDeadline(parent context.Context, t time.Time) (context.Context, context.CancelFunc) {
	return context.WithDeadline(parent, t)
}

// WithValue 返回继承自 parent ，同时额外携带键值的 Context 接口（即上下文接口）和取消函数
// 当调用返回的取消函数时，整个上下文会被取消
// Example:
// ```
// ctx = context.WithValue(context.Background(), "key", "value")
// ctx.Value("key") // "value"
// ```
func _withValue(parent context.Context, key, val any) context.Context {
	return context.WithValue(parent, key, val)
}

var ContextExports = map[string]interface{}{
	"Seconds":            _seconds,
	"New":                _newContext,
	"Background":         _background,
	"WithCancel":         _withCancel,
	"WithTimeout":        _withTimeout,
	"WithTimeoutSeconds": _withTimeoutSeconds,
	"WithDeadline":       _withDeadline,
	"WithValue":          _withValue,
}
