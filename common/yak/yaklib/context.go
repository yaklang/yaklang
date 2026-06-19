package yaklib

import (
	"context"
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

// Seconds 返回一个超时时间为 d 秒的 Context 接口（即上下文接口）
// 它实际是 context.WithTimeoutSeconds 的别名
// 参数:
//   - d: 超时时间，单位为秒
//
// 返回值:
//   - 带有指定超时的上下文接口
//
// Example:
// ```
// // VARS: 创建 10 秒超时上下文
// ctx = context.Seconds(10)
// // assert: 刚创建时尚未超时，Err 为 nil
// assert ctx.Err() == nil, "fresh timeout context should have no error yet"
// ```
func _seconds(d float64) context.Context {
	return utils.TimeoutContextSeconds(d)
}

// WithTimeoutSeconds 返回超时时间为 d 秒的 Context 接口（即上下文接口）
// 参数:
//   - d: 超时时间，单位为秒
//
// 返回值:
//   - 带有指定超时的上下文接口
//
// Example:
// ```
// // VARS: 创建 10 秒超时上下文
// ctx = context.WithTimeoutSeconds(10)
// // assert: 刚创建时尚未超时，Err 为 nil
// assert ctx.Err() == nil, "fresh timeout context should have no error yet"
// ```
func _withTimeoutSeconds(d float64) context.Context {
	return utils.TimeoutContextSeconds(d)
}

// New 返回空的 Context 接口（即上下文接口）
// 它实际是 context.Background 的别名
// 返回值:
//   - 一个空的根上下文接口
//
// Example:
// ```
// // VARS: 创建根上下文
// ctx = context.New()
// // assert: 根上下文没有错误
// assert ctx.Err() == nil, "background context should have no error"
// ```
func _newContext() context.Context { return context.Background() }

// Background 返回空的 Context 接口（即上下文接口）
// 返回值:
//   - 一个空的根上下文接口
//
// Example:
// ```
// // VARS: 创建根上下文
// ctx = context.Background()
// // assert: 根上下文没有错误
// assert ctx.Err() == nil, "background context should have no error"
// ```
func _background() context.Context { return context.Background() }

// WithCancel 返回继承自 parent 的 Context 接口（即上下文接口）和取消函数
// 当调用返回的取消函数或者 parent 的取消函数时，整个上下文会被取消
// 参数:
//   - parent: 父上下文
//
// 返回值:
//   - 派生出的可取消上下文
//   - 取消函数，调用后会取消该上下文
//
// Example:
// ```
// // VARS: 派生一个可取消上下文
// ctx, cancel = context.WithCancel(context.New())
// // 取消前没有错误
// assert ctx.Err() == nil, "context should have no error before cancel"
// // 取消后产生错误
// cancel()
// assert ctx.Err() != nil, "context should report error after cancel"
// ```
func _withCancel(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithCancel(parent)
}

// WithTimeout 返回继承自 parent 的 Context 接口（即上下文接口）和取消函数
// 当调用返回的取消函数或者超时，整个上下文会被取消
// 参数:
//   - parent: 父上下文
//   - timeout: 超时时间间隔
//
// 返回值:
//   - 派生出的带超时的上下文
//   - 取消函数，调用后会取消该上下文
//
// Example:
// ```
// // VARS: 派生一个 10 秒超时的上下文
// dur = time.ParseDuration("10s")~
// ctx, cancel = context.WithTimeout(context.New(), dur)
// defer cancel()
// // assert: 刚创建时尚未超时
// assert ctx.Err() == nil, "context should have no error before timeout"
// ```
func _withTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, timeout)
}

// WithDeadline 返回继承自 parent 的 Context 接口（即上下文接口）和取消函数
// 当调用返回的取消函数或者超出指定时间，整个上下文会被取消
// 参数:
//   - parent: 父上下文
//   - t: 截止时间点
//
// 返回值:
//   - 派生出的带截止时间的上下文
//   - 取消函数，调用后会取消该上下文
//
// Example:
// ```
// // VARS: 派生一个带未来截止时间的上下文
// dur = time.ParseDuration("10s")~
// after = time.Now().Add(dur)
// ctx, cancel = context.WithDeadline(context.New(), after)
// defer cancel()
// // assert: 截止时间尚未到达
// assert ctx.Err() == nil, "context should have no error before deadline"
// ```
func _withDeadline(parent context.Context, t time.Time) (context.Context, context.CancelFunc) {
	return context.WithDeadline(parent, t)
}

// WithValue 返回继承自 parent ，同时额外携带键值的 Context 接口（即上下文接口）
// 可通过 ctx.Value(key) 读取携带的值
// 参数:
//   - parent: 父上下文
//   - key: 携带值的键
//   - val: 携带的值
//
// 返回值:
//   - 携带了指定键值的派生上下文
//
// Example:
// ```
// // VARS: 在上下文中携带键值
// ctx = context.WithValue(context.New(), "key", "value")
// // STDOUT: 读取携带的值
// println(ctx.Value("key"))   // OUT: value
// // assert: 锁定结论
// assert ctx.Value("key") == "value", "WithValue should carry the value"
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
