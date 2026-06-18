//go:build !darwin

package browser

// applyBrandedIcon 在非 macOS 平台为空实现:
// 仅 macOS 的 .app bundle 通过 .icns 文件渲染 Dock 图标,
// Windows/Linux 的图标机制不同, 暂不处理。
func applyBrandedIcon(binPath string) {}
