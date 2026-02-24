package hids

import (
	"sync"
	"time"
)

// JournalSSHEventType SSH 事件类型
type JournalSSHEventType string

const (
	JournalSSHEventLoginSuccess  JournalSSHEventType = "login_success"   // 登录成功
	JournalSSHEventLoginFailed   JournalSSHEventType = "login_failed"    // 登录失败
	JournalSSHEventInvalidUser   JournalSSHEventType = "invalid_user"    // 无效用户
	JournalSSHEventDisconnected  JournalSSHEventType = "disconnected"    // 会话断开
	JournalSSHEventMaxAuthFailed JournalSSHEventType = "max_auth_failed" // 超过最大认证次数
)

// JournalSSHEvent 基于 journal 解析的 SSH 登录事件
type JournalSSHEvent struct {
	Timestamp  time.Time           `json:"timestamp"`   // 事件时间
	EventType  JournalSSHEventType `json:"event_type"`  // 事件类型
	Username   string              `json:"username"`    // 登录用户名
	RemoteIP   string              `json:"remote_ip"`   // 远程 IP 地址
	RemotePort string              `json:"remote_port"` // 远程端口
	AuthMethod string              `json:"auth_method"` // 认证方式 (password/publickey)
	PID        string              `json:"pid"`         // sshd 进程 PID
	Hostname   string              `json:"hostname"`    // 本机主机名
	Message    string              `json:"message"`     // 原始日志消息
}

// JournalSSHMonitor 基于 systemd journal 的 SSH 登录监控器
// 通过解析 journalctl 输出监控 sshd 服务的认证日志，无需 root 权限
// （用户需属于 systemd-journal 组，或 journal 已设置为全局可读）
type JournalSSHMonitor struct {
	// 事件回调
	onLoginSuccess func(*JournalSSHEvent)
	onLoginFailed  func(*JournalSSHEvent)
	onDisconnected func(*JournalSSHEvent)
	onAnyEvent     func(*JournalSSHEvent)

	// 过滤器
	filterUsers     []string // 只关注特定用户名
	filterRemoteIPs []string // 只关注特定来源 IP

	// journalctl 参数
	journalUnits []string // 要监听的 systemd unit（默认 sshd/ssh）
	sinceTime    string   // --since 参数，默认 "now"

	// 运行状态
	running bool
	stopCh  chan struct{}
	mu      sync.RWMutex
}

// JournalSSHMonitorOption Journal SSH 监控器配置选项
type JournalSSHMonitorOption func(*JournalSSHMonitor)

// journalSSHOnLoginSuccess 设置登录成功事件回调
// Example:
// ```
// monitor, err = hids.NewJournalSSHMonitor(
//
//	hids.journalSSHOnLoginSuccess(fn(event) {
//	    println("SSH login success:", event.Username, "from", event.RemoteIP)
//	}),
//
// )
// ```
func WithJournalSSHOnLoginSuccess(callback func(*JournalSSHEvent)) JournalSSHMonitorOption {
	return func(m *JournalSSHMonitor) {
		m.onLoginSuccess = callback
	}
}

// journalSSHOnLoginFailed 设置登录失败事件回调
// Example:
// ```
// monitor, err = hids.NewJournalSSHMonitor(
//
//	hids.journalSSHOnLoginFailed(fn(event) {
//	    println("SSH login failed:", event.Username, "from", event.RemoteIP)
//	}),
//
// )
// ```
func WithJournalSSHOnLoginFailed(callback func(*JournalSSHEvent)) JournalSSHMonitorOption {
	return func(m *JournalSSHMonitor) {
		m.onLoginFailed = callback
	}
}

// journalSSHOnDisconnected 设置会话断开事件回调
// Example:
// ```
// monitor, err = hids.NewJournalSSHMonitor(
//
//	hids.journalSSHOnDisconnected(fn(event) {
//	    println("SSH disconnected:", event.Username, "from", event.RemoteIP)
//	}),
//
// )
// ```
func WithJournalSSHOnDisconnected(callback func(*JournalSSHEvent)) JournalSSHMonitorOption {
	return func(m *JournalSSHMonitor) {
		m.onDisconnected = callback
	}
}

// journalSSHOnAnyEvent 设置任意 SSH 事件回调（成功/失败/断开均触发）
// Example:
// ```
// monitor, err = hids.NewJournalSSHMonitor(
//
//	hids.journalSSHOnAnyEvent(fn(event) {
//	    println("SSH event:", event.EventType, event.Username, event.RemoteIP)
//	}),
//
// )
// ```
func WithJournalSSHOnAnyEvent(callback func(*JournalSSHEvent)) JournalSSHMonitorOption {
	return func(m *JournalSSHMonitor) {
		m.onAnyEvent = callback
	}
}

// journalSSHFilterUsers 只监控指定用户名的 SSH 登录事件
// Example:
// ```
// monitor, err = hids.NewJournalSSHMonitor(
//
//	hids.journalSSHFilterUsers("root", "admin"),
//
// )
// ```
func WithJournalSSHFilterUsers(users ...string) JournalSSHMonitorOption {
	return func(m *JournalSSHMonitor) {
		m.filterUsers = users
	}
}

// journalSSHFilterRemoteIPs 只监控来自指定 IP 的 SSH 登录事件
// Example:
// ```
// monitor, err = hids.NewJournalSSHMonitor(
//
//	hids.journalSSHFilterRemoteIPs("192.168.1.1", "10.0.0.2"),
//
// )
// ```
func WithJournalSSHFilterRemoteIPs(ips ...string) JournalSSHMonitorOption {
	return func(m *JournalSSHMonitor) {
		m.filterRemoteIPs = ips
	}
}

// journalSSHUnits 设置要监听的 systemd unit 名称（默认自动检测 sshd/ssh）
// Example:
// ```
// monitor, err = hids.NewJournalSSHMonitor(
//
//	hids.journalSSHUnits("sshd.service"),
//
// )
// ```
func WithJournalSSHUnits(units ...string) JournalSSHMonitorOption {
	return func(m *JournalSSHMonitor) {
		m.journalUnits = units
	}
}

// journalSSHSince 设置从何时开始读取日志（journalctl --since 参数格式）
// 默认为 "now"，即只读取启动后的新日志
// Example:
// ```
// // 读取最近1小时的历史日志并持续监控
// monitor, err = hids.NewJournalSSHMonitor(
//
//	hids.journalSSHSince("1 hour ago"),
//
// )
// ```
func WithJournalSSHSince(since string) JournalSSHMonitorOption {
	return func(m *JournalSSHMonitor) {
		m.sinceTime = since
	}
}
