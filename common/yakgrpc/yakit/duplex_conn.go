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

	ServerPushType_Global       = "global"
	ServerPushType_HttpFlow     = "httpflow"
	ServerPushType_YakScript    = "yakscript"
	ServerPushType_Risk         = "risk"
	ServerPushType_AIMemory     = "ai_memory"
	ServerPushType_File_Monitor = "file_monitor"
	ServerPushType_Error        = "error"
	ServerPushType_Warning      = "warning"
	ServerPushType_RPS          = "rps"
	ServerPushType_CPS          = "cps"
	ServerPushType_Fuzzer       = "fuzzer_server_push"
	ServerPushType_WebFuzzerTab = "web_fuzzer_tab"
	ServerPushType_Project      = "project"
	ServerPushType_OpenAPIParse = "openapi_parse"

	ProjectPushActionPromptEnter = "prompt_enter"
	ProjectPushActionAutoEnter   = "auto_enter"
	ServerPushType_SlowInsertSQL = "httpflow_slow_insert_sql"
	ServerPushType_SlowQuerySQL  = "httpflow_slow_query_sql"
	ServerPushType_SlowRuleHook  = "mitm_slow_rule_hook"
)

type WebFuzzerTabPush struct {
	OpenFlag bool                `json:"openFlag"` // 创建 Web Fuzzer Tab 之后，要不要把左侧一级菜单切到「Web Fuzzer」并聚焦新 Tab
	Data     []*ypb.FuzzerConfig `json:"data"`
}

type ProjectPush struct {
	Action      string `json:"action"`
	ID          int64  `json:"id"`
	ProjectName string `json:"project_name,omitempty"`
	Type        string `json:"type,omitempty"`
}

func BroadcastWebFuzzerTab(openFlag bool, data ...*ypb.FuzzerConfig) {
	if len(data) == 0 {
		return
	}
	BroadcastData(ServerPushType_WebFuzzerTab, &WebFuzzerTabPush{
		OpenFlag: openFlag,
		Data:     data,
	})
}

func BroadcastProjectChanged(action string, id int64, projectName, projectType string) {
	if action == "" || id <= 0 {
		return
	}
	BroadcastData(ServerPushType_Project, &ProjectPush{
		Action:      action,
		ID:          id,
		ProjectName: projectName,
		Type:        projectType,
	})
}

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
