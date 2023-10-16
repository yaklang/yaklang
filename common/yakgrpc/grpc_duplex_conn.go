package yakgrpc

import (
	"github.com/samber/lo"
	uuid "github.com/satori/go.uuid"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"sync"
	"time"
)

type serverPushDescription struct {
	Name   string
	Handle func(response *ypb.DuplexConnectionResponse)
}

var (
	serverPushMutex    = new(sync.Mutex)
	serverPushCallback []serverPushDescription
)

func registerServerPushCallback(id string, stream ypb.Yak_DuplexConnectionServer) {
	serverPushMutex.Lock()
	defer serverPushMutex.Unlock()

	serverPushCallback = append(serverPushCallback, serverPushDescription{
		Name: id,
		Handle: func(response *ypb.DuplexConnectionResponse) {
			_ = stream.Send(response)
		},
	})
}

func unregisterServerPushCallback(id string) {
	serverPushMutex.Lock()
	defer serverPushMutex.Unlock()

	serverPushCallback = lo.Filter(serverPushCallback, func(item serverPushDescription, index int) bool {
		return item.Name != id
	})
}

func broadcastRaw(i any) {
	serverPushMutex.Lock()
	defer serverPushMutex.Unlock()
	for _, item := range serverPushCallback {
		item.Handle(&ypb.DuplexConnectionResponse{Data: utils.Jsonify(i)})
	}
}

var (
	broadcastWithTypeMu      = new(sync.Mutex)
	broadcastTypeCallerTable = make(map[string]func(func()))
)

func BroadcastData(typeString string, data any) {
	broadcastWithTypeMu.Lock()
	defer broadcastWithTypeMu.Unlock()

	buildData := func() map[string]any {
		return map[string]any{
			"type":      typeString,
			`data`:      data,
			"timestamp": time.Now().UnixNano(),
		}
	}

	if caller, ok := broadcastTypeCallerTable[typeString]; ok {
		caller(func() {
			broadcastRaw(buildData())
		})
	} else {
		broadcastTypeCallerTable[typeString] = utils.NewThrottle(1)
		broadcastTypeCallerTable[typeString](func() {
			broadcastRaw(buildData())
		})
	}
}

func (s *Server) DuplexConnection(stream ypb.Yak_DuplexConnectionServer) error {
	id := uuid.NewV4().String()
	registerServerPushCallback(id, stream)
	defer func() {
		unregisterServerPushCallback(id)
	}()
	select {
	case <-stream.Context().Done():
		return nil
	}
}
