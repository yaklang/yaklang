package yakit

import (
	"sync"
	"time"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type serverPushDescription struct {
	Name   string
	Handle func(response *ypb.DuplexConnectionResponse)
}

var (
	serverPushMutex          = new(sync.Mutex)
	broadcastWithTypeMutex   = new(sync.Mutex)
	serverPushCallback       []serverPushDescription
	broadcastTypeCallerTable = make(map[string]func(func()))
)

func RegisterServerPushCallback(id string, stream ypb.Yak_DuplexConnectionServer) {
	serverPushMutex.Lock()
	defer serverPushMutex.Unlock()

	log.Infof("Register server push callback: %v", id)
	serverPushCallback = append(serverPushCallback, serverPushDescription{
		Name: id,
		Handle: func(response *ypb.DuplexConnectionResponse) {
			_ = stream.Send(response)
		},
	})
}

func UnRegisterServerPushCallback(id string) {
	serverPushMutex.Lock()
	defer serverPushMutex.Unlock()

	log.Infof("UnRegister server push callback: %v", id)
	serverPushCallback = lo.Filter(serverPushCallback, func(item serverPushDescription, index int) bool {
		return item.Name != id
	})
}

func broadcastRaw(i any) {
	broadcastWithTypeMutex.Lock()
	defer broadcastWithTypeMutex.Unlock()

	for _, item := range serverPushCallback {
		item.Handle(&ypb.DuplexConnectionResponse{Data: utils.Jsonify(i)})
	}
}

func BroadcastData(typeString string, data any) {
	broadcastWithTypeMutex.Lock()
	defer broadcastWithTypeMutex.Unlock()

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
