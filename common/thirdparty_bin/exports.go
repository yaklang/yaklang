package thirdparty_bin

import "context"

type InstallOptionFunc func(o *InstallOptions)

// WithProgress 设置安装进度回调（导出名为 toolbox.progress）
// 参数:
//   - progress: 进度回调函数
//
// 返回值:
//   - 安装可选项
//
// Example:
// ```
// // 示意性示例，需要网络下载第三方工具
// err = toolbox.Install("ffmpeg", toolbox.progress(func(p) { println(p) }))
// ```
func WithProgress(progress ProgressCallback) InstallOptionFunc {
	return func(o *InstallOptions) {
		o.Progress = progress
	}
}

// WithProxy 设置下载代理（导出名为 toolbox.proxy）
// 参数:
//   - proxy: 代理地址，如 http://127.0.0.1:7890
//
// 返回值:
//   - 安装可选项
//
// Example:
// ```
// // 示意性示例，需要网络下载第三方工具
// err = toolbox.Install("ffmpeg", toolbox.proxy("http://127.0.0.1:7890"))
// ```
func WithProxy(proxy string) InstallOptionFunc {
	return func(o *InstallOptions) {
		o.Proxy = proxy
	}
}

// WithForce 设置是否强制重新安装（导出名为 toolbox.force）
// 参数:
//   - force: 为 true 时即使已安装也会重新安装
//
// 返回值:
//   - 安装可选项
//
// Example:
// ```
// // 示意性示例，需要网络下载第三方工具
// err = toolbox.Install("ffmpeg", toolbox.force(true))
// ```
func WithForce(force bool) InstallOptionFunc {
	return func(o *InstallOptions) {
		o.Force = force
	}
}

// WithContext 设置安装上下文，用于控制取消与超时（导出名为 toolbox.context）
// 参数:
//   - ctx: 上下文对象
//
// 返回值:
//   - 安装可选项
//
// Example:
// ```
// // 示意性示例，需要网络下载第三方工具
// ctx, cancel = context.WithTimeout(context.Background(), 60 * time.Second)
// defer cancel()
// err = toolbox.Install("ffmpeg", toolbox.context(ctx))
// ```
func WithContext(ctx context.Context) InstallOptionFunc {
	return func(o *InstallOptions) {
		o.Context = ctx
	}
}

// _install 安装指定的第三方二进制工具（导出名为 toolbox.Install）
// 从远端下载并安装如 ffmpeg、whisper 等第三方工具到本地
// 参数:
//   - name: 工具名称，如 "ffmpeg"
//   - options: 可选项，如 toolbox.proxy / toolbox.force / toolbox.context / toolbox.progress
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// // 示意性示例，需要网络下载第三方工具
// err = toolbox.Install("ffmpeg")
// if err != nil { die(err) }
// ```
func _install(name string, options ...InstallOptionFunc) error {
	opts := &InstallOptions{}
	for _, option := range options {
		option(opts)
	}
	return Install(name, opts)
}

var Exports = map[string]interface{}{
	"Install":   _install,
	"Uninstall": Uninstall,
	"List":      GetAllStatus,

	"proxy":    WithProxy,
	"force":    WithForce,
	"context":  WithContext,
	"progress": WithProgress,
}
