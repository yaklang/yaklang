//go:build !linux

package hids

import (
	"context"
	"fmt"
)

// CheckAuditSystem 检查 audit 子系统状态
// Example:
// ```
// status, err = hids.CheckAuditSystem()
// if err != nil { println("Audit not available:", err) }
// println("Audit enabled:", status.Enabled)
// ```
func CheckAuditSystem() (*AuditStatus, error) {
	return nil, fmt.Errorf("audit subsystem is only supported on Linux")
}

// NewAuditMonitor 创建Audit监控器
// Example:
// ```
// monitor = hids.NewAuditMonitor(
//
//	hids.auditMonitorLogin(true),
//	hids.auditMonitorCommand(true),
//	hids.onLoginEvent(fn(event) {
//	    println("Login:", event.Username, "from", event.RemoteIP)
//	}),
//	hids.onCommandEvent(fn(event) {
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

// WatchAuditEvents 简化的audit监控函数
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
