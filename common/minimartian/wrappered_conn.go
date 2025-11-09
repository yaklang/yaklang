package minimartian

import (
	"net"
	"sync"
)

// WrapperedConn 是一个包装的 net.Conn，用于携带额外的元数据信息
// 主要用于支持强主机模式和其他连接级别的配置
type WrapperedConn struct {
	net.Conn
	strongHostMode bool
	metaInfo       map[string]any
	mu             sync.RWMutex
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

// IsStrongHostMode 返回是否启用强主机模式
func (w *WrapperedConn) IsStrongHostMode() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.strongHostMode
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
