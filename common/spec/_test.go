package test

import (
	"context"
	"encoding/json"
	"github.com/streadway/amqp"
	"yaklang.io/yaklang/common/mq"
	"yaklang.io/yaklang/common/utils"
)


type ManagerAPI_ShutdownRequest struct {
	Name	[]string
	Z	string
}

type ManagerAPI_ShutdownResponse struct {
	Ok	bool
	Reason	string
}

var (
	MethodList = []string{
		"ManagerAPI_Shutdown",
	}
)



type ManagerAPIServerHelper struct {

	doManagerAPI_Shutdown func(ctx context.Context, node string, req *ManagerAPI_ShutdownRequest, broker *mq.Broker) (ManagerAPI_ShutdownResponse, error)
}

func (h *ManagerAPIServerHelper) Do(broker *mq.Broker, ctx context.Context, f, node string, delivery *amqp.Delivery) (message interface{}, e error) {
	switch f {

	case "ManagerAPI_Shutdown":
		var req ManagerAPI_ShutdownRequest
		err := json.Unmarshal(delivery.Body, &req)
		if err != nil {
			return nil, err
		}
		if h.doManagerAPI_Shutdown == nil {
			return nil, utils.Errorf("not implemented")
		}
		return h.doManagerAPI_Shutdown(ctx, node, &req, broker)

	default:
		return nil, utils.Errorf("unknown: func: %v", f)
	}
}

func NewManagerAPIServerHelper() *ManagerAPIServerHelper {
	return &ManagerAPIServerHelper{}
}



//
type callRpcHandler func(ctx context.Context, funcName, node string, req interface{}) ([]byte, error)
type ManagerAPIClientHelper struct {
	callRpc callRpcHandler
}

func (h *ManagerAPIClientHelper) ManagerAPI_Shutdown(ctx context.Context, node string, req *ManagerAPI_ShutdownRequest) (ManagerAPI_ShutdownResponse, error){
	rsp, err := h.callRpc(ctx, ManagerAPI_Shutdown, node, req)
	if err != nil {
		return nil, err
	}

	var rspIns ManagerAPI_ShutdownResponse
	err := json.Unmarshal(rsp, &rspIns)
	if err != nil {
		return nil, err
	}
	return &rspIns, nil
}

func GenerateManagerAPIClientHelper(callRpc callRpcHandler) *ManagerAPIClientHelper {
	return &ManagerAPIClientHelper{callRpc: callRpc}
}