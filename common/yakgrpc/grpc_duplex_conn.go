package yakgrpc

import (
	"github.com/samber/lo"
	uuid "github.com/satori/go.uuid"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"sync"
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

func BroadcastAny(i any) {
	serverPushMutex.Lock()
	defer serverPushMutex.Unlock()

	for _, item := range serverPushCallback {
		item.Handle(&ypb.DuplexConnectionResponse{Data: utils.InterfaceToBytes(i)})
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
