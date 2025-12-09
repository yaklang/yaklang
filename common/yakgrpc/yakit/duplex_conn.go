package yakit

import (
	"fmt"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/schema"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	// set broadcast schema
	schema.SetBroadCast_Data(BroadcastData)
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

	ServerPushType_Global            = "global"
	ServerPushType_HttpFlow          = "httpflow"
	ServerPushType_YakScript         = "yakscript"
	ServerPushType_Risk              = "risk"
	ServerPushType_File_Monitor      = "file_monitor"
	ServerPushType_Error             = "error"
	ServerPushType_Warning           = "warning"
	ServerPushType_RPS               = "rps"
	ServerPushType_CPS               = "cps"
	ServerPushType_Fuzzer            = "fuzzer_server_push"
	ServerPushType_SlowInsertSQL     = "httpflow_slow_insert_sql"
	ServerPushType_SlowQuerySQL      = "httpflow_slow_query_sql"
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

func BroadcastData(typeString string, msg any) {
	broadcastWithTypeMutex.Lock()
	defer broadcastWithTypeMutex.Unlock()

	jsonMsg := utils.Jsonify(msg)
	data := &ypb.DuplexConnectionResponse{
		Data:        jsonMsg,
		MessageType: typeString,
		Timestamp:   time.Now().UnixNano(),
	}
	switch msg.(type) {
	case string:
	default:
		// if complex object, broadcast now, no need to throttle
		broadcastRaw(data)
		return
	}

	hash := utils.CalcMd5(typeString, jsonMsg)

	if caller, ok := broadcastTypeCallerTable[hash]; ok {
		caller(func() {
			broadcastRaw(data)
		})
	} else {
		broadcastTypeCallerTable[hash] = utils.NewThrottle(1)
		broadcastTypeCallerTable[hash](func() {
			broadcastRaw(data)
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
