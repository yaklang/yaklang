package minimartian

import (
	"net"
	"sync"
)

// TransparentConn marks a connection whose original upstream destination is
// already known by the connection producer (for example, a TUN/netstack TCP
// interception). Ordinary accepted proxy connections must not implement this
// interface: their target comes from HTTP CONNECT, the Host header, or SOCKS5.
type TransparentConn interface {
	net.Conn
	OriginalDestination() net.Addr
}

// WrapperedConn 是一个包装的 net.Conn，用于携带额外的元数据信息
// 主要用于支持强主机模式和其他连接级别的配置
type WrapperedConn struct {
	net.Conn
	strongHostMode      bool
	strongHostLocalAddr string // Local IP address for strong host mode binding
	originalDestination string // Original upstream target for transparently injected connections
	metaInfo            map[string]any
	mu                  sync.RWMutex
	isListened          bool
}

// NewWrapperedConn 创建一个新的 WrapperedConn
func NewWrapperedConn(conn net.Conn, strongHostMode bool, metaInfo map[string]any) *WrapperedConn {
	if metaInfo == nil {
		metaInfo = make(map[string]any)
	}
	return &WrapperedConn{
		Conn:           conn,
		strongHostMode: strongHostMode,
		metaInfo:       metaInfo,
	}
}

func NewWrapperedConnEx(conn net.Conn, strongHostMode bool, metaInfo map[string]any, listened bool) *WrapperedConn {
	if metaInfo == nil {
		metaInfo = make(map[string]any)
	}
	return &WrapperedConn{
		Conn:           conn,
		strongHostMode: strongHostMode,
		metaInfo:       metaInfo,
		isListened:     listened,
	}
}

// NewWrapperedConnWithStrongLocalHost 创建一个新的 WrapperedConn，并设置强主机模式的本地地址
func NewWrapperedConnWithStrongLocalHost(conn net.Conn, localAddr string, metaInfo map[string]any) *WrapperedConn {
	if metaInfo == nil {
		metaInfo = make(map[string]any)
	}
	return &WrapperedConn{
		Conn:                conn,
		strongHostMode:      true, // 强制启用强主机模式
		strongHostLocalAddr: localAddr,
		metaInfo:            metaInfo,
	}
}

// NewWrapperedConnWithStrongLocalHostAndOriginalDestination wraps a
// transparently intercepted connection. originalDestination must be the
// connection's upstream target, not the address of a MITM listener.
func NewWrapperedConnWithStrongLocalHostAndOriginalDestination(conn net.Conn, localAddr, originalDestination string, metaInfo map[string]any) *WrapperedConn {
	if metaInfo == nil {
		metaInfo = make(map[string]any)
	}
	return &WrapperedConn{
		Conn:                conn,
		strongHostMode:      true,
		strongHostLocalAddr: localAddr,
		originalDestination: originalDestination,
		metaInfo:            metaInfo,
	}
}

// IsStrongHostMode 返回是否启用强主机模式
func (w *WrapperedConn) IsStrongHostMode() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.strongHostMode
}

// GetStrongHostLocalAddr 返回强主机模式的本地地址
func (w *WrapperedConn) GetStrongHostLocalAddr() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.strongHostLocalAddr
}

// GetOriginalDestination returns the upstream target carried by a
// transparently injected connection. An empty value means the connection must
// obtain its target from a frontend proxy protocol such as HTTP or SOCKS5.
func (w *WrapperedConn) GetOriginalDestination() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.originalDestination
}

// GetMetaInfo 返回连接的元数据信息
func (w *WrapperedConn) GetMetaInfo() map[string]any {
	w.mu.RLock()
	defer w.mu.RUnlock()
	// 返回一个副本，避免外部修改影响内部状态
	result := make(map[string]any)
	if len(w.metaInfo) > 0 {
		for k, v := range w.metaInfo {
			result[k] = v
		}
	}
	return result
}

// SetMetaInfo 设置元数据信息
func (w *WrapperedConn) SetMetaInfo(key string, value any) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.metaInfo == nil {
		w.metaInfo = make(map[string]any)
	}
	w.metaInfo[key] = value
}

// MergeMetaInfo 合并元数据信息
func (w *WrapperedConn) MergeMetaInfo(metaInfo map[string]any) {
	if len(metaInfo) == 0 {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.metaInfo == nil {
		w.metaInfo = make(map[string]any)
	}
	for k, v := range metaInfo {
		w.metaInfo[k] = v
	}
}
