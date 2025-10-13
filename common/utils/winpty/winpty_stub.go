//go:build !windows
// +build !windows

package winpty

import (
	"fmt"
	"os"
)

// WinptyCfg 配置选项 (存根版本)
type WinptyCfg struct {
	DLLPrefix   string
	AppName     string
	Command     string
	Dir         string
	Env         []string
	Flags       uint32
	InitialCols uint32
	InitialRows uint32
}

// CfgOptionFunc 配置选项函数类型 (存根版本)
type CfgOptionFunc func(*WinptyCfg)

// WithDLLPrefix 设置 DLL 路径 (存根版本)
func WithDLLPrefix(prefix string) CfgOptionFunc {
	return func(cfg *WinptyCfg) {}
}

// WithAppName 设置应用程序名称 (存根版本)
func WithAppName(name string) CfgOptionFunc {
	return func(cfg *WinptyCfg) {}
}

// WithCommand 设置要执行的命令 (存根版本)
func WithCommand(command string) CfgOptionFunc {
	return func(cfg *WinptyCfg) {}
}

// WithDir 设置工作目录 (存根版本)
func WithDir(dir string) CfgOptionFunc {
	return func(cfg *WinptyCfg) {}
}

// WithEnv 设置环境变量 (存根版本)
func WithEnv(env []string) CfgOptionFunc {
	return func(cfg *WinptyCfg) {}
}

// WithFlags 设置标志 (存根版本)
func WithFlags(flags uint32) CfgOptionFunc {
	return func(cfg *WinptyCfg) {}
}

// WithInitialSize 设置初始大小 (存根版本)
func WithInitialSize(cols, rows uint32) CfgOptionFunc {
	return func(cfg *WinptyCfg) {}
}

// WithDefaultEnv 使用默认环境变量 (存根版本)
func WithDefaultEnv() CfgOptionFunc {
	return func(cfg *WinptyCfg) {}
}

// WithCurrentDir 使用当前工作目录 (存根版本)
func WithCurrentDir() CfgOptionFunc {
	return func(cfg *WinptyCfg) {}
}

// WinPTY 结构体 (存根版本)
type WinPTY struct {
	StdIn  *os.File
	StdOut *os.File
	closed bool
}

// New 创建新的 WinPTY 实例 (存根版本)
func New(opts ...CfgOptionFunc) (*WinPTY, error) {
	return nil, fmt.Errorf("WinPTY is only supported on Windows")
}

// NewDefault 使用默认配置创建 WinPTY 实例 (存根版本)
func NewDefault(dllPrefix, command string) (*WinPTY, error) {
	return nil, fmt.Errorf("WinPTY is only supported on Windows")
}

// NewWithCfg 使用配置结构体创建 WinPTY 实例 (存根版本)
func NewWithCfg(cfg *WinptyCfg) (*WinPTY, error) {
	return nil, fmt.Errorf("WinPTY is only supported on Windows")
}

// SetSize 设置终端大小 (存根版本)
func (w *WinPTY) SetSize(cols, rows uint32) error {
	return fmt.Errorf("WinPTY is only supported on Windows")
}

// GetProcessHandle 获取子进程句柄 (存根版本)
func (w *WinPTY) GetProcessHandle() uintptr {
	return 0
}

// IsClosed 检查是否已关闭 (存根版本)
func (w *WinPTY) IsClosed() bool {
	return true
}

// Close 关闭 WinPTY 实例 (存根版本)
func (w *WinPTY) Close() error {
	return nil
}

// Example 展示如何使用 WinPTY 接口的示例 (存根版本)
func Example() error {
	return fmt.Errorf("WinPTY examples are only supported on Windows")
}

// ExampleWithOptions 展示如何使用函数式选项创建 WinPTY 实例 (存根版本)
func ExampleWithOptions() error {
	return fmt.Errorf("WinPTY examples are only supported on Windows")
}

// ExampleSimple 展示最简单的使用方式 (存根版本)
func ExampleSimple() error {
	return fmt.Errorf("WinPTY examples are only supported on Windows")
}

// ExampleWithBuilder 展示构建器模式的使用 (存根版本)
func ExampleWithBuilder() error {
	return fmt.Errorf("WinPTY examples are only supported on Windows")
}
