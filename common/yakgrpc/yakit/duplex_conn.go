package yakit

import (
	"fmt"
	"github.com/yaklang/yaklang/common/schema"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	// set broadcast schema
	schema.SetBoardCast_Data(BroadcastData)
}

type serverPushDescription struct {
	Name   string
	Handle func(response *ypb.DuplexConnectionResponse)
}

var (
	serverPushMutex    = new(sync.Mutex)
	serverPushCallback = make(map[string]serverPushDescription)

	broadcastWithTypeMutex   = new(sync.Mutex)
	broadcastTypeCallerTable = make(map[string]func(func()))

	signalWithTypeMutex   = new(sync.Mutex)
	signalTypeCallerTable = make(map[string]func(func()))

	ServerPushType_Global       = "global"
	ServerPushType_HttpFlow     = "httpflow"
	ServerPushType_YakScript    = "yakscript"
	ServerPushType_Risk         = "risk"
	ServerPushType_File_Monitor = "file_monitor"
	ServerPushType_Error        = "error"
	ServerPushType_Warning      = "warning"
)

func RegisterServerPushCallback(id string, stream ypb.Yak_DuplexConnectionServer) {
	serverPushMutex.Lock()
	defer serverPushMutex.Unlock()

	log.Infof("Register server push callback: %v", id)

	serverPushCallback[id] = serverPushDescription{
		Name: id,
		Handle: func(response *ypb.DuplexConnectionResponse) {
			_ = stream.Send(response)
		},
	}
}

func UnRegisterServerPushCallback(id string) {
	serverPushMutex.Lock()
	defer serverPushMutex.Unlock()

	log.Infof("UnRegister server push callback: %v", id)
	delete(serverPushCallback, id)
}

func broadcastRaw(data *ypb.DuplexConnectionResponse) {
	serverPushMutex.Lock()
	defer serverPushMutex.Unlock()

	for _, item := range serverPushCallback {
		item.Handle(data)
	}
}

func BroadcastData(typeString string, data any) {
	broadcastWithTypeMutex.Lock()
	defer broadcastWithTypeMutex.Unlock()

	Data := &ypb.DuplexConnectionResponse{
		Data:        utils.Jsonify(data),
		MessageType: typeString,
		Timestamp:   time.Now().UnixNano(),
	}

	if caller, ok := broadcastTypeCallerTable[typeString]; ok {
		caller(func() {
			broadcastRaw(Data)
		})
	} else {
		broadcastTypeCallerTable[typeString] = utils.NewThrottle(1)
		broadcastTypeCallerTable[typeString](func() {
			broadcastRaw(Data)
		})
	}
}
func signalRaw(id string, data *ypb.DuplexConnectionResponse) {
	serverPushMutex.Lock()
	defer serverPushMutex.Unlock()

	serverPushCallback[id].Handle(data)
}

func SignalDate(id string, typeString string, data any) {
	signalWithTypeMutex.Lock()
	defer signalWithTypeMutex.Unlock()

	Data := &ypb.DuplexConnectionResponse{
		Data:        utils.Jsonify(data),
		MessageType: typeString,
		Timestamp:   time.Now().UnixNano(),
	}
	signalIndex := fmt.Sprintf("%s_%s", id, typeString)

	if caller, ok := signalTypeCallerTable[signalIndex]; ok {
		caller(func() {
			signalRaw(id, Data)
		})
	} else {
		signalTypeCallerTable[signalIndex] = utils.NewThrottle(1)
		signalTypeCallerTable[signalIndex](func() {
			signalRaw(id, Data)
		})
	}
}
