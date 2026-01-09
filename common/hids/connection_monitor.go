package hids

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	gopsnet "github.com/shirou/gopsutil/v4/net"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// ConnectionInfo 网络连接信息
type ConnectionInfo struct {
	Fd         uint32   `json:"fd"`
	Family     string   `json:"family"`     // AF_INET, AF_INET6, AF_UNIX
	Type       string   `json:"type"`       // SOCK_STREAM, SOCK_DGRAM
	LocalAddr  string   `json:"local_addr"` // IP:Port
	LocalIP    string   `json:"local_ip"`
	LocalPort  uint32   `json:"local_port"`
	RemoteAddr string   `json:"remote_addr"` // IP:Port
	RemoteIP   string   `json:"remote_ip"`
	RemotePort uint32   `json:"remote_port"`
	Status     string   `json:"status"` // LISTEN, ESTABLISHED, TIME_WAIT, etc.
	Pid        int32    `json:"pid"`
	Uids       []uint32 `json:"uids"`
}

// ConnectionFilter 连接过滤器
type ConnectionFilter struct {
	Protocol    string // tcp, udp, tcp4, tcp6, udp4, udp6, unix
	Status      string // LISTEN, ESTABLISHED, TIME_WAIT, etc.
	LocalIP     string
	LocalPort   uint32
	RemoteIP    string
	RemotePort  uint32
	Pid         int32
	PortPattern string // 端口匹配正则
}

// NewConnectionFilter 创建新的连接过滤器
// Example:
// ```
// filter = hids.NewConnectionFilter()
// filter.Status = "LISTEN"
// filter.Protocol = "tcp"
// conns = hids.Netstat(filter)
// ```
func NewConnectionFilter() *ConnectionFilter {
	return &ConnectionFilter{}
}

// addressFamilyToString 将地址族转换为字符串
func addressFamilyToString(family uint32) string {
	switch family {
	case 2:
		return "AF_INET"
	case 10, 30: // Linux is 10, Darwin/BSD is 30
		return "AF_INET6"
	case 1:
		return "AF_UNIX"
	default:
		return fmt.Sprintf("AF_%d", family)
	}
}

// socketTypeToString 将套接字类型转换为字符串
func socketTypeToString(sockType uint32) string {
	switch sockType {
	case 1:
		return "SOCK_STREAM"
	case 2:
		return "SOCK_DGRAM"
	case 3:
		return "SOCK_RAW"
	default:
		return fmt.Sprintf("SOCK_%d", sockType)
	}
}

// getConnectionInfoFromStat 从gopsutil的ConnectionStat获取连接信息
func getConnectionInfoFromStat(conn gopsnet.ConnectionStat) *ConnectionInfo {
	localAddr := conn.Laddr.IP
	if conn.Laddr.Port > 0 {
		localAddr = fmt.Sprintf("%s:%d", conn.Laddr.IP, conn.Laddr.Port)
	}

	remoteAddr := conn.Raddr.IP
	if conn.Raddr.Port > 0 {
		remoteAddr = fmt.Sprintf("%s:%d", conn.Raddr.IP, conn.Raddr.Port)
	}

	uids := make([]uint32, len(conn.Uids))
	for i, uid := range conn.Uids {
		uids[i] = uint32(uid)
	}

	return &ConnectionInfo{
		Fd:         conn.Fd,
		Family:     addressFamilyToString(conn.Family),
		Type:       socketTypeToString(conn.Type),
		LocalAddr:  localAddr,
		LocalIP:    conn.Laddr.IP,
		LocalPort:  conn.Laddr.Port,
		RemoteAddr: remoteAddr,
		RemoteIP:   conn.Raddr.IP,
		RemotePort: conn.Raddr.Port,
		Status:     conn.Status,
		Pid:        conn.Pid,
		Uids:       uids,
	}
}

// filterConnection 检查连接是否匹配过滤器
func filterConnection(conn *ConnectionInfo, filter *ConnectionFilter) bool {
	if filter == nil {
		return true
	}

	// 检查协议
	if filter.Protocol != "" {
		protocol := strings.ToLower(filter.Protocol)
		family := strings.ToLower(conn.Family)
		connType := strings.ToLower(conn.Type)

		switch protocol {
		case "tcp", "tcp4":
			if !strings.Contains(connType, "stream") || !strings.Contains(family, "inet") {
				return false
			}
			if protocol == "tcp4" && strings.Contains(family, "inet6") {
				return false
			}
		case "tcp6":
			if !strings.Contains(connType, "stream") || !strings.Contains(family, "inet6") {
				return false
			}
		case "udp", "udp4":
			if !strings.Contains(connType, "dgram") || !strings.Contains(family, "inet") {
				return false
			}
			if protocol == "udp4" && strings.Contains(family, "inet6") {
				return false
			}
		case "udp6":
			if !strings.Contains(connType, "dgram") || !strings.Contains(family, "inet6") {
				return false
			}
		case "unix":
			if !strings.Contains(family, "unix") {
				return false
			}
		}
	}

	// 检查状态
	if filter.Status != "" && !strings.EqualFold(conn.Status, filter.Status) {
		return false
	}

	// 检查本地IP
	if filter.LocalIP != "" && conn.LocalIP != filter.LocalIP {
		return false
	}

	// 检查本地端口
	if filter.LocalPort > 0 && conn.LocalPort != filter.LocalPort {
		return false
	}

	// 检查远程IP
	if filter.RemoteIP != "" && conn.RemoteIP != filter.RemoteIP {
		return false
	}

	// 检查远程端口
	if filter.RemotePort > 0 && conn.RemotePort != filter.RemotePort {
		return false
	}

	// 检查PID
	if filter.Pid > 0 && conn.Pid != filter.Pid {
		return false
	}

	// 检查端口正则
	if filter.PortPattern != "" {
		localPortStr := strconv.Itoa(int(conn.LocalPort))
		remotePortStr := strconv.Itoa(int(conn.RemotePort))
		localMatch, _ := regexp.MatchString(filter.PortPattern, localPortStr)
		remoteMatch, _ := regexp.MatchString(filter.PortPattern, remotePortStr)
		if !localMatch && !remoteMatch {
			return false
		}
	}

	return true
}

// Netstat 获取网络连接列表（类似netstat命令）
// Example:
// ```
// // 获取所有连接
// conns, err = hids.Netstat()
//
// // 使用过滤器
// filter = hids.NewConnectionFilter()
// filter.Status = "LISTEN"
// conns, err = hids.Netstat(filter)
// ```
func Netstat(filters ...*ConnectionFilter) ([]*ConnectionInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 获取所有类型的连接
	conns, err := gopsnet.ConnectionsWithContext(ctx, "all")
	if err != nil {
		return nil, utils.Errorf("failed to get connections: %v", err)
	}

	var filter *ConnectionFilter
	if len(filters) > 0 {
		filter = filters[0]
	}

	var result []*ConnectionInfo
	for _, conn := range conns {
		info := getConnectionInfoFromStat(conn)
		if filterConnection(info, filter) {
			result = append(result, info)
		}
	}

	return result, nil
}

// GetTCPConnections 获取TCP连接列表
// Example:
// ```
// conns, err = hids.GetTCPConnections()
// ```
func GetTCPConnections() ([]*ConnectionInfo, error) {
	filter := NewConnectionFilter()
	filter.Protocol = "tcp"
	return Netstat(filter)
}

// GetUDPConnections 获取UDP连接列表
// Example:
// ```
// conns, err = hids.GetUDPConnections()
// ```
func GetUDPConnections() ([]*ConnectionInfo, error) {
	filter := NewConnectionFilter()
	filter.Protocol = "udp"
	return Netstat(filter)
}

// GetListeningPorts 获取所有监听端口
// Example:
// ```
// conns, err = hids.GetListeningPorts()
// for _, conn := range conns {
//
//	println("Port:", conn.LocalPort, "PID:", conn.Pid)
//
// }
// ```
func GetListeningPorts() ([]*ConnectionInfo, error) {
	filter := NewConnectionFilter()
	filter.Status = "LISTEN"
	return Netstat(filter)
}

// GetEstablishedConnections 获取已建立的连接
// Example:
// ```
// conns, err = hids.GetEstablishedConnections()
// ```
func GetEstablishedConnections() ([]*ConnectionInfo, error) {
	filter := NewConnectionFilter()
	filter.Status = "ESTABLISHED"
	return Netstat(filter)
}

// GetConnectionsByPid 获取指定进程的连接
// Example:
// ```
// conns, err = hids.GetConnectionsByPid(1234)
// ```
func GetConnectionsByPid(pid int32) ([]*ConnectionInfo, error) {
	filter := NewConnectionFilter()
	filter.Pid = pid
	return Netstat(filter)
}

// GetConnectionsByPort 获取指定端口的连接
// Example:
// ```
// conns, err = hids.GetConnectionsByPort(80)
// ```
func GetConnectionsByPort(port uint32) ([]*ConnectionInfo, error) {
	conns, err := Netstat()
	if err != nil {
		return nil, err
	}

	var result []*ConnectionInfo
	for _, conn := range conns {
		if conn.LocalPort == port || conn.RemotePort == port {
			result = append(result, conn)
		}
	}

	return result, nil
}

// ConnectionEventType 连接事件类型
type ConnectionEventType string

const (
	ConnectionEventNew       ConnectionEventType = "new"       // 新连接
	ConnectionEventDisappear ConnectionEventType = "disappear" // 连接消失
)

// ConnectionEvent 连接事件
type ConnectionEvent struct {
	Type       ConnectionEventType `json:"type"`
	Connection *ConnectionInfo     `json:"connection"`
	Timestamp  int64               `json:"timestamp"`
}

// ConnectionMonitor 连接监控器
type ConnectionMonitor struct {
	ctx             context.Context
	cancel          context.CancelFunc
	interval        time.Duration
	knownConns      map[string]*ConnectionInfo
	mu              sync.RWMutex
	filter          *ConnectionFilter
	onNewConn       func(event *ConnectionEvent)
	onConnDisappear func(event *ConnectionEvent)
	running         bool

	// 历史记录
	historyEnabled bool
	history        []*ConnectionEvent
	historyMu      sync.RWMutex
	maxHistory     int
}

// ConnectionMonitorOption 连接监控器配置选项
type ConnectionMonitorOption func(*ConnectionMonitor)

// WithConnectionMonitorInterval 设置监控间隔
func WithConnectionMonitorInterval(seconds float64) ConnectionMonitorOption {
	return func(m *ConnectionMonitor) {
		m.interval = time.Duration(seconds * float64(time.Second))
	}
}

// WithConnectionFilter 设置连接过滤器
func WithConnectionFilter(filter *ConnectionFilter) ConnectionMonitorOption {
	return func(m *ConnectionMonitor) {
		m.filter = filter
	}
}

// WithOnNewConnection 设置新连接回调
func WithOnNewConnection(callback func(event *ConnectionEvent)) ConnectionMonitorOption {
	return func(m *ConnectionMonitor) {
		m.onNewConn = callback
	}
}

// WithOnConnectionDisappear 设置连接消失回调
func WithOnConnectionDisappear(callback func(event *ConnectionEvent)) ConnectionMonitorOption {
	return func(m *ConnectionMonitor) {
		m.onConnDisappear = callback
	}
}

// WithConnectionHistory 启用历史记录
func WithConnectionHistory(maxHistory int) ConnectionMonitorOption {
	return func(m *ConnectionMonitor) {
		m.historyEnabled = true
		m.maxHistory = maxHistory
		if m.maxHistory <= 0 {
			m.maxHistory = 1000
		}
	}
}

// NewConnectionMonitor 创建连接监控器
// Example:
// ```
// monitor = hids.NewConnectionMonitor(
//
//	hids.WithConnectionMonitorInterval(1),
//	hids.WithOnNewConnection(func(event) {
//	    println("New connection:", event.Connection.LocalAddr, "->", event.Connection.RemoteAddr)
//	}),
//
// )
// ```
func NewConnectionMonitor(opts ...ConnectionMonitorOption) *ConnectionMonitor {
	m := &ConnectionMonitor{
		interval:   5 * time.Second,
		knownConns: make(map[string]*ConnectionInfo),
		maxHistory: 1000,
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// connKey 生成连接的唯一标识
func connKey(conn *ConnectionInfo) string {
	return fmt.Sprintf("%d:%s:%s:%s:%d", conn.Fd, conn.Family, conn.LocalAddr, conn.RemoteAddr, conn.Pid)
}

// Start 启动连接监控
func (m *ConnectionMonitor) Start() error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return utils.Error("connection monitor is already running")
	}
	m.running = true
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.mu.Unlock()

	// 初始化已知连接列表
	if err := m.initKnownConnections(); err != nil {
		return err
	}

	go m.monitorLoop()
	return nil
}

// Stop 停止连接监控
func (m *ConnectionMonitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return
	}

	m.running = false
	if m.cancel != nil {
		m.cancel()
	}
}

// IsRunning 检查监控器是否在运行
func (m *ConnectionMonitor) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

// initKnownConnections 初始化已知连接列表
func (m *ConnectionMonitor) initKnownConnections() error {
	conns, err := Netstat(m.filter)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, conn := range conns {
		m.knownConns[connKey(conn)] = conn
	}

	return nil
}

// monitorLoop 监控循环
func (m *ConnectionMonitor) monitorLoop() {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.checkConnections()
		}
	}
}

// checkConnections 检查连接变化
func (m *ConnectionMonitor) checkConnections() {
	conns, err := Netstat(m.filter)
	if err != nil {
		log.Errorf("failed to get connections: %v", err)
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	currentKeys := make(map[string]bool)
	now := time.Now().Unix()

	// 检查新连接
	for _, conn := range conns {
		key := connKey(conn)
		currentKeys[key] = true

		if _, known := m.knownConns[key]; !known {
			m.knownConns[key] = conn

			event := &ConnectionEvent{
				Type:       ConnectionEventNew,
				Connection: conn,
				Timestamp:  now,
			}

			if m.historyEnabled {
				m.addHistory(event)
			}

			if m.onNewConn != nil {
				go m.onNewConn(event)
			}
		}
	}

	// 检查消失的连接
	for key, conn := range m.knownConns {
		if !currentKeys[key] {
			delete(m.knownConns, key)

			event := &ConnectionEvent{
				Type:       ConnectionEventDisappear,
				Connection: conn,
				Timestamp:  now,
			}

			if m.historyEnabled {
				m.addHistory(event)
			}

			if m.onConnDisappear != nil {
				go m.onConnDisappear(event)
			}
		}
	}
}

// addHistory 添加历史记录
func (m *ConnectionMonitor) addHistory(event *ConnectionEvent) {
	m.historyMu.Lock()
	defer m.historyMu.Unlock()

	m.history = append(m.history, event)
	if len(m.history) > m.maxHistory {
		m.history = m.history[len(m.history)-m.maxHistory:]
	}
}

// GetHistory 获取历史记录
func (m *ConnectionMonitor) GetHistory() []*ConnectionEvent {
	m.historyMu.RLock()
	defer m.historyMu.RUnlock()

	result := make([]*ConnectionEvent, len(m.history))
	copy(result, m.history)
	return result
}

// ClearHistory 清空历史记录
func (m *ConnectionMonitor) ClearHistory() {
	m.historyMu.Lock()
	defer m.historyMu.Unlock()
	m.history = nil
}

// WatchConnections 简单的连接监控函数，监控指定时长后返回事件列表
// Example:
// ```
// events, err = hids.WatchConnections(5) // 监控5秒
// for _, event := range events {
//
//	println(event.Type, event.Connection.LocalAddr, "->", event.Connection.RemoteAddr)
//
// }
// ```
func WatchConnections(durationSeconds float64) ([]*ConnectionEvent, error) {
	var events []*ConnectionEvent
	var mu sync.Mutex

	monitor := NewConnectionMonitor(
		WithConnectionMonitorInterval(0.5),
		WithOnNewConnection(func(event *ConnectionEvent) {
			mu.Lock()
			events = append(events, event)
			mu.Unlock()
		}),
		WithOnConnectionDisappear(func(event *ConnectionEvent) {
			mu.Lock()
			events = append(events, event)
			mu.Unlock()
		}),
	)

	if err := monitor.Start(); err != nil {
		return nil, err
	}

	time.Sleep(time.Duration(durationSeconds * float64(time.Second)))
	monitor.Stop()

	return events, nil
}

// ConnectionStats 连接统计信息
type ConnectionStats struct {
	Total       int            `json:"total"`
	ByStatus    map[string]int `json:"by_status"`
	ByProtocol  map[string]int `json:"by_protocol"`
	TCPCount    int            `json:"tcp_count"`
	UDPCount    int            `json:"udp_count"`
	ListenCount int            `json:"listen_count"`
}

// GetConnectionStats 获取连接统计信息
// Example:
// ```
// stats, err = hids.GetConnectionStats()
// println("Total connections:", stats.Total)
// println("TCP connections:", stats.TCPCount)
// println("Listening ports:", stats.ListenCount)
// ```
func GetConnectionStats() (*ConnectionStats, error) {
	conns, err := Netstat()
	if err != nil {
		return nil, err
	}

	stats := &ConnectionStats{
		ByStatus:   make(map[string]int),
		ByProtocol: make(map[string]int),
	}

	for _, conn := range conns {
		stats.Total++

		// 按状态统计
		status := conn.Status
		if status == "" {
			status = "NONE"
		}
		stats.ByStatus[status]++

		// 按协议统计
		if strings.Contains(conn.Type, "STREAM") {
			stats.TCPCount++
			stats.ByProtocol["TCP"]++
		} else if strings.Contains(conn.Type, "DGRAM") {
			stats.UDPCount++
			stats.ByProtocol["UDP"]++
		}

		// 监听计数
		if strings.EqualFold(status, "LISTEN") {
			stats.ListenCount++
		}
	}

	return stats, nil
}

// String 连接信息字符串表示
func (c *ConnectionInfo) String() string {
	if c.RemoteAddr != "" && c.RemoteAddr != ":" && c.RemoteAddr != "0.0.0.0:0" {
		return fmt.Sprintf("%s %s -> %s (%s) [PID: %d]",
			c.Type, c.LocalAddr, c.RemoteAddr, c.Status, c.Pid)
	}
	return fmt.Sprintf("%s %s (%s) [PID: %d]",
		c.Type, c.LocalAddr, c.Status, c.Pid)
}
