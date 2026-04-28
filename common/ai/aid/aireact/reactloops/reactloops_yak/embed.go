package reactloops_yak

import "embed"

//go:embed focus_modes
var focusModesFS embed.FS

// FocusModesFS 暴露内置 yak 专注模式资源的只读文件系统。
// 关键词: yak focus mode embed fs, builtin focus modes
func FocusModesFS() embed.FS {
	return focusModesFS
}
