//go:build !linux

package hids

import (
	"context"
	"fmt"
)

// CheckJournalAvailable 检查 journalctl 是否可用
// Example:
// ```
// err = hids.CheckJournalAvailable()
// if err != nil { println("journal not available:", err) }
// ```
func CheckJournalAvailable() error {
	return fmt.Errorf("journal SSH monitor is only supported on Linux with systemd")
}

// NewJournalSSHMonitor 创建基于 systemd journal 的 SSH 登录监控器
//
// 相比 audit 监控，journal 监控有以下优势：
//   - 不需要 root 权限（用户属于 systemd-journal 组即可）
//   - 不依赖 audit 子系统安装和启用
//   - 直接解析 sshd 的认证日志，信息直观
//
// Example:
// ```
// monitor, err = hids.NewJournalSSHMonitor(
//
//	hids.journalSSHOnLoginSuccess(fn(event) {
//	    printf("SSH login success: user=%s from=%s method=%s\n", event.Username, event.RemoteIP, event.AuthMethod)
//	}),
//	hids.journalSSHOnLoginFailed(fn(event) {
//	    printf("SSH login failed: user=%s from=%s\n", event.Username, event.RemoteIP)
//	}),
//
// )
// if err != nil { die(err) }
// err = monitor.Start()
// ```
func NewJournalSSHMonitor(opts ...JournalSSHMonitorOption) (*JournalSSHMonitor, error) {
	return nil, fmt.Errorf("journal SSH monitor is only supported on Linux with systemd")
}

// Start 启动 SSH journal 监控
// Example:
// ```
// monitor, _ = hids.NewJournalSSHMonitor(...)
// err = monitor.Start()
// if err != nil { die(err) }
// ```
func (m *JournalSSHMonitor) Start() error {
	return fmt.Errorf("journal SSH monitor is only supported on Linux with systemd")
}

// Stop 停止监控
// Example:
// ```
// monitor.Stop()
// ```
func (m *JournalSSHMonitor) Stop() {
	// no-op on non-Linux
}

// IsRunning 检查监控器是否正在运行
func (m *JournalSSHMonitor) IsRunning() bool {
	return false
}

// WatchJournalSSHEvents 简化的 SSH journal 监控函数
// 使用 context 控制生命周期，onSuccess 和 onFailed 可以为 nil
// Example:
// ```
// ctx, cancel = context.WithTimeout(context.Background(), 60)
// defer cancel()
// err = hids.WatchJournalSSHEvents(ctx,
//
//	fn(event) { printf("Login success: %s from %s\n", event.Username, event.RemoteIP) },
//	fn(event) { printf("Login failed: %s from %s\n", event.Username, event.RemoteIP) },
//
// )
// ```
func WatchJournalSSHEvents(ctx context.Context, onSuccess func(*JournalSSHEvent), onFailed func(*JournalSSHEvent)) error {
	return fmt.Errorf("journal SSH monitor is only supported on Linux with systemd")
}
