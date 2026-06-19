package yaklib

import (
	"context"
	"net"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/chanx"
)

type MitmExtraConnManager struct {
	extraConnMap map[string]*chanx.UnlimitedChan[net.Conn]
	mutex        sync.Mutex
}

func NewMitmExtraConnManager() *MitmExtraConnManager {
	return &MitmExtraConnManager{
		extraConnMap: make(map[string]*chanx.UnlimitedChan[net.Conn]),
		mutex:        sync.Mutex{},
	}
}

// GetDefaultExtraConnManager 获取默认的 MITM 额外连接管理器，用于向运行中的 MITM 服务注入外部连接
// 返回值:
//   - 默认的额外连接管理器实例
//
// Example:
// ```
// // 获取默认额外连接管理器，此处仅作示意
// manager = mitm.GetDefaultExtraConnManager()
// println(manager)
// ```
func getDefaultExtraConnManager() *MitmExtraConnManager {
	log.Debug("fetching default mitm extra conn manager")
	return DefaultMitmExtraConnManager
}

func (m *MitmExtraConnManager) GetExtraConnChan(id string) *chanx.UnlimitedChan[net.Conn] {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if ch, ok := m.extraConnMap[id]; ok {
		return ch
	}
	return nil
}

func (m *MitmExtraConnManager) Register(ctx context.Context, id string) *chanx.UnlimitedChan[net.Conn] {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	ch := chanx.NewUnlimitedChan[net.Conn](ctx, 10)
	m.extraConnMap[id] = ch
	return ch
}

func (m *MitmExtraConnManager) Unregister(id string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	delete(m.extraConnMap, id)
}

func init() {
	DefaultMitmExtraConnManager = NewMitmExtraConnManager()
}

var (
	DefaultGRPCMitmKey = "grpc_mitm_extra_conn_key"
)
var DefaultMitmExtraConnManager *MitmExtraConnManager
