package sfreport

type Config struct {
	showFileContent  bool
	showDataflowPath bool
}

type Option func(*Config)

// WithFileContent 设置报告中是否包含源码文件内容（导出名为 sfreport.withFileContent）
// 参数:
//   - show: 是否展示文件内容
//
// 返回值:
//   - 报告配置可选项
//
// Example:
// ```
// // 用于 ConvertSingleResultToJSONWithOptions 等接口的配置
// opt = sfreport.withFileContent(true)
// println(opt)
// ```
func WithFileContent(show bool) func(*Config) {
	return func(c *Config) {
		c.showFileContent = show
	}
}

// WithDataflowPath 设置报告中是否包含数据流路径（导出名为 sfreport.withDataflowPath）
// 参数:
//   - show: 是否展示数据流路径
//
// 返回值:
//   - 报告配置可选项
//
// Example:
// ```
// // 用于 ConvertSingleResultToJSONWithOptions 等接口的配置
// opt = sfreport.withDataflowPath(true)
// println(opt)
// ```
func WithDataflowPath(show bool) func(*Config) {
	return func(c *Config) {
		c.showDataflowPath = show
	}
}
