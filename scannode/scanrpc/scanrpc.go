package scanrpc

import (
	"context"
	"encoding/json"
	amqp "github.com/streadway/amqp"
	mq "yaklang/common/mq"
	utils "yaklang/common/utils"
)

type SCAN_StartScriptRequest struct {
	Content string
}
type SCAN_StartScriptResponse struct{}
type SCAN_GetRunningTasksRequest struct{}
type SCAN_GetRunningTasksResponse struct {
	Tasks []*Task
}
type SCAN_StopTaskRequest struct {
	TaskId string
}
type SCAN_StopTaskResponse struct{}
type SCAN_RadCrawlerRequest struct {
	Targets    []string
	Proxy      string
	EnableXray bool
	Cookie     string
}
type SCAN_RadCrawlerResponse struct{}
type SCAN_DownloadXrayAndRadRequest struct {
	Proxy       string
	ForceUpdate bool
}
type SCAN_DownloadXrayAndRadResponse struct{}
type SCAN_IsXrayAndRadAvailableRequest struct{}
type SCAN_IsXrayAndRadAvailableResponse struct {
	Ok     bool
	Reason string
}
type SCAN_ScanFingerprintRequest struct {
	Hosts          string
	Ports          string
	IsUDP          bool
	TimeoutSeconds int
	Concurrent     int
}
type SCAN_ScanFingerprintResponse struct{}
type SCAN_BasicCrawlerRequest struct {
	Targets    []string
	EnableXray bool
	Proxy      string
}
type SCAN_BasicCrawlerResponse struct{}
type SCAN_ProxyCollectorRequest struct {
	Port int
}
type SCAN_ProxyCollectorResponse struct{}
type SCAN_InvokeScriptRequest struct {
	TaskId          string
	RuntimeId       string
	SubTaskId       string
	ScriptContent   string
	ScriptJsonParam string
}
type SCAN_InvokeScriptResponse struct {
	Data interface{}
}

var (
	MethodList = []string{"SCAN_StartScript", "SCAN_GetRunningTasks", "SCAN_StopTask", "SCAN_RadCrawler", "SCAN_DownloadXrayAndRad", "SCAN_IsXrayAndRadAvailable", "SCAN_ScanFingerprint", "SCAN_BasicCrawler", "SCAN_ProxyCollector", "SCAN_InvokeScript"}
)

type SCANServerHelper struct {
	DoSCAN_StartScript           func(ctx context.Context, node string, req *SCAN_StartScriptRequest, broker *mq.Broker) (*SCAN_StartScriptResponse, error)
	DoSCAN_GetRunningTasks       func(ctx context.Context, node string, req *SCAN_GetRunningTasksRequest, broker *mq.Broker) (*SCAN_GetRunningTasksResponse, error)
	DoSCAN_StopTask              func(ctx context.Context, node string, req *SCAN_StopTaskRequest, broker *mq.Broker) (*SCAN_StopTaskResponse, error)
	DoSCAN_RadCrawler            func(ctx context.Context, node string, req *SCAN_RadCrawlerRequest, broker *mq.Broker) (*SCAN_RadCrawlerResponse, error)
	DoSCAN_DownloadXrayAndRad    func(ctx context.Context, node string, req *SCAN_DownloadXrayAndRadRequest, broker *mq.Broker) (*SCAN_DownloadXrayAndRadResponse, error)
	DoSCAN_IsXrayAndRadAvailable func(ctx context.Context, node string, req *SCAN_IsXrayAndRadAvailableRequest, broker *mq.Broker) (*SCAN_IsXrayAndRadAvailableResponse, error)
	DoSCAN_ScanFingerprint       func(ctx context.Context, node string, req *SCAN_ScanFingerprintRequest, broker *mq.Broker) (*SCAN_ScanFingerprintResponse, error)
	DoSCAN_BasicCrawler          func(ctx context.Context, node string, req *SCAN_BasicCrawlerRequest, broker *mq.Broker) (*SCAN_BasicCrawlerResponse, error)
	DoSCAN_ProxyCollector        func(ctx context.Context, node string, req *SCAN_ProxyCollectorRequest, broker *mq.Broker) (*SCAN_ProxyCollectorResponse, error)
	DoSCAN_InvokeScript          func(ctx context.Context, node string, req *SCAN_InvokeScriptRequest, broker *mq.Broker) (*SCAN_InvokeScriptResponse, error)
}

func (h *SCANServerHelper) Do(broker *mq.Broker, ctx context.Context, f, node string, delivery *amqp.Delivery) (message interface{}, e error) {
	switch f {
	case "SCAN_StartScript":
		var req SCAN_StartScriptRequest
		err := json.Unmarshal(delivery.Body, &req)
		if err != nil {
			return nil, err
		}
		if h.DoSCAN_StartScript == nil {
			return nil, utils.Errorf("not implemented")
		}
		return h.DoSCAN_StartScript(ctx, node, &req, broker)
	case "SCAN_GetRunningTasks":
		var req SCAN_GetRunningTasksRequest
		err := json.Unmarshal(delivery.Body, &req)
		if err != nil {
			return nil, err
		}
		if h.DoSCAN_GetRunningTasks == nil {
			return nil, utils.Errorf("not implemented")
		}
		return h.DoSCAN_GetRunningTasks(ctx, node, &req, broker)
	case "SCAN_StopTask":
		var req SCAN_StopTaskRequest
		err := json.Unmarshal(delivery.Body, &req)
		if err != nil {
			return nil, err
		}
		if h.DoSCAN_StopTask == nil {
			return nil, utils.Errorf("not implemented")
		}
		return h.DoSCAN_StopTask(ctx, node, &req, broker)
	case "SCAN_RadCrawler":
		var req SCAN_RadCrawlerRequest
		err := json.Unmarshal(delivery.Body, &req)
		if err != nil {
			return nil, err
		}
		if h.DoSCAN_RadCrawler == nil {
			return nil, utils.Errorf("not implemented")
		}
		return h.DoSCAN_RadCrawler(ctx, node, &req, broker)
	case "SCAN_DownloadXrayAndRad":
		var req SCAN_DownloadXrayAndRadRequest
		err := json.Unmarshal(delivery.Body, &req)
		if err != nil {
			return nil, err
		}
		if h.DoSCAN_DownloadXrayAndRad == nil {
			return nil, utils.Errorf("not implemented")
		}
		return h.DoSCAN_DownloadXrayAndRad(ctx, node, &req, broker)
	case "SCAN_IsXrayAndRadAvailable":
		var req SCAN_IsXrayAndRadAvailableRequest
		err := json.Unmarshal(delivery.Body, &req)
		if err != nil {
			return nil, err
		}
		if h.DoSCAN_IsXrayAndRadAvailable == nil {
			return nil, utils.Errorf("not implemented")
		}
		return h.DoSCAN_IsXrayAndRadAvailable(ctx, node, &req, broker)
	case "SCAN_ScanFingerprint":
		var req SCAN_ScanFingerprintRequest
		err := json.Unmarshal(delivery.Body, &req)
		if err != nil {
			return nil, err
		}
		if h.DoSCAN_ScanFingerprint == nil {
			return nil, utils.Errorf("not implemented")
		}
		return h.DoSCAN_ScanFingerprint(ctx, node, &req, broker)
	case "SCAN_BasicCrawler":
		var req SCAN_BasicCrawlerRequest
		err := json.Unmarshal(delivery.Body, &req)
		if err != nil {
			return nil, err
		}
		if h.DoSCAN_BasicCrawler == nil {
			return nil, utils.Errorf("not implemented")
		}
		return h.DoSCAN_BasicCrawler(ctx, node, &req, broker)
	case "SCAN_ProxyCollector":
		var req SCAN_ProxyCollectorRequest
		err := json.Unmarshal(delivery.Body, &req)
		if err != nil {
			return nil, err
		}
		if h.DoSCAN_ProxyCollector == nil {
			return nil, utils.Errorf("not implemented")
		}
		return h.DoSCAN_ProxyCollector(ctx, node, &req, broker)
	case "SCAN_InvokeScript":
		var req SCAN_InvokeScriptRequest
		err := json.Unmarshal(delivery.Body, &req)
		if err != nil {
			return nil, err
		}
		if h.DoSCAN_InvokeScript == nil {
			return nil, utils.Errorf("not implemented")
		}
		return h.DoSCAN_InvokeScript(ctx, node, &req, broker)
	default:
		return nil, utils.Errorf("unknown func: %v", f)
	}
}
func NewSCANServerHelper() *SCANServerHelper {
	return &SCANServerHelper{}
}

type callRpcHandler func(ctx context.Context, funcName, node string, gen interface{}) ([]byte, error)
type SCANClientHelper struct {
	callRpc callRpcHandler
}

func (h *SCANClientHelper) SCAN_StartScript(ctx context.Context, node string, req *SCAN_StartScriptRequest) (*SCAN_StartScriptResponse, error) {
	rsp, err := h.callRpc(ctx, "SCAN_StartScript", node, req)
	if err != nil {
		return nil, err
	}
	var rspIns SCAN_StartScriptResponse
	err = json.Unmarshal(rsp, &rspIns)
	if err != nil {
		return nil, err
	}
	return &rspIns, nil
}
func (h *SCANClientHelper) SCAN_GetRunningTasks(ctx context.Context, node string, req *SCAN_GetRunningTasksRequest) (*SCAN_GetRunningTasksResponse, error) {
	rsp, err := h.callRpc(ctx, "SCAN_GetRunningTasks", node, req)
	if err != nil {
		return nil, err
	}
	var rspIns SCAN_GetRunningTasksResponse
	err = json.Unmarshal(rsp, &rspIns)
	if err != nil {
		return nil, err
	}
	return &rspIns, nil
}
func (h *SCANClientHelper) SCAN_StopTask(ctx context.Context, node string, req *SCAN_StopTaskRequest) (*SCAN_StopTaskResponse, error) {
	rsp, err := h.callRpc(ctx, "SCAN_StopTask", node, req)
	if err != nil {
		return nil, err
	}
	var rspIns SCAN_StopTaskResponse
	err = json.Unmarshal(rsp, &rspIns)
	if err != nil {
		return nil, err
	}
	return &rspIns, nil
}
func (h *SCANClientHelper) SCAN_RadCrawler(ctx context.Context, node string, req *SCAN_RadCrawlerRequest) (*SCAN_RadCrawlerResponse, error) {
	rsp, err := h.callRpc(ctx, "SCAN_RadCrawler", node, req)
	if err != nil {
		return nil, err
	}
	var rspIns SCAN_RadCrawlerResponse
	err = json.Unmarshal(rsp, &rspIns)
	if err != nil {
		return nil, err
	}
	return &rspIns, nil
}
func (h *SCANClientHelper) SCAN_DownloadXrayAndRad(ctx context.Context, node string, req *SCAN_DownloadXrayAndRadRequest) (*SCAN_DownloadXrayAndRadResponse, error) {
	rsp, err := h.callRpc(ctx, "SCAN_DownloadXrayAndRad", node, req)
	if err != nil {
		return nil, err
	}
	var rspIns SCAN_DownloadXrayAndRadResponse
	err = json.Unmarshal(rsp, &rspIns)
	if err != nil {
		return nil, err
	}
	return &rspIns, nil
}
func (h *SCANClientHelper) SCAN_IsXrayAndRadAvailable(ctx context.Context, node string, req *SCAN_IsXrayAndRadAvailableRequest) (*SCAN_IsXrayAndRadAvailableResponse, error) {
	rsp, err := h.callRpc(ctx, "SCAN_IsXrayAndRadAvailable", node, req)
	if err != nil {
		return nil, err
	}
	var rspIns SCAN_IsXrayAndRadAvailableResponse
	err = json.Unmarshal(rsp, &rspIns)
	if err != nil {
		return nil, err
	}
	return &rspIns, nil
}
func (h *SCANClientHelper) SCAN_ScanFingerprint(ctx context.Context, node string, req *SCAN_ScanFingerprintRequest) (*SCAN_ScanFingerprintResponse, error) {
	rsp, err := h.callRpc(ctx, "SCAN_ScanFingerprint", node, req)
	if err != nil {
		return nil, err
	}
	var rspIns SCAN_ScanFingerprintResponse
	err = json.Unmarshal(rsp, &rspIns)
	if err != nil {
		return nil, err
	}
	return &rspIns, nil
}
func (h *SCANClientHelper) SCAN_BasicCrawler(ctx context.Context, node string, req *SCAN_BasicCrawlerRequest) (*SCAN_BasicCrawlerResponse, error) {
	rsp, err := h.callRpc(ctx, "SCAN_BasicCrawler", node, req)
	if err != nil {
		return nil, err
	}
	var rspIns SCAN_BasicCrawlerResponse
	err = json.Unmarshal(rsp, &rspIns)
	if err != nil {
		return nil, err
	}
	return &rspIns, nil
}
func (h *SCANClientHelper) SCAN_ProxyCollector(ctx context.Context, node string, req *SCAN_ProxyCollectorRequest) (*SCAN_ProxyCollectorResponse, error) {
	rsp, err := h.callRpc(ctx, "SCAN_ProxyCollector", node, req)
	if err != nil {
		return nil, err
	}
	var rspIns SCAN_ProxyCollectorResponse
	err = json.Unmarshal(rsp, &rspIns)
	if err != nil {
		return nil, err
	}
	return &rspIns, nil
}
func (h *SCANClientHelper) SCAN_InvokeScript(ctx context.Context, node string, req *SCAN_InvokeScriptRequest) (*SCAN_InvokeScriptResponse, error) {
	rsp, err := h.callRpc(ctx, "SCAN_InvokeScript", node, req)
	if err != nil {
		return nil, err
	}
	var rspIns SCAN_InvokeScriptResponse
	err = json.Unmarshal(rsp, &rspIns)
	if err != nil {
		return nil, err
	}
	return &rspIns, nil
}
func GenerateSCANClientHelper(callRpc callRpcHandler) *SCANClientHelper {
	return &SCANClientHelper{callRpc: callRpc}
}

type Task struct {
	TaskID            string
	TaskType          string
	StartTimestamp    int64
	DeadlineTimestamp int64
}
