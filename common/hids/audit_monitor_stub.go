//go:build !linux

package hids

import (
	"context"
	"fmt"
)

// NewAuditMonitor 创建Audit监控器 (非Linux平台不支持)
// Example:
// ```
// monitor, err = hids.NewAuditMonitor()
// ```
func NewAuditMonitor(opts ...AuditMonitorOption) (*AuditMonitor, error) {
	return nil, fmt.Errorf("audit monitor is only supported on Linux")
}

// Start 启动监控 (非Linux平台不支持)
func (m *AuditMonitor) Start() error {
	return fmt.Errorf("audit monitor is only supported on Linux")
}

// Stop 停止监控
func (m *AuditMonitor) Stop() {
	// no-op on non-Linux
}

// IsRunning 检查是否正在运行
func (m *AuditMonitor) IsRunning() bool {
	return false
}

// WatchAuditEvents 简化的监控函数 (非Linux平台不支持)
func WatchAuditEvents(ctx context.Context, onLogin func(*LoginEvent), onCommand func(*CommandEvent)) error {
	return fmt.Errorf("audit monitor is only supported on Linux")
}
