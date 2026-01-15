//go:build linux

package hids

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/user"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/elastic/go-libaudit"
	"github.com/elastic/go-libaudit/auparse"
)

// uidToUsername 将 UID 映射到用户名
func uidToUsername(uid string) string {
	if uid == "" || uid == "unset" || uid == "4294967295" {
		return ""
	}

	u, err := user.LookupId(uid)
	if err != nil {
		return uid // 如果查找失败，返回原始 UID
	}
	return u.Username
}

// CheckAuditSystem 检查 audit 子系统状态
// Example:
// ```
// status, err = hids.CheckAuditSystem()
// if err != nil { println("Audit not available:", err) }
// println("Audit enabled:", status.Enabled)
// ```
func CheckAuditSystem() (*AuditStatus, error) {
	status := &AuditStatus{}

	// 检查是否有 root 权限
	if os.Geteuid() != 0 {
		return nil, fmt.Errorf("root privileges required to check audit status")
	}

	// 使用 libaudit 获取 audit 状态
	client, err := libaudit.NewAuditClient(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to audit subsystem: %v", err)
	}
	defer client.Close()

	// 获取 audit 状态
	auditStatus, err := client.GetStatus()
	if err != nil {
		return nil, fmt.Errorf("failed to get audit status: %v", err)
	}

	status.Enabled = auditStatus.Enabled == 1
	status.BacklogLimit = auditStatus.BacklogLimit
	status.Backlog = auditStatus.Backlog
	status.Lost = auditStatus.Lost
	status.PID = auditStatus.PID
	status.Running = auditStatus.PID > 0

	return status, nil
}

// checkAuditAvailable 内部检查 audit 是否可用
func checkAuditAvailable() error {
	// 使用 libaudit 检查 audit 状态
	client, err := libaudit.NewAuditClient(nil)
	if err != nil {
		return fmt.Errorf("audit subsystem not available: %v", err)
	}
	defer client.Close()

	// 获取 audit 状态
	auditStatus, err := client.GetStatus()
	if err != nil {
		return fmt.Errorf("failed to get audit status: %v", err)
	}

	// 检查 audit 是否启用
	if auditStatus.Enabled != 1 {
		return fmt.Errorf("audit subsystem is disabled (enabled=%d), enable it with: sudo auditctl -e 1", auditStatus.Enabled)
	}

	return nil
}

// NewAuditMonitor 创建Audit监控器
// Example:
// ```
// monitor = hids.NewAuditMonitor(
//
//	hids.WithAuditMonitorLogin(true),
//	hids.WithAuditMonitorCommand(true),
//	hids.WithOnLoginEvent(func(event) {
//	    println("Login:", event.Username, "from", event.RemoteIP)
//	}),
//	hids.WithOnCommandEvent(func(event) {
//	    println("Command:", event.Command)
//	}),
//
// )
// ```
func NewAuditMonitor(opts ...AuditMonitorOption) (*AuditMonitor, error) {
	m := &AuditMonitor{
		monitorLogin:   true,
		monitorCommand: true,
		bufferSize:     8192,
		running:        false,
		stopCh:         make(chan struct{}),
	}

	for _, opt := range opts {
		opt(m)
	}

	// 设置默认缓冲区大小
	if m.bufferSize == 0 {
		m.bufferSize = 8192
	}

	return m, nil
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
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return fmt.Errorf("audit monitor is already running")
	}

	// 检查是否有root权限
	if os.Geteuid() != 0 {
		m.mu.Unlock()
		return fmt.Errorf("audit monitor requires root privileges")
	}

	// 检查 audit 子系统是否可用
	if err := checkAuditAvailable(); err != nil {
		m.mu.Unlock()
		return err
	}

	m.running = true
	m.stopCh = make(chan struct{})
	m.mu.Unlock()

	// 创建audit客户端 (多播模式，只读监听)
	client, err := libaudit.NewMulticastAuditClient(nil)
	if err != nil {
		m.mu.Lock()
		m.running = false
		m.mu.Unlock()
		return fmt.Errorf("failed to create audit client: %v", err)
	}

	// 启动监控协程
	go func() {
		defer func() {
			client.Close()
			m.mu.Lock()
			m.running = false
			m.mu.Unlock()
		}()

		// 创建消息重组器
		reassembler, err := libaudit.NewReassembler(100, 5*time.Second, &auditStream{
			monitor: m,
		})
		if err != nil {
			fmt.Printf("Failed to create reassembler: %v\n", err)
			return
		}
		defer reassembler.Close()

		// 启动维护协程
		maintainDone := make(chan struct{})
		go func() {
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-m.stopCh:
					close(maintainDone)
					return
				case <-ticker.C:
					reassembler.Maintain()
				}
			}
		}()

		for {
			select {
			case <-m.stopCh:
				<-maintainDone
				return
			default:
				// 接收audit消息
				rawMsg, err := client.Receive(true)
				if err != nil {
					if err == io.EOF {
						return
					}
					// 非阻塞模式下，EAGAIN是正常的
					time.Sleep(100 * time.Millisecond)
					continue
				}

				// 推送消息到重组器
				if rawMsg != nil {
					reassembler.Push(rawMsg.Type, rawMsg.Data)
				}
			}
		}
	}()

	return nil
}

// Stop 停止监控
// Example:
// ```
// monitor.Stop()
// ```
func (m *AuditMonitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return
	}

	close(m.stopCh)
	m.running = false
}

// IsRunning 检查是否正在运行
func (m *AuditMonitor) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

// auditStream 实现 libaudit.Stream 接口
type auditStream struct {
	monitor *AuditMonitor
}

// ReassemblyComplete 处理重组完成的事件
func (s *auditStream) ReassemblyComplete(msgs []*auparse.AuditMessage) {
	if len(msgs) == 0 {
		return
	}

	// 遍历所有消息，检查包含的消息类型
	var hasLoginEvent bool
	var hasCommandEvent bool

	for _, msg := range msgs {
		msgType := msg.RecordType
		if isLoginEvent(msgType) {
			hasLoginEvent = true
		}
		if isCommandEvent(msgType) {
			hasCommandEvent = true
		}
	}

	// 处理登录事件
	if s.monitor.monitorLogin && hasLoginEvent {
		event := parseLoginEvent(msgs)
		if event != nil && s.shouldProcessLogin(event) {
			if s.monitor.onLoginEvent != nil {
				go s.monitor.onLoginEvent(event)
			}
		}
	}

	// 处理命令执行事件
	if s.monitor.monitorCommand && hasCommandEvent {
		event := parseCommandEvent(msgs)
		if event != nil && s.shouldProcessCommand(event) {
			if s.monitor.onCommandEvent != nil {
				go s.monitor.onCommandEvent(event)
			}
		}
	}
}

// EventsLost 处理事件丢失
func (s *auditStream) EventsLost(count int) {
	// 可以记录日志或统计
}

// isLoginEvent 判断是否为登录事件
func isLoginEvent(msgType auparse.AuditMessageType) bool {
	switch msgType {
	case auparse.AUDIT_USER_LOGIN,
		auparse.AUDIT_USER_AUTH,
		auparse.AUDIT_USER_START,
		auparse.AUDIT_USER_END,
		auparse.AUDIT_CRED_ACQ:
		return true
	}
	return false
}

// isCommandEvent 判断是否为命令执行事件
// 命令执行事件必须包含 EXECVE 消息，SYSCALL 消息会作为同一事件组的一部分被自动处理
func isCommandEvent(msgType auparse.AuditMessageType) bool {
	return msgType == auparse.AUDIT_EXECVE
}

// parseLoginEvent 解析登录事件
func parseLoginEvent(msgs []*auparse.AuditMessage) *LoginEvent {
	if len(msgs) == 0 {
		return nil
	}

	event := &LoginEvent{
		Timestamp: msgs[0].Timestamp,
		AuditID:   fmt.Sprintf("%d.%d", msgs[0].Timestamp.Unix(), msgs[0].Sequence),
		ExtraData: make(map[string]string),
	}

	for _, msg := range msgs {
		// 调用 Data() 方法获取数据
		data, err := msg.Data()
		if err != nil {
			continue
		}

		// 提取用户信息
		if uid, ok := data["uid"]; ok {
			event.UID = uid
			// 使用 uid 映射到用户名
			if event.Username == "" {
				event.Username = uidToUsername(uid)
			}
		}
		// auid (audit uid) 是登录时的原始用户 ID，优先使用
		if auid, ok := data["auid"]; ok {
			if username := uidToUsername(auid); username != "" {
				event.Username = username
			}
		}
		// acct 字段直接包含用户名，优先级最高
		if acct, ok := data["acct"]; ok && acct != "" {
			event.Username = acct
		}

		// 提取远程信息
		if addr, ok := data["addr"]; ok {
			event.RemoteIP = addr
		}
		if hostname, ok := data["hostname"]; ok {
			event.RemoteHost = hostname
		}

		// 提取终端信息
		if terminal, ok := data["terminal"]; ok {
			event.Terminal = terminal
		}

		// 提取会话ID
		if ses, ok := data["ses"]; ok {
			event.SessionID = ses
		}

		// 提取登录方式和结果
		if op, ok := data["op"]; ok {
			event.LoginMethod = op
		}
		if res, ok := data["res"]; ok {
			event.Result = res
		} else if result, ok := data["result"]; ok {
			event.Result = result
		}

		// 提取可执行文件 (用于判断登录方式)
		if exe, ok := data["exe"]; ok {
			if strings.Contains(exe, "sshd") {
				event.LoginMethod = "ssh"
			} else if strings.Contains(exe, "login") {
				event.LoginMethod = "console"
			}
			event.ExtraData["exe"] = exe
		}

		// 保存原始消息
		if event.Message == "" {
			event.Message = msg.RawData
		}

		// 保存其他有用的字段
		for k, v := range data {
			if _, exists := event.ExtraData[k]; !exists {
				event.ExtraData[k] = v
			}
		}
	}

	return event
}

// parseCommandEvent 解析命令执行事件
func parseCommandEvent(msgs []*auparse.AuditMessage) *CommandEvent {
	if len(msgs) == 0 {
		return nil
	}

	event := &CommandEvent{
		Timestamp: msgs[0].Timestamp,
		AuditID:   fmt.Sprintf("%d.%d", msgs[0].Timestamp.Unix(), msgs[0].Sequence),
		ExtraData: make(map[string]string),
		Arguments: make([]string, 0),
	}

	var hasExecve bool
	var hasSyscall bool

	for _, msg := range msgs {
		msgType := msg.RecordType

		// 调用 Data() 方法获取数据
		data, err := msg.Data()
		if err != nil {
			continue
		}

		// 处理 SYSCALL 消息
		if msgType == auparse.AUDIT_SYSCALL {
			hasSyscall = true

			// 提取进程信息
			if pid, ok := data["pid"]; ok {
				if p, err := strconv.ParseInt(pid, 10, 32); err == nil {
					event.PID = int32(p)
				}
			}
			if ppid, ok := data["ppid"]; ok {
				if p, err := strconv.ParseInt(ppid, 10, 32); err == nil {
					event.PPID = int32(p)
				}
			}

			// 提取用户信息
			if uid, ok := data["uid"]; ok {
				event.UID = uid
				// 使用 uid 映射到用户名
				if event.Username == "" {
					event.Username = uidToUsername(uid)
				}
			}
			// auid (audit uid) 是登录时的原始用户 ID，优先使用
			if auid, ok := data["auid"]; ok {
				if username := uidToUsername(auid); username != "" {
					event.Username = username
				}
			}

			// 提取终端和会话
			if tty, ok := data["tty"]; ok {
				event.Terminal = tty
			}
			if ses, ok := data["ses"]; ok {
				event.SessionID = ses
			}

			// 提取命令名
			if comm, ok := data["comm"]; ok {
				event.Command = comm
			}

			// 提取可执行文件
			if exe, ok := data["exe"]; ok {
				event.Executable = exe
			}

			// 提取结果
			if success, ok := data["success"]; ok {
				event.Result = success
			}
			if exit, ok := data["exit"]; ok {
				if e, err := strconv.Atoi(exit); err == nil {
					event.ExitCode = e
				}
			}
		}

		// 处理 EXECVE 消息 (包含命令参数)
		if msgType == auparse.AUDIT_EXECVE {
			hasExecve = true

			// 解析参数
			if argc, ok := data["argc"]; ok {
				count, _ := strconv.Atoi(argc)
				for i := 0; i < count; i++ {
					argKey := fmt.Sprintf("a%d", i)
					if arg, ok := data[argKey]; ok {
						event.Arguments = append(event.Arguments, arg)
					}
				}
			}

			// 构建完整命令行
			if len(event.Arguments) > 0 {
				event.CommandLine = strings.Join(event.Arguments, " ")
				if event.Command == "" {
					event.Command = event.Arguments[0]
				}
			}
		}

		// 处理 CWD 消息 (工作目录)
		if msgType == auparse.AUDIT_CWD {
			if cwd, ok := data["cwd"]; ok {
				event.WorkingDir = cwd
			}
		}

		// 保存原始消息
		if event.Message == "" {
			event.Message = msg.RawData
		}

		// 保存其他字段
		for k, v := range data {
			if _, exists := event.ExtraData[k]; !exists {
				event.ExtraData[k] = v
			}
		}
	}

	// 只有同时有 SYSCALL 和 EXECVE 消息才认为是完整的命令执行事件
	if !hasSyscall || !hasExecve {
		return nil
	}

	return event
}

// shouldProcessLogin 检查是否应该处理该登录事件
func (s *auditStream) shouldProcessLogin(event *LoginEvent) bool {
	// 如果配置了用户过滤器
	if len(s.monitor.filterUsers) > 0 {
		found := false
		for _, user := range s.monitor.filterUsers {
			if event.Username == user {
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

// shouldProcessCommand 检查是否应该处理该命令事件
func (s *auditStream) shouldProcessCommand(event *CommandEvent) bool {
	// 如果配置了用户过滤器
	if len(s.monitor.filterUsers) > 0 {
		found := false
		for _, user := range s.monitor.filterUsers {
			if event.Username == user {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// 如果配置了命令过滤器
	if len(s.monitor.filterCommands) > 0 {
		found := false
		for _, cmd := range s.monitor.filterCommands {
			// 支持通配符匹配
			matched, _ := regexp.MatchString(cmd, event.Command)
			if matched {
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

// WatchAuditEvents 简化的监控函数 - 监控audit事件
// Example:
// ```
// ctx, cancel = context.WithTimeout(context.Background(), 10)
// defer cancel()
// err = hids.WatchAuditEvents(ctx, func(event) {
//
//	println("Login:", event.Username)
//
// }, func(event) {
//
//	println("Command:", event.Command)
//
// })
// ```
func WatchAuditEvents(ctx context.Context, onLogin func(*LoginEvent), onCommand func(*CommandEvent)) error {
	opts := []AuditMonitorOption{
		WithAuditMonitorLogin(onLogin != nil),
		WithAuditMonitorCommand(onCommand != nil),
	}

	if onLogin != nil {
		opts = append(opts, WithOnLoginEvent(onLogin))
	}
	if onCommand != nil {
		opts = append(opts, WithOnCommandEvent(onCommand))
	}

	monitor, err := NewAuditMonitor(opts...)
	if err != nil {
		return err
	}

	if err := monitor.Start(); err != nil {
		return err
	}

	// 等待上下文取消
	<-ctx.Done()
	monitor.Stop()

	return nil
}
