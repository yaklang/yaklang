//go:build !linux

package hids

import (
	"context"
	"fmt"
)

// CheckAuditSystem 检查 audit 子系统状态（需要 root 权限，仅 Linux 可用）
//
// 返回值:
//   - audit 子系统状态（是否启用、积压、丢失事件数等）
//   - 错误信息（无权限或 audit 不可用时返回）
//
// Example:
// ```
// status, err = hids.CheckAuditSystem()
// if err != nil { println("Audit not available:", err) }
// println("Audit enabled:", status.Enabled)
// ```
func CheckAuditSystem() (*AuditStatus, error) {
	return nil, fmt.Errorf("audit subsystem is only supported on Linux")
}

// NewAuditMonitor 创建Audit监控器（需要 root 权限，仅 Linux 可用）
//
// 参数:
//   - opts: 可选配置项，如 hids.auditMonitorLogin / hids.auditOnLoginEvent 等
//
// 返回值:
//   - Audit 监控器对象，调用 Start() 开始监控、Stop() 停止
//   - 错误信息
//
// Example:
// ```
// monitor = hids.NewAuditMonitor(
//
//	hids.auditMonitorLogin(true),
//	hids.auditMonitorCommand(true),
//	hids.auditOnLoginEvent(fn(event) {
//	    println("Login:", event.Username, "from", event.RemoteIP)
//	}),
//	hids.auditOnCommandEvent(fn(event) {
//	    println("Command:", event.Command)
//	}),
//
// )
// ```
func NewAuditMonitor(opts ...AuditMonitorOption) (*AuditMonitor, error) {
	return nil, fmt.Errorf("audit monitor is only supported on Linux")
}

// Start 启动监控
// Example:
// ```
// monitor = hids.NewAuditMonitor()
// err = monitor.Start()
// time.Sleep(10)
// monitor.Stop()
// ```
func (m *AuditMonitor) Start() error {
	return fmt.Errorf("audit monitor is only supported on Linux")
}

// Stop 停止监控
// Example:
// ```
// monitor.Stop()
// ```
func (m *AuditMonitor) Stop() {
	// no-op on non-Linux
}

// IsRunning 检查是否正在运行
func (m *AuditMonitor) IsRunning() bool {
	return false
}

// WatchAuditEvents 简化的audit监控函数（需要 root 权限，仅 Linux 可用）
//
// 参数:
//   - ctx: 上下文，取消时停止监控
//   - onLogin: 登录事件回调（可为 nil）
//   - onCommand: 命令执行事件回调（可为 nil）
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// ctx, cancel = context.WithTimeout(context.Background(), 10)
// defer cancel()
// err = hids.WatchAuditEvents(ctx,
//
//	fn(event) { println("Login:", event.Username) },
//	fn(event) { println("Command:", event.Command) },
//
// )
// ```
func WatchAuditEvents(ctx context.Context, onLogin func(*LoginEvent), onCommand func(*CommandEvent)) error {
	return fmt.Errorf("audit monitor is only supported on Linux")
}
