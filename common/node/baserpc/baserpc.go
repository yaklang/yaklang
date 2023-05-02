package baserpc

import (
	"context"
	"encoding/json"
	amqp "github.com/streadway/amqp"
	mq "yaklang.io/yaklang/common/mq"
	utils "yaklang.io/yaklang/common/utils"
)

type ManagerAPI_ReadDirRequest struct {
	Target string
}
type ManagerAPI_ReadDirResponse struct {
	Infos []*FileInfo
}
type ManagerAPI_ReadDirRecursiveRequest struct {
	Target string
}
type ManagerAPI_ReadDirRecursiveResponse struct {
	Infos []*FileInfo
}
type ManagerAPI_ReadFileRequest struct {
	FileName string
}
type ManagerAPI_ReadFileResponse struct {
	Raw []byte
}
type ManagerAPI_ShutdownRequest struct{}
type ManagerAPI_ShutdownResponse struct {
	Ok     bool
	Reason string
}
type ManagerAPI_RestartRequest struct{}
type ManagerAPI_RestartResponse struct {
	Ok     bool
	Reason string
}
type ManagerAPI_EchoRequest struct {
	Data string
}
type ManagerAPI_EchoResponse struct {
	Data string
}
type ManagerAPI_ExecRequest struct {
	TimeoutStr string
	Binary     string
	Args       []string
}
type ManagerAPI_ExecResponse struct {
	CombinedOutput []byte
}

var (
	MethodList = []string{"ManagerAPI_ReadDir", "ManagerAPI_ReadDirRecursive", "ManagerAPI_ReadFile", "ManagerAPI_Shutdown", "ManagerAPI_Restart", "ManagerAPI_Echo", "ManagerAPI_Exec"}
)

type ManagerAPIServerHelper struct {
	DoManagerAPI_ReadDir          func(ctx context.Context, node string, req *ManagerAPI_ReadDirRequest, broker *mq.Broker) (*ManagerAPI_ReadDirResponse, error)
	DoManagerAPI_ReadDirRecursive func(ctx context.Context, node string, req *ManagerAPI_ReadDirRecursiveRequest, broker *mq.Broker) (*ManagerAPI_ReadDirRecursiveResponse, error)
	DoManagerAPI_ReadFile         func(ctx context.Context, node string, req *ManagerAPI_ReadFileRequest, broker *mq.Broker) (*ManagerAPI_ReadFileResponse, error)
	DoManagerAPI_Shutdown         func(ctx context.Context, node string, req *ManagerAPI_ShutdownRequest, broker *mq.Broker) (*ManagerAPI_ShutdownResponse, error)
	DoManagerAPI_Restart          func(ctx context.Context, node string, req *ManagerAPI_RestartRequest, broker *mq.Broker) (*ManagerAPI_RestartResponse, error)
	DoManagerAPI_Echo             func(ctx context.Context, node string, req *ManagerAPI_EchoRequest, broker *mq.Broker) (*ManagerAPI_EchoResponse, error)
	DoManagerAPI_Exec             func(ctx context.Context, node string, req *ManagerAPI_ExecRequest, broker *mq.Broker) (*ManagerAPI_ExecResponse, error)
}

func (h *ManagerAPIServerHelper) Do(broker *mq.Broker, ctx context.Context, f, node string, delivery *amqp.Delivery) (message interface{}, e error) {
	switch f {
	case "ManagerAPI_ReadDir":
		var req ManagerAPI_ReadDirRequest
		err := json.Unmarshal(delivery.Body, &req)
		if err != nil {
			return nil, err
		}
		if h.DoManagerAPI_ReadDir == nil {
			return nil, utils.Errorf("not implemented")
		}
		return h.DoManagerAPI_ReadDir(ctx, node, &req, broker)
	case "ManagerAPI_ReadDirRecursive":
		var req ManagerAPI_ReadDirRecursiveRequest
		err := json.Unmarshal(delivery.Body, &req)
		if err != nil {
			return nil, err
		}
		if h.DoManagerAPI_ReadDirRecursive == nil {
			return nil, utils.Errorf("not implemented")
		}
		return h.DoManagerAPI_ReadDirRecursive(ctx, node, &req, broker)
	case "ManagerAPI_ReadFile":
		var req ManagerAPI_ReadFileRequest
		err := json.Unmarshal(delivery.Body, &req)
		if err != nil {
			return nil, err
		}
		if h.DoManagerAPI_ReadFile == nil {
			return nil, utils.Errorf("not implemented")
		}
		return h.DoManagerAPI_ReadFile(ctx, node, &req, broker)
	case "ManagerAPI_Shutdown":
		var req ManagerAPI_ShutdownRequest
		err := json.Unmarshal(delivery.Body, &req)
		if err != nil {
			return nil, err
		}
		if h.DoManagerAPI_Shutdown == nil {
			return nil, utils.Errorf("not implemented")
		}
		return h.DoManagerAPI_Shutdown(ctx, node, &req, broker)
	case "ManagerAPI_Restart":
		var req ManagerAPI_RestartRequest
		err := json.Unmarshal(delivery.Body, &req)
		if err != nil {
			return nil, err
		}
		if h.DoManagerAPI_Restart == nil {
			return nil, utils.Errorf("not implemented")
		}
		return h.DoManagerAPI_Restart(ctx, node, &req, broker)
	case "ManagerAPI_Echo":
		var req ManagerAPI_EchoRequest
		err := json.Unmarshal(delivery.Body, &req)
		if err != nil {
			return nil, err
		}
		if h.DoManagerAPI_Echo == nil {
			return nil, utils.Errorf("not implemented")
		}
		return h.DoManagerAPI_Echo(ctx, node, &req, broker)
	case "ManagerAPI_Exec":
		var req ManagerAPI_ExecRequest
		err := json.Unmarshal(delivery.Body, &req)
		if err != nil {
			return nil, err
		}
		if h.DoManagerAPI_Exec == nil {
			return nil, utils.Errorf("not implemented")
		}
		return h.DoManagerAPI_Exec(ctx, node, &req, broker)
	default:
		return nil, utils.Errorf("unknown func: %v", f)
	}
}
func NewManagerAPIServerHelper() *ManagerAPIServerHelper {
	return &ManagerAPIServerHelper{}
}

type callRpcHandler func(ctx context.Context, funcName, node string, gen interface{}) ([]byte, error)
type ManagerAPIClientHelper struct {
	callRpc callRpcHandler
}

func (h *ManagerAPIClientHelper) ManagerAPI_ReadDir(ctx context.Context, node string, req *ManagerAPI_ReadDirRequest) (*ManagerAPI_ReadDirResponse, error) {
	rsp, err := h.callRpc(ctx, "ManagerAPI_ReadDir", node, req)
	if err != nil {
		return nil, err
	}
	var rspIns ManagerAPI_ReadDirResponse
	err = json.Unmarshal(rsp, &rspIns)
	if err != nil {
		return nil, err
	}
	return &rspIns, nil
}
func (h *ManagerAPIClientHelper) ManagerAPI_ReadDirRecursive(ctx context.Context, node string, req *ManagerAPI_ReadDirRecursiveRequest) (*ManagerAPI_ReadDirRecursiveResponse, error) {
	rsp, err := h.callRpc(ctx, "ManagerAPI_ReadDirRecursive", node, req)
	if err != nil {
		return nil, err
	}
	var rspIns ManagerAPI_ReadDirRecursiveResponse
	err = json.Unmarshal(rsp, &rspIns)
	if err != nil {
		return nil, err
	}
	return &rspIns, nil
}
func (h *ManagerAPIClientHelper) ManagerAPI_ReadFile(ctx context.Context, node string, req *ManagerAPI_ReadFileRequest) (*ManagerAPI_ReadFileResponse, error) {
	rsp, err := h.callRpc(ctx, "ManagerAPI_ReadFile", node, req)
	if err != nil {
		return nil, err
	}
	var rspIns ManagerAPI_ReadFileResponse
	err = json.Unmarshal(rsp, &rspIns)
	if err != nil {
		return nil, err
	}
	return &rspIns, nil
}
func (h *ManagerAPIClientHelper) ManagerAPI_Shutdown(ctx context.Context, node string, req *ManagerAPI_ShutdownRequest) (*ManagerAPI_ShutdownResponse, error) {
	rsp, err := h.callRpc(ctx, "ManagerAPI_Shutdown", node, req)
	if err != nil {
		return nil, err
	}
	var rspIns ManagerAPI_ShutdownResponse
	err = json.Unmarshal(rsp, &rspIns)
	if err != nil {
		return nil, err
	}
	return &rspIns, nil
}
func (h *ManagerAPIClientHelper) ManagerAPI_Restart(ctx context.Context, node string, req *ManagerAPI_RestartRequest) (*ManagerAPI_RestartResponse, error) {
	rsp, err := h.callRpc(ctx, "ManagerAPI_Restart", node, req)
	if err != nil {
		return nil, err
	}
	var rspIns ManagerAPI_RestartResponse
	err = json.Unmarshal(rsp, &rspIns)
	if err != nil {
		return nil, err
	}
	return &rspIns, nil
}
func (h *ManagerAPIClientHelper) ManagerAPI_Echo(ctx context.Context, node string, req *ManagerAPI_EchoRequest) (*ManagerAPI_EchoResponse, error) {
	rsp, err := h.callRpc(ctx, "ManagerAPI_Echo", node, req)
	if err != nil {
		return nil, err
	}
	var rspIns ManagerAPI_EchoResponse
	err = json.Unmarshal(rsp, &rspIns)
	if err != nil {
		return nil, err
	}
	return &rspIns, nil
}
func (h *ManagerAPIClientHelper) ManagerAPI_Exec(ctx context.Context, node string, req *ManagerAPI_ExecRequest) (*ManagerAPI_ExecResponse, error) {
	rsp, err := h.callRpc(ctx, "ManagerAPI_Exec", node, req)
	if err != nil {
		return nil, err
	}
	var rspIns ManagerAPI_ExecResponse
	err = json.Unmarshal(rsp, &rspIns)
	if err != nil {
		return nil, err
	}
	return &rspIns, nil
}
func GenerateManagerAPIClientHelper(callRpc callRpcHandler) *ManagerAPIClientHelper {
	return &ManagerAPIClientHelper{callRpc: callRpc}
}

type FileInfo struct {
	Name            string
	Path            string
	IsDir           bool
	ModifyTimestamp int64
	BytesSize       int64
	Mode            uint32
}
