package yaklib

import (
	"context"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"net"
	"sync"
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
