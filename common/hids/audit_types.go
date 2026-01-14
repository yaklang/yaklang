package hids

import (
	"sync"
	"time"
)

// LoginEvent 登录事件
type LoginEvent struct {
	Timestamp   time.Time         `json:"timestamp"`    // 登录时间
	Username    string            `json:"username"`     // 用户名
	UID         string            `json:"uid"`          // 用户ID
	RemoteIP    string            `json:"remote_ip"`    // 远程IP地址
	RemoteHost  string            `json:"remote_host"`  // 远程主机名
	LoginMethod string            `json:"login_method"` // 登录方式 (ssh, console, etc)
	Terminal    string            `json:"terminal"`     // 终端
	SessionID   string            `json:"session_id"`   // 会话ID
	Result      string            `json:"result"`       // 登录结果 (success, failed)
	Message     string            `json:"message"`      // 原始消息
	AuditID     string            `json:"audit_id"`     // Audit事件ID
	ExtraData   map[string]string `json:"extra_data"`   // 额外数据
}

// CommandEvent 命令执行事件
type CommandEvent struct {
	Timestamp   time.Time         `json:"timestamp"`    // 执行时间
	Username    string            `json:"username"`     // 用户名
	UID         string            `json:"uid"`          // 用户ID
	PID         int32             `json:"pid"`          // 进程ID
	PPID        int32             `json:"ppid"`         // 父进程ID
	Command     string            `json:"command"`      // 命令名称
	CommandLine string            `json:"command_line"` // 完整命令行
	Arguments   []string          `json:"arguments"`    // 命令参数
	WorkingDir  string            `json:"working_dir"`  // 工作目录
	Executable  string            `json:"executable"`   // 可执行文件路径
	Terminal    string            `json:"terminal"`     // 终端
	SessionID   string            `json:"session_id"`   // 会话ID
	Result      string            `json:"result"`       // 执行结果
	ExitCode    int               `json:"exit_code"`    // 退出码
	Message     string            `json:"message"`      // 原始消息
	AuditID     string            `json:"audit_id"`     // Audit事件ID
	ExtraData   map[string]string `json:"extra_data"`   // 额外数据
}

// AuditMonitor Audit监控器
type AuditMonitor struct {
	// 监控功能开关
	monitorLogin   bool
	monitorCommand bool

	// 回调函数
	onLoginEvent   func(*LoginEvent)
	onCommandEvent func(*CommandEvent)

	// 过滤器
	filterUsers    []string
	filterCommands []string

	// 性能配置
	bufferSize int

	// 运行状态
	running bool
	stopCh  chan struct{}
	mu      sync.RWMutex
}

// AuditMonitorOption Audit监控器配置选项
type AuditMonitorOption func(*AuditMonitor)

// WithAuditMonitorLogin 设置是否监控登录事件
// Example:
// ```
// monitor = hids.NewAuditMonitor(hids.WithAuditMonitorLogin(true))
// ```
func WithAuditMonitorLogin(enable bool) AuditMonitorOption {
	return func(m *AuditMonitor) {
		m.monitorLogin = enable
	}
}

// WithAuditMonitorCommand 设置是否监控命令执行事件
// Example:
// ```
// monitor = hids.NewAuditMonitor(hids.WithAuditMonitorCommand(true))
// ```
func WithAuditMonitorCommand(enable bool) AuditMonitorOption {
	return func(m *AuditMonitor) {
		m.monitorCommand = enable
	}
}

// WithOnLoginEvent 设置登录事件回调
// Example:
// ```
// monitor = hids.NewAuditMonitor(hids.WithOnLoginEvent(func(event) {
//
//	println("Login:", event.Username, "from", event.RemoteIP)
//
// }))
// ```
func WithOnLoginEvent(callback func(*LoginEvent)) AuditMonitorOption {
	return func(m *AuditMonitor) {
		m.onLoginEvent = callback
	}
}

// WithOnCommandEvent 设置命令执行事件回调
// Example:
// ```
// monitor = hids.NewAuditMonitor(hids.WithOnCommandEvent(func(event) {
//
//	println("Command:", event.Command, "by", event.Username)
//
// }))
// ```
func WithOnCommandEvent(callback func(*CommandEvent)) AuditMonitorOption {
	return func(m *AuditMonitor) {
		m.onCommandEvent = callback
	}
}

// WithAuditFilterUsers 设置用户过滤器
// Example:
// ```
// monitor = hids.NewAuditMonitor(hids.WithAuditFilterUsers("root", "admin"))
// ```
func WithAuditFilterUsers(users ...string) AuditMonitorOption {
	return func(m *AuditMonitor) {
		m.filterUsers = users
	}
}

// WithAuditFilterCommands 设置命令过滤器（支持正则）
// Example:
// ```
// monitor = hids.NewAuditMonitor(hids.WithAuditFilterCommands(".*ssh.*", "sudo"))
// ```
func WithAuditFilterCommands(commands ...string) AuditMonitorOption {
	return func(m *AuditMonitor) {
		m.filterCommands = commands
	}
}

// WithAuditBufferSize 设置缓冲区大小
// Example:
// ```
// monitor = hids.NewAuditMonitor(hids.WithAuditBufferSize(16384))
// ```
func WithAuditBufferSize(size int) AuditMonitorOption {
	return func(m *AuditMonitor) {
		m.bufferSize = size
	}
}
