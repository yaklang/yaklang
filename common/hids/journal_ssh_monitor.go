//go:build linux

package hids

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// SSH 日志正则表达式
var (
	// Accepted password for root from 1.2.3.4 port 22222 ssh2
	reSSHAccepted = regexp.MustCompile(`Accepted (\S+) for (\S+) from ([\d.a-fA-F:]+) port (\d+)`)
	// Failed password for root from 1.2.3.4 port 22222 ssh2
	// Failed password for invalid user foo from 1.2.3.4 port 22222 ssh2
	reSSHFailed = regexp.MustCompile(`Failed (\S+) for (?:invalid user )?(\S+) from ([\d.a-fA-F:]+) port (\d+)`)
	// Invalid user foo from 1.2.3.4 port 22222
	reSSHInvalidUser = regexp.MustCompile(`Invalid user (\S+) from ([\d.a-fA-F:]+) port (\d+)`)
	// Disconnected from user root 1.2.3.4 port 22222
	reSSHDisconnected = regexp.MustCompile(`Disconnected from (?:user )?(\S+) ([\d.a-fA-F:]+) port (\d+)`)
	// Disconnecting user root 1.2.3.4 port 22222: ...
	reSSHDisconnecting = regexp.MustCompile(`Disconnecting user (\S+) ([\d.a-fA-F:]+) port (\d+)`)
	// error: maximum authentication attempts exceeded for root from 1.2.3.4 port 22222
	reSSHMaxAuth = regexp.MustCompile(`maximum authentication attempts exceeded for (?:invalid user )?(\S+) from ([\d.a-fA-F:]+) port (\d+)`)
)

// journalEntry journalctl --output=json 的单条记录结构
// MESSAGE 字段在 journald 中可能是字符串或字节数组（当内容含非 UTF-8 时），
// 用 json.RawMessage 接收后再按需解析。
type journalEntry struct {
	RawMessage        json.RawMessage `json:"MESSAGE"`
	RealtimeTimestamp string          `json:"__REALTIME_TIMESTAMP"` // 微秒时间戳
	PID               string          `json:"_PID"`
	Hostname          string          `json:"_HOSTNAME"`
	SyslogIdentifier  string          `json:"SYSLOG_IDENTIFIER"`
}

// getMessage 解析 MESSAGE 字段，兼容字符串和字节数组两种 journald 编码格式
func (e *journalEntry) getMessage() string {
	if len(e.RawMessage) == 0 {
		return ""
	}
	// 优先尝试字符串格式
	var s string
	if err := json.Unmarshal(e.RawMessage, &s); err == nil {
		return s
	}
	// journald 对含二进制内容的消息会编码为 []uint8 数组
	var raw []byte
	if err := json.Unmarshal(e.RawMessage, &raw); err == nil {
		return string(raw)
	}
	return ""
}

// parseJournalTimestamp 解析 journal 时间戳（微秒）
func parseJournalTimestamp(ts string) time.Time {
	if ts == "" {
		return time.Now()
	}
	us, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return time.Now()
	}
	return time.Unix(us/1e6, (us%1e6)*1000)
}

// buildJournalArgs 构建 journalctl 参数
// 默认使用 -t sshd（SYSLOG_IDENTIFIER），比 -u unit 更可靠：
//   - sshd 无论以什么 unit 名称运行，写日志时 SYSLOG_IDENTIFIER 始终为 "sshd"
//   - 避免因发行版 service 名不同（ssh/sshd/openssh-server）导致漏监
//
// 只有用户通过 journalSSHUnits 显式指定 unit 时才使用 -u 过滤。
func buildJournalArgs(m *JournalSSHMonitor) []string {
	args := []string{"--no-pager", "--output=json", "--follow"}

	if len(m.journalUnits) > 0 {
		for _, unit := range m.journalUnits {
			args = append(args, "-u", unit)
		}
	} else {
		// 用 syslog 标识符过滤，兼容所有发行版
		args = append(args, "-t", "sshd")
	}

	if m.sinceTime != "" {
		args = append(args, "--since", m.sinceTime)
	}
	return args
}

// CheckJournalAvailable 检查 journalctl 是否可用且当前用户有权读取系统级 sshd 日志
//
// sshd 日志属于 system journal，非 root 用户需要加入 systemd-journal（或 adm）组才能读取。
// 如果权限不足，journalctl 会静默返回空结果而不报错，导致监控无法捕获任何事件。
// 本函数通过不带 -q 执行 journalctl，捕获其向 stderr 输出的权限提示来判断。
//
// Example:
// ```
// err = hids.CheckJournalAvailable()
// if err != nil { println("journal not available:", err) }
// ```
func CheckJournalAvailable() error {
	path, err := exec.LookPath("journalctl")
	if err != nil {
		return fmt.Errorf("journalctl not found in PATH, systemd may not be running: %v", err)
	}

	// 不加 -q，让 journalctl 将权限不足的提示输出到 stderr
	// 同时用 -t sshd 定向查询 sshd 相关条目，以触发系统日志的权限检查
	cmd := exec.Command(path, "--no-pager", "-n", "0", "-t", "sshd")
	out, _ := cmd.CombinedOutput()
	output := string(out)

	// 硬性权限错误：journal 文件无法打开
	if strings.Contains(output, "Permission denied") ||
		strings.Contains(output, "Failed to open journal") ||
		strings.Contains(output, "No journal files were opened") {
		return fmt.Errorf(
			"cannot access system journal: permission denied\n" +
				"sshd logs are stored in the system journal and require elevated access.\n")
	}

	// 软性权限提示：journalctl 运行成功但隐藏了其他用户和系统的日志
	// 此时监控能启动但捕获不到任何 sshd 事件
	if strings.Contains(output, "not seeing messages from other users") ||
		strings.Contains(output, "not seeing messages from the system") ||
		strings.Contains(output, "Cannot access system journal") {
		return fmt.Errorf(
			"current user cannot read system-level journal entries (sshd runs as a system service)\n")
	}

	return nil
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
	m := &JournalSSHMonitor{
		sinceTime: "now",
		stopCh:    make(chan struct{}),
	}

	for _, opt := range opts {
		opt(m)
	}

	return m, nil
}

// Start 启动 SSH journal 监控
// Example:
// ```
// monitor, _ = hids.NewJournalSSHMonitor(...)
// err = monitor.Start()
// if err != nil { die(err) }
// ```
func (m *JournalSSHMonitor) Start() error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return fmt.Errorf("journal SSH monitor is already running")
	}

	if err := CheckJournalAvailable(); err != nil {
		m.mu.Unlock()
		return err
	}

	args := buildJournalArgs(m)
	cmd := exec.Command("journalctl", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		m.mu.Unlock()
		return fmt.Errorf("failed to create journalctl pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("failed to start journalctl: %v", err)
	}

	m.running = true
	m.stopCh = make(chan struct{})
	m.mu.Unlock()

	go func() {
		defer func() {
			cmd.Process.Kill()
			cmd.Wait()
			m.mu.Lock()
			m.running = false
			m.mu.Unlock()
		}()

		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 64*1024), 64*1024)

		doneCh := make(chan struct{})
		go func() {
			defer close(doneCh)
			for scanner.Scan() {
				select {
				case <-m.stopCh:
					return
				default:
				}
				line := scanner.Text()
				if line == "" {
					continue
				}
				var entry journalEntry
				if err := json.Unmarshal([]byte(line), &entry); err != nil {
					continue
				}
				event := m.parseSSHEvent(&entry)
				if event == nil {
					continue
				}
				if !m.shouldProcess(event) {
					continue
				}
				m.dispatchEvent(event)
			}
		}()

		select {
		case <-m.stopCh:
		case <-doneCh:
		}
	}()

	return nil
}

// Stop 停止监控
// Example:
// ```
// monitor.Stop()
// ```
func (m *JournalSSHMonitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running {
		return
	}
	close(m.stopCh)
	m.running = false
}

// IsRunning 检查监控器是否正在运行
func (m *JournalSSHMonitor) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

// parseSSHEvent 从 journal 条目中解析 SSH 事件
func (m *JournalSSHMonitor) parseSSHEvent(entry *journalEntry) *JournalSSHEvent {
	msg := entry.getMessage()
	if msg == "" {
		return nil
	}

	ts := parseJournalTimestamp(entry.RealtimeTimestamp)
	base := &JournalSSHEvent{
		Timestamp: ts,
		PID:       entry.PID,
		Hostname:  entry.Hostname,
		Message:   msg,
	}

	// Accepted password/publickey for user from ip port port
	if m := reSSHAccepted.FindStringSubmatch(msg); m != nil {
		base.EventType = JournalSSHEventLoginSuccess
		base.AuthMethod = m[1]
		base.Username = m[2]
		base.RemoteIP = m[3]
		base.RemotePort = m[4]
		return base
	}

	// Failed password/publickey for [invalid user] user from ip port port
	if m := reSSHFailed.FindStringSubmatch(msg); m != nil {
		base.EventType = JournalSSHEventLoginFailed
		base.AuthMethod = m[1]
		base.Username = m[2]
		base.RemoteIP = m[3]
		base.RemotePort = m[4]
		// 如果原始消息含 "invalid user"，归类为无效用户
		if strings.Contains(msg, "invalid user") {
			base.EventType = JournalSSHEventInvalidUser
		}
		return base
	}

	// Invalid user user from ip port port
	if m := reSSHInvalidUser.FindStringSubmatch(msg); m != nil {
		base.EventType = JournalSSHEventInvalidUser
		base.Username = m[1]
		base.RemoteIP = m[2]
		base.RemotePort = m[3]
		return base
	}

	// Disconnected from [user] user ip port port
	if m := reSSHDisconnected.FindStringSubmatch(msg); m != nil {
		base.EventType = JournalSSHEventDisconnected
		base.Username = m[1]
		base.RemoteIP = m[2]
		base.RemotePort = m[3]
		return base
	}

	// Disconnecting user user ip port port
	if m := reSSHDisconnecting.FindStringSubmatch(msg); m != nil {
		base.EventType = JournalSSHEventDisconnected
		base.Username = m[1]
		base.RemoteIP = m[2]
		base.RemotePort = m[3]
		return base
	}

	// maximum authentication attempts exceeded
	if m := reSSHMaxAuth.FindStringSubmatch(msg); m != nil {
		base.EventType = JournalSSHEventMaxAuthFailed
		base.Username = m[1]
		base.RemoteIP = m[2]
		base.RemotePort = m[3]
		return base
	}

	return nil
}

// shouldProcess 判断事件是否应该被处理（过滤器检查）
func (m *JournalSSHMonitor) shouldProcess(event *JournalSSHEvent) bool {
	if len(m.filterUsers) > 0 {
		found := false
		for _, u := range m.filterUsers {
			if event.Username == u {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if len(m.filterRemoteIPs) > 0 {
		found := false
		for _, ip := range m.filterRemoteIPs {
			if event.RemoteIP == ip {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// dispatchEvent 将事件分发到对应的回调函数
func (m *JournalSSHMonitor) dispatchEvent(event *JournalSSHEvent) {
	if m.onAnyEvent != nil {
		go m.onAnyEvent(event)
	}

	switch event.EventType {
	case JournalSSHEventLoginSuccess:
		if m.onLoginSuccess != nil {
			go m.onLoginSuccess(event)
		}
	case JournalSSHEventLoginFailed, JournalSSHEventInvalidUser, JournalSSHEventMaxAuthFailed:
		if m.onLoginFailed != nil {
			go m.onLoginFailed(event)
		}
	case JournalSSHEventDisconnected:
		if m.onDisconnected != nil {
			go m.onDisconnected(event)
		}
	}
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
	var opts []JournalSSHMonitorOption
	if onSuccess != nil {
		opts = append(opts, WithJournalSSHOnLoginSuccess(onSuccess))
	}
	if onFailed != nil {
		opts = append(opts, WithJournalSSHOnLoginFailed(onFailed))
	}

	monitor, err := NewJournalSSHMonitor(opts...)
	if err != nil {
		return err
	}

	if err := monitor.Start(); err != nil {
		return err
	}

	<-ctx.Done()
	monitor.Stop()
	return nil
}
